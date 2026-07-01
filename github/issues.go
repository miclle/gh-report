package github

import (
	"context"
	"time"

	gh "github.com/google/go-github/v69/github"
)

// ListIssues 获取仓库中自指定时间以来有更新的 Issue 列表。
// 返回结果中包含 Pull Request（可通过 IsPullRequest 方法区分）。
func (c *Client) ListIssues(ctx context.Context, owner, repo string, since time.Time) ([]*gh.Issue, error) {
	opts := &gh.IssueListByRepoOptions{
		State:       "all",
		Since:       since,
		Sort:        "updated",
		ListOptions: gh.ListOptions{PerPage: 100},
	}

	var all []*gh.Issue
	for {
		var issues []*gh.Issue
		var resp *gh.Response
		if err := c.doGitHubRequest(ctx, func() error {
			var err error
			issues, resp, err = c.REST.Issues.ListByRepo(ctx, owner, repo, opts)
			return err
		}); err != nil {
			return nil, err
		}
		all = append(all, issues...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

// ListIssueComments 获取仓库中自指定时间以来的所有 Issue 评论。
// 包括 Issue 和 Pull Request 上的普通评论。
func (c *Client) ListIssueComments(ctx context.Context, owner, repo string, since time.Time) ([]*gh.IssueComment, error) {
	opts := &gh.IssueListCommentsOptions{
		Since:       &since,
		Sort:        gh.String("updated"),
		ListOptions: gh.ListOptions{PerPage: 100},
	}

	var all []*gh.IssueComment
	for {
		var comments []*gh.IssueComment
		var resp *gh.Response
		if err := c.doGitHubRequest(ctx, func() error {
			var err error
			comments, resp, err = c.REST.Issues.ListComments(ctx, owner, repo, 0, opts)
			return err
		}); err != nil {
			return nil, err
		}
		all = append(all, comments...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}
