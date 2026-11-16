package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestContributorsFlagsAndConfig(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "environment-token")
	config := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(config, []byte("repos: [team/config]\ntoken: config-token\ndays: 1\nuser: stale-user\nformat: summary\nai: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, repo, format, token, failure string
		args                               []string
		local                              bool
	}{
		{name: "defaults", args: []string{"contributors", "-r", "team/repo"}, repo: "team/repo", format: "table", token: "environment-token"},
		{name: "local infers repo", args: []string{"contributors", "--local"}, format: "table", token: "environment-token", local: true},
		{name: "local ignores config repos", args: []string{"-c", config, "contributors", "--local"}, format: "table", token: "config-token", local: true},
		{name: "local explicit repo", args: []string{"contributors", "--local", "-r", "team/repo"}, repo: "team/repo", format: "table", token: "environment-token", local: true},
		{name: "local empty explicit repo", args: []string{"contributors", "--local", "-r", ""}, failure: "owner/repo"},
		{name: "local multiple repos", args: []string{"contributors", "--local", "-r", "team/one,team/two"}, failure: "恰好一个仓库"},
		{name: "config report fields ignored", args: []string{"-c", config, "contributors"}, repo: "team/config", format: "table", token: "config-token"},
		{name: "CLI precedence and inherited before subcommand", args: []string{"-c", config, "-r", "team/override", "--token", "flag-token", "contributors", "-f", "json"}, repo: "team/override", format: "json", token: "flag-token"},
		{name: "days", args: []string{"contributors", "-r", "team/repo", "--days", "1"}, failure: "不支持 --days"},
		{name: "user before command", args: []string{"--user", "alice", "contributors", "-r", "team/repo"}, failure: "不支持 --user"},
		{name: "ai explicitly false", args: []string{"contributors", "-r", "team/repo", "--ai=false"}, failure: "不支持 --ai"},
		{name: "multiple repos", args: []string{"contributors", "-r", "team/one,team/two"}, failure: "恰好一个仓库"},
		{name: "invalid repo", args: []string{"contributors", "-r", "https://github.com/team/repo"}, failure: "owner/repo"},
		{name: "invalid format", args: []string{"contributors", "-r", "team/repo", "-f", "summary"}, failure: "不支持输出格式"},
		{name: "invalid sort", args: []string{"contributors", "-r", "team/repo", "--sort", "score"}, failure: "不支持排序字段"},
		{name: "positional args", args: []string{"contributors", "extra"}, failure: "unknown command"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := &cobra.Command{Use: "test", SilenceErrors: true, SilenceUsage: true}
			f := root.PersistentFlags()
			f.StringP("config", "c", "", "")
			f.StringP("repos", "r", "", "")
			f.String("token", "", "")
			f.StringP("format", "f", "", "")
			f.Int("days", 0, "")
			f.String("user", "", "")
			f.Bool("ai", false, "")
			cmd := newContributorsCmd()
			cmd.RunE = func(cmd *cobra.Command, _ []string) error {
				opts, format, token, err := contributorOptions(cmd)
				if err == nil && (opts.Repo != test.repo || format != test.format || token != test.token) {
					t.Errorf("options=%+v format=%s token precedence incorrect", opts, format)
				}
				if err == nil && (opts.LocalPath != "") != test.local {
					t.Errorf("local mode=%q, want %t", opts.LocalPath, test.local)
				}
				return err
			}
			root.AddCommand(cmd)
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(test.args)
			err := root.Execute()
			if test.failure == "" && err != nil {
				t.Fatal(err)
			}
			if test.failure != "" && (err == nil || !strings.Contains(err.Error(), test.failure)) {
				t.Errorf("got %v, want error containing %q", err, test.failure)
			}
		})
	}
}
