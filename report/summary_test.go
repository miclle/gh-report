package report

import (
	"strings"
	"testing"
	"time"

	gh "github.com/google/go-github/v69/github"
)

func TestClosedDraftPRExcludedFromPlans(t *testing.T) {
	for _, rt := range []ReportType{ReportDaily, ReportWeekly, ReportMonthly, ReportYearly} {
		for _, recentlyClosed := range []bool{false, true} {
			name := string(rt) + "/old"
			if recentlyClosed {
				name = string(rt) + "/recent"
			}
			t.Run(name, func(t *testing.T) {
				now := time.Now()
				reports := testReports()
				pr := reports[0].PullRequests[0]
				old := &gh.Timestamp{Time: now.AddDate(-2, 0, 0)}
				pr.State, pr.Draft = gh.Ptr("closed"), gh.Ptr(true)
				pr.CreatedAt, pr.ClosedAt = old, old
				if recentlyClosed {
					pr.ClosedAt = &gh.Timestamp{Time: now}
				}
				// 保留一个打开的草稿，确保修复不会排除仍需推进的 PR。
				openPR := &gh.PullRequest{
					Number: gh.Ptr(121), Title: gh.Ptr("继续实现"), State: gh.Ptr("open"), Draft: gh.Ptr(true),
					HTMLURL: gh.Ptr("https://github.com/team/code/pull/121"), User: pr.User, CreatedAt: old,
				}
				reports[0].PullRequests = append(reports[0].PullRequests, openPR)

				prompt := BuildSummaryPrompt(reports, old.Time, now, "alice", rt)
				_, plan, ok := strings.Cut(prompt, "=== "+labelsForType(rt).planTitle+"数据 ===\n")
				if !ok {
					t.Fatalf("missing plan section: %s", prompt)
				}
				if strings.Contains(plan, pr.GetHTMLURL()) {
					t.Errorf("closed draft PR reintroduced into plan: %s", plan)
				}
				for _, url := range []string{openPR.GetHTMLURL(), reports[1].Issues[0].GetHTMLURL()} {
					if !strings.Contains(plan, url) {
						t.Errorf("plan lost unfinished work %s: %s", url, plan)
					}
				}
				work := formatWorkData(groupTaskItems(extractWorkItems(reports, "alice", rt), reports), rt)
				if recentlyClosed {
					want := pr.GetHTMLURL() + " "
					if rt != ReportDaily {
						want += prActivityDate(pr) + " "
					}
					if !strings.Contains(work, want+"已关闭") {
						t.Errorf("recently closed draft PR must be shown as closed: %s", work)
					}
				} else if strings.Contains(work, pr.GetHTMLURL()) {
					t.Errorf("old closed draft PR counted as current work: %s", work)
				}
			})
		}
	}
}
