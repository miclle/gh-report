package cmd

import "github.com/spf13/cobra"

var weeklyCmd = &cobra.Command{
	Use:   "weekly",
	Short: "生成周报（默认拉取最近 14 天数据）",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runReportWithType(cmd, ReportWeekly)
	},
}

func init() {
	rootCmd.AddCommand(weeklyCmd)
}
