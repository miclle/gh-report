package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// redirectTransport 将真实客户端的 GraphQL 请求送入本地测试服务器。
type redirectTransport struct{ target string }

func (tr redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = strings.TrimPrefix(tr.target, "http://")
	return http.DefaultTransport.RoundTrip(clone)
}

func TestListClosingIssuesPaginationAndFailure(t *testing.T) {
	for _, mode := range []string{"success", "second page error", "inaccessible", "invalid cursor"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var req graphqlRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if req.Variables["owner"] != "team" || req.Variables["repo"] != "code" || req.Variables["number"] != float64(12) {
					t.Errorf("unexpected variables: %#v", req.Variables)
				}
				if !strings.Contains(req.Query, "excludeUserLinked: false") {
					t.Error("manual links must be included")
				}
				w.Header().Set("Content-Type", "application/json")
				if mode == "inaccessible" {
					fmt.Fprint(w, `{"data":{"repository":{"pullRequest":null}}}`)
					return
				}
				if calls == 1 {
					cursor := "page2"
					if mode == "invalid cursor" {
						cursor = ""
					}
					fmt.Fprintf(w, `{"data":{"repository":{"pullRequest":{"closingIssuesReferences":{"pageInfo":{"hasNextPage":true,"endCursor":%q},"nodes":[{"number":100,"title":"任务一","url":"https://github.com/team/tasks/issues/100","state":"OPEN","repository":{"nameWithOwner":"team/tasks"}}]}}}}}`, cursor)
					return
				}
				if calls > 2 {
					t.Error("unexpected extra page")
				}
				if req.Variables["cursor"] != "page2" {
					t.Errorf("cursor = %v", req.Variables["cursor"])
				}
				if mode == "second page error" {
					fmt.Fprint(w, `{"errors":[{"message":"unavailable"}]}`)
					return
				}
				fmt.Fprint(w, `{"data":{"repository":{"pullRequest":{"closingIssuesReferences":{"pageInfo":{"hasNextPage":false},"nodes":[{"number":101,"title":"任务二","url":"https://github.com/other/tasks/issues/101","state":"CLOSED","repository":{"nameWithOwner":"other/tasks"}}]}}}}}`)
			}))
			defer server.Close()
			client := NewClient("test-token")
			client.httpClient = &http.Client{Transport: redirectTransport{server.URL}}
			got, err := client.ListClosingIssues(context.Background(), "team", "code", 12)
			if mode != "success" {
				if err == nil || got != nil {
					t.Fatalf("got %v, %v; want error without partial links", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 2 || got[0].Repo != "team/tasks" || got[1].Repo != "other/tasks" || got[1].State != "CLOSED" {
				t.Fatalf("incorrect linked issues: %#v", got)
			}
		})
	}
}
