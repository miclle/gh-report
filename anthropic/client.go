// Package anthropic 提供 Anthropic Messages API 的轻量级客户端。
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const defaultBaseURL = "https://api.anthropic.com"

// Client 是 Anthropic Messages API 客户端。
type Client struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client
}

// NewClient 创建一个新的 Anthropic API 客户端。
// baseURL 为空时使用默认值 https://api.anthropic.com。
func NewClient(apiKey, model, baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		http:    &http.Client{},
	}
}

// message 表示 Messages API 的请求体。
type message struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	Messages  []msgItem `json:"messages"`
}

// msgItem 表示对话中的单条消息。
type msgItem struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// response 表示 Messages API 的响应体。
type response struct {
	Content []contentBlock `json:"content"`
	Error   *apiError      `json:"error,omitempty"`
}

// contentBlock 表示响应中的内容块。
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// apiError 表示 API 返回的错误。
type apiError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// CreateMessage 向 Claude 发送 prompt 并返回生成的文本。
func (c *Client) CreateMessage(ctx context.Context, prompt string) (string, error) {
	body := message{
		Model:     c.model,
		MaxTokens: 4096,
		Messages: []msgItem{
			{Role: "user", Content: prompt},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	url := c.baseURL + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result response
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("API error (%s): %s", result.Error.Type, result.Error.Message)
	}

	// 拼接所有 text 类型的 content block
	var text string
	for _, block := range result.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	return text, nil
}
