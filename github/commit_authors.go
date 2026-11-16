package github

import (
	"context"
	"fmt"
	"strings"
)

// LookupCommitAuthors 批量关联本地作者的代表提交，只查询身份，不下载提交 diff。
// GitHub 上不存在的本地提交或未关联账号的作者不出现在结果中。
func (c *Client) LookupCommitAuthors(ctx context.Context, owner, repo string, shas []string) (map[string]CommitAuthor, error) {
	result := make(map[string]CommitAuthor)
	for start := 0; start < len(shas); start += 50 {
		batch := shas[start:min(start+50, len(shas))]
		declarations := []string{"$owner: String!", "$repo: String!"}
		fields := make([]string, len(batch))
		vars := map[string]any{"owner": owner, "repo": repo}
		for i, sha := range batch {
			name := fmt.Sprintf("sha%d", i)
			declarations = append(declarations, "$"+name+": String!")
			fields[i] = fmt.Sprintf("c%d: object(expression: $%s) { ... on Commit { author { name email user { id login } } } }", i, name)
			vars[name] = sha
		}
		query := "query(" + strings.Join(declarations, ", ") + ") { repository(owner: $owner, name: $repo) { " + strings.Join(fields, "\n") + " } }"
		var resp struct {
			Repository map[string]*struct{ Author CommitAuthor }
		}
		if err := c.GraphQL(ctx, query, vars, &resp); err != nil {
			return nil, fmt.Errorf("关联本地提交作者: %w", err)
		}
		if resp.Repository == nil {
			return nil, fmt.Errorf("仓库 %s/%s 不存在或不可访问", owner, repo)
		}
		for i, sha := range batch {
			commit, ok := resp.Repository[fmt.Sprintf("c%d", i)]
			if !ok {
				return nil, fmt.Errorf("本地提交 %s 的作者查询结果不完整", sha)
			}
			if commit != nil && commit.Author.User != nil && commit.Author.User.ID != "" {
				result[sha] = commit.Author
			}
		}
	}
	return result, nil
}
