package github

import (
	"context"
	"fmt"
)

// BranchHead 保存采集开始时默认分支的名称和固定提交 SHA。
type BranchHead struct {
	Name string
	SHA  string
}

// CommitAuthor 保存主作者的 Git 身份和可选的 GitHub 账号关联。
type CommitAuthor struct {
	Name, Email string
	User        *struct{ ID, Login string }
}

// CommitActivity 保存单个提交的贡献统计；空提交通过根树判断，保留二进制变更。
type CommitActivity struct {
	SHA                  string
	Author               CommitAuthor
	Additions, Deletions int64
	Merge, Empty         bool
}

const defaultBranchHeadQuery = `
query($owner: String!, $repo: String!) {
  repository(owner: $owner, name: $repo) {
    defaultBranchRef { name target { oid } }
  }
}`

const commitHistoryQuery = `
query($owner: String!, $repo: String!, $sha: String!, $cursor: String) {
  repository(owner: $owner, name: $repo) {
    object(expression: $sha) {
      ... on Commit {
        history(first: 50, after: $cursor) {
          pageInfo { hasNextPage endCursor }
          nodes {
            oid additions deletions
            author { name email user { id login } }
            tree { oid }
            parents(first: 1) { totalCount nodes { tree { oid } } }
          }
        }
      }
    }
  }
}`

// GetDefaultBranchHead 获取默认分支及其当前 SHA；空仓库返回零值。
func (c *Client) GetDefaultBranchHead(ctx context.Context, owner, repo string) (BranchHead, error) {
	var resp struct {
		Repository *struct {
			DefaultBranchRef *struct {
				Name   string
				Target struct{ OID string }
			}
		}
	}
	if err := c.GraphQL(ctx, defaultBranchHeadQuery, map[string]any{"owner": owner, "repo": repo}, &resp); err != nil {
		return BranchHead{}, err
	}
	if resp.Repository == nil {
		return BranchHead{}, fmt.Errorf("仓库 %s/%s 不存在或不可访问", owner, repo)
	}
	ref := resp.Repository.DefaultBranchRef
	if ref == nil {
		return BranchHead{}, nil
	}
	if ref.Name == "" || ref.Target.OID == "" {
		return BranchHead{}, fmt.Errorf("仓库 %s/%s 的默认分支数据不完整", owner, repo)
	}
	return BranchHead{Name: ref.Name, SHA: ref.Target.OID}, nil
}

// WalkCommits 分页遍历固定 SHA 可达的全部提交，按 SHA 去重并逐条交给 visit。
// 调用方必须在成功返回后才能发布统计，任何一页失败都意味着结果不完整。
func (c *Client) WalkCommits(ctx context.Context, owner, repo, sha string, visit func(CommitActivity) error) error {
	if sha == "" {
		return fmt.Errorf("提交 SHA 不能为空")
	}
	seen := make(map[string]bool)
	cursors := make(map[string]bool)
	var cursor string
	for {
		vars := map[string]any{"owner": owner, "repo": repo, "sha": sha}
		if cursor != "" {
			vars["cursor"] = cursor
		}
		var resp struct {
			Repository *struct {
				Object *struct {
					History *struct {
						PageInfo gqlPageInfo
						Nodes    []struct {
							OID                  string
							Additions, Deletions *int64
							Author               CommitAuthor
							Tree                 struct{ OID string }
							Parents              struct {
								TotalCount int
								Nodes      []struct{ Tree struct{ OID string } }
							}
						}
					}
				}
			}
		}
		if err := c.GraphQL(ctx, commitHistoryQuery, vars, &resp); err != nil {
			return fmt.Errorf("读取 %s/%s 提交历史: %w", owner, repo, err)
		}
		if resp.Repository == nil || resp.Repository.Object == nil || resp.Repository.Object.History == nil {
			return fmt.Errorf("仓库 %s/%s 的提交 %s 不存在或不可访问", owner, repo, sha)
		}
		history := resp.Repository.Object.History
		if len(history.Nodes) == 0 {
			return fmt.Errorf("提交 %s 的历史分页为空，无法确认统计完整性", sha)
		}
		for _, node := range history.Nodes {
			if err := ctx.Err(); err != nil {
				return err
			}
			if node.OID == "" || node.Tree.OID == "" || node.Additions == nil || node.Deletions == nil {
				return fmt.Errorf("仓库 %s/%s 的提交数据不完整", owner, repo)
			}
			if node.Parents.TotalCount > 0 && (len(node.Parents.Nodes) == 0 || node.Parents.Nodes[0].Tree.OID == "") {
				return fmt.Errorf("提交 %s 的父提交数据不完整", node.OID)
			}
			if seen[node.OID] {
				continue
			}
			seen[node.OID] = true
			// Git 的空根树；根提交也可能没有文件。
			empty := node.Tree.OID == "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
			if node.Parents.TotalCount > 0 {
				empty = node.Tree.OID == node.Parents.Nodes[0].Tree.OID
			}
			if err := visit(CommitActivity{
				SHA: node.OID, Author: node.Author, Additions: *node.Additions, Deletions: *node.Deletions,
				Merge: node.Parents.TotalCount > 1, Empty: empty,
			}); err != nil {
				return err
			}
		}
		if !history.PageInfo.HasNextPage {
			return nil
		}
		next := history.PageInfo.EndCursor
		if next == "" || cursors[next] {
			return fmt.Errorf("仓库 %s/%s 的提交分页游标无效", owner, repo)
		}
		cursors[next] = true
		cursor = next
	}
}
