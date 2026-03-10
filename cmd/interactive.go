package cmd

import (
	"github.com/spf13/cobra"
	"github.com/miclle/gh-report/ui"
)

var interactiveCmd = &cobra.Command{
	Use:   "interactive",
	Short: "交互式模式（引导式选择参数）",
	Long: `交互式模式，通过引导式表单选择报告参数。

适用于不熟悉命令行参数的用户，或需要快速选择多个仓库的场景。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteractive()
	},
}

func init() {
	rootCmd.AddCommand(interactiveCmd)
}

// runInteractive 运行交互式表单。
func runInteractive() error {
	// 运行完整表单
	form, err := ui.RunFullForm(nil)
	if err != nil {
		return err
	}

	// 根据选择的报告类型确定报告类型
	var reportType ReportType
	switch form.ReportType {
	case "weekly":
		reportType = ReportWeekly
	case "monthly":
		reportType = ReportMonthly
	case "yearly":
		reportType = ReportYearly
	default:
		reportType = ReportDaily
	}

	// 构建 Config
	cfg := &Config{
		Repos:      form.Repos,
		Format:     form.Format,
		AI:         form.UseAI,
		AIProvider: form.AIProvider,
	}

	// 应用默认天数
	cfg.Days = defaultDays(reportType)

	// 运行报告
	return runReportWithConfig(reportType, cfg)
}
