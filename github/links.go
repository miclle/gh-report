package github

import (
	"context"
	"fmt"
)

// LinkedIssue 表示 GitHub 明确关联到 PR 的 Issue，仅作为任务归并依据。
type LinkedIssue struct {
	Repo   string
	Number int
	Title  string
	URL    string
	State  string
}

const closingIssuesQuery = `
query($owner: String!, $repo: String!, $number: Int!, $cursor: String) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      closingIssuesReferences(first: 100, after: $cursor, excludeUserLinked: false) {
        pageInfo { hasNextPage endCursor }
        nodes { number title url state repository { nameWithOwner } }
      }
    }
  }
}`

// ListClosingIssues 获取 PR 明确关联的全部 Issue（含手动关联和跨仓库关联）。
// 任意一页失败时不返回部分关系，避免使用不完整结果归并任务。
func (c *Client) ListClosingIssues(ctx context.Context, owner, repo string, number int) ([]LinkedIssue, error) {
	var all []LinkedIssue
	var cursor string
	for {
		vars := map[string]any{"owner": owner, "repo": repo, "number": number}
		if cursor != "" {
			vars["cursor"] = cursor
		}
		var resp struct {
			Repository *struct {
				PullRequest *struct {
					ClosingIssuesReferences struct {
						PageInfo gqlPageInfo
						Nodes    []struct {
							Number            int
							Title, URL, State string
							Repository        struct{ NameWithOwner string }
						}
					}
				}
			}
		}
		if err := c.GraphQL(ctx, closingIssuesQuery, vars, &resp); err != nil {
			return nil, err
		}
		if resp.Repository == nil || resp.Repository.PullRequest == nil {
			return nil, fmt.Errorf("PR %s/%s#%d 不存在或不可访问", owner, repo, number)
		}
		connection := resp.Repository.PullRequest.ClosingIssuesReferences
		for _, issue := range connection.Nodes {
			if issue.Number <= 0 || issue.Repository.NameWithOwner == "" || issue.URL == "" {
				return nil, fmt.Errorf("PR %s/%s#%d 的关联 Issue 数据不完整", owner, repo, number)
			}
			all = append(all, LinkedIssue{
				Repo: issue.Repository.NameWithOwner, Number: issue.Number,
				Title: issue.Title, URL: issue.URL, State: issue.State,
			})
		}
		if !connection.PageInfo.HasNextPage {
			return all, nil
		}
		if connection.PageInfo.EndCursor == "" || connection.PageInfo.EndCursor == cursor {
			return nil, fmt.Errorf("PR %s/%s#%d 的关联分页游标无效", owner, repo, number)
		}
		cursor = connection.PageInfo.EndCursor
	}
}
