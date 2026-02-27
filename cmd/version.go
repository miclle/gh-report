package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version 构建版本号，通过 ldflags 注入。
var Version = "dev"

// Commit 构建时的 Git commit SHA，通过 ldflags 注入。
var Commit = "unknown"

// BuildDate 构建日期，通过 ldflags 注入。
var BuildDate = "unknown"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "显示版本信息",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("gh-report %s\ncommit: %s\nbuilt:  %s\n", Version, Commit, BuildDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
