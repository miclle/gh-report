package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLookupCommitAuthorsBatchingAndFailure(t *testing.T) {
	for _, mode := range []string{"success", "second batch error", "missing alias"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var request graphqlRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if mode == "second batch error" && calls == 2 {
					fmt.Fprint(w, `{"errors":[{"message":"unavailable"}]}`)
					return
				}
				fields := make(map[string]any)
				for i := 0; i < len(request.Variables)-2; i++ {
					alias := fmt.Sprintf("c%d", i)
					if i%3 == 0 {
						fields[alias] = nil
					} else if i%3 == 1 {
						fields[alias] = map[string]any{"author": map[string]any{"user": nil}}
					} else {
						fields[alias] = map[string]any{"author": map[string]any{"user": map[string]string{"id": "U_1", "login": "alice"}}}
					}
				}
				if mode == "missing alias" {
					delete(fields, "c0")
				}
				if err := json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"repository": fields}}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			client := NewClient("test-token")
			client.httpClient = &http.Client{Transport: redirectTransport{server.URL}}
			shas := make([]string, 51)
			for i := range shas {
				shas[i] = fmt.Sprintf("sha%d", i)
			}
			got, err := client.LookupCommitAuthors(context.Background(), "team", "repo", shas)
			if mode != "success" {
				if err == nil || got != nil {
					t.Fatalf("must fail without partial identities: %+v, %v", got, err)
				}
				return
			}
			if err != nil || calls != 2 || len(got) != 16 || got["sha2"].User.Login != "alice" {
				t.Fatalf("batch result: calls=%d identities=%d err=%v", calls, len(got), err)
			}
		})
	}
}
