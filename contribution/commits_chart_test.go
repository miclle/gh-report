package contribution

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"strings"
	"testing"
	"time"
)

func TestWeeklyActivityUsesUTCAuthorsAndSameIdentity(t *testing.T) {
	path := newLocalFixture(t)
	t.Setenv("GIT_COMMITTER_DATE", "2030-01-01T00:00:00Z")
	for i, commit := range []struct{ date, author string }{
		{"2026-01-04T23:30:00-02:00", "Alice <ALICE@example.com>"},
		{"2026-01-11T23:30:00+02:00", "Alias <alice@example.com>"},
		{"2026-01-12T00:10:00Z", "Bob <bob@example.com>"},
	} {
		fixtureFile(t, path, "file", []byte(strings.Repeat("line\n", i+1)))
		gitFixture(t, path, "add", ".")
		gitFixture(t, path, "commit", "--date", commit.date, "--author", commit.author, "-m", "activity")
	}
	gitFixture(t, path, "commit", "--allow-empty", "--date", "2025-01-01T00:00:00Z", "-m", "empty")
	report, err := CollectCommits(context.Background(), CommitOptions{Path: path, Ref: "HEAD", Weekly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Users) != 2 || report.Totals.Commits != 3 {
		t.Fatalf("wrong totals: %+v", report)
	}
	for _, user := range report.Users {
		if len(user.Weeks) != 1 || user.Weeks[0].CommitCounts != user.CommitCounts {
			t.Fatalf("weekly counters differ from totals: %+v", user)
		}
		want := "2026-01-05"
		if user.Email == "bob@example.com" {
			want = "2026-01-12"
		}
		if user.Weeks[0].Week != want {
			t.Fatalf("author time or UTC boundary incorrect: %+v", user)
		}
	}
	var svg bytes.Buffer
	if err := PrintCommitsChart(&svg, report); err != nil {
		t.Fatal(err)
	}
	assertValidChartXML(t, svg.Bytes(), 2)
	if strings.Contains(svg.String(), "2030-") || strings.Contains(svg.String(), "2025-") {
		t.Fatal("chart included committer time or empty commit")
	}
	plain, err := CollectCommits(context.Background(), CommitOptions{Path: path, Ref: "HEAD"})
	if err != nil || plain.Totals != report.Totals || len(plain.Users[0].Weeks) != 0 {
		t.Fatalf("normal collection changed: %v %+v", err, plain)
	}
}

func TestCommitChartEscapingAndEmptyReport(t *testing.T) {
	report := &CommitReport{Users: []CommitUser{{
		Name: "Alice <script>alert(1)</script> & \x01 Bob", Email: "alice@example.com",
		CommitCounts: CommitCounts{Commits: 1, Additions: 2},
		Weeks:        []CommitWeek{{Week: "2026-01-05", CommitCounts: CommitCounts{Commits: 1, Additions: 2}}},
	}}}
	var out bytes.Buffer
	if err := PrintCommitsChart(&out, report); err != nil {
		t.Fatal(err)
	}
	assertValidChartXML(t, out.Bytes(), 1)
	if strings.Contains(out.String(), "<script>") || !strings.Contains(out.String(), "&lt;script&gt;") {
		t.Fatal("SVG did not escape author text")
	}
	if err := PrintCommitsChart(failedCommitWriter{}, report); err == nil {
		t.Fatal("chart swallowed writer error")
	}
	out.Reset()
	if err := PrintCommitsChart(&out, &CommitReport{}); err != nil {
		t.Fatal(err)
	}
	assertValidChartXML(t, out.Bytes(), 0)
	if !strings.Contains(out.String(), "没有符合统计口径的提交") {
		t.Fatal("empty state missing")
	}
	report.Users[0].Weeks = nil
	if err := PrintCommitsChart(io.Discard, report); err == nil {
		t.Fatal("missing activity must not become a zero chart")
	}
}

func TestActivityGapsPreserveZeroWeeks(t *testing.T) {
	first := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	last := first.Add(8 * chartWeek)
	points := chartPoints([]activityPoint{{date: last, commits: 3}, {date: first, commits: 2}}, first, last)
	if len(points) != 4 || points[0].commits != 2 || points[1].commits != 0 || points[2].commits != 0 || points[3].commits != 3 {
		t.Fatalf("gaps must return to zero, not interpolate activity across idle weeks: %+v", points)
	}
	if !points[1].date.Equal(first.Add(chartWeek)) || !points[2].date.Equal(last.Add(-chartWeek)) {
		t.Fatal("zero weeks misplaced")
	}
}

func assertValidChartXML(t *testing.T, svg []byte, cards int) {
	t.Helper()
	decoder := xml.NewDecoder(bytes.NewReader(svg))
	groups := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("invalid SVG: %v", err)
		}
		if start, ok := token.(xml.StartElement); ok {
			if start.Name.Local == "g" {
				groups++
			}
			if start.Name.Local == "script" || start.Name.Local == "foreignObject" {
				t.Fatalf("active SVG element: %s", start.Name.Local)
			}
		}
	}
	if groups != cards {
		t.Fatalf("got %d cards, want %d", groups, cards)
	}
}
