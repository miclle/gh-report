package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/miclle/gh-report/contribution"
)

func TestCommitsCommandWithoutToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	path := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", path}, args...)...)
		command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Alice", "GIT_AUTHOR_EMAIL=alice@example.com", "GIT_COMMITTER_NAME=Alice", "GIT_COMMITTER_EMAIL=alice@example.com")
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(path, "file"), []byte("one\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("-c", "commit.gpgSign=false", "commit", "-m", "root")
	for _, test := range []struct {
		args    []string
		failure string
	}{
		{args: []string{"--path", path, "--ref", "HEAD", "-f", "json"}},
		{args: []string{"--path", path, "--ref", "HEAD", "--sort", "issues"}, failure: "不支持排序字段"},
		{args: []string{"-f", "summary"}, failure: "不支持输出格式"},
		{args: []string{"--config", "missing.yaml"}, failure: "不支持 --config"},
		{args: []string{"--token", "unused"}, failure: "不支持 --token"},
		{args: []string{"--repos", "owner/repo"}, failure: "不支持 --repos"},
	} {
		root := &cobra.Command{Use: "test", SilenceErrors: true, SilenceUsage: true}
		root.PersistentFlags().String("config", "", "")
		root.PersistentFlags().String("token", "", "")
		root.PersistentFlags().String("repos", "", "")
		root.AddCommand(newCommitsCmd())
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(append([]string{"commits"}, test.args...))
		err := root.Execute()
		if test.failure != "" {
			if err == nil || !strings.Contains(err.Error(), test.failure) || out.Len() != 0 {
				t.Fatalf("args %v: err=%v output=%s", test.args, err, out.String())
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var report contribution.CommitReport
		if err := json.Unmarshal(out.Bytes(), &report); err != nil || report.Totals.Commits != 1 {
			t.Fatalf("command failed offline: %v %s", err, out.String())
		}
	}
}
