// Package cmd 提供 CLI 命令定义和执行逻辑。
package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/miclle/gh-report/ai"
	"github.com/miclle/gh-report/github"
	"github.com/miclle/gh-report/report"
	"github.com/miclle/gh-report/ui"
)

// ReportType 表示报告类型。
type ReportType string

const (
	// ReportDaily 日报，默认拉取最近 1 天数据。
	ReportDaily ReportType = "daily"
	// ReportWeekly 周报，默认拉取最近 14 天数据。
	ReportWeekly ReportType = "weekly"
	// ReportMonthly 月报，默认拉取最近 60 天数据。
	ReportMonthly ReportType = "monthly"
	// ReportYearly 年报，默认拉取最近 730 天数据。
	ReportYearly ReportType = "yearly"
)

// defaultDays 返回指定报告类型的默认数据拉取天数。
func defaultDays(rt ReportType) int {
	switch rt {
	case ReportWeekly:
		return 14
	case ReportMonthly:
		return 60
	case ReportYearly:
		return 730
	default:
		return 1
	}
}

// Config 表示 YAML 配置文件的结构。
type Config struct {
	Token string   `yaml:"token"` // GitHub Token（可选，也可通过 GITHUB_TOKEN 环境变量设置）
	Repos []string `yaml:"repos"` // 仓库列表，格式为 "owner/repo"
	Days  int      `yaml:"days"`  // 查看最近几天的活动
	User  string   `yaml:"user"`  // 按用户过滤（可选）

	Format string `yaml:"format"` // 输出格式：csv（默认）或 summary
	AI     bool   `yaml:"ai"`     // 是否调用 AI API 直接生成日报
	Model  string `yaml:"model"`  // 模型名称

	// 新字段
	AIProvider string `yaml:"ai_provider"` // AI 服务提供商: anthropic（默认）或 openai
	AIKey      string `yaml:"ai_key"`      // AI API Key
	AIBaseURL  string `yaml:"ai_base_url"` // AI API Base URL

	// 已废弃字段（向后兼容）
	AnthropicKey     string `yaml:"anthropic_key"`      // 已废弃，请使用 ai_key
	AnthropicBaseURL string `yaml:"anthropic_base_url"` // 已废弃，请使用 ai_base_url
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
Projects v2 迭代信息，生成结构化的工作报告。支持日报、周报、月报、年报，
提供 CSV 原始数据输出和 Summary 模式，并可通过 AI API（支持 Anthropic
Claude 和 OpenAI）直接生成报告。

不指定子命令时默认生成日报。`,
	Example: `  # 使用配置文件生成日报（默认）
  gh-report -c config.yaml

  # 生成周报（默认拉取最近 14 天数据）
  gh-report weekly -c config.yaml -f summary --ai

  # 生成月报（默认拉取最近 60 天数据）
  gh-report monthly -c config.yaml -f summary --ai

  # 生成年报（默认拉取最近 730 天数据）
  gh-report yearly -c config.yaml -f summary --ai

  # 指定仓库，自定义天数
  gh-report weekly -r owner/repo1,owner/repo2 -d 21 -u mylogin

  # 生成摘要（可粘贴给 AI）
  gh-report -c config.yaml -f summary

  # 使用 OpenAI 生成日报
  gh-report -c config.yaml -f summary --ai --ai-provider openai`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runReportWithType(cmd, ReportDaily)
	},
}

func init() {
	f := rootCmd.PersistentFlags()
	f.StringP("config", "c", "", "YAML 配置文件路径")
	f.StringP("repos", "r", "", "仓库列表，逗号分隔（owner/repo 格式）")
	f.IntP("days", "d", 0, "查看最近几天的活动")
	f.StringP("user", "u", "", "按用户过滤")
	f.String("token", "", "GitHub Token（默认: $GITHUB_TOKEN）")
	f.StringP("format", "f", "", "输出格式: csv（默认）或 summary")
	f.Bool("ai", false, "调用 AI API 生成报告")
	f.String("ai-provider", "", "AI 服务提供商: anthropic（默认）或 openai")
	f.String("ai-key", "", "AI API Key（默认: 按 provider 查环境变量）")
	f.String("ai-base-url", "", "AI API Base URL")
	f.String("model", "", "AI 模型名")

	// 已废弃 flags（向后兼容）
	f.String("anthropic-key", "", "Anthropic API Key（已废弃，请使用 --ai-key）")
	f.String("anthropic-base-url", "", "Anthropic API Base URL（已废弃，请使用 --ai-base-url）")
	_ = f.MarkDeprecated("anthropic-key", "请使用 --ai-key")
	_ = f.MarkDeprecated("anthropic-base-url", "请使用 --ai-base-url")
}

// Execute 执行根命令。
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		errColor := color.New(color.FgRed, color.Bold)
		errColor.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

// runReportWithType 是报告生成的通用逻辑，接受报告类型参数。
func runReportWithType(cmd *cobra.Command, reportType ReportType) error {
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
		parts := strings.Split(reposStr, ",")
		cfg.Repos = make([]string, 0, len(parts))
		for _, r := range parts {
			r = strings.TrimSpace(r)
			if r != "" {
				cfg.Repos = append(cfg.Repos, r)
			}
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
	if cmd.Flags().Changed("ai-provider") {
		cfg.AIProvider, _ = cmd.Flags().GetString("ai-provider")
	}
	if cmd.Flags().Changed("ai-key") {
		cfg.AIKey, _ = cmd.Flags().GetString("ai-key")
	}
	if cmd.Flags().Changed("ai-base-url") {
		cfg.AIBaseURL, _ = cmd.Flags().GetString("ai-base-url")
	}
	// 已废弃 flags 向后兼容
	if cmd.Flags().Changed("anthropic-key") {
		cfg.AnthropicKey, _ = cmd.Flags().GetString("anthropic-key")
	}
	if cmd.Flags().Changed("anthropic-base-url") {
		cfg.AnthropicBaseURL, _ = cmd.Flags().GetString("anthropic-base-url")
	}
	if cmd.Flags().Changed("model") {
		cfg.Model, _ = cmd.Flags().GetString("model")
	}

	return runReportWithConfig(reportType, &cfg)
}

// runReportWithConfig 使用配置运行报告生成。
func runReportWithConfig(reportType ReportType, cfg *Config) error {
	// 应用默认值：按报告类型设置默认天数
	if cfg.Days == 0 {
		cfg.Days = defaultDays(reportType)
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

	// 使用新的 UI 进度组件获取 GitHub 数据
	progress := ui.NewProgress(opts.Repos)
	wrapper := progress.Start()
	reports, err := report.Collect(ctx, client, opts, wrapper)
	if err != nil {
		progress.SetError(err)
		progress.Stop()
		return fmt.Errorf("获取数据失败: %w", err)
	}
	progress.Complete()
	progress.Stop()

	now := time.Now()
	since := now.AddDate(0, 0, -cfg.Days)

	switch cfg.Format {
	case "summary":
		if cfg.AI {
			// 解析 AI Provider
			provider := ai.ProviderName(cfg.AIProvider)
			if provider == "" {
				provider = ai.ProviderAnthropic
			}

			// 解析 API Key: --ai-key > ai_key > --anthropic-key(兼容) > anthropic_key(兼容) > 按 provider 查环境变量 > $AI_API_KEY
			apiKey := cfg.AIKey
			if apiKey == "" {
				apiKey = cfg.AnthropicKey // 兼容旧配置
			}
			if apiKey == "" {
				switch provider {
				case ai.ProviderAnthropic:
					apiKey = os.Getenv("ANTHROPIC_API_KEY")
				case ai.ProviderOpenAI:
					apiKey = os.Getenv("OPENAI_API_KEY")
				}
			}
			if apiKey == "" {
				apiKey = os.Getenv("AI_API_KEY")
			}
			if apiKey == "" {
				return fmt.Errorf("未提供 AI API Key（使用 --ai-key 参数、配置文件或环境变量）")
			}

			// 解析 Base URL: --ai-base-url > ai_base_url > --anthropic-base-url(兼容) > anthropic_base_url(兼容) > 按 provider 查环境变量
			baseURL := cfg.AIBaseURL
			if baseURL == "" {
				baseURL = cfg.AnthropicBaseURL // 兼容旧配置
			}
			if baseURL == "" {
				switch provider {
				case ai.ProviderAnthropic:
					baseURL = os.Getenv("ANTHROPIC_BASE_URL")
				case ai.ProviderOpenAI:
					baseURL = os.Getenv("OPENAI_BASE_URL")
				}
			}

			prompt := report.BuildSummaryPrompt(reports, since, now, cfg.User, report.ReportType(reportType))
			aiClient, err := ai.NewClient(ai.Config{
				Provider: provider,
				APIKey:   apiKey,
				Model:    cfg.Model,
				BaseURL:  baseURL,
			})
			if err != nil {
				return err
			}

			// 使用新的 UI Spinner 调用 AI API
			reportName := reportTypeLabel(reportType)
			spinnerText := fmt.Sprintf("正在调用 %s API 生成%s...", provider, reportName)
			result, err := ui.RunSpinnerWithResult(spinnerText, func() (string, error) {
				return aiClient.CreateMessage(ctx, prompt)
			})
			if err != nil {
				return fmt.Errorf("调用 %s API 失败: %w", provider, err)
			}
			fmt.Fprintln(os.Stdout, result)
		} else {
			report.PrintSummaryData(os.Stdout, reports, since, now, cfg.User, report.ReportType(reportType))
		}
	default:
		report.Print(os.Stdout, reports, since, now)
	}

	return nil
}

// reportTypeLabel 返回报告类型的中文显示名称。
func reportTypeLabel(rt ReportType) string {
	switch rt {
	case ReportWeekly:
		return "周报"
	case ReportMonthly:
		return "月报"
	case ReportYearly:
		return "年报"
	default:
		return "日报"
	}
}
