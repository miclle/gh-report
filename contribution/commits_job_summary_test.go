package contribution

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func TestJobSummaryGroupsWeeksOnSharedScale(t *testing.T) {
	first := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	week := func(index int, count int64) CommitWeek {
		return CommitWeek{Week: first.AddDate(0, 0, index*7).Format(time.DateOnly), CommitCounts: CommitCounts{Commits: count}}
	}
	report := &CommitReport{Totals: CommitCounts{Commits: 10}, Users: []CommitUser{
		{Name: "Alice", Email: "alice@example.com", CommitCounts: CommitCounts{Commits: 6, Additions: 12, Deletions: 2}, Weeks: []CommitWeek{week(0, 3), week(1, 2), week(100, 1)}},
		{Name: "Bob", Email: "bob@example.com", CommitCounts: CommitCounts{Commits: 4}, Weeks: []CommitWeek{week(100, 4)}},
	}}
	var out bytes.Buffer
	if err := PrintCommitsJobSummary(&out, report, nil, false); err != nil {
		t.Fatal(err)
	}
	document := out.String()
	for _, want := range []string{
		"每列 2 周提交数合计 · 共享纵轴 0–5",
		`height="72" alt="" title="2024-01-01 — 2024-01-14: 5 commits"`,
		`height="0" alt="" title="2024-01-15 — 2024-01-28: 0 commits"`,
		`height="15" alt="" title="2025-12-01 — 2025-12-07: 1 commits"`,
		`height="58" alt="" title="2025-12-01 — 2025-12-07: 4 commits"`,
		"6 commits</strong> · +12 / −2 lines", "<strong>[A]</strong>",
		"<details open>\n<summary>贡献图表</summary>", "<details>\n<summary>数据表格</summary>",
		"| :--- | :--- | ---: | ---: | ---: | ---: |", "| TOTAL | | 10 |",
	} {
		if !strings.Contains(document, want) {
			t.Fatalf("missing %q:\n%s", want, document)
		}
	}
	if strings.Count(document, `width="5"`) != 102 || strings.Count(document, `width="1" height="72"`) != 2 {
		t.Fatal("cards must share 51 columns and fixed plot height")
	}
	for _, forbidden := range []string{"<svg", "data:image", ".svg", "api.github.com"} {
		if strings.Contains(document, forbidden) {
			t.Fatalf("Summary must not depend on %s", forbidden)
		}
	}
}

func TestJobSummaryEscapesAuthorsAndUsesAvailableAvatars(t *testing.T) {
	user := CommitUser{Name: `<script>"&`, Email: "42+alice@users.noreply.github.com", CommitCounts: CommitCounts{Commits: 1},
		Weeks: []CommitWeek{{Week: "2026-01-05", CommitCounts: CommitCounts{Commits: 1}}}}
	report := &CommitReport{Users: []CommitUser{user}, Rules: []string{"<rule>"}}
	for _, loaded := range []bool{false, true} {
		if loaded {
			report.Users[0].avatarPNG = []byte("loaded")
		}
		var out bytes.Buffer
		if err := PrintCommitsJobSummary(&out, report, nil, true); err != nil {
			t.Fatal(err)
		}
		document := out.String()
		if strings.Contains(document, "<script>") || !strings.Contains(document, "&lt;script&gt;") || !strings.Contains(document, "&lt;rule&gt;") {
			t.Fatalf("unsafe author or rules: %s", document)
		}
		if strings.Contains(document, `src="https://avatars.githubusercontent.com/u/42?s=64"`) != loaded {
			t.Fatalf("avatar must use only known available identity: %s", document)
		}
		if strings.Count(document, `width="5"`) != 3 {
			t.Fatal("single active week needs zero weeks on each side")
		}
	}
	// 普通邮箱必须经过显式映射；映射不应泄漏到其他作者。
	report.Users[0].Email = "alice@example.com"
	var out bytes.Buffer
	if err := PrintCommitsJobSummary(&out, report, map[string]string{"alice@example.com": "alice"}, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `https://avatars.githubusercontent.com/alice?s=64`) {
		t.Fatal("explicit avatar mapping missing")
	}
}

func TestJobSummaryEmptyInvalidAndWriterFailure(t *testing.T) {
	var out bytes.Buffer
	if err := PrintCommitsJobSummary(&out, &CommitReport{}, nil, false); err != nil || !strings.Contains(out.String(), "没有符合统计口径的提交") {
		t.Fatalf("empty report: %v %s", err, out.String())
	}
	if err := PrintCommitsJobSummary(failedCommitWriter{}, &CommitReport{}, nil, false); err == nil {
		t.Fatal("writer error swallowed")
	}
	for _, weeks := range [][]CommitWeek{nil, {{Week: "invalid"}}} {
		out.Reset()
		if err := PrintCommitsJobSummary(&out, &CommitReport{Users: []CommitUser{{Weeks: weeks}}}, nil, false); err == nil || out.Len() != 0 {
			t.Fatal("invalid activity must fail before writing")
		}
	}
	if err := PrintCommitsJobSummary(io.Discard, &CommitReport{}, nil, true); err != nil {
		t.Fatal(err)
	}
}
