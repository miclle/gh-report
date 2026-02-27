// Package cmd 提供 CLI 命令定义和执行逻辑。
package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/miclle/gh-report/anthropic"
	"github.com/miclle/gh-report/github"
	"github.com/miclle/gh-report/report"
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

var rootCmd = &cobra.Command{
	Use:   "gh-report",
	Short: "GitHub 仓库活动报告生成工具",
	Long: `GitHub 仓库活动报告生成工具。

通过 GitHub API 获取指定仓库的 Issue、Pull Request、评论、Review 以及
Projects v2 迭代信息，生成结构化的工作报告。支持 CSV 原始数据输出和
Summary 日报模式，并可通过 Claude API 直接生成工作日报。`,
	Example: `  # 使用配置文件生成报告
  gh-report -c config.yaml

  # 指定仓库，查看最近 7 天
  gh-report -r owner/repo1,owner/repo2 -d 7 -u mylogin

  # 生成摘要（可粘贴给 AI）
  gh-report -c config.yaml -f summary

  # 调用 Claude API 直接生成日报
  gh-report -c config.yaml -f summary --ai`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runReport,
}

func init() {
	f := rootCmd.Flags()
	f.StringP("config", "c", "", "YAML 配置文件路径")
	f.StringP("repos", "r", "", "仓库列表，逗号分隔（owner/repo 格式）")
	f.IntP("days", "d", 0, "查看最近几天的活动")
	f.StringP("user", "u", "", "按用户过滤")
	f.String("token", "", "GitHub Token（默认: $GITHUB_TOKEN）")
	f.StringP("format", "f", "", "输出格式: csv（默认）或 summary")
	f.Bool("ai", false, "调用 Claude API 生成日报")
	f.String("anthropic-key", "", "Anthropic API Key（默认: $ANTHROPIC_API_KEY）")
	f.String("anthropic-base-url", "", "Anthropic API Base URL（默认: $ANTHROPIC_BASE_URL）")
	f.String("model", "", "Claude 模型名（默认: claude-sonnet-4-20250514）")
}

// Execute 执行根命令。
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		errColor := color.New(color.FgRed, color.Bold)
		errColor.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

// runReport 是根命令的主逻辑，负责加载配置、获取数据、生成报告。
func runReport(cmd *cobra.Command, args []string) error {
	// 加载配置文件
	var cfg Config
	configFile, _ := cmd.Flags().GetString("config")
	if configFile != "" {
		c, err := LoadConfig(configFile)
		if err != nil {
			return err
		}
		cfg = *c
	}

	// CLI flags 覆盖配置文件值（仅覆盖显式指定的字段）
	if cmd.Flags().Changed("repos") {
		reposStr, _ := cmd.Flags().GetString("repos")
		cfg.Repos = nil
		for _, r := range strings.Split(reposStr, ",") {
			cfg.Repos = append(cfg.Repos, strings.TrimSpace(r))
		}
	}
	if cmd.Flags().Changed("days") {
		cfg.Days, _ = cmd.Flags().GetInt("days")
	}
	if cmd.Flags().Changed("user") {
		cfg.User, _ = cmd.Flags().GetString("user")
	}
	if cmd.Flags().Changed("token") {
		cfg.Token, _ = cmd.Flags().GetString("token")
	}
	if cmd.Flags().Changed("format") {
		cfg.Format, _ = cmd.Flags().GetString("format")
	}
	if cmd.Flags().Changed("ai") {
		cfg.AI, _ = cmd.Flags().GetBool("ai")
	}
	if cmd.Flags().Changed("anthropic-key") {
		cfg.AnthropicKey, _ = cmd.Flags().GetString("anthropic-key")
	}
	if cmd.Flags().Changed("anthropic-base-url") {
		cfg.AnthropicBaseURL, _ = cmd.Flags().GetString("anthropic-base-url")
	}
	if cmd.Flags().Changed("model") {
		cfg.Model, _ = cmd.Flags().GetString("model")
	}

	// 应用默认值
	if cfg.Days == 0 {
		cfg.Days = 1
	}

	if len(cfg.Repos) == 0 {
		return fmt.Errorf("未指定仓库（使用 -r 参数或配置文件指定）")
	}

	// 解析 Token: flag/config > 环境变量
	ghToken := cfg.Token
	if ghToken == "" {
		ghToken = os.Getenv("GITHUB_TOKEN")
	}
	if ghToken == "" {
		return fmt.Errorf("未提供 GitHub Token（使用 --token 参数、配置文件或 GITHUB_TOKEN 环境变量）")
	}

	client := github.NewClient(ghToken)
	ctx := context.Background()

	opts := report.Options{
		Repos: cfg.Repos,
		Days:  cfg.Days,
		User:  cfg.User,
	}

	// 获取 GitHub 数据（带 spinner）
	s := spinner.New(spinner.CharSets[14], 80*time.Millisecond)
	s.Suffix = "  正在获取 GitHub 数据..."
	s.Writer = os.Stderr
	s.Start()
	reports, err := report.Collect(ctx, client, opts)
	s.Stop()
	if err != nil {
		return fmt.Errorf("获取数据失败: %w", err)
	}

	now := time.Now()
	since := now.AddDate(0, 0, -cfg.Days)

	switch cfg.Format {
	case "summary":
		if cfg.AI {
			// 解析 Anthropic API Key: flag > config > 环境变量
			apiKey := cfg.AnthropicKey
			if apiKey == "" {
				apiKey = os.Getenv("ANTHROPIC_API_KEY")
			}
			if apiKey == "" {
				return fmt.Errorf("未提供 Anthropic API Key（使用 --anthropic-key 参数、配置文件或 ANTHROPIC_API_KEY 环境变量）")
			}

			modelName := cfg.Model
			if modelName == "" {
				modelName = "claude-sonnet-4-20250514"
			}

			// 解析 Anthropic Base URL: flag > config > 环境变量
			baseURL := cfg.AnthropicBaseURL
			if baseURL == "" {
				baseURL = os.Getenv("ANTHROPIC_BASE_URL")
			}

			prompt := report.BuildSummaryPrompt(reports, since, now, cfg.User)
			aiClient := anthropic.NewClient(apiKey, modelName, baseURL)

			// 调用 Claude API（带 spinner）
			s2 := spinner.New(spinner.CharSets[14], 80*time.Millisecond)
			s2.Suffix = "  正在调用 Claude API 生成日报..."
			s2.Writer = os.Stderr
			s2.Start()
			result, err := aiClient.CreateMessage(ctx, prompt)
			s2.Stop()
			if err != nil {
				return fmt.Errorf("调用 Claude API 失败: %w", err)
			}
			fmt.Fprintln(os.Stdout, result)
		} else {
			report.PrintSummaryData(os.Stdout, reports, since, now, cfg.User)
		}
	default:
		report.Print(os.Stdout, reports, since, now)
	}

	return nil
}
