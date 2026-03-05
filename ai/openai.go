package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const openaiDefaultBaseURL = "https://api.openai.com"

// openaiClient 是 OpenAI Chat Completions API 客户端。
type openaiClient struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client
}

// newOpenAIClient 创建一个新的 OpenAI API 客户端。
// baseURL 为空时使用默认值 https://api.openai.com。
func newOpenAIClient(apiKey, model, baseURL string) *openaiClient {
	if baseURL == "" {
		baseURL = openaiDefaultBaseURL
	}
	return &openaiClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		http:    &http.Client{},
	}
}

// openaiRequest 表示 Chat Completions API 的请求体。
type openaiRequest struct {
	Model    string          `json:"model"`
	Messages []openaiMessage `json:"messages"`
}

// openaiMessage 表示对话中的单条消息。
type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openaiResponse 表示 Chat Completions API 的响应体。
type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
	Error   *openaiError   `json:"error,omitempty"`
}

// openaiChoice 表示响应中的一个选择。
type openaiChoice struct {
	Message openaiMessage `json:"message"`
}

// openaiError 表示 API 返回的错误。
type openaiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// CreateMessage 向 OpenAI 发送 prompt 并返回生成的文本。
func (c *openaiClient) CreateMessage(ctx context.Context, prompt string) (string, error) {
	body := openaiRequest{
		Model: c.model,
		Messages: []openaiMessage{
			{Role: "user", Content: prompt},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	// 兼容 base URL 末尾带 /v1 的情况（OpenAI SDK 惯例）
	base := strings.TrimRight(c.baseURL, "/")
	if strings.HasSuffix(base, "/v1") {
		base = base + "/chat/completions"
	} else {
		base = base + "/v1/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

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

	var result openaiResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("API error (%s): %s", result.Error.Type, result.Error.Message)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("API returned empty choices")
	}

	return result.Choices[0].Message.Content, nil
}
