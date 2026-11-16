package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWalkCommitsPaginationAndClassification(t *testing.T) {
	for _, mode := range []string{"success", "second page error", "missing commit", "repeated cursor", "missing lines", "missing parent"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var req graphqlRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if req.Variables["sha"] != "fixed-head" {
					t.Errorf("history must stay pinned: %v", req.Variables)
				}
				if calls > 1 && req.Variables["cursor"] != "page2" {
					t.Errorf("unexpected cursor: %v", req.Variables)
				}
				w.Header().Set("Content-Type", "application/json")
				if mode == "missing commit" {
					fmt.Fprint(w, `{"data":{"repository":{"object":null}}}`)
					return
				}
				if mode == "second page error" && calls == 2 {
					fmt.Fprint(w, `{"errors":[{"message":"cannot calculate diff"}]}`)
					return
				}
				nodes := []map[string]any{commitFixture("root", "tree1", "", 0, 10, 0), commitFixture("binary", "tree2", "tree1", 1, 0, 0)}
				if calls > 1 {
					nodes = []map[string]any{commitFixture("binary", "tree2", "tree1", 1, 0, 0), commitFixture("merge", "tree3", "tree2", 2, 5, 0), commitFixture("empty", "tree3", "tree3", 1, 0, 0)}
				}
				if mode == "missing lines" {
					delete(nodes[0], "additions")
				}
				if mode == "missing parent" {
					nodes[0]["parents"] = map[string]any{"totalCount": 1, "nodes": []any{}}
				}
				history := map[string]any{"nodes": nodes, "pageInfo": map[string]any{"hasNextPage": calls == 1 || mode == "repeated cursor", "endCursor": "page2"}}
				if err := json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"repository": map[string]any{"object": map[string]any{"history": history}}}}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			client := NewClient("test-token")
			client.httpClient = &http.Client{Transport: redirectTransport{server.URL}}
			var commits []CommitActivity
			err := client.WalkCommits(context.Background(), "team", "code", "fixed-head", func(commit CommitActivity) error {
				commits = append(commits, commit)
				return nil
			})
			if mode != "success" {
				if err == nil {
					t.Fatal("expected incomplete history to fail")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if calls != 2 || len(commits) != 4 {
				t.Fatalf("calls=%d commits=%d, want 2 pages and 4 unique commits", calls, len(commits))
			}
			if commits[1].Empty || commits[1].Merge || !commits[2].Merge || !commits[3].Empty {
				t.Fatalf("binary/merge/empty classification: %+v", commits)
			}
		})
	}
}

func commitFixture(sha, tree, parent string, parentCount int, additions, deletions int64) map[string]any {
	parents := []any{}
	if parent != "" {
		parents = append(parents, map[string]any{"tree": map[string]string{"oid": parent}})
	}
	return map[string]any{
		"oid": sha, "additions": additions, "deletions": deletions,
		"author": map[string]any{"name": "Alice", "email": "alice@example.com", "user": map[string]string{"id": "U_1", "login": "alice"}},
		"tree":   map[string]string{"oid": tree}, "parents": map[string]any{"totalCount": parentCount, "nodes": parents},
	}
}

func TestDefaultBranchHeadEmptyAndInaccessible(t *testing.T) {
	for _, test := range []struct {
		body string
		want BranchHead
		fail bool
	}{
		{`{"data":{"repository":{"defaultBranchRef":{"name":"main","target":{"oid":"abc"}}}}}`, BranchHead{Name: "main", SHA: "abc"}, false},
		{`{"data":{"repository":{"defaultBranchRef":null}}}`, BranchHead{}, false},
		{`{"data":{"repository":null}}`, BranchHead{}, true},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, test.body) }))
		client := NewClient("test-token")
		client.httpClient = &http.Client{Transport: redirectTransport{server.URL}}
		got, err := client.GetDefaultBranchHead(context.Background(), "team", "code")
		server.Close()
		if (err != nil) != test.fail || got != test.want {
			t.Errorf("GetDefaultBranchHead = %+v, %v; want %+v, fail=%t", got, err, test.want, test.fail)
		}
	}
}
