package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 表示 YAML 配置文件的结构。
type Config struct {
	Token string   `yaml:"token"` // GitHub Token（可选，也可通过 GITHUB_TOKEN 环境变量设置）
	Repos []string `yaml:"repos"` // 仓库列表，格式为 "owner/repo"
	Days  int      `yaml:"days"`  // 查看最近几天的活动
	User  string   `yaml:"user"`  // 按用户过滤（可选）

	Format           string `yaml:"format"`             // 输出格式：csv（默认）或 summary
	AI               bool   `yaml:"ai"`                 // 是否调用 Claude API 直接生成日报
	AnthropicKey     string `yaml:"anthropic_key"`      // Anthropic API Key
	AnthropicBaseURL string `yaml:"anthropic_base_url"` // Anthropic API Base URL（可选）
	Model            string `yaml:"model"`              // Claude 模型名（默认 claude-sonnet-4-20250514）
}

// LoadConfig 读取并解析 YAML 配置文件。
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return &cfg, nil
}
