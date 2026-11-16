package contribution

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/miclle/gh-report/github"
)

// Collect 获取单仓库全历史贡献数据；失败时不返回部分报告。
func Collect(ctx context.Context, client *github.Client, opts Options) (*Report, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if opts.Sort == "" {
		opts.Sort = "commits"
	}
	var local *localRepository
	if opts.LocalPath != "" {
		var err error
		local, err = openLocalRepository(ctx, opts.LocalPath, opts.Repo)
		if err != nil {
			return nil, err
		}
		opts.Repo = local.repo
	}
	report := &Report{
		Repository: opts.Repo, StartedAt: time.Now().UTC(), Scope: "default_branch_history",
		Sort: opts.Sort, ExcludeBots: opts.ExcludeBots, Rules: statisticsRules(), CommitSource: "github_api",
	}
	parts := strings.SplitN(opts.Repo, "/", 2)
	owner, repo := parts[0], parts[1]
	head, err := client.GetDefaultBranchHead(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("获取默认分支: %w", err)
	}
	report.DefaultBranch, report.HeadSHA = head.Name, head.SHA
	a := newAccumulator()
	if local != nil {
		if err := collectLocalCommits(ctx, client, local, head.Name, report, a); err != nil {
			return nil, err
		}
	} else if head.SHA != "" {
		if err := client.WalkCommits(ctx, owner, repo, head.SHA, a.addCommit); err != nil {
			return nil, err
		}
	}
	// 零时间不发送 since，避免继承日报的近期活动过滤。
	issues, err := client.ListIssues(ctx, owner, repo, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("获取全部 Issues: %w", err)
	}
	seenIssues := make(map[int64]bool)
	for _, issue := range issues {
		if issue == nil || issue.GetID() == 0 {
			return nil, fmt.Errorf("仓库 %s 的 Issue 数据不完整", opts.Repo)
		}
		if !seenIssues[issue.GetID()] {
			a.addIssue(issue)
			seenIssues[issue.GetID()] = true
		}
	}
	prs, err := client.ListPullRequests(ctx, owner, repo, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("获取全部 PRs: %w", err)
	}
	seenPRs := make(map[int64]bool)
	for _, pr := range prs {
		if pr == nil || pr.GetID() == 0 {
			return nil, fmt.Errorf("仓库 %s 的 PR 数据不完整", opts.Repo)
		}
		if !seenPRs[pr.GetID()] {
			a.addPR(pr)
			seenPRs[pr.GetID()] = true
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a.finish(report)
	report.CollectedAt = time.Now().UTC()
	return report, nil
}
