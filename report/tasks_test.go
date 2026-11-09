package report

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	gh "github.com/google/go-github/v69/github"

	"github.com/miclle/gh-report/github"
)

func testSource(kind, repo string, number int, title, state string) WorkItem {
	path := "issues"
	if kind == "pr" {
		path = "pull"
	}
	return WorkItem{Type: kind, Repo: repo, Number: number, Title: title, State: state,
		URL: fmt.Sprintf("https://github.com/%s/%s/%d", repo, path, number), Date: "2026-11-09"}
}

func testLinked(item WorkItem) github.LinkedIssue {
	return github.LinkedIssue{Repo: item.Repo, Number: item.Number, Title: item.Title, URL: item.URL, State: strings.ToUpper(item.State)}
}

func TestTaskGroupingAndPlainURLs(t *testing.T) {
	issue := testSource("issue", "team/tasks", 100, "支持存储", "open")
	pr := testSource("pr", "team/code", 120, "实现存储", "merged")
	otherPR := testSource("pr", "team/code", 121, "补充验证", "open")
	otherPR.ReviewInfo = "@bob APPROVED"
	otherIssue := testSource("issue", "other/tasks", 100, "迁移数据", "closed")
	for _, tc := range []struct {
		name                   string
		items                  []WorkItem
		links                  map[int][]github.LinkedIssue
		wantTasks, wantSources int
		wantBackground         bool
	}{
		{"one issue one PR", []WorkItem{pr, issue}, map[int][]github.LinkedIssue{120: {testLinked(issue)}}, 1, 2, false},
		{"multiple PRs", []WorkItem{pr, issue, otherPR}, map[int][]github.LinkedIssue{120: {testLinked(issue)}, 121: {testLinked(issue)}}, 1, 3, false},
		{"shared PR distinct issues", []WorkItem{pr, issue, otherIssue}, map[int][]github.LinkedIssue{120: {testLinked(issue), testLinked(otherIssue)}}, 2, 4, false},
		{"filtered issue is context", []WorkItem{pr}, map[int][]github.LinkedIssue{120: {testLinked(issue)}}, 1, 2, true},
		{"missing links stay separate", []WorkItem{pr, issue}, nil, 2, 2, false},
		{"duplicate links", []WorkItem{issue, pr}, map[int][]github.LinkedIssue{120: {testLinked(issue), testLinked(issue)}}, 1, 2, false},
		{"same number different repo", []WorkItem{pr, otherIssue}, map[int][]github.LinkedIssue{120: {testLinked(issue)}}, 2, 3, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tasks := groupTaskItems(tc.items, []RepoReport{{Owner: "team", Repo: "code", PullRequestIssues: tc.links}})
			if len(tasks) != tc.wantTasks {
				t.Fatalf("tasks = %#v", tasks)
			}
			sources := 0
			for _, task := range tasks {
				sources += len(task.Sources)
			}
			if sources != tc.wantSources {
				t.Fatalf("sources = %d, want %d", sources, tc.wantSources)
			}
			text := formatWorkData(tasks, ReportWeekly)
			if strings.Contains(text, "](") || strings.Contains(text, "<https://") {
				t.Errorf("URL must not use Markdown wrapping: %s", text)
			}
			if strings.Count(text, "\n") != tc.wantTasks {
				t.Fatalf("not one line per task: %s", text)
			}
			for _, input := range tc.items {
				if !containsString(strings.Fields(text), input.URL) {
					t.Errorf("missing plain URL %s in %s", input.URL, text)
				}
			}
			if strings.Contains(text, "仅关联背景") != tc.wantBackground {
				t.Errorf("incorrect context marker: %s", text)
			}
			if !strings.Contains(text, "2026-11-09") {
				t.Errorf("lost source date: %s", text)
			}
			if tc.name == "multiple PRs" {
				for _, expected := range []string{"进行中", "已合并", "已提交(审查中)", "补充验证", "@bob APPROVED"} {
					if !strings.Contains(text, expected) {
						t.Errorf("lost fact %q: %s", expected, text)
					}
				}
			}
		})
	}
}

func testReports() []RepoReport {
	now := time.Now()
	issue := &gh.Issue{Number: gh.Ptr(100), Title: gh.Ptr("支持存储"), State: gh.Ptr("open"),
		HTMLURL: gh.Ptr("https://github.com/team/tasks/issues/100"), User: &gh.User{Login: gh.Ptr("alice")}, CreatedAt: &gh.Timestamp{Time: now}}
	pr := &gh.PullRequest{Number: gh.Ptr(120), Title: gh.Ptr("实现存储"), State: gh.Ptr("open"),
		HTMLURL: gh.Ptr("https://github.com/team/code/pull/120"), User: &gh.User{Login: gh.Ptr("alice")}, CreatedAt: &gh.Timestamp{Time: now}}
	project := github.Project{Iterations: []github.ProjectIteration{{Title: "本期", StartDate: now.AddDate(0, 0, -1).Format("2006-01-02"), Duration: 7}},
		Items: []github.ProjectItem{{Type: "ISSUE", Title: issue.GetTitle(), Number: 100, URL: issue.GetHTMLURL(), State: "OPEN", Status: "In Progress", Iteration: "本期", Assignees: []string{"alice"}}}}
	return []RepoReport{
		{Owner: "team", Repo: "code", PullRequests: []*gh.PullRequest{pr}, PullRequestIssues: map[int][]github.LinkedIssue{120: {{Repo: "team/tasks", Number: 100, Title: issue.GetTitle(), URL: issue.GetHTMLURL(), State: "OPEN"}}}},
		{Owner: "team", Repo: "tasks", Issues: []*gh.Issue{issue}, Projects: []github.Project{project}},
	}
}

func TestSummaryAndPromptShareGroupedTasks(t *testing.T) {
	reports := testReports()
	for _, rt := range []ReportType{ReportDaily, ReportWeekly, ReportMonthly, ReportYearly} {
		t.Run(string(rt), func(t *testing.T) {
			now := time.Now()
			prompt := BuildSummaryPrompt(reports, now.AddDate(0, 0, -1), now, "alice", rt)
			// 工作与计划各一条，且各自保留 Issue 和 PR 链接。
			if strings.Count(prompt, "- 支持存储 |") != 2 {
				t.Fatalf("duplicate tasks: %s", prompt)
			}
			for _, target := range []string{"https://github.com/team/tasks/issues/100", "https://github.com/team/code/pull/120"} {
				if strings.Count(prompt, target) != 2 {
					t.Errorf("lost links: %s", prompt)
				}
			}
			if strings.Contains(prompt, "](") || strings.Contains(prompt, "<https://") {
				t.Errorf("prompt data must contain plain URLs: %s", prompt)
			}
			plan := strings.Split(prompt, "=== "+labelsForType(rt).planTitle+"数据 ===\n")[1]
			if strings.Contains(plan, "进行中") || strings.Contains(plan, "Status:") || strings.Contains(plan, "In Progress") {
				t.Errorf("plan includes status: %s", plan)
			}
			var output bytes.Buffer
			PrintSummaryData(&output, reports, now.AddDate(0, 0, -1), now, "alice", rt)
			if !strings.HasSuffix(output.String(), prompt) {
				t.Error("manual and API prompts differ")
			}
		})
	}
}

func TestGroupingPreservesActivityFilters(t *testing.T) {
	for _, mode := range []string{"old issue", "other author", "other assignee", "old PR", "other PR author"} {
		t.Run(mode, func(t *testing.T) {
			reports := testReports()
			issue, pr := reports[1].Issues[0], reports[0].PullRequests[0]
			old := &gh.Timestamp{Time: time.Now().AddDate(-2, 0, 0)}
			switch mode {
			case "old issue":
				issue.State, issue.CreatedAt, issue.ClosedAt = gh.Ptr("closed"), old, old
			case "other author":
				issue.User.Login = gh.Ptr("bob")
			case "other assignee":
				issue.Assignees = []*gh.User{{Login: gh.Ptr("bob")}}
			case "old PR":
				pr.State, pr.CreatedAt, pr.ClosedAt = gh.Ptr("closed"), old, old
			case "other PR author":
				pr.User.Login = gh.Ptr("bob")
			}
			tasks := groupTaskItems(extractWorkItems(reports, "alice", ReportDaily), reports)
			if len(tasks) != 1 {
				t.Fatalf("tasks = %#v", tasks)
			}
			if mode == "old PR" || mode == "other PR author" {
				if len(tasks[0].Sources) != 1 || tasks[0].Sources[0].Type != "issue" {
					t.Fatal("excluded PR reintroduced")
				}
			} else if len(tasks[0].Sources) != 2 || !tasks[0].Sources[0].ContextOnly || tasks[0].Sources[0].Date != "" {
				t.Fatalf("excluded issue counted as activity: %#v", tasks)
			}
		})
	}
}

func TestUnlinkedSimilarTitlesAndCommentAnchors(t *testing.T) {
	issue := testSource("issue", "team/tasks", 100, "存储 [升级]\n下一行", "open")
	pr := testSource("pr", "team/tasks", 101, issue.Title, "closed")
	comment := testSource("comment", "team/tasks", 100, "参与讨论", "")
	comment.URL += "#issuecomment-123"
	tasks := groupTaskItems([]WorkItem{issue, pr, comment}, nil)
	text := formatWorkData(tasks, ReportDaily)
	if len(tasks) != 3 || strings.Count(text, "\n") != 3 || !strings.Contains(text, "存储 \\[升级\\] 下一行") || !containsString(strings.Fields(text), comment.URL) {
		t.Fatalf("incorrect formatting: %s", text)
	}
	if !strings.Contains(text, "已关闭") || strings.Contains(text, "已合并") {
		t.Fatalf("incorrect PR state: %s", text)
	}
}

func TestCSVUnaffectedByTaskLinks(t *testing.T) {
	reports := testReports()
	var before, after bytes.Buffer
	now := time.Now()
	Print(&before, reports, now, now)
	reports[0].PullRequestIssues = nil
	Print(&after, reports, now, now)
	if before.String() != after.String() {
		t.Fatal("task links changed raw CSV")
	}
}
