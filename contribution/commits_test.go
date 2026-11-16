package contribution

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type offlineTransport struct{ t *testing.T }

func (o offlineTransport) RoundTrip(*http.Request) (*http.Response, error) {
	o.t.Error("offline collection attempted an HTTP request")
	return nil, fmt.Errorf("network disabled")
}

func TestCollectCommitsOfflineWithoutRemoteOrToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	previous := http.DefaultTransport
	http.DefaultTransport = offlineTransport{t}
	t.Cleanup(func() { http.DefaultTransport = previous })
	path := newLocalFixture(t)
	gitFixture(t, path, "remote", "remove", "origin")
	fixtureFile(t, path, "file", []byte("one\ntwo\n"))
	gitFixture(t, path, "add", ".")
	gitFixture(t, path, "commit", "--author=Alice <ALICE@example.com>", "-m", "root")
	fixtureFile(t, path, "file", []byte("two\nthree\n"))
	gitFixture(t, path, "add", ".")
	gitFixture(t, path, "commit", "--author=Alias <alice@example.com>", "-m", "edit")
	gitFixture(t, path, "commit", "--allow-empty", "-m", "empty")
	fixtureFile(t, path, "dirty", []byte("uncommitted\n"))
	report, err := CollectCommits(context.Background(), CommitOptions{Path: path, Ref: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Totals != (CommitCounts{Commits: 2, Additions: 3, Deletions: 1, Changes: 4}) || len(report.Users) != 1 || report.Users[0].Email != "alice@example.com" {
		t.Fatalf("wrong offline aggregation: %+v", report)
	}
	for _, format := range []string{"json", "csv", "table", "markdown"} {
		var out bytes.Buffer
		if err := PrintCommits(&out, report, format, false); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "issues_created") || strings.Contains(out.String(), "prs_merged") || strings.Contains(out.String(), "IssuesDone") {
			t.Fatalf("%s includes uncollected metrics: %s", format, out.String())
		}
		switch format {
		case "json":
			var decoded CommitReport
			if err := json.Unmarshal(out.Bytes(), &decoded); err != nil || decoded.Totals != report.Totals {
				t.Fatalf("invalid JSON: %v", err)
			}
		case "csv":
			rows, err := csv.NewReader(&out).ReadAll()
			if err != nil || len(rows) != 3 || rows[2][0] != "total" || rows[1][11] != "2" {
				t.Fatalf("invalid CSV: %v %v", rows, err)
			}
		}
	}
	if _, err := CollectCommits(context.Background(), CommitOptions{Path: path}); err == nil || !strings.Contains(err.Error(), "--ref") {
		t.Fatalf("missing default must require explicit scope: %v", err)
	}
	if _, err := CollectCommits(context.Background(), CommitOptions{Path: path, Ref: "--all"}); err == nil {
		t.Fatal("ref option injection must fail")
	}
}

func TestCollectCommitsCachedDefaultBranch(t *testing.T) {
	path := newLocalFixture(t)
	fixtureFile(t, path, "file", []byte("main\n"))
	gitFixture(t, path, "add", ".")
	gitFixture(t, path, "commit", "-m", "main")
	sha := gitFixture(t, path, "rev-parse", "HEAD")
	gitFixture(t, path, "update-ref", "refs/remotes/origin/main", sha)
	gitFixture(t, path, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	// 本地 main 的未推送提交也应计入。
	fixtureFile(t, path, "local", []byte("local\n"))
	gitFixture(t, path, "add", ".")
	gitFixture(t, path, "commit", "-m", "local main")
	localSHA := gitFixture(t, path, "rev-parse", "HEAD")
	gitFixture(t, path, "checkout", "-b", "feature")
	fixtureFile(t, path, "feature", []byte("feature\n"))
	gitFixture(t, path, "add", ".")
	gitFixture(t, path, "commit", "-m", "feature")
	for _, tracking := range []bool{false, true} {
		wantSHA, wantRef, wantCount := localSHA, "refs/heads/main", int64(2)
		if tracking {
			gitFixture(t, path, "branch", "-D", "main")
			wantSHA, wantRef, wantCount = sha, "refs/remotes/origin/main", 1
		}
		report, err := CollectCommits(context.Background(), CommitOptions{Path: path})
		if err != nil {
			t.Fatal(err)
		}
		if report.HeadSHA != wantSHA || report.SourceRef != wantRef || report.Totals.Commits != wantCount {
			t.Fatalf("wrong default selection: %+v", report)
		}
		if gitFixture(t, path, "branch", "--show-current") != "feature" {
			t.Fatal("collection switched branch")
		}
	}
	clone := t.TempDir() + "/shallow"
	gitFixture(t, path, "clone", "--depth=1", "file://"+path, clone)
	if _, err := CollectCommits(context.Background(), CommitOptions{Path: clone, Ref: "HEAD"}); err == nil || !strings.Contains(err.Error(), "浅克隆") {
		t.Fatalf("shallow history must fail: %v", err)
	}
}

func TestPrintCommitsFailuresAndZeroUsers(t *testing.T) {
	report := &CommitReport{Users: []CommitUser{}}
	for _, format := range []string{"table", "markdown", "csv", "json"} {
		if err := PrintCommits(failedCommitWriter{}, report, format, false); err == nil {
			t.Errorf("%s swallowed write error", format)
		}
	}
	var out bytes.Buffer
	if err := PrintCommits(&out, report, "csv", false); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&out).ReadAll()
	if err != nil || len(rows) != 2 || rows[1][0] != "total" {
		t.Fatalf("zero-user CSV lost metadata: %v %v", rows, err)
	}
}

type failedCommitWriter struct{}

func (failedCommitWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestCommitsMarkdownPreservesTableWithSpecialAuthorText(t *testing.T) {
	report := &CommitReport{
		Users:  []CommitUser{{Email: "author@example.com", Name: "A|B\r\n<script>*name*", CommitCounts: CommitCounts{Commits: 2}}},
		Totals: CommitCounts{Commits: 2},
		Rules:  []string{"示例口径"},
	}
	var out bytes.Buffer
	if err := PrintCommits(&out, report, "markdown", false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "<script>") || !strings.Contains(out.String(), "A&#124;B<br>&lt;script&gt;\\*name\\*") {
		t.Fatalf("author text was not escaped: %s", out.String())
	}
	rows := 0
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(line, "|") {
			rows++
			if strings.Count(line, "|") != 7 {
				t.Fatalf("broken table row: %s", line)
			}
		}
	}
	if rows != 4 || strings.Contains(out.String(), "示例口径") {
		t.Fatalf("unexpected table or visible rules: %s", out.String())
	}
	out.Reset()
	if err := PrintCommits(&out, report, "markdown", true); err != nil || !strings.Contains(out.String(), "- 示例口径") {
		t.Fatalf("explicit rules missing: %v %s", err, out.String())
	}
}
