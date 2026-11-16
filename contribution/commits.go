package contribution

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CommitOptions 指定离线提交统计的工作区、引用和排序。
type CommitOptions struct {
	Path string
	Ref  string
	Sort string
}

// Validate 检查离线统计支持的排序字段。
func (o CommitOptions) Validate() error {
	switch o.Sort {
	case "", "commits", "changes", "user":
		return nil
	default:
		return fmt.Errorf("不支持排序字段 %q（可用 commits、changes、user）", o.Sort)
	}
}

// CommitCounts 仅保存本地提交及文本变更计数。
type CommitCounts struct {
	Commits   int64 `json:"commits"`
	Additions int64 `json:"additions"`
	Deletions int64 `json:"deletions"`
	Changes   int64 `json:"changes"`
}

// CommitUser 保存按邮箱聚合的 Git 作者统计。
type CommitUser struct {
	Identity string `json:"identity"`
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	CommitCounts
}

// CommitReport 保存离线提交统计结果，不包含未采集的 Issue / PR 数据。
type CommitReport struct {
	LocalPath   string       `json:"local_path"`
	SourceRef   string       `json:"source_ref"`
	HeadSHA     string       `json:"head_sha"`
	StartedAt   time.Time    `json:"started_at"`
	CollectedAt time.Time    `json:"collected_at"`
	Sort        string       `json:"sort"`
	Rules       []string     `json:"rules"`
	Users       []CommitUser `json:"users"`
	Totals      CommitCounts `json:"totals"`
}

func commitCounts(c Counts) CommitCounts {
	return CommitCounts{Commits: c.Commits, Additions: c.Additions, Deletions: c.Deletions, Changes: c.Changes}
}

// CollectCommits 只读取本地 Git 对象，不查询 API、关联 GitHub 身份或下载缺失对象。
func CollectCommits(ctx context.Context, opts CommitOptions) (*CommitReport, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if opts.Path == "" {
		opts.Path = "."
	}
	if opts.Sort == "" {
		opts.Sort = "commits"
	}
	report := &CommitReport{StartedAt: time.Now().UTC(), Sort: opts.Sort, Rules: []string{
		"统计所选本地 ref 固定 SHA 可达的全历史，仅计主作者，排除 merge 和空提交",
		"新增和删除包含所有文本文件；变更行数为新增加删除，二进制、重命名及模式变更仍计提交",
		"按去除首尾空格并转小写的作者邮箱聚合，无邮箱按提交单列；不查询 GitHub 身份，不应用 mailmap，不过滤 Bot",
		"不联网、不切换分支、不 fetch；不包含未提交修改，不允许浅克隆或隐式下载缺失对象",
	}}
	root, err := openGitWorkspace(ctx, opts.Path)
	if err != nil {
		return nil, err
	}
	local := &localRepository{path: root, remote: "origin"}
	ref := opts.Ref
	var sha string
	if ref == "" {
		// origin/HEAD 是本地缓存，只用它定位默认分支，不从当前检出分支猜测。
		target, err := localGit(ctx, root, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD")
		if err != nil || !strings.HasPrefix(target, "refs/remotes/origin/") {
			return nil, fmt.Errorf("无法从本地 origin/HEAD 确定默认分支，请用 --ref 明确指定本地分支、标签或 SHA（如 --ref main 或 --ref HEAD）")
		}
		ref, sha, err = local.branchHead(ctx, strings.TrimPrefix(target, "refs/remotes/origin/"))
		if err != nil {
			return nil, err
		}
	} else {
		sha, err = localGit(ctx, root, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
		if err != nil {
			return nil, fmt.Errorf("无法解析本地 ref %q: %w", ref, err)
		}
	}
	report.LocalPath, report.SourceRef, report.HeadSHA = root, ref, sha
	a := newAccumulator()
	if err := local.walkCommits(ctx, sha, a.addCommit); err != nil {
		return nil, err
	}
	stats := &Report{Sort: opts.Sort}
	a.finish(stats)
	report.Users = make([]CommitUser, 0, len(stats.Users))
	for _, u := range stats.Users {
		report.Users = append(report.Users, CommitUser{Identity: u.Identity, Name: u.Name, Email: u.Email, CommitCounts: commitCounts(u.Counts)})
	}
	report.Totals = commitCounts(stats.Totals)
	report.CollectedAt = time.Now().UTC()
	return report, nil
}
