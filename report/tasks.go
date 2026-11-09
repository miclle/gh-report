package report

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/miclle/gh-report/github"
)

// reportTask 以 Issue 为任务锚点，保留各来源独立的状态、日期和链接。
type reportTask struct {
	Title   string
	Sources []WorkItem
}

func itemKey(repo string, number int) string {
	return fmt.Sprintf("%s#%d", strings.ToLower(repo), number)
}

func projectItemInRepo(rawURL, repo string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	return len(parts) == 4 && strings.EqualFold(parts[0]+"/"+parts[1], repo)
}

func projectItemType(item github.ProjectItem) string {
	if strings.EqualFold(item.Type, "PullRequest") || strings.EqualFold(item.Type, "PULL_REQUEST") {
		return "pr"
	}
	return "issue"
}

func groupPlanItems(items []PlanItem, reports []RepoReport) []reportTask {
	sources := make([]WorkItem, 0, len(items))
	for _, item := range items {
		sources = append(sources, WorkItem{
			Type: item.Type, Repo: item.Repo, Number: item.Number,
			Title: item.Title, URL: item.URL,
		})
	}
	return groupTaskItems(sources, reports)
}

// groupTaskItems 只归并明确的 PR→Issue 关系，不沿共享 PR 继续合并不同 Issue。
// 输入已经通过用户和时间过滤；输入外的 Issue 只能作为背景来源。
func groupTaskItems(items []WorkItem, reports []RepoReport) []reportTask {
	issues := make(map[string]WorkItem)
	links := make(map[string][]github.LinkedIssue)
	for _, item := range items {
		if item.Type == "issue" {
			issues[itemKey(item.Repo, item.Number)] = item
		}
	}
	for _, rr := range reports {
		for number, linked := range rr.PullRequestIssues {
			links[itemKey(rr.Owner+"/"+rr.Repo, number)] = linked
		}
	}

	var tasks []reportTask
	indices := make(map[string]int)
	addSource := func(key, title string, source WorkItem) {
		idx, exists := indices[key]
		if !exists {
			idx = len(tasks)
			indices[key] = idx
			tasks = append(tasks, reportTask{Title: title})
		}
		for _, existing := range tasks[idx].Sources {
			if existing.Type == source.Type && itemKey(existing.Repo, existing.Number) == itemKey(source.Repo, source.Number) {
				return
			}
		}
		tasks[idx].Sources = append(tasks[idx].Sources, source)
	}
	for _, item := range items {
		key := itemKey(item.Repo, item.Number)
		grouped := false
		if item.Type == "pr" {
			for _, linked := range links[key] {
				if linked.Repo == "" || linked.Number <= 0 || linked.URL == "" {
					continue
				}
				issueKey := itemKey(linked.Repo, linked.Number)
				issue, exists := issues[issueKey]
				if !exists {
					issue = WorkItem{
						Type: "issue", Repo: linked.Repo, Number: linked.Number,
						Title: linked.Title, URL: linked.URL, State: strings.ToLower(linked.State),
						ContextOnly: true,
					}
				}
				addSource("issue:"+issueKey, issue.Title, issue)
				addSource("issue:"+issueKey, issue.Title, item)
				grouped = true
			}
		}
		if !grouped {
			addSource(item.Type+":"+key, item.Title, item)
		}
	}
	return tasks
}

// markdownText 转义描述中的 Markdown 符号并折叠换行，不用于 URL。
func markdownText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]", "*", "\\*", "_", "\\_", "`", "\\`", "<", "&lt;", ">", "&gt;").Replace(s)
}

func sourceState(item WorkItem) string {
	switch strings.ToLower(item.State) {
	case "merged":
		return "已合并"
	case "closed":
		return "已关闭"
	case "draft":
		return "草稿"
	case "open":
		if item.Type == "issue" {
			return "进行中"
		}
		if item.ReviewInfo != "" {
			return "已提交(审查中)"
		}
		return "已提交"
	default:
		return item.State
	}
}

// formatWorkData 输出任务及各来源事实，不从 PR 状态推断整个任务完成。
func formatWorkData(tasks []reportTask, rt ReportType) string {
	if len(tasks) == 0 {
		return "（无工作数据）\n"
	}
	var sb strings.Builder
	for _, task := range tasks {
		fmt.Fprintf(&sb, "- %s", markdownText(task.Title))
		prCount := 0
		for _, source := range task.Sources {
			if source.Type == "pr" {
				prCount++
			}
		}
		for _, source := range task.Sources {
			fmt.Fprintf(&sb, " | %s", source.URL)
			if source.ContextOnly {
				sb.WriteString(" （仅关联背景，不计本期活动）")
			}
			if rt != ReportDaily && source.Date != "" {
				fmt.Fprintf(&sb, " %s", source.Date)
			}
			if state := sourceState(source); state != "" {
				fmt.Fprintf(&sb, " %s", markdownText(state))
			}
			if source.Type == "pr" && prCount > 1 && source.Title != task.Title {
				fmt.Fprintf(&sb, "（%s）", markdownText(source.Title))
			}
			if source.ReviewInfo != "" {
				fmt.Fprintf(&sb, "；Review: %s", markdownText(source.ReviewInfo))
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// formatPlanData 每项计划只输出任务描述和所有来源链接，不展示状态或优先级。
func formatPlanData(tasks []reportTask) string {
	if len(tasks) == 0 {
		return "（无计划数据）\n"
	}
	var sb strings.Builder
	for _, task := range tasks {
		fmt.Fprintf(&sb, "- %s", markdownText(task.Title))
		for _, source := range task.Sources {
			fmt.Fprintf(&sb, " | %s", source.URL)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}
