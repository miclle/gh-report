package github

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// projectsQuery 获取组织下的 Projects v2 列表及迭代字段配置（不含工作项）。
const projectsQuery = `
query($org: String!, $cursor: String) {
  organization(login: $org) {
    projectsV2(first: 20, after: $cursor) {
      pageInfo { hasNextPage endCursor }
      nodes {
        id
        title
        number
        fields(first: 30) {
          nodes {
            ... on ProjectV2IterationField {
              id
              name
              configuration {
                iterations {
                  id
                  title
                  startDate
                  duration
                }
                completedIterations {
                  id
                  title
                  startDate
                  duration
                }
              }
            }
          }
        }
      }
    }
  }
}
`

// projectItemsQuery 分页获取单个 Project 的工作项。
const projectItemsQuery = `
query($projectID: ID!, $cursor: String) {
  node(id: $projectID) {
    ... on ProjectV2 {
      items(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
        nodes {
          content {
            ... on Issue {
              title
              number
              url
              state
              assignees(first: 10) { nodes { login } }
            }
            ... on PullRequest {
              title
              number
              url
              state
              assignees(first: 10) { nodes { login } }
            }
          }
          type
          fieldValues(first: 20) {
            nodes {
              ... on ProjectV2ItemFieldIterationValue {
                title
                iterationId
              }
              ... on ProjectV2ItemFieldSingleSelectValue {
                name
              }
            }
          }
        }
      }
    }
  }
}
`

// gqlProjectsResponse 是 Projects 列表查询的响应结构。
type gqlProjectsResponse struct {
	Organization struct {
		ProjectsV2 struct {
			PageInfo gqlPageInfo  `json:"pageInfo"`
			Nodes    []gqlProject `json:"nodes"`
		} `json:"projectsV2"`
	} `json:"organization"`
}

// gqlProjectItemsResponse 是单个 Project 工作项查询的响应结构。
type gqlProjectItemsResponse struct {
	Node struct {
		Items struct {
			PageInfo gqlPageInfo    `json:"pageInfo"`
			Nodes    []gqlProjectItem `json:"nodes"`
		} `json:"items"`
	} `json:"node"`
}

// gqlPageInfo 是 GraphQL 分页信息。
type gqlPageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// gqlProject 是单个项目的 GraphQL 响应结构。
type gqlProject struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Number int    `json:"number"`
	Fields struct {
		Nodes []json.RawMessage `json:"nodes"`
	} `json:"fields"`
}

// gqlIterationField 是迭代字段的 GraphQL 响应结构。
type gqlIterationField struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Configuration struct {
		Iterations          []ProjectIteration `json:"iterations"`
		CompletedIterations []ProjectIteration `json:"completedIterations"`
	} `json:"configuration"`
}

// gqlProjectItem 是项目工作项的 GraphQL 响应结构。
type gqlProjectItem struct {
	Content struct {
		Title     string `json:"title"`
		Number    int    `json:"number"`
		URL       string `json:"url"`
		State     string `json:"state"`
		Assignees struct {
			Nodes []struct {
				Login string `json:"login"`
			} `json:"nodes"`
		} `json:"assignees"`
	} `json:"content"`
	Type        string `json:"type"`
	FieldValues struct {
		Nodes []json.RawMessage `json:"nodes"`
	} `json:"fieldValues"`
}

// ListProjects 获取指定组织下的所有 Projects v2 项目，包含迭代和工作项数据。
// 先获取项目列表及迭代配置，再对每个项目分页获取全部工作项。
func (c *Client) ListProjects(ctx context.Context, org string) ([]Project, error) {
	// 第一步：获取所有项目及其迭代字段
	type projectMeta struct {
		id     string
		project Project
	}
	var metas []projectMeta

	var cursor *string
	for {
		vars := map[string]any{"org": org}
		if cursor != nil {
			vars["cursor"] = *cursor
		}

		var resp gqlProjectsResponse
		if err := c.GraphQL(ctx, projectsQuery, vars, &resp); err != nil {
			return nil, fmt.Errorf("fetching projects for %s: %w", org, err)
		}

		for _, gp := range resp.Organization.ProjectsV2.Nodes {
			p := Project{
				Title:  gp.Title,
				Number: gp.Number,
			}
			// 解析迭代字段
			for _, raw := range gp.Fields.Nodes {
				var field gqlIterationField
				if err := json.Unmarshal(raw, &field); err != nil || field.ID == "" {
					continue
				}
				allIter := append(field.Configuration.Iterations, field.Configuration.CompletedIterations...)
				p.Iterations = append(p.Iterations, allIter...)
			}
			metas = append(metas, projectMeta{id: gp.ID, project: p})
		}

		if !resp.Organization.ProjectsV2.PageInfo.HasNextPage {
			break
		}
		next := resp.Organization.ProjectsV2.PageInfo.EndCursor
		cursor = &next
	}

	// 第二步：并发获取每个项目的全部工作项
	allProjects := make([]Project, len(metas))
	itemErrs := make([]error, len(metas))
	var itemsWg sync.WaitGroup
	for i, m := range metas {
		itemsWg.Add(1)
		go func(idx int, projectID string, project Project) {
			defer itemsWg.Done()
			items, err := c.fetchAllProjectItems(ctx, projectID)
			if err != nil {
				itemErrs[idx] = fmt.Errorf("fetching items for project %q: %w", project.Title, err)
				return
			}
			project.Items = items
			allProjects[idx] = project
		}(i, m.id, m.project)
	}
	itemsWg.Wait()

	for _, err := range itemErrs {
		if err != nil {
			return nil, err
		}
	}

	return allProjects, nil
}

// fetchAllProjectItems 分页获取单个 Project 的全部工作项。
func (c *Client) fetchAllProjectItems(ctx context.Context, projectID string) ([]ProjectItem, error) {
	var all []ProjectItem

	var cursor *string
	for {
		vars := map[string]any{"projectID": projectID}
		if cursor != nil {
			vars["cursor"] = *cursor
		}

		var resp gqlProjectItemsResponse
		if err := c.GraphQL(ctx, projectItemsQuery, vars, &resp); err != nil {
			return nil, err
		}

		for _, item := range resp.Node.Items.Nodes {
			pi := parseProjectItem(item)
			if pi.Number == 0 {
				continue // 跳过草稿项或无内容的项
			}
			all = append(all, pi)
		}

		if !resp.Node.Items.PageInfo.HasNextPage {
			break
		}
		next := resp.Node.Items.PageInfo.EndCursor
		cursor = &next
	}

	return all, nil
}

// parseProjectItem 将 GraphQL 工作项响应转换为 ProjectItem。
func parseProjectItem(item gqlProjectItem) ProjectItem {
	pi := ProjectItem{
		Title:  item.Content.Title,
		Number: item.Content.Number,
		URL:    item.Content.URL,
		State:  item.Content.State,
		Type:   item.Type,
	}

	for _, node := range item.Content.Assignees.Nodes {
		pi.Assignees = append(pi.Assignees, node.Login)
	}

	// 从字段值中提取迭代标题和状态
	for _, fvRaw := range item.FieldValues.Nodes {
		var iterVal struct {
			Title       string `json:"title"`
			IterationID string `json:"iterationId"`
		}
		if err := json.Unmarshal(fvRaw, &iterVal); err == nil && iterVal.IterationID != "" {
			pi.Iteration = iterVal.Title
		}

		var selectVal struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(fvRaw, &selectVal); err == nil && selectVal.Name != "" {
			pi.Status = selectVal.Name
		}
	}

	return pi
}
