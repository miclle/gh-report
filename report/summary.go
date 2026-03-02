package report

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	gh "github.com/google/go-github/v69/github"

	"github.com/miclle/gh-report/github"
)

// WorkItem 表示今日的一条工作活动。
type WorkItem struct {
	Type       string // "pr", "issue", "comment", "review"
	Repo       string // "owner/repo"
	Number     int
	Title      string
	State      string // "merged", "open", "closed", "draft"
	URL        string
	ReviewInfo string // PR 的 review 摘要
}

// PlanItem 表示明日计划的一条项目。
type PlanItem struct {
	Repo   string
	Number int
	Title  string
	URL    string
	Status string // Project item status（如 "P0", "In Development"）
	Source string // "open_pr" 或 "project_item"
}

// extractWorkItems 从报告数据中提取今日工作条目。
// 始终以当天零点为基准过滤，只保留今天有实际活动（创建、合并、关闭）的条目。
func extractWorkItems(reports []RepoReport, user string) []WorkItem {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var items []WorkItem
	// 记录用户作为 PR 作者的所有条目（不限日期），用于去重评论和 review
	prAuthorKeys := make(map[string]bool)

	for _, rr := range reports {
		fullRepo := rr.Owner + "/" + rr.Repo

		// 先收集用户的所有 PR 编号，用于后续去重（用户自己 PR 上的评论不单独列出）
		for _, pr := range rr.PullRequests {
			if user != "" && pr.GetUser().GetLogin() != user {
				continue
			}
			prAuthorKeys[fmt.Sprintf("%s#%d", fullRepo, pr.GetNumber())] = true
		}

		// 用户的 PR（只保留在时间范围内创建、合并或关闭的）
		for _, pr := range rr.PullRequests {
			if user != "" && pr.GetUser().GetLogin() != user {
				continue
			}
			if !prWorkedSince(pr, today) {
				continue
			}
			state := prDisplayState(pr)
			reviews := buildReviewSummary(rr.Reviews[pr.GetNumber()])

			items = append(items, WorkItem{
				Type:       "pr",
				Repo:       fullRepo,
				Number:     pr.GetNumber(),
				Title:      pr.GetTitle(),
				State:      state,
				URL:        pr.GetHTMLURL(),
				ReviewInfo: reviews,
			})
		}

		// 用户的 Issue（只保留在时间范围内创建或关闭的）
		for _, issue := range rr.Issues {
			if user != "" && issue.GetUser().GetLogin() != user {
				continue
			}
			// 有 Assignees 但不包含当前用户时跳过（属于别人的任务）
			if user != "" && len(issue.Assignees) > 0 && !hasAssignee(issue.Assignees, user) {
				continue
			}
			if !issueWorkedSince(issue, today) {
				continue
			}
			items = append(items, WorkItem{
				Type:   "issue",
				Repo:   fullRepo,
				Number: issue.GetNumber(),
				Title:  issue.GetTitle(),
				State:  issue.GetState(),
				URL:    issue.GetHTMLURL(),
			})
		}

		// Issue 评论（按 Issue 分组去重，只记一条；只保留今天的评论）
		commentedIssues := make(map[string]bool)
		for _, c := range rr.IssueComments {
			if user != "" && c.GetUser().GetLogin() != user {
				continue
			}
			if c.GetCreatedAt().Before(today) {
				continue
			}
			num := extractNumber(c.GetIssueURL())
			key := fullRepo + "#" + num
			// 如果用户是 PR 作者，跳过该条评论
			if prAuthorKeys[key] {
				continue
			}
			if commentedIssues[key] {
				continue
			}
			commentedIssues[key] = true
			n, _ := strconv.Atoi(num)
			items = append(items, WorkItem{
				Type:   "comment",
				Repo:   fullRepo,
				Number: n,
				Title:  fmt.Sprintf("Commented on #%s", num),
				URL:    c.GetHTMLURL(),
			})
		}

		// Review（审查他人 PR，按 PR 分组去重；只保留今天的 review）
		reviewedPRs := make(map[string]bool)
		for _, c := range rr.ReviewComments {
			if user != "" && c.GetUser().GetLogin() != user {
				continue
			}
			if c.GetCreatedAt().Before(today) {
				continue
			}
			num := extractNumber(c.GetPullRequestURL())
			key := fullRepo + "#" + num
			if prAuthorKeys[key] {
				continue
			}
			if reviewedPRs[key] {
				continue
			}
			reviewedPRs[key] = true
			n, _ := strconv.Atoi(num)
			items = append(items, WorkItem{
				Type:   "review",
				Repo:   fullRepo,
				Number: n,
				Title:  fmt.Sprintf("Reviewed PR #%s", num),
				URL:    c.GetHTMLURL(),
			})
		}
	}

	return items
}

// extractPlanItems 从报告数据中提取明日计划条目。
func extractPlanItems(reports []RepoReport, user string) []PlanItem {
	var items []PlanItem
	seen := make(map[string]bool) // 按 owner/repo#number 去重

	now := time.Now()

	// 来源 1：未合并且未关闭的 PR
	for _, rr := range reports {
		fullRepo := rr.Owner + "/" + rr.Repo
		for _, pr := range rr.PullRequests {
			if user != "" && pr.GetUser().GetLogin() != user {
				continue
			}
			// 有 Assignees 但不包含当前用户时跳过（属于别人的任务）
			if user != "" && len(pr.Assignees) > 0 && !hasAssignee(pr.Assignees, user) {
				continue
			}
			state := prDisplayState(pr)
			if state == "merged" || state == "closed" {
				continue
			}
			key := fmt.Sprintf("%s#%d", fullRepo, pr.GetNumber())
			if seen[key] {
				continue
			}
			seen[key] = true
			items = append(items, PlanItem{
				Repo:   fullRepo,
				Number: pr.GetNumber(),
				Title:  pr.GetTitle(),
				URL:    pr.GetHTMLURL(),
				Source: "open_pr",
			})
		}
	}

	// 来源 2：当前迭代中未完成的项目
	for _, rr := range reports {
		fullRepo := rr.Owner + "/" + rr.Repo
		for _, project := range rr.Projects {
			relevant := github.FindRelevantIterations(project.Iterations, now)
			if relevant.Current == nil {
				continue
			}
			for _, item := range project.Items {
				if item.Iteration != relevant.Current.Title {
					continue
				}
				if !strings.Contains(item.URL, fullRepo) {
					continue
				}
				// 指定用户时，只纳入 Assignees 包含该用户的项目
				if user != "" && !containsString(item.Assignees, user) {
					continue
				}
				// 跳过已完成的项目
				upperStatus := strings.ToUpper(item.Status)
				upperState := strings.ToUpper(item.State)
				if upperStatus == "DONE" || upperState == "CLOSED" || upperState == "MERGED" {
					continue
				}
				key := fmt.Sprintf("%s#%d", fullRepo, item.Number)
				if seen[key] {
					// 如果已经从 open_pr 来源添加，补充 status 信息
					for i := range items {
						if fmt.Sprintf("%s#%d", items[i].Repo, items[i].Number) == key && items[i].Status == "" {
							items[i].Status = item.Status
						}
					}
					continue
				}
				seen[key] = true
				items = append(items, PlanItem{
					Repo:   fullRepo,
					Number: item.Number,
					Title:  item.Title,
					URL:    item.URL,
					Status: item.Status,
					Source: "project_item",
				})
			}
		}
	}

	return items
}

// buildPromptTemplate 构建 Prompt 指令模板。
func buildPromptTemplate(since, until time.Time, user string) string {
	dateRange := fmt.Sprintf("%s ~ %s", since.Format("2006-01-02"), until.Format("2006-01-02"))

	return fmt.Sprintf(`你是一个工作日报助手。请根据以下活动数据，生成工作日报。

日期范围: %s
用户: %s

请严格按照以下格式输出，不要添加任何额外内容:

今日工作
<工作描述>, <状态>, <URL>

明日计划
<计划描述>, <URL>

格式要求:
- 每条记录一行
- PR 状态映射: merged→已合并, open(有 review)→已提交(审查中), open(无 review)→已提交, draft→草稿, closed→已关闭
- Issue 状态: open→进行中, closed→已关闭
- 评论和 review 类型的活动描述参考格式: 参与 Issue #N / Review PR #N 讨论
- 明日计划来自未完成的 PR 和当前迭代中未完成的工作项
- 明日计划不要包含任何状态或优先级标识（如 Testing、开发中、Todo、P0、P1 等）

以下是活动数据:
`, dateRange, user)
}

// formatWorkData 将今日工作数据格式化为文本。
func formatWorkData(items []WorkItem) string {
	if len(items) == 0 {
		return "（无今日工作数据）\n"
	}
	var sb strings.Builder
	for _, item := range items {
		switch item.Type {
		case "pr":
			sb.WriteString(fmt.Sprintf("- [PR] %s#%d %s | 状态: %s | Review: %s | %s\n",
				item.Repo, item.Number, item.Title, item.State, item.ReviewInfo, item.URL))
		case "issue":
			sb.WriteString(fmt.Sprintf("- [Issue] %s#%d %s | 状态: %s | %s\n",
				item.Repo, item.Number, item.Title, item.State, item.URL))
		case "comment":
			sb.WriteString(fmt.Sprintf("- [Comment] %s#%d %s | %s\n",
				item.Repo, item.Number, item.Title, item.URL))
		case "review":
			sb.WriteString(fmt.Sprintf("- [Review] %s#%d %s | %s\n",
				item.Repo, item.Number, item.Title, item.URL))
		}
	}
	return sb.String()
}

// formatPlanData 将明日计划数据格式化为文本。
func formatPlanData(items []PlanItem) string {
	if len(items) == 0 {
		return "（无明日计划数据）\n"
	}
	var sb strings.Builder
	for _, item := range items {
		status := ""
		if item.Status != "" {
			status = fmt.Sprintf(" | Status: %s", item.Status)
		}
		sb.WriteString(fmt.Sprintf("- [%s] %s#%d %s%s | %s\n",
			item.Source, item.Repo, item.Number, item.Title, status, item.URL))
	}
	return sb.String()
}

// PrintSummaryData 输出结构化的今日工作和明日计划数据，以及可供手动粘贴给 AI 的 Prompt 模板。
func PrintSummaryData(w io.Writer, reports []RepoReport, since, until time.Time, user string) {
	workItems := extractWorkItems(reports, user)
	planItems := extractPlanItems(reports, user)

	// 输出结构化数据
	fmt.Fprintln(w, "========== 今日工作 ==========")
	fmt.Fprint(w, formatWorkData(workItems))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "========== 明日计划 ==========")
	fmt.Fprint(w, formatPlanData(planItems))
	fmt.Fprintln(w)

	// 输出完整 Prompt（方便用户复制粘贴给 AI）
	fmt.Fprintln(w, "========== Prompt（复制以下内容粘贴给 AI）==========")
	fmt.Fprintln(w)
	fmt.Fprint(w, BuildSummaryPrompt(reports, since, until, user))
}

// BuildSummaryPrompt 构建完整的 Prompt 文本，供 API 调用或手动粘贴。
func BuildSummaryPrompt(reports []RepoReport, since, until time.Time, user string) string {
	workItems := extractWorkItems(reports, user)
	planItems := extractPlanItems(reports, user)

	var sb strings.Builder
	sb.WriteString(buildPromptTemplate(since, until, user))
	sb.WriteString("\n")
	sb.WriteString("=== 今日工作数据 ===\n")
	sb.WriteString(formatWorkData(workItems))
	sb.WriteString("\n")
	sb.WriteString("=== 明日计划数据 ===\n")
	sb.WriteString(formatPlanData(planItems))

	return sb.String()
}

// hasAssignee 检查 GitHub User 列表中是否包含指定用户。
func hasAssignee(assignees []*gh.User, login string) bool {
	for _, a := range assignees {
		if a.GetLogin() == login {
			return true
		}
	}
	return false
}

// prWorkedSince 判断 PR 是否应纳入今日工作。
// open/draft 状态的 PR 视为进行中的工作，始终纳入；已合并或已关闭的 PR 仅在当天操作时纳入。
func prWorkedSince(pr *gh.PullRequest, since time.Time) bool {
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

// issueWorkedSince 判断 Issue 是否应纳入今日工作。
// open 状态的 Issue 视为进行中的工作，始终纳入；已关闭的 Issue 仅在当天关闭时纳入。
func issueWorkedSince(issue *gh.Issue, since time.Time) bool {
	if issue.GetState() == "open" {
		return true
	}
	if !issue.GetCreatedAt().Before(since) {
		return true
	}
	if issue.ClosedAt != nil && !issue.ClosedAt.Before(since) {
		return true
	}
	return false
}

// containsString 检查字符串切片中是否包含指定值。
func containsString(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}
