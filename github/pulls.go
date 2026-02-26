package github

import (
	"context"
	"time"

	gh "github.com/google/go-github/v69/github"
)

// ListPullRequests 获取仓库的 Pull Request 列表，按更新时间倒序排列。
// 当遇到更新时间早于 since 的 PR 时停止获取。
func (c *Client) ListPullRequests(ctx context.Context, owner, repo string, since time.Time) ([]*gh.PullRequest, error) {
	opts := &gh.PullRequestListOptions{
		State:     "all",
		Sort:      "updated",
		Direction: "desc",
		ListOptions: gh.ListOptions{PerPage: 100},
	}

	var all []*gh.PullRequest
	for {
		prs, resp, err := c.REST.PullRequests.List(ctx, owner, repo, opts)
		if err != nil {
			return nil, err
		}

		for _, pr := range prs {
			if pr.GetUpdatedAt().Before(since) {
				return all, nil
			}
			all = append(all, pr)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

// ListReviews 获取指定 Pull Request 的所有 Review。
func (c *Client) ListReviews(ctx context.Context, owner, repo string, prNumber int) ([]*gh.PullRequestReview, error) {
	opts := &gh.ListOptions{PerPage: 100}

	var all []*gh.PullRequestReview
	for {
		reviews, resp, err := c.REST.PullRequests.ListReviews(ctx, owner, repo, prNumber, opts)
		if err != nil {
			return nil, err
		}
		all = append(all, reviews...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

// ListReviewComments 获取仓库中自指定时间以来的所有 PR Review 评论（代码行级别评论）。
func (c *Client) ListReviewComments(ctx context.Context, owner, repo string, since time.Time) ([]*gh.PullRequestComment, error) {
	opts := &gh.PullRequestListCommentsOptions{
		Since: since,
		Sort:  "updated",
		ListOptions: gh.ListOptions{PerPage: 100},
	}

	var all []*gh.PullRequestComment
	for {
		comments, resp, err := c.REST.PullRequests.ListComments(ctx, owner, repo, 0, opts)
		if err != nil {
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
