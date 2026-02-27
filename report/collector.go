// Package report 提供 GitHub 活动数据的收集和格式化输出功能。
package report

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	gh "github.com/google/go-github/v69/github"

	"github.com/miclle/gh-report/github"
)

// RepoReport 保存单个仓库的所有收集数据。
type RepoReport struct {
	Owner          string                          // 仓库所有者
	Repo           string                          // 仓库名称
	Issues         []*gh.Issue                     // Issue 列表
	PullRequests   []*gh.PullRequest               // Pull Request 列表
	IssueComments  []*gh.IssueComment              // Issue 评论列表
	ReviewComments []*gh.PullRequestComment         // PR Review 评论列表
	Reviews        map[int][]*gh.PullRequestReview // PR Review 列表，以 PR 编号为键
	Projects       []github.Project                // 关联的 Projects v2 项目
}

// Options 指定数据收集的参数。
type Options struct {
	Repos []string // 仓库列表，格式为 "owner/repo"
	Days  int      // 查看最近几天的活动
	User  string   // 按用户过滤（为空则不过滤）
}

// Progress 报告数据收集进度的接口。
// 实现方可用于在终端展示进度条等 UI 反馈。
type Progress interface {
	// SetTotal 设置指定仓库的总步骤数。
	SetTotal(repoIndex int, total int)
	// Increment 报告指定仓库完成一个步骤。
	Increment(repoIndex int)
}

// Collect 收集指定仓库的所有活动数据。
// 使用三层并发策略加速数据获取：组织 Projects 与仓库数据并发、仓库内 4 个接口并发、PR Review 并发。
// progress 参数可选，用于报告每个仓库的数据获取进度。
func Collect(ctx context.Context, client *github.Client, opts Options, progress Progress) ([]RepoReport, error) {
	since := time.Now().AddDate(0, 0, -opts.Days)

	// 解析仓库列表，收集唯一 owner
	type repoInfo struct {
		owner string
		repo  string
	}
	repos := make([]repoInfo, len(opts.Repos))
	owners := make(map[string]struct{})
	for i, fullRepo := range opts.Repos {
		parts := strings.SplitN(fullRepo, "/", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid repo format %q, expected owner/repo", fullRepo)
		}
		repos[i] = repoInfo{owner: parts[0], repo: parts[1]}
		owners[parts[0]] = struct{}{}
	}

	// 第一层并发：同时获取组织 Projects 和各仓库数据
	var wg sync.WaitGroup

	// 并发获取各组织的 Projects
	var mu sync.Mutex
	orgProjects := make(map[string][]github.Project)
	for owner := range owners {
		wg.Add(1)
		go func(owner string) {
			defer wg.Done()
			projects, err := client.ListProjects(ctx, owner)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not fetch projects for %s: %v\n", owner, err)
				orgProjects[owner] = nil
			} else {
				orgProjects[owner] = projects
			}
		}(owner)
	}

	// 并发收集每个仓库的数据
	reports := make([]RepoReport, len(repos))
	repoErrs := make([]error, len(repos))
	for i, ri := range repos {
		wg.Add(1)
		go func(idx int, owner, repo string) {
			defer wg.Done()
			rr, err := collectRepo(ctx, client, owner, repo, since, opts.User, progress, idx)
			if err != nil {
				repoErrs[idx] = err
				return
			}
			reports[idx] = *rr
		}(i, ri.owner, ri.repo)
	}

	wg.Wait()

	// 检查仓库数据收集错误
	for _, err := range repoErrs {
		if err != nil {
			return nil, err
		}
	}

	// 关联 Projects 到对应仓库
	for i := range reports {
		reports[i].Projects = orgProjects[reports[i].Owner]
	}

	return reports, nil
}

// collectRepo 并发收集单个仓库的所有活动数据。
// 第二层并发：同时获取 Issues、PRs、Issue Comments、Review Comments。
// 第三层并发：并发获取每个 PR 的 Review。
func collectRepo(ctx context.Context, client *github.Client, owner, repo string, since time.Time, user string, progress Progress, repoIndex int) (*RepoReport, error) {
	rr := &RepoReport{
		Owner:   owner,
		Repo:    repo,
		Reviews: make(map[int][]*gh.PullRequestReview),
	}

	// 第二层并发：4 个列表接口同时发起
	var (
		wg             sync.WaitGroup
		rawIssues      []*gh.Issue
		rawPRs         []*gh.PullRequest
		rawComments    []*gh.IssueComment
		rawRevComments []*gh.PullRequestComment
		errIssues      error
		errPRs         error
		errComments    error
		errRevComments error
	)

	wg.Add(4)

	go func() {
		defer wg.Done()
		rawIssues, errIssues = client.ListIssues(ctx, owner, repo, since)
		if progress != nil {
			progress.Increment(repoIndex)
		}
	}()

	go func() {
		defer wg.Done()
		rawPRs, errPRs = client.ListPullRequests(ctx, owner, repo, since)
		if progress != nil {
			progress.Increment(repoIndex)
		}
	}()

	go func() {
		defer wg.Done()
		rawComments, errComments = client.ListIssueComments(ctx, owner, repo, since)
		if progress != nil {
			progress.Increment(repoIndex)
		}
	}()

	go func() {
		defer wg.Done()
		rawRevComments, errRevComments = client.ListReviewComments(ctx, owner, repo, since)
		if progress != nil {
			progress.Increment(repoIndex)
		}
	}()

	wg.Wait()

	// 检查错误
	if errIssues != nil {
		return nil, fmt.Errorf("listing issues for %s/%s: %w", owner, repo, errIssues)
	}
	if errPRs != nil {
		return nil, fmt.Errorf("listing PRs for %s/%s: %w", owner, repo, errPRs)
	}
	if errComments != nil {
		return nil, fmt.Errorf("listing issue comments for %s/%s: %w", owner, repo, errComments)
	}
	if errRevComments != nil {
		return nil, fmt.Errorf("listing review comments for %s/%s: %w", owner, repo, errRevComments)
	}

	// 按用户过滤 Issues（排除 PR）
	for _, issue := range rawIssues {
		if issue.IsPullRequest() {
			continue
		}
		if user == "" || issue.GetUser().GetLogin() == user {
			rr.Issues = append(rr.Issues, issue)
		}
	}

	// 按用户过滤 Pull Requests
	// 仅保留在时间范围内有实际活动（创建、合并、关闭）或仍处于 open 状态的 PR
	for _, pr := range rawPRs {
		if user != "" && pr.GetUser().GetLogin() != user {
			continue
		}
		if !prHasActivitySince(pr, since) {
			continue
		}
		rr.PullRequests = append(rr.PullRequests, pr)
	}

	// 按用户过滤 Issue Comments
	for _, c := range rawComments {
		if user == "" || c.GetUser().GetLogin() == user {
			rr.IssueComments = append(rr.IssueComments, c)
		}
	}

	// 按用户过滤 Review Comments
	for _, rc := range rawRevComments {
		if user == "" || rc.GetUser().GetLogin() == user {
			rr.ReviewComments = append(rr.ReviewComments, rc)
		}
	}

	// 第三层并发：并发获取每个 PR 的 Review
	if len(rr.PullRequests) > 0 {
		if progress != nil {
			progress.SetTotal(repoIndex, 4+len(rr.PullRequests))
		}

		reviewResults := make([][]*gh.PullRequestReview, len(rr.PullRequests))
		reviewErrs := make([]error, len(rr.PullRequests))

		var reviewWg sync.WaitGroup
		for i, pr := range rr.PullRequests {
			reviewWg.Add(1)
			go func(idx int, prNumber int) {
				defer reviewWg.Done()
				reviewResults[idx], reviewErrs[idx] = client.ListReviews(ctx, owner, repo, prNumber)
				if progress != nil {
					progress.Increment(repoIndex)
				}
			}(i, pr.GetNumber())
		}
		reviewWg.Wait()

		// 检查错误并填充 Reviews map
		for i, pr := range rr.PullRequests {
			if reviewErrs[i] != nil {
				return nil, fmt.Errorf("listing reviews for %s/%s#%d: %w", owner, repo, pr.GetNumber(), reviewErrs[i])
			}
			if len(reviewResults[i]) > 0 {
				rr.Reviews[pr.GetNumber()] = reviewResults[i]
			}
		}
	}

	return rr, nil
}

// prHasActivitySince 判断 PR 在指定时间之后是否有实际活动。
// 仅当 PR 在时间范围内创建、合并、关闭，或仍处于 open 状态时返回 true。
// 避免因 GitHub 自动更新 UpdatedAt（如 bot 评论、标签变更）导致旧 PR 被误收录。
func prHasActivitySince(pr *gh.PullRequest, since time.Time) bool {
	if pr.GetState() == "open" {
		return true
	}
	if !pr.GetCreatedAt().Before(since) {
		return true
	}
	if pr.MergedAt != nil && !pr.MergedAt.Before(since) {
		return true
	}
	if pr.ClosedAt != nil && !pr.ClosedAt.Before(since) {
		return true
	}
	return false
}
