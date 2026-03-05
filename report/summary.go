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

// ReportType 表示报告类型。
type ReportType string

const (
	// ReportDaily 日报。
	ReportDaily ReportType = "daily"
	// ReportWeekly 周报。
	ReportWeekly ReportType = "weekly"
	// ReportMonthly 月报。
	ReportMonthly ReportType = "monthly"
	// ReportYearly 年报。
	ReportYearly ReportType = "yearly"
)

// reportTypeLabels 定义各报告类型的显示文本。
type reportTypeLabels struct {
	workTitle    string // 工作标题（如 "今日工作"）
	planTitle    string // 计划标题（如 "明日计划"）
	roleName     string // AI 角色名称（如 "工作日报助手"）
	reportName   string // 报告名称（如 "日报"）
	planDesc     string // 计划来源说明
	noPlanStatus string // 计划状态排除说明
	dateHint     string // 日期格式提示（非日报时提示 AI 保留日期前缀）
}

// labelsForType 返回指定报告类型的显示文本。
func labelsForType(rt ReportType) reportTypeLabels {
	switch rt {
	case ReportWeekly:
		return reportTypeLabels{
			workTitle:    "本周工作",
			planTitle:    "下周计划",
			roleName:     "工作周报助手",
			reportName:   "周报",
			planDesc:     "下周计划来自未完成的 PR 和当前迭代中未完成的工作项",
			noPlanStatus: "下周计划不要包含任何状态或优先级标识（如 Testing、开发中、Todo、P0、P1 等）",
			dateHint:     "每条工作记录前有日期前缀，请在输出中保留该日期",
		}
	case ReportMonthly:
		return reportTypeLabels{
			workTitle:    "本月工作",
			planTitle:    "下月计划",
			roleName:     "工作月报助手",
			reportName:   "月报",
			planDesc:     "下月计划来自未完成的 PR 和当前迭代中未完成的工作项",
			noPlanStatus: "下月计划不要包含任何状态或优先级标识（如 Testing、开发中、Todo、P0、P1 等）",
			dateHint:     "每条工作记录前有日期前缀，请在输出中保留该日期",
		}
	case ReportYearly:
		return reportTypeLabels{
			workTitle:    "年度工作",
			planTitle:    "下年计划",
			roleName:     "年度总结助手",
			reportName:   "年报",
			planDesc:     "下年计划来自未完成的 PR 和当前迭代中未完成的工作项",
			noPlanStatus: "下年计划不要包含任何状态或优先级标识（如 Testing、开发中、Todo、P0、P1 等）",
			dateHint:     "每条工作记录前有日期前缀，请在输出中保留该日期",
		}
	default:
		return reportTypeLabels{
			workTitle:    "今日工作",
			planTitle:    "明日计划",
			roleName:     "工作日报助手",
			reportName:   "日报",
			planDesc:     "明日计划来自未完成的 PR 和当前迭代中未完成的工作项",
			noPlanStatus: "明日计划不要包含任何状态或优先级标识（如 Testing、开发中、Todo、P0、P1 等）",
		}
	}
}

// workTimeCutoff 根据报告类型计算工作条目的时间过滤基准。
// 日报：当天零点；周报：本周一零点；月报：本月一号零点；年报：今年一月一号零点。
func workTimeCutoff(rt ReportType) time.Time {
	now := time.Now()
	switch rt {
	case ReportWeekly:
		// 本周一零点
		weekday := now.Weekday()
		if weekday == time.Sunday {
			weekday = 7
		}
		monday := now.AddDate(0, 0, -int(weekday-time.Monday))
		return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, now.Location())
	case ReportMonthly:
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	case ReportYearly:
		return time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
	default:
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	}
}

// WorkItem 表示一条工作活动。
type WorkItem struct {
	Type       string // "pr", "issue", "comment", "review"
	Repo       string // "owner/repo"
	Number     int
	Title      string
	State      string // "merged", "open", "closed", "draft"
	URL        string
	ReviewInfo string // PR 的 review 摘要
	Date       string // 活动日期，格式 "2006-01-02"
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

// extractWorkItems 从报告数据中提取工作条目。
// 根据 reportType 确定时间过滤基准：日报用当天零点，周报用本周一零点，月报用本月一号零点，年报用今年一月一号零点。
func extractWorkItems(reports []RepoReport, user string, rt ReportType) []WorkItem {
	cutoff := workTimeCutoff(rt)
	var items []WorkItem
	// 记录用户作为 PR 作者的所有条目（不限日期），用于去重评论和 review
	prAuthorKeys := make(map[string]bool)
	// 记录已纳入工作的 Issue，用于去重评论
	issueKeys := make(map[string]bool)

	for _, rr := range reports {
		fullRepo := rr.Owner + "/" + rr.Repo

		// 用户的 PR：收集 prAuthorKeys 用于评论/review 去重，同时过滤今日活动条目
		for _, pr := range rr.PullRequests {
			if user != "" && pr.GetUser().GetLogin() != user {
				continue
			}
			prAuthorKeys[fmt.Sprintf("%s#%d", fullRepo, pr.GetNumber())] = true

			if !prHasActivitySince(pr, cutoff) {
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
				Date:       prActivityDate(pr),
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
			if !issueWorkedSince(issue, cutoff) {
				continue
			}
			issueKeys[fmt.Sprintf("%s#%d", fullRepo, issue.GetNumber())] = true
			items = append(items, WorkItem{
				Type:   "issue",
				Repo:   fullRepo,
				Number: issue.GetNumber(),
				Title:  issue.GetTitle(),
				State:  issue.GetState(),
				URL:    issue.GetHTMLURL(),
				Date:   issueActivityDate(issue),
			})
		}

		// Issue 评论（按 Issue 分组去重，只记一条；只保留时间范围内的评论）
		commentedIssues := make(map[string]bool)
		for _, c := range rr.IssueComments {
			if user != "" && c.GetUser().GetLogin() != user {
				continue
			}
			if c.GetCreatedAt().Before(cutoff) {
				continue
			}
			num := extractNumber(c.GetIssueURL())
			key := fullRepo + "#" + num
			// 如果用户是 PR 作者，跳过该条评论
			if prAuthorKeys[key] {
				continue
			}
			// 如果该 Issue 已纳入工作条目，跳过评论（避免重复）
			if issueKeys[key] {
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
				Date:   c.GetCreatedAt().Format("2006-01-02"),
			})
		}

		// Review（审查他人 PR，按 PR 分组去重；只保留时间范围内的 review）
		reviewedPRs := make(map[string]bool)
		for _, c := range rr.ReviewComments {
			if user != "" && c.GetUser().GetLogin() != user {
				continue
			}
			if c.GetCreatedAt().Before(cutoff) {
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
				Date:   c.GetCreatedAt().Format("2006-01-02"),
			})
		}
	}

	return items
}

// extractPlanItems 从报告数据中提取明日计划条目。
func extractPlanItems(reports []RepoReport, user string) []PlanItem {
	var items []PlanItem
	seen := make(map[string]int) // 按 owner/repo#number 去重，值为 items 中的索引

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
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = len(items)
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
				if idx, ok := seen[key]; ok {
					// 如果已经从 open_pr 来源添加，补充 status 信息
					if items[idx].Status == "" {
						items[idx].Status = item.Status
					}
					continue
				}
				seen[key] = len(items)
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

// prActivityDate 返回 PR 最具代表性的活动日期。
// 优先级：merged > closed > created。
func prActivityDate(pr *gh.PullRequest) string {
	if pr.MergedAt != nil {
		return pr.MergedAt.Format("2006-01-02")
	}
	if pr.ClosedAt != nil {
		return pr.ClosedAt.Format("2006-01-02")
	}
	return pr.GetCreatedAt().Format("2006-01-02")
}

// issueActivityDate 返回 Issue 最具代表性的活动日期。
// 优先级：closed > created。
func issueActivityDate(issue *gh.Issue) string {
	if issue.ClosedAt != nil {
		return issue.ClosedAt.Format("2006-01-02")
	}
	return issue.GetCreatedAt().Format("2006-01-02")
}

// buildPromptTemplate 构建 Prompt 指令模板，根据报告类型调整措辞。
func buildPromptTemplate(since, until time.Time, user string, rt ReportType) string {
	labels := labelsForType(rt)
	dateRange := fmt.Sprintf("%s ~ %s", since.Format("2006-01-02"), until.Format("2006-01-02"))

	var sb strings.Builder
	fmt.Fprintf(&sb, "你是一个%s。请根据以下活动数据，生成%s。\n\n", labels.roleName, labels.reportName)
	fmt.Fprintf(&sb, "日期范围: %s\n", dateRange)
	fmt.Fprintf(&sb, "用户: %s\n\n", user)
	sb.WriteString("请严格按照以下格式输出，不要添加任何额外内容:\n\n")
	fmt.Fprintf(&sb, "%s\n", labels.workTitle)
	sb.WriteString("<工作描述>, <状态>, <URL>\n\n")
	fmt.Fprintf(&sb, "%s\n", labels.planTitle)
	sb.WriteString("<计划描述>, <URL>\n\n")
	sb.WriteString("格式要求:\n")
	sb.WriteString("- 每条记录一行\n")
	sb.WriteString("- PR 状态映射: merged→已合并, open(有 review)→已提交(审查中), open(无 review)→已提交, draft→草稿, closed→已关闭\n")
	sb.WriteString("- Issue 状态: open→进行中, closed→已关闭\n")
	sb.WriteString("- 评论和 review 类型的活动描述参考格式: 参与 Issue #N / Review PR #N 讨论\n")
	fmt.Fprintf(&sb, "- %s\n", labels.planDesc)
	fmt.Fprintf(&sb, "- %s\n", labels.noPlanStatus)
	if labels.dateHint != "" {
		fmt.Fprintf(&sb, "- %s\n", labels.dateHint)
	}
	sb.WriteString("\n以下是活动数据:\n")

	return sb.String()
}

// formatWorkData 将工作数据格式化为文本。
// 非日报模式下，在标题前加上活动日期。
func formatWorkData(items []WorkItem, rt ReportType) string {
	if len(items) == 0 {
		return "（无工作数据）\n"
	}
	showDate := rt != ReportDaily
	var sb strings.Builder
	for _, item := range items {
		datePrefix := ""
		if showDate && item.Date != "" {
			datePrefix = item.Date + " "
		}
		switch item.Type {
		case "pr":
			fmt.Fprintf(&sb, "- [PR] %s#%d %s%s | 状态: %s | Review: %s | %s\n",
				item.Repo, item.Number, datePrefix, item.Title, item.State, item.ReviewInfo, item.URL)
		case "issue":
			fmt.Fprintf(&sb, "- [Issue] %s#%d %s%s | 状态: %s | %s\n",
				item.Repo, item.Number, datePrefix, item.Title, item.State, item.URL)
		case "comment":
			fmt.Fprintf(&sb, "- [Comment] %s#%d %s%s | %s\n",
				item.Repo, item.Number, datePrefix, item.Title, item.URL)
		case "review":
			fmt.Fprintf(&sb, "- [Review] %s#%d %s%s | %s\n",
				item.Repo, item.Number, datePrefix, item.Title, item.URL)
		}
	}
	return sb.String()
}

// formatPlanData 将计划数据格式化为文本。
func formatPlanData(items []PlanItem) string {
	if len(items) == 0 {
		return "（无计划数据）\n"
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

// PrintSummaryData 输出结构化的工作和计划数据，以及可供手动粘贴给 AI 的 Prompt 模板。
func PrintSummaryData(w io.Writer, reports []RepoReport, since, until time.Time, user string, rt ReportType) {
	labels := labelsForType(rt)
	workItems := extractWorkItems(reports, user, rt)
	planItems := extractPlanItems(reports, user)

	// 输出结构化数据
	fmt.Fprintf(w, "========== %s ==========\n", labels.workTitle)
	fmt.Fprint(w, formatWorkData(workItems, rt))
	fmt.Fprintln(w)
	fmt.Fprintf(w, "========== %s ==========\n", labels.planTitle)
	fmt.Fprint(w, formatPlanData(planItems))
	fmt.Fprintln(w)

	// 输出完整 Prompt（方便用户复制粘贴给 AI）
	fmt.Fprintln(w, "========== Prompt（复制以下内容粘贴给 AI）==========")
	fmt.Fprintln(w)
	fmt.Fprint(w, buildSummaryPromptFromItems(workItems, planItems, since, until, user, rt))
}

// BuildSummaryPrompt 构建完整的 Prompt 文本，供 API 调用或手动粘贴。
func BuildSummaryPrompt(reports []RepoReport, since, until time.Time, user string, rt ReportType) string {
	workItems := extractWorkItems(reports, user, rt)
	planItems := extractPlanItems(reports, user)
	return buildSummaryPromptFromItems(workItems, planItems, since, until, user, rt)
}

// buildSummaryPromptFromItems 根据已提取的工作和计划条目构建 Prompt 文本。
func buildSummaryPromptFromItems(workItems []WorkItem, planItems []PlanItem, since, until time.Time, user string, rt ReportType) string {
	labels := labelsForType(rt)
	var sb strings.Builder
	sb.WriteString(buildPromptTemplate(since, until, user, rt))
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "=== %s数据 ===\n", labels.workTitle)
	sb.WriteString(formatWorkData(workItems, rt))
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "=== %s数据 ===\n", labels.planTitle)
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
