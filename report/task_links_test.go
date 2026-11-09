package report

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/miclle/gh-report/github"
)

type stubIssueLinks struct {
	mu    sync.Mutex
	calls []string
}

func (s *stubIssueLinks) ListClosingIssues(ctx context.Context, owner, repo string, number int) ([]github.LinkedIssue, error) {
	s.mu.Lock()
	s.calls = append(s.calls, fmt.Sprintf("%s/%s#%d", owner, repo, number))
	s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	issue := github.LinkedIssue{Repo: "team/tasks", Number: 100, Title: "支持存储", URL: "https://github.com/team/tasks/issues/100", State: "OPEN"}
	if number == 130 {
		return []github.LinkedIssue{issue}, errors.New("permission denied")
	}
	return []github.LinkedIssue{issue}, nil
}

func TestCollectTaskLinksScopeAndFailureFallback(t *testing.T) {
	reports := testReports()
	project := reports[1].Projects[0]
	for _, tc := range []struct {
		number                           int
		repo, state, assignee, iteration string
	}{
		{130, "team/tasks", "OPEN", "alice", "本期"},
		{131, "team/tasks", "MERGED", "alice", "本期"},
		{132, "team/tasks", "OPEN", "bob", "本期"},
		{133, "team/tasks", "OPEN", "alice", "下期"},
		{134, "team/tasks-extra", "OPEN", "alice", "本期"},
	} {
		project.Items = append(project.Items, github.ProjectItem{
			Type: "PULL_REQUEST", Number: tc.number, Title: "后续实现",
			URL:   fmt.Sprintf("https://github.com/%s/pull/%d", tc.repo, tc.number),
			State: tc.state, Assignees: []string{tc.assignee}, Iteration: tc.iteration,
		})
	}
	reports[1].Projects = []github.Project{project}
	stub := &stubIssueLinks{}
	// 同步测试中捕获警告，确保查询失败可见且不污染报告正文。
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	os.Stderr = writer
	t.Cleanup(func() { os.Stderr = oldStderr; reader.Close(); writer.Close() })
	if err := collectTaskLinks(context.Background(), stub, reports, "alice"); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	os.Stderr = oldStderr
	warning, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(warning), "team/tasks#130") || !strings.Contains(string(warning), "保留独立条目") {
		t.Fatalf("missing diagnostic: %s", warning)
	}
	sort.Strings(stub.calls)
	if got := strings.Join(stub.calls, ","); got != "team/code#120,team/tasks#130" {
		t.Fatalf("queried out-of-scope PRs: %s", got)
	}
	if len(reports[0].PullRequestIssues[120]) != 1 {
		t.Fatal("successful links lost")
	}
	if _, exists := reports[1].PullRequestIssues[130]; exists {
		t.Fatal("partial failed links retained")
	}
	plans := formatPlanData(groupPlanItems(extractPlanItems(reports, "alice"), reports))
	if strings.Count(plans, "\n") != 2 || !containsString(strings.Fields(plans), "https://github.com/team/tasks/pull/130") {
		t.Fatalf("failed PR was hidden: %s", plans)
	}
}

func TestCollectTaskLinksCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := collectTaskLinks(ctx, &stubIssueLinks{}, testReports(), "alice"); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want cancellation", err)
	}
}
