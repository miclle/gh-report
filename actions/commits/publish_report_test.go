package commits_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func publisherFixture(t *testing.T) (map[string]string, string, string) {
	t.Helper()
	directory := t.TempDir()
	owner := filepath.Join(directory, "owner")
	if err := os.Mkdir(owner, 0700); err != nil {
		t.Fatal(err)
	}
	remote := filepath.Join(owner, "repo.git")
	git(t, directory, "init", "--bare", remote)
	reportDirectory := filepath.Join(directory, "report")
	if err := os.Mkdir(reportDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(reportDirectory, "report.md"), "# Full report\n\n![chart](./chart.svg)\n")
	writeFile(t, filepath.Join(reportDirectory, "summary.md"), "| TOTAL | | 1 | 2 | 3 | 5 |\n")
	chart := filepath.Join(reportDirectory, "chart.svg")
	writeFile(t, chart, `<svg xmlns="http://www.w3.org/2000/svg"><text>original area chart</text></svg>`)
	output := filepath.Join(directory, "outputs")
	writeFile(t, output, "")
	env := map[string]string{
		"GH_REPORT_CHART_FILE": chart, "GH_REPORT_REPORT_BRANCH": "gh-report",
		"GH_REPORT_REPORT_DIRECTORY": reportDirectory, "GH_REPORT_ARTIFACT_NAME": "team report",
		"GITHUB_SERVER_URL": "file://" + directory, "GITHUB_REPOSITORY": "owner/repo",
		"GITHUB_OUTPUT": output, "RUNNER_TEMP": directory, "GH_REPORT_READ_ONLY": "false",
	}
	return env, remote, directory
}

func publishedURL(t *testing.T, env map[string]string) string {
	t.Helper()
	writeFile(t, env["GITHUB_OUTPUT"], "")
	out, err := runScript(t, "publish-report.sh", env)
	if err != nil {
		t.Fatalf("publish: %v\n%s", err, out)
	}
	for _, line := range strings.Split(readFile(t, env["GITHUB_OUTPUT"]), "\n") {
		if value, found := strings.CutPrefix(line, "chart-image-url="); found {
			return value
		}
	}
	return ""
}

func TestPublishOriginalChartToIsolatedBranch(t *testing.T) {
	env, remote, directory := publisherFixture(t)
	url := publishedURL(t, env)
	sha := git(t, directory, "--git-dir", remote, "rev-parse", "refs/heads/gh-report")
	digest := git(t, directory, "hash-object", env["GH_REPORT_CHART_FILE"])
	wantURL := env["GITHUB_SERVER_URL"] + "/owner/repo/raw/" + sha + "/charts/" + digest + ".svg"
	if url != wantURL {
		t.Fatalf("image URL must pin a commit: got %s, want %s", url, wantURL)
	}
	if !strings.Contains(readFile(t, env["GITHUB_OUTPUT"]), "/blob/"+sha+"/reports/team%20report/README.md") {
		t.Fatal("full report URL must pin the same commit and escape its name")
	}
	for _, file := range []string{"report.md", "summary.md", "chart.svg"} {
		got := git(t, directory, "--git-dir", remote, "show", sha+":reports/team report/"+file)
		if got != strings.TrimSpace(readFile(t, filepath.Join(env["GH_REPORT_REPORT_DIRECTORY"], file))) {
			t.Fatalf("published report lost or changed %s", file)
		}
	}
	if got := git(t, directory, "--git-dir", remote, "show", sha+":reports/team report/README.md"); got != strings.TrimSpace(readFile(t, filepath.Join(env["GH_REPORT_REPORT_DIRECTORY"], "report.md"))) {
		t.Fatal("report directory must render the full report on GitHub")
	}
	if git(t, directory, "--git-dir", remote, "rev-list", "--count", sha) != "1" {
		t.Fatal("report branch must start without source history")
	}
	if got := git(t, directory, "--git-dir", remote, "show", sha+":charts/"+digest+".svg"); got != readFile(t, env["GH_REPORT_CHART_FILE"]) {
		t.Fatal("published SVG differs from original")
	}
	if publishedURL(t, env) != url {
		t.Fatal("identical report must reuse its existing commit")
	}
	env["GH_REPORT_READ_ONLY"] = "true"
	if publishedURL(t, env) != url {
		t.Fatal("PR must be able to reuse an existing image without publishing")
	}
	writeFile(t, env["GH_REPORT_CHART_FILE"], "<svg>new report</svg>")
	writeFile(t, env["GITHUB_OUTPUT"], "")
	if _, err := runScript(t, "publish-report.sh", env); err == nil || readFile(t, env["GITHUB_OUTPUT"]) != "" {
		t.Fatal("read-only PR must not publish new assets")
	}
	env["GH_REPORT_READ_ONLY"] = "false"
	newURL := publishedURL(t, env)
	if newURL == url || git(t, directory, "--git-dir", remote, "rev-list", "--count", "gh-report") != "2" {
		t.Fatal("changed report must append a new commit")
	}
	// 旧报告继续使用其原始提交，不能随最新报告变化。
	if got := git(t, directory, "--git-dir", remote, "show", sha+":charts/"+digest+".svg"); !strings.Contains(got, "original area chart") {
		t.Fatal("old report image was lost")
	}
}

func TestPublishRejectsOrdinaryBranchAndMissingReadOnlyBranch(t *testing.T) {
	env, remote, directory := publisherFixture(t)
	env["GH_REPORT_READ_ONLY"] = "true"
	if _, err := runScript(t, "publish-report.sh", env); err == nil || readFile(t, env["GITHUB_OUTPUT"]) != "" {
		t.Fatal("read-only run must not create a branch")
	}
	_, target := fixture(t)
	git(t, target, "push", remote, "HEAD:refs/heads/main")
	original := git(t, directory, "--git-dir", remote, "rev-parse", "main")
	env["GH_REPORT_REPORT_BRANCH"] = "main"
	env["GH_REPORT_READ_ONLY"] = "false"
	out, err := runScript(t, "publish-report.sh", env)
	if err == nil || !strings.Contains(string(out), "不是 gh-report 报告分支") || readFile(t, env["GITHUB_OUTPUT"]) != "" {
		t.Fatalf("ordinary branch was not protected: %v %s", err, out)
	}
	if git(t, directory, "--git-dir", remote, "rev-parse", "main") != original {
		t.Fatal("ordinary branch changed")
	}
}

func TestConcurrentChartPublishPreservesBothReports(t *testing.T) {
	env, remote, directory := publisherFixture(t)
	publishedURL(t, env)
	// 两个发布者从同一个报告分支并发追加不同图表，最终必须都可达。
	for _, name := range []string{"one", "two"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			other := make(map[string]string, len(env))
			for key, value := range env {
				other[key] = value
			}
			other["GH_REPORT_CHART_FILE"] = filepath.Join(directory, name+".svg")
			other["GITHUB_OUTPUT"] = filepath.Join(directory, name+"-outputs")
			other["GH_REPORT_ARTIFACT_NAME"] = name
			writeFile(t, other["GH_REPORT_CHART_FILE"], "<svg>"+name+"</svg>")
			publishedURL(t, other)
		})
	}
	t.Cleanup(func() {
		files := git(t, directory, "--git-dir", remote, "ls-tree", "-r", "--name-only", "gh-report", "charts/")
		if len(strings.Split(files, "\n")) != 3 {
			t.Fatalf("concurrent publish lost a report: %s", files)
		}
		for _, name := range []string{"team report", "one", "two"} {
			git(t, directory, "--git-dir", remote, "cat-file", "-e", "gh-report:reports/"+name+"/README.md")
		}
	})
}

func TestPublishTableOnlyReport(t *testing.T) {
	env, remote, directory := publisherFixture(t)
	env["GH_REPORT_CHART_FILE"] = ""
	if err := os.Remove(filepath.Join(env["GH_REPORT_REPORT_DIRECTORY"], "chart.svg")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(env["GH_REPORT_REPORT_DIRECTORY"], "report.md"), "# Table only\n")
	if url := publishedURL(t, env); url != "" {
		t.Fatalf("table-only report returned chart URL: %s", url)
	}
	if !strings.Contains(readFile(t, env["GITHUB_OUTPUT"]), "report-url=") {
		t.Fatal("table-only report was not published")
	}
	files := git(t, directory, "--git-dir", remote, "ls-tree", "-r", "--name-only", "gh-report")
	if strings.Contains(files, ".svg") || !strings.Contains(files, "reports/team report/README.md") {
		t.Fatalf("wrong published files: %s", files)
	}
}
