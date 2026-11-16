package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/miclle/gh-report/contribution"
)

func newCommitsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "commits",
		Short: "离线统计本地 Git 仓库的提交数和增删行数，无需 Token",
		Long: `按 Git 作者邮箱统计本地提交数、新增、删除及变更行数，提交统计完全离线，不读取配置文件。
默认从本地 origin/HEAD 确定默认分支，优先本地分支，再使用 origin 的本地跟踪分支。
无法确定默认分支时须传 --ref；支持本地分支、标签或 SHA，也可显式使用 HEAD。
排除 merge 和空提交，不切换分支或 fetch，不支持浅克隆。只需本机 Git，不要求 GitHub remote。
不应用 mailmap、不过滤 Bot；Issue / PR 统计请使用 contributors。
图表模式默认联网获取 GitHub 头像并嵌入 SVG，无需 Token；--github-avatars=false 可关闭头像获取。
头像从 noreply 邮箱识别账号，或使用 --avatar-user email=username 显式映射（可重复）。
所有模式均不调用 GitHub API、不读取 Token；普通报表或关闭头像获取时完全离线。
头像不可用时保留首字母，不改变按邮箱归并的计数。`,
		Example: `  gh-report commits
  gh-report commits --ref main --sort changes
  gh-report commits --path /path/to/repo --ref HEAD -f json
  gh-report commits --ref main -f csv > commits.csv
  gh-report commits --ref main -f markdown > commits.md
  gh-report commits --ref main -f markdown --chart -o report.md`,
		Args: cobra.NoArgs,
		RunE: runCommits,
	}
	cmd.Flags().String("path", ".", "本地 Git 工作区路径（也可为子目录）")
	cmd.Flags().String("ref", "", "本地分支、标签或 SHA；默认根据本地 origin/HEAD 选择默认分支")
	cmd.Flags().StringP("format", "f", "table", "提交统计输出格式: table、markdown、csv、json")
	cmd.Flags().String("sort", "commits", "排序: commits、changes、user（数字降序，用户升序）")
	cmd.Flags().Bool("show-rules", false, "在表格或 Markdown 末尾显示统计口径（CSV / JSON 始终保留）")
	cmd.Flags().Bool("chart", false, "在 Markdown 折叠区块中同时展示贡献者图表和表格（需 -f markdown 和 --output）")
	cmd.Flags().Bool("github-avatars", true, "图表模式默认联网获取并嵌入 GitHub 头像；设为 false 可完全离线（无需 Token）")
	cmd.Flags().StringArray("avatar-user", nil, "作者邮箱到 GitHub 用户名的映射: email=username（可重复，需 --chart）")
	cmd.Flags().StringP("output", "o", "", "保存报告到文件；图表模式同时生成配套 SVG")
	cmd.Flags().String("summary-output", "", "另存 GitHub Job Summary 使用的 Markdown 数据表格，复用同一次采集（需 --chart）")
	return cmd
}

func init() {
	rootCmd.AddCommand(newCommitsCmd())
}

func commitOptions(cmd *cobra.Command) (contribution.CommitOptions, string, error) {
	avatars, _ := cmd.Flags().GetBool("github-avatars")
	chart, _ := cmd.Flags().GetBool("chart")
	entries, _ := cmd.Flags().GetStringArray("avatar-user")
	if len(entries) > 0 && !(chart && avatars) {
		return contribution.CommitOptions{}, "", fmt.Errorf("--avatar-user 需要 --chart，且不能关闭 --github-avatars")
	}
	if _, err := contribution.ParseAvatarUsers(entries); err != nil {
		return contribution.CommitOptions{}, "", err
	}
	for _, name := range []string{"config", "token", "repos", "days", "user", "ai", "ai-provider", "ai-key", "ai-base-url", "model", "anthropic-key", "anthropic-base-url"} {
		if cmd.Flags().Changed(name) {
			return contribution.CommitOptions{}, "", fmt.Errorf("commits 不支持 --%s；只读取本地 Git，请使用 --path 和 --ref 指定范围", name)
		}
	}
	path, _ := cmd.Flags().GetString("path")
	ref, _ := cmd.Flags().GetString("ref")
	sortBy, _ := cmd.Flags().GetString("sort")
	opts := contribution.CommitOptions{Path: path, Ref: ref, Sort: sortBy}
	if err := opts.Validate(); err != nil {
		return opts, "", err
	}
	format, _ := cmd.Flags().GetString("format")
	if cmd.Flags().Changed("github-avatars") && avatars && !chart {
		return opts, "", fmt.Errorf("--github-avatars 需要 --chart")
	}
	output, _ := cmd.Flags().GetString("output")
	if chart && (format != "markdown" || output == "") {
		return opts, "", fmt.Errorf("--chart 需要 -f markdown 和 --output 指定 Markdown 文件路径")
	}
	summaryOutput, _ := cmd.Flags().GetString("summary-output")
	if summaryOutput != "" {
		if !chart {
			return opts, "", fmt.Errorf("--summary-output 需要 --chart")
		}
		outputPath, err := filepath.Abs(output)
		if err != nil {
			return opts, "", err
		}
		summaryPath, err := filepath.Abs(summaryOutput)
		if err != nil {
			return opts, "", err
		}
		if outputPath == summaryPath {
			return opts, "", fmt.Errorf("--summary-output 不能与 --output 使用相同路径")
		}
	}
	opts.Weekly = chart
	switch format {
	case "table", "markdown", "csv", "json":
		return opts, format, nil
	default:
		return opts, "", fmt.Errorf("commits 不支持输出格式 %q（可用 table、markdown、csv、json）", format)
	}
}

func runCommits(cmd *cobra.Command, _ []string) error {
	opts, format, err := commitOptions(cmd)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	report, err := contribution.CollectCommits(ctx, opts)
	if err != nil {
		return fmt.Errorf("提交统计失败，未输出部分报告: %w", err)
	}
	avatars, _ := cmd.Flags().GetBool("github-avatars")
	entries, _ := cmd.Flags().GetStringArray("avatar-user")
	users, err := contribution.ParseAvatarUsers(entries)
	if err != nil {
		return err
	}
	if opts.Weekly && avatars {
		loaded, avatarErr := contribution.LoadGitHubAvatars(ctx, report, users)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if avatarErr != nil {
			if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "GitHub 头像：%v；已获取 %d/%d，其余自动使用首字母头像，继续生成报告。\n", avatarErr, loaded, len(report.Users)); err != nil {
				return err
			}
		} else if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "GitHub 头像：已获取 %d/%d；未获取的使用首字母头像。\n", loaded, len(report.Users)); err != nil {
			return err
		}
	}
	showRules, _ := cmd.Flags().GetBool("show-rules")
	output, _ := cmd.Flags().GetString("output")
	if output != "" {
		if err := saveCommitReport(report, format, output, showRules, opts.Weekly); err != nil {
			return err
		}
		summaryOutput, _ := cmd.Flags().GetString("summary-output")
		if summaryOutput != "" {
			return saveCommitReport(report, "markdown", summaryOutput, showRules, false)
		}
		return nil
	}
	return contribution.PrintCommits(cmd.OutOrStdout(), report, format, showRules)
}
