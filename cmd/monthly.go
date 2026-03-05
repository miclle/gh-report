package cmd

import "github.com/spf13/cobra"

var monthlyCmd = &cobra.Command{
	Use:   "monthly",
	Short: "生成月报（默认拉取最近 60 天数据）",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runReportWithType(cmd, ReportMonthly)
	},
}

func init() {
	rootCmd.AddCommand(monthlyCmd)
}
