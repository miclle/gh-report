package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"

	"github.com/miclle/gh-report/contribution"
	"github.com/miclle/gh-report/github"
)

func newContributorsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "contributors",
		Short: "统计单仓库默认分支全历史的贡献（支持本地 Git）",
		Long: `统计所有参与者的提交数、新增/删除/变更行数、创建 Issue 数、其中已完成数，以及创建/已合并 PR 数。

Issue 完成数按 Issue 作者归属，仅计当前 closed 且 state_reason=completed，不表示解决者贡献。
提交仅计默认分支主作者，排除 merge 和空提交。所有数据采集成功后才输出。

--local 在当前 Git 工作区读取默认分支的本地提交历史，自动识别 GitHub remote。
优先本地默认分支，缺失时使用对应的本地 remote-tracking ref；不切换分支或自动 fetch。
Issue/PR 及作者身份关联仍联网获取，需要 Token。浅克隆不支持全历史统计。

配置文件只复用 token 和 repos（必须恰好一个）；忽略工作报告的 days、user、format 和 AI 配置。
--local 忽略配置文件 repos；显式 -r 必须匹配本地 GitHub remote。
不支持显式传入 --days、--user 或 AI 参数。`,
		Example: `  gh-report contributors -r owner/repo
  gh-report contributors -r owner/repo -f csv > contributors.csv
  gh-report contributors -r owner/repo -f json --sort changes --exclude-bots
  gh-report contributors --local --sort changes`,
		Args: cobra.NoArgs,
		RunE: runContributors,
	}
	// 覆盖工作报告的 format 默认值，避免贡献统计继承 csv/summary 语义。
	cmd.Flags().StringP("format", "f", "table", "贡献统计输出格式: table、csv、json")
	cmd.Flags().String("sort", "commits", "排序: commits、changes、issues、issues-completed、prs、prs-merged、user（数字降序，用户升序）")
	cmd.Flags().Bool("exclude-bots", false, "排除账号类型为 Bot 或用户名以 [bot] 结尾的参与者")
	cmd.Flags().Bool("local", false, "从当前 Git 工作区的本地默认分支读取 commits，Issue/PR 仍联网获取")
	return cmd
}

func init() {
	rootCmd.AddCommand(newContributorsCmd())
}

func contributorOptions(cmd *cobra.Command) (contribution.Options, string, string, error) {
	for _, name := range []string{"days", "user", "ai", "ai-provider", "ai-key", "ai-base-url", "model", "anthropic-key", "anthropic-base-url"} {
		if cmd.Flags().Changed(name) {
			return contribution.Options{}, "", "", fmt.Errorf("contributors 不支持 --%s；统计范围固定为全部用户、默认分支全历史", name)
		}
	}
	var cfg Config
	configFile, _ := cmd.Flags().GetString("config")
	if configFile != "" {
		loaded, err := LoadConfig(configFile)
		if err != nil {
			return contribution.Options{}, "", "", err
		}
		cfg = *loaded
	}
	if cmd.Flags().Changed("repos") {
		repos, _ := cmd.Flags().GetString("repos")
		cfg.Repos = strings.Split(repos, ",")
	}
	local, _ := cmd.Flags().GetBool("local")
	if local && !cmd.Flags().Changed("repos") {
		cfg.Repos = nil
	}
	if (!local || cmd.Flags().Changed("repos")) && len(cfg.Repos) != 1 {
		return contribution.Options{}, "", "", fmt.Errorf("contributors 需要恰好一个仓库（使用 -r owner/repo 或配置文件 repos）")
	}
	sortBy, _ := cmd.Flags().GetString("sort")
	excludeBots, _ := cmd.Flags().GetBool("exclude-bots")
	opts := contribution.Options{Sort: sortBy, ExcludeBots: excludeBots}
	if len(cfg.Repos) == 1 {
		opts.Repo = strings.TrimSpace(cfg.Repos[0])
	}
	if cmd.Flags().Changed("repos") && opts.Repo == "" {
		return contribution.Options{}, "", "", fmt.Errorf("请指定一个 owner/repo 格式的仓库")
	}
	if local {
		opts.LocalPath = "."
	}
	if err := opts.Validate(); err != nil {
		return contribution.Options{}, "", "", err
	}
	format, _ := cmd.Flags().GetString("format")
	switch format {
	case "table", "csv", "json":
	default:
		return contribution.Options{}, "", "", fmt.Errorf("contributors 不支持输出格式 %q（可用 table、csv、json）", format)
	}
	token := cfg.Token
	if cmd.Flags().Changed("token") {
		token, _ = cmd.Flags().GetString("token")
	}
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		return contribution.Options{}, "", "", fmt.Errorf("未提供 GitHub Token（使用 --token 参数、配置文件或 GITHUB_TOKEN 环境变量）")
	}
	return opts, format, token, nil
}

func runContributors(cmd *cobra.Command, _ []string) error {
	opts, format, token, err := contributorOptions(cmd)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	target := opts.Repo
	if opts.LocalPath != "" {
		target = "当前本地 Git 仓库"
	}
	if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "正在采集 %s 默认分支全历史贡献（大仓库可能耗时较长）…\n", target); err != nil {
		return err
	}
	report, err := contribution.Collect(ctx, github.NewClient(token), opts)
	if err != nil {
		return fmt.Errorf("贡献统计失败，未输出部分报告: %w", err)
	}
	return contribution.Print(cmd.OutOrStdout(), report, format)
}
