package cmd

import "github.com/spf13/cobra"

var yearlyCmd = &cobra.Command{
	Use:   "yearly",
	Short: "生成年报（默认拉取最近 730 天数据）",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runReportWithType(cmd, ReportYearly)
	},
}

func init() {
	rootCmd.AddCommand(yearlyCmd)
}
