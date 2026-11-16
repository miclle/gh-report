package contribution

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/miclle/gh-report/github"
)

type localRepository struct {
	path, repo, remote string
}

func collectLocalCommits(ctx context.Context, client *github.Client, local *localRepository, branch string, report *Report, a *accumulator) error {
	ref, sha, err := local.branchHead(ctx, branch)
	if err != nil {
		return err
	}
	report.HeadSHA, report.SourceRef, report.LocalPath, report.CommitSource = sha, ref, local.path, "local_git"
	report.Rules[0] = "提交取本地保存的默认分支固定 SHA 全历史，仅计主作者，排除 merge 和空提交"
	report.Rules = append(report.Rules,
		"优先本地默认分支，缺失时使用对应 remote 的本地跟踪分支；不自动 fetch，不包含工作区未提交修改",
		"本地 numstat 启用重命名识别并禁用外部 diff/textconv；作者按邮箱代表提交查询 GitHub 身份，无法关联时保留 Git 身份")
	if sha == "" {
		return nil
	}
	// git log 默认从新到旧遍历，为每个邮箱留下较早的提交，提高已推送提交的关联机会。
	samples := make(map[string]string)
	if err := local.walkCommits(ctx, sha, func(commit github.CommitActivity) error {
		if err := a.addCommit(commit); err != nil {
			return err
		}
		email := strings.ToLower(strings.TrimSpace(commit.Author.Email))
		if email != "" {
			samples["git:"+email] = commit.SHA
		}
		return nil
	}); err != nil {
		return err
	}
	keys := make([]string, 0, len(samples))
	for key := range samples {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	shas := make([]string, 0, len(keys))
	for _, key := range keys {
		shas = append(shas, samples[key])
	}
	parts := strings.SplitN(local.repo, "/", 2)
	authors, err := client.LookupCommitAuthors(ctx, parts[0], parts[1], shas)
	if err != nil {
		return err
	}
	for _, key := range keys {
		author, ok := authors[samples[key]]
		if !ok {
			continue
		}
		old := a.users[key]
		delete(a.users, key)
		a.user(githubIdentity(author.User.ID, author.User.Login)).Counts.add(old.Counts)
	}
	return nil
}

func localGitCommand(ctx context.Context, path string, args ...string) *exec.Cmd {
	args = append([]string{"--no-replace-objects", "-C", path}, args...)
	cmd := exec.CommandContext(ctx, "git", args...)
	// 部分克隆缺少对象时也不允许 Git 隐式联网下载。
	cmd.Env = append(os.Environ(), "GIT_NO_LAZY_FETCH=1", "GIT_TERMINAL_PROMPT=0")
	return cmd
}

func localGit(ctx context.Context, path string, args ...string) (string, error) {
	cmd := localGitCommand(ctx, path, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("git %s 失败: %w（%s）", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

func githubRemoteRepo(remoteURL string) string {
	var path string
	if strings.HasPrefix(remoteURL, "git@github.com:") {
		path = strings.TrimPrefix(remoteURL, "git@github.com:")
	} else {
		parsed, err := url.Parse(remoteURL)
		if err != nil || !strings.EqualFold(parsed.Hostname(), "github.com") || parsed.Port() != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return ""
		}
		switch parsed.Scheme {
		case "https", "ssh", "git":
			path = strings.TrimPrefix(parsed.Path, "/")
		default:
			return ""
		}
	}
	path = strings.TrimSuffix(strings.TrimSuffix(path, "/"), ".git")
	if !(Options{Repo: path}).validRepo() {
		return ""
	}
	return path
}

func openGitWorkspace(ctx context.Context, path string) (string, error) {
	root, err := localGit(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("请在本地 Git 工作区内执行: %w", err)
	}
	shallow, err := localGit(ctx, root, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return "", err
	}
	if shallow == "true" {
		return "", fmt.Errorf("本地仓库是浅克隆，无法统计全历史；请先补全历史后重试")
	}
	return root, nil
}

func openLocalRepository(ctx context.Context, path, expectedRepo string) (*localRepository, error) {
	root, err := openGitWorkspace(ctx, path)
	if err != nil {
		return nil, err
	}
	names, err := localGit(ctx, root, "remote")
	if err != nil {
		return nil, err
	}
	var matches []*localRepository
	for _, name := range strings.Fields(names) {
		remoteURL, err := localGit(ctx, root, "remote", "get-url", name)
		if err != nil {
			// 不把可能含凭据的 remote URL 写入错误。
			return nil, fmt.Errorf("无法读取本地 remote %q", name)
		}
		repo := githubRemoteRepo(remoteURL)
		if repo != "" && (expectedRepo == "" || strings.EqualFold(repo, expectedRepo)) {
			matches = append(matches, &localRepository{path: root, repo: repo, remote: name})
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("未找到匹配的 GitHub remote；--local 需要 GitHub remote，-r 必须与其仓库一致")
	}
	for _, match := range matches {
		if match.remote == "origin" {
			return match, nil
		}
	}
	for _, match := range matches[1:] {
		if !strings.EqualFold(match.repo, matches[0].repo) {
			return nil, fmt.Errorf("存在多个不同的 GitHub remote，请用 -r owner/repo 明确选择")
		}
	}
	return matches[0], nil
}

func (r *localRepository) branchHead(ctx context.Context, branch string) (ref, sha string, err error) {
	if branch == "" {
		refs, err := localGit(ctx, r.path, "for-each-ref", "--format=%(refname)", "refs/heads", "refs/remotes")
		if err != nil {
			return "", "", err
		}
		if refs != "" {
			return "", "", fmt.Errorf("GitHub 仓库尚无默认分支，无法确认本地提交的统计范围")
		}
		return "", "", nil
	}
	for _, ref := range []string{"refs/heads/" + branch, "refs/remotes/" + r.remote + "/" + branch} {
		sha, err := localGit(ctx, r.path, "rev-parse", "--verify", ref+"^{commit}")
		if err == nil {
			return ref, sha, nil
		}
		if ctx.Err() != nil {
			return "", "", ctx.Err()
		}
	}
	return "", "", fmt.Errorf("本地没有默认分支 %q 或 remote %q 对应的跟踪分支；请先获取该分支的完整历史", branch, r.remote)
}

func (r *localRepository) walkCommits(ctx context.Context, sha string, visit func(github.CommitActivity) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := localGitCommand(ctx, r.path, "-c", "diff.algorithm=myers", "log", "--no-merges", "--root", "--numstat", "-z", "--find-renames=50%", "--ignore-submodules=none",
		"--no-ext-diff", "--no-textconv", "--no-color", "--no-notes", "--no-show-signature", "--format=%x00GHREPORT-COMMIT%x00%H%x00%an%x00%ae%x00%at%x00", sha, "--")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动本地 Git 统计: %w", err)
	}
	parseErr := readGitCommits(stdout, visit)
	if parseErr != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	if parseErr != nil {
		return parseErr
	}
	if waitErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("读取本地提交历史失败: %w（%s）", waitErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// readGitCommits 解析 NUL 分隔的 numstat，避免换行、制表符及重命名路径破坏记录边界。
func readGitCommits(reader io.Reader, visit func(github.CommitActivity) error) error {
	r := bufio.NewReader(reader)
	readField := func() (string, error) {
		field, err := r.ReadString(0)
		if errors.Is(err, io.EOF) && len(field) > 0 {
			err = io.ErrUnexpectedEOF
		}
		if err != nil {
			return "", err
		}
		return strings.TrimSuffix(field, "\x00"), nil
	}
	var commit github.CommitActivity
	changed := false
	flush := func() error {
		if commit.SHA != "" && changed {
			return visit(commit)
		}
		return nil
	}
	for {
		field, err := readField()
		if errors.Is(err, io.EOF) {
			return flush()
		}
		if err != nil {
			return fmt.Errorf("解析本地 Git 统计: %w", err)
		}
		field = strings.TrimLeft(field, "\n")
		if field == "" {
			continue
		}
		if field == "GHREPORT-COMMIT" {
			if err := flush(); err != nil {
				return err
			}
			sha, err := readField()
			if err != nil {
				return fmt.Errorf("读取本地提交 SHA: %w", err)
			}
			name, err := readField()
			if err != nil {
				return fmt.Errorf("读取本地主作者名称: %w", err)
			}
			email, err := readField()
			if err != nil {
				return fmt.Errorf("读取本地主作者邮箱: %w", err)
			}
			date, err := readField()
			if err != nil {
				return fmt.Errorf("读取本地主作者时间: %w", err)
			}
			seconds, err := strconv.ParseInt(date, 10, 64)
			if err != nil {
				return fmt.Errorf("本地提交 %s 的作者时间无效: %w", sha, err)
			}
			commit = github.CommitActivity{SHA: sha, Author: github.CommitAuthor{Name: name, Email: email}, AuthoredAt: time.Unix(seconds, 0).UTC()}
			changed = false
			continue
		}
		fields := strings.SplitN(field, "\t", 3)
		if commit.SHA == "" || len(fields) != 3 {
			return fmt.Errorf("本地 Git numstat 格式无效")
		}
		if fields[2] == "" {
			// 重命名记录在计数后额外携带两个完整路径。
			for range 2 {
				if _, err := readField(); err != nil {
					return fmt.Errorf("读取本地重命名路径: %w", err)
				}
			}
		}
		changed = true
		if fields[0] == "-" && fields[1] == "-" {
			continue
		}
		added, errA := strconv.ParseInt(fields[0], 10, 64)
		deleted, errD := strconv.ParseInt(fields[1], 10, 64)
		if errA != nil || errD != nil || added < 0 || deleted < 0 {
			return fmt.Errorf("本地提交 %s 的变更行数无效", commit.SHA)
		}
		commit.Additions += added
		commit.Deletions += deleted
	}
}
