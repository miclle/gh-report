// Package ai 提供统一的 AI 客户端接口，支持多个 AI 服务提供商。
package ai

import (
	"context"
	"fmt"
)

// ProviderName 表示 AI 服务提供商名称。
type ProviderName string

const (
	// ProviderAnthropic 表示 Anthropic Claude 服务。
	ProviderAnthropic ProviderName = "anthropic"
	// ProviderOpenAI 表示 OpenAI 服务。
	ProviderOpenAI ProviderName = "openai"
)

// Client 是 AI 服务的统一接口。
type Client interface {
	// CreateMessage 向 AI 发送 prompt 并返回生成的文本。
	CreateMessage(ctx context.Context, prompt string) (string, error)
}

// Config 表示 AI 客户端的配置。
type Config struct {
	Provider ProviderName // AI 服务提供商（默认: anthropic）
	APIKey   string       // API 密钥
	Model    string       // 模型名称（为空时使用 DefaultModel）
	BaseURL  string       // API Base URL（为空时使用各 provider 默认值）
}

// DefaultModel 返回指定 provider 的默认模型名称。
func DefaultModel(provider ProviderName) string {
	switch provider {
	case ProviderOpenAI:
		return "gpt-4o"
	default:
		return "claude-sonnet-4-20250514"
	}
}

// NewClient 根据配置创建对应 provider 的 AI 客户端。
// 空 provider 默认使用 anthropic。
func NewClient(cfg Config) (Client, error) {
	if cfg.Provider == "" {
		cfg.Provider = ProviderAnthropic
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel(cfg.Provider)
	}

	switch cfg.Provider {
	case ProviderAnthropic:
		return newAnthropicClient(cfg.APIKey, cfg.Model, cfg.BaseURL), nil
	case ProviderOpenAI:
		return newOpenAIClient(cfg.APIKey, cfg.Model, cfg.BaseURL), nil
	default:
		return nil, fmt.Errorf("不支持的 AI 服务提供商: %s", cfg.Provider)
	}
}
