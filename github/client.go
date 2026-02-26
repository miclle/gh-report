// Package github 提供 GitHub REST 和 GraphQL API 客户端。
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	gh "github.com/google/go-github/v69/github"
)

// graphqlURL 是 GitHub GraphQL API 的端点地址。
const graphqlURL = "https://api.github.com/graphql"

// Client 封装 go-github 客户端，并提供额外的 GraphQL 支持。
type Client struct {
	// REST 是 go-github 提供的 REST API 客户端。
	REST *gh.Client
	// httpClient 复用 go-github 的 HTTP 客户端（携带认证信息），用于 GraphQL 请求。
	httpClient *http.Client
	token      string
}

// NewClient 创建一个新的 GitHub API 客户端。
func NewClient(token string) *Client {
	c := &Client{token: token}
	c.REST = gh.NewClient(nil).WithAuthToken(token)
	c.httpClient = c.REST.Client()
	return c
}

// graphqlRequest 表示 GraphQL 请求体。
type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// graphqlResponse 表示 GraphQL 响应体。
type graphqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// GraphQL 执行 GitHub GraphQL API 查询，将结果解码到 result 中。
func (c *Client) GraphQL(ctx context.Context, query string, variables map[string]any, result any) error {
	body, err := json.Marshal(graphqlRequest{Query: query, Variables: variables})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", graphqlURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub GraphQL error: %d %s — %s", resp.StatusCode, resp.Status, string(respBody))
	}

	var gqlResp graphqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return err
	}

	if len(gqlResp.Errors) > 0 {
		msgs := make([]string, len(gqlResp.Errors))
		for i, e := range gqlResp.Errors {
			msgs[i] = e.Message
		}
		return fmt.Errorf("GraphQL errors: %s", strings.Join(msgs, "; "))
	}

	return json.Unmarshal(gqlResp.Data, result)
}
