package commits_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, path string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", path, "-c", "commit.gpgsign=false"}, args...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func fixture(t *testing.T) (string, string) {
	t.Helper()
	workspace := t.TempDir()
	target := filepath.Join(workspace, "repo with spaces")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	git(t, target, "init", "-b", "main")
	git(t, target, "config", "user.name", "Alice | & test")
	git(t, target, "config", "user.email", "alice@example.com")
	writeFile(t, filepath.Join(target, "file.txt"), "one\ntwo\n")
	git(t, target, "add", ".")
	git(t, target, "commit", "-m", "first")
	git(t, target, "config", "user.name", "Bob")
	git(t, target, "config", "user.email", "bob@example.com")
	writeFile(t, filepath.Join(target, "file.txt"), "two\nthree\nfour\n")
	git(t, target, "commit", "-am", "second")
	// 调用仓库的无效工作区不能影响从 Action 自身源码构建。
	writeFile(t, filepath.Join(target, "go.work"), "invalid workspace\n")
	return workspace, target
}

func runScript(t *testing.T, script string, env map[string]string) ([]byte, error) {
	t.Helper()
	actionPath, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("bash", filepath.Join(actionPath, script))
	command.Env = append(os.Environ(), "GH_REPORT_ACTION_PATH="+actionPath)
	for key, value := range env {
		command.Env = append(command.Env, key+"="+value)
	}
	return command.CombinedOutput()
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestReportsAndSummary(t *testing.T) {
	for _, chart := range []string{"true", "false"} {
		t.Run("chart="+chart, func(t *testing.T) {
			workspace, target := fixture(t)
			temporary := t.TempDir()
			outputPath := filepath.Join(temporary, "outputs")
			summaryPath := filepath.Join(temporary, "summary")
			env := map[string]string{
				"GITHUB_WORKSPACE": workspace, "RUNNER_TEMP": temporary, "GITHUB_OUTPUT": outputPath,
				"GH_REPORT_PATH": filepath.Base(target), "GH_REPORT_REF": "HEAD", "GH_REPORT_SORT": "commits",
				"GH_REPORT_CHART": chart, "GH_REPORT_AVATARS": "false",
			}
			output, err := runScript(t, "run.sh", env)
			if err != nil {
				t.Fatalf("generate: %v\n%s", err, output)
			}
			outputs := map[string]string{}
			for _, line := range strings.Split(strings.TrimSpace(readFile(t, outputPath)), "\n") {
				key, value, _ := strings.Cut(line, "=")
				outputs[key] = value
			}
			sha := git(t, target, "rev-parse", "HEAD")
			if outputs["head-sha"] != sha {
				t.Fatalf("wrong snapshot: %v", outputs)
			}
			reportDirectory := filepath.Join(outputs["directory"], "report")
			report := readFile(t, filepath.Join(reportDirectory, "report.md"))
			for _, expected := range []string{sha, "| TOTAL | | 2 | 4 | 1 | 5 |", "---:", "Alice &#124; &amp; test"} {
				if !strings.Contains(report, expected) {
					t.Fatalf("missing %q in report:\n%s", expected, report)
				}
			}
			images, err := filepath.Glob(filepath.Join(reportDirectory, "*.svg"))
			if err != nil || (chart == "true" && len(images) != 1) || (chart == "false" && len(images) != 0) {
				t.Fatalf("unexpected images: %v, %v", images, err)
			}
			if chart == "true" && !strings.Contains(report, "./"+filepath.Base(images[0])) {
				t.Fatal("report does not reference adjacent SVG")
			}
			env["GH_REPORT_DIRECTORY"] = outputs["directory"]
			env["GH_REPORT_HEAD_SHA"] = sha
			env["GH_REPORT_ARTIFACT_URL"] = "https://github.com/example/repo/actions/runs/1/artifacts/2"
			env["GITHUB_STEP_SUMMARY"] = summaryPath
			output, err = runScript(t, "summary.sh", env)
			if err != nil {
				t.Fatalf("summary: %v\n%s", err, output)
			}
			summary := readFile(t, summaryPath)
			if !strings.Contains(summary, "| TOTAL | | 2 | 4 | 1 | 5 |") || !strings.Contains(summary, env["GH_REPORT_ARTIFACT_URL"]) || strings.Contains(summary, ".svg") {
				t.Fatalf("invalid Job Summary:\n%s", summary)
			}
			hasChart := strings.Contains(summary, `<table><tbody>`) && strings.Contains(summary, `width="5" height=`) && strings.Contains(summary, "共享纵轴")
			if hasChart != (chart == "true") {
				t.Fatalf("chart=%s: wrong inline chart state: %s", chart, summary)
			}
		})
	}
}

func TestFailedCollectionPublishesNoOutputs(t *testing.T) {
	workspace, target := fixture(t)
	shallow := filepath.Join(workspace, "shallow")
	git(t, workspace, "clone", "--depth=1", "file://"+target, shallow)
	for _, test := range []struct{ name, path, ref, chart string }{
		{"shallow", shallow, "HEAD", "false"},
		{"missing-ref", target, "does-not-exist", "false"},
		{"invalid-boolean", target, "HEAD", "yes"},
	} {
		t.Run(test.name, func(t *testing.T) {
			temporary := t.TempDir()
			outputPath := filepath.Join(temporary, "outputs")
			writeFile(t, outputPath, "")
			output, err := runScript(t, "run.sh", map[string]string{
				"GITHUB_WORKSPACE": workspace, "RUNNER_TEMP": temporary, "GITHUB_OUTPUT": outputPath,
				"GH_REPORT_PATH": test.path, "GH_REPORT_REF": test.ref, "GH_REPORT_CHART": test.chart,
			})
			if err == nil || readFile(t, outputPath) != "" {
				t.Fatalf("expected failure without partial outputs: %v\n%s", err, output)
			}
		})
	}
}

func TestOversizedSummaryKeepsTotalsAndDownload(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "report"), 0700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(directory, "report", "summary.md"), strings.Repeat("long author name ", 70000)+"\n| TOTAL | | 99 | 100 | 10 | 110 |\n")
	summaryPath := filepath.Join(directory, "summary")
	output, err := runScript(t, "summary.sh", map[string]string{
		"GH_REPORT_DIRECTORY": directory, "GH_REPORT_HEAD_SHA": strings.Repeat("a", 40),
		"GH_REPORT_ARTIFACT_URL": "https://github.com/example/repo/actions/runs/1/artifacts/2", "GITHUB_STEP_SUMMARY": summaryPath,
	})
	if err != nil {
		t.Fatalf("summary: %v\n%s", err, output)
	}
	summary := readFile(t, summaryPath)
	if len(summary) >= 1024*1024 || !strings.Contains(summary, "| TOTAL | | 99 | 100 | 10 | 110 |") || !strings.Contains(summary, "仅展示全量总计") || !strings.Contains(summary, "/artifacts/2") {
		t.Fatalf("invalid truncated summary: %s", summary)
	}
}
