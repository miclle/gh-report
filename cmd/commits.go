package cmd

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/miclle/gh-report/contribution"
)

func newCommitsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "commits",
		Short: "离线统计本地 Git 仓库的提交数和增删行数，无需 Token",
		Long: `按 Git 作者邮箱统计本地提交数、新增、删除及变更行数，完全离线，不读取配置文件或 Token。
默认从本地 origin/HEAD 确定默认分支，优先本地分支，再使用 origin 的本地跟踪分支。
无法确定默认分支时须传 --ref；支持本地分支、标签或 SHA，也可显式使用 HEAD。
排除 merge 和空提交，不切换分支或 fetch，不支持浅克隆。只需本机 Git，不要求 GitHub remote。
不查询 GitHub 身份、不应用 mailmap、不过滤 Bot；Issue / PR 统计请使用 contributors。`,
		Example: `  gh-report commits
  gh-report commits --ref main --sort changes
  gh-report commits --path /path/to/repo --ref HEAD -f json
  gh-report commits --ref main -f csv > commits.csv
  gh-report commits --ref main -f markdown > commits.md`,
		Args: cobra.NoArgs,
		RunE: runCommits,
	}
	cmd.Flags().String("path", ".", "本地 Git 工作区路径（也可为子目录）")
	cmd.Flags().String("ref", "", "本地分支、标签或 SHA；默认根据本地 origin/HEAD 选择默认分支")
	cmd.Flags().StringP("format", "f", "table", "提交统计输出格式: table、markdown、csv、json")
	cmd.Flags().String("sort", "commits", "排序: commits、changes、user（数字降序，用户升序）")
	cmd.Flags().Bool("show-rules", false, "在表格或 Markdown 末尾显示统计口径（CSV / JSON 始终保留）")
	return cmd
}

func init() {
	rootCmd.AddCommand(newCommitsCmd())
}

func commitOptions(cmd *cobra.Command) (contribution.CommitOptions, string, error) {
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
	showRules, _ := cmd.Flags().GetBool("show-rules")
	return contribution.PrintCommits(cmd.OutOrStdout(), report, format, showRules)
}
