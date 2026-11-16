package contribution

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/miclle/gh-report/github"
)

type fixtureTransport struct {
	target string
	base   http.RoundTripper
}

func (tr fixtureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	copy.URL.Scheme = "http"
	copy.URL.Host = strings.TrimPrefix(tr.target, "http://")
	return tr.base.RoundTrip(copy)
}

func TestCollectFullHistoryAndFailure(t *testing.T) {
	for _, mode := range []string{"success", "local success", "local author failure", "empty repository", "issue page failure", "pr failure", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			var localPath string
			if strings.HasPrefix(mode, "local") {
				localPath = newLocalFixture(t)
				fixtureFile(t, localPath, "file", []byte("one\ntwo\n"))
				gitFixture(t, localPath, "add", ".")
				gitFixture(t, localPath, "commit", "-m", "root")
			}
			issuePages, prPages := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/graphql":
					var request struct {
						Query     string
						Variables map[string]any
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					if strings.Contains(request.Query, "defaultBranchRef") {
						if mode == "empty repository" {
							fmt.Fprint(w, `{"data":{"repository":{"defaultBranchRef":null}}}`)
						} else {
							fmt.Fprint(w, `{"data":{"repository":{"defaultBranchRef":{"name":"main","target":{"oid":"pinned"}}}}}`)
						}
						return
					}
					if strings.HasPrefix(mode, "local") {
						if strings.Contains(request.Query, "history(") {
							t.Error("local statistics must not download commit history/diffs")
						}
						if mode == "local author failure" {
							fmt.Fprint(w, `{"errors":[{"message":"author lookup unavailable"}]}`)
						} else {
							fmt.Fprint(w, `{"data":{"repository":{"c0":{"author":{"user":{"id":"U_1","login":"alice"}}}}}}`)
						}
						return
					}
					if request.Variables["sha"] != "pinned" {
						t.Errorf("head not pinned: %+v", request.Variables)
					}
					fmt.Fprint(w, `{"data":{"repository":{"object":{"history":{"pageInfo":{"hasNextPage":false},"nodes":[{"oid":"pinned","additions":12,"deletions":2,"author":{"user":{"id":"U_1","login":"alice"}},"tree":{"oid":"tree"},"parents":{"totalCount":0,"nodes":[]}}]}}}}}`)
				case "/repos/team/repo/issues":
					issuePages++
					if r.URL.Query().Get("state") != "all" || r.URL.Query().Get("since") != "" {
						t.Errorf("full-history query required: %s", r.URL)
					}
					if issuePages == 1 {
						w.Header().Set("Link", fmt.Sprintf(`<%s/repos/team/repo/issues?page=2>; rel="next"`, "http://"+r.Host))
						fmt.Fprint(w, `[{"id":1,"user":{"node_id":"U_2","login":"bob"},"state":"closed","state_reason":"completed","created_at":"2010-01-01T00:00:00Z"},{"id":2,"pull_request":{},"user":{"node_id":"U_1","login":"alice"}}]`)
					} else if mode == "issue page failure" {
						w.WriteHeader(http.StatusForbidden)
						fmt.Fprint(w, `{"message":"forbidden"}`)
					} else {
						fmt.Fprint(w, `[{"id":1,"user":{"node_id":"U_2","login":"bob"},"state":"closed","state_reason":"completed"},{"id":3,"user":{"node_id":"U_1","login":"alice"},"state":"open"}]`)
					}
				case "/repos/team/repo/pulls":
					prPages++
					if r.URL.Query().Get("state") != "all" {
						t.Errorf("all PRs required: %s", r.URL)
					}
					if mode == "pr failure" {
						w.WriteHeader(http.StatusForbidden)
						fmt.Fprint(w, `{"message":"forbidden"}`)
						return
					}
					if prPages == 1 {
						w.Header().Set("Link", fmt.Sprintf(`<%s/repos/team/repo/pulls?page=2>; rel="next"`, "http://"+r.Host))
						fmt.Fprint(w, `[{"id":10,"user":{"node_id":"U_1","login":"alice"},"state":"closed","merged_at":"2010-01-01T00:00:00Z","updated_at":"2010-01-01T00:00:00Z"}]`)
					} else {
						fmt.Fprint(w, `[{"id":11,"user":{"node_id":"U_1","login":"alice"},"state":"closed","updated_at":"2010-01-01T00:00:00Z"}]`)
					}
				default:
					t.Errorf("unexpected API (must not fetch projects/comments/reviews): %s", r.URL)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			// NewClient 在构造时捕获默认 Transport；随即恢复，避免测试期间修改全局网络行为。
			// 本包测试不并行执行客户端构造。
			original := http.DefaultTransport
			http.DefaultTransport = fixtureTransport{target: server.URL, base: original}
			client := github.NewClient("test-token")
			http.DefaultTransport = original
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancelled" {
				cancel()
			}
			opts := Options{Repo: "team/repo", LocalPath: localPath}
			if localPath != "" {
				opts.Repo = ""
			}
			report, err := Collect(ctx, client, opts)
			if mode != "success" && mode != "empty repository" && mode != "local success" {
				if err == nil || report != nil {
					t.Fatalf("must fail without partial report: %+v, %v", report, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if issuePages != 2 || prPages != 2 || len(report.Users) != 2 || report.Totals.IssuesCreated != 2 || report.Totals.PRsCreated != 2 || report.Totals.PRsMerged != 1 || report.Totals.IssuesAuthoredCompleted != 1 {
				t.Fatalf("wrong full-history counters/pages: %+v, issues=%d prs=%d", report, issuePages, prPages)
			}
			if mode == "success" && (report.HeadSHA != "pinned" || report.Totals.Commits != 1 || report.Totals.Changes != 14) {
				t.Fatalf("wrong commit history: %+v", report)
			}
			if mode == "local success" && (report.CommitSource != "local_git" || report.SourceRef != "refs/heads/main" || report.Totals.Commits != 1 || report.Totals.Changes != 2 || report.Users[0].Login != "alice" || report.Users[0].PRsCreated != 2) {
				t.Fatalf("local author must join Issue/PR identity without downloading diff: %+v", report)
			}
			if mode == "empty repository" && (report.HeadSHA != "" || report.Totals.Commits != 0) {
				t.Fatalf("empty repository must retain Issue/PR data without commits: %+v", report)
			}
		})
	}
}
