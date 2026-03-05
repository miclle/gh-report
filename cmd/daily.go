package cmd

import "github.com/spf13/cobra"

var dailyCmd = &cobra.Command{
	Use:   "daily",
	Short: "生成日报（默认拉取最近 1 天数据）",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runReportWithType(cmd, ReportDaily)
	},
}

func init() {
	rootCmd.AddCommand(dailyCmd)
}
