package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const anthropicDefaultBaseURL = "https://api.anthropic.com"

// anthropicClient 是 Anthropic Messages API 客户端。
type anthropicClient struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client
}

// newAnthropicClient 创建一个新的 Anthropic API 客户端。
// baseURL 为空时使用默认值 https://api.anthropic.com。
func newAnthropicClient(apiKey, model, baseURL string) *anthropicClient {
	if baseURL == "" {
		baseURL = anthropicDefaultBaseURL
	}
	return &anthropicClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		http:    &http.Client{},
	}
}

// anthropicMessage 表示 Messages API 的请求体。
type anthropicMessage struct {
	Model     string               `json:"model"`
	MaxTokens int                  `json:"max_tokens"`
	Messages  []anthropicMsgItem   `json:"messages"`
}

// anthropicMsgItem 表示对话中的单条消息。
type anthropicMsgItem struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicResponse 表示 Messages API 的响应体。
type anthropicResponse struct {
	Content []anthropicContentBlock `json:"content"`
	Error   *anthropicAPIError      `json:"error,omitempty"`
}

// anthropicContentBlock 表示响应中的内容块。
type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// anthropicAPIError 表示 API 返回的错误。
type anthropicAPIError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// CreateMessage 向 Claude 发送 prompt 并返回生成的文本。
func (c *anthropicClient) CreateMessage(ctx context.Context, prompt string) (string, error) {
	body := anthropicMessage{
		Model:     c.model,
		MaxTokens: 4096,
		Messages: []anthropicMsgItem{
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

	var result anthropicResponse
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
