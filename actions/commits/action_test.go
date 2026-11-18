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
				"GH_REPORT_ARTIFACT_NAME": "team report",
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
			if chart == "true" {
				original := readFile(t, images[0])
				if outputs["chart-file"] != filepath.Join(outputs["directory"], "chart", "team report.svg") || readFile(t, outputs["chart-file"]) != original {
					t.Fatal("separate chart must preserve the original SVG byte for byte")
				}
				if !strings.Contains(original, `<path d="M44 230`) || !strings.Contains(original, "Weekly commits") {
					t.Fatal("original area chart missing")
				}
				env["GH_REPORT_CHART_URL"] = "https://github.com/example/repo/actions/runs/1/artifacts/3"
			} else if outputs["chart-file"] != "" {
				t.Fatal("disabled chart must not publish a chart path")
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
			if strings.Contains(summary, "/artifacts/3") != (chart == "true") || strings.Contains(summary, "<img") || strings.Contains(summary, "<svg") || strings.Contains(summary, "githubassets.com") {
				t.Fatalf("chart=%s: must link to original chart without an inline substitute: %s", chart, summary)
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
		"GH_REPORT_CHART_URL": "https://github.com/example/repo/actions/runs/1/artifacts/3",
	})
	if err != nil {
		t.Fatalf("summary: %v\n%s", err, output)
	}
	summary := readFile(t, summaryPath)
	if len(summary) >= 1024*1024 || !strings.Contains(summary, "| TOTAL | | 99 | 100 | 10 | 110 |") || !strings.Contains(summary, "仅展示全量总计") || !strings.Contains(summary, "/artifacts/2") || !strings.Contains(summary, "/artifacts/3") {
		t.Fatalf("invalid truncated summary: %s", summary)
	}
}

func TestArtifactNameCannotEscapeChartDirectory(t *testing.T) {
	for _, name := range []string{"../outside", `nested\file`, "name\nchart-file=outside", "name\routside"} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			outputPath := filepath.Join(directory, "outputs")
			writeFile(t, outputPath, "")
			output, err := runScript(t, "run.sh", map[string]string{
				"GITHUB_WORKSPACE": directory, "RUNNER_TEMP": directory, "GITHUB_OUTPUT": outputPath,
				"GH_REPORT_ARTIFACT_NAME": name,
			})
			if err == nil || !strings.Contains(string(output), "artifact-name") || readFile(t, outputPath) != "" {
				t.Fatalf("unsafe name accepted: %v %s", err, output)
			}
		})
	}
}

func TestSummaryEmbedsPinnedSVGAndKeepsFallbacks(t *testing.T) {
	for _, oversized := range []bool{false, true} {
		directory := t.TempDir()
		if err := os.Mkdir(filepath.Join(directory, "report"), 0700); err != nil {
			t.Fatal(err)
		}
		document := "| TOTAL | | 99 | 100 | 10 | 110 |\n"
		if oversized {
			document = strings.Repeat("long author name ", 70000) + "\n" + document
		}
		writeFile(t, filepath.Join(directory, "report", "summary.md"), document)
		summaryPath := filepath.Join(directory, "summary")
		imageURL := "https://github.com/example/repo/raw/" + strings.Repeat("a", 40) + "/charts/chart.svg"
		env := map[string]string{
			"GH_REPORT_DIRECTORY": directory, "GH_REPORT_HEAD_SHA": strings.Repeat("b", 40),
			"GH_REPORT_ARTIFACT_URL":    "https://github.com/example/repo/actions/runs/1/artifacts/2",
			"GH_REPORT_CHART_URL":       "https://github.com/example/repo/actions/runs/1/artifacts/3",
			"GH_REPORT_CHART_IMAGE_URL": imageURL, "GITHUB_STEP_SUMMARY": summaryPath,
			"GH_REPORT_PUBLISHED_REPORT_URL": "https://github.com/example/repo/blob/sha/reports/commit-report/README.md",
		}
		if out, err := runScript(t, "summary.sh", env); err != nil {
			t.Fatalf("summary: %v %s", err, out)
		}
		summary := readFile(t, summaryPath)
		for _, want := range []string{"![贡献者提交统计](<" + imageURL + ">)", "/artifacts/2", "/artifacts/3", "| TOTAL | | 99 | 100 | 10 | 110 |", "[在线查看完整报告](" + env["GH_REPORT_PUBLISHED_REPORT_URL"] + ")"} {
			if !strings.Contains(summary, want) {
				t.Fatalf("missing %q in Summary", want)
			}
		}
		if len(summary) >= 1000000 {
			t.Fatal("summary exceeds size budget")
		}
		writeFile(t, summaryPath, "")
		env["GH_REPORT_CHART_IMAGE_URL"] = ""
		env["GH_REPORT_CHART_PUBLISH_OUTCOME"] = "failure"
		if out, err := runScript(t, "summary.sh", env); err != nil {
			t.Fatalf("fallback: %v %s", err, out)
		}
		summary = readFile(t, summaryPath)
		if strings.Contains(summary, "![") || !strings.Contains(summary, "未能发布分支报告") || !strings.Contains(summary, "/artifacts/3") {
			t.Fatal("failed publication must explain fallback and retain the original chart link")
		}
	}
}

func TestSummaryWithUnavailableArtifacts(t *testing.T) {
	for _, test := range []struct {
		name, artifactOutcome, chartOutcome string
		published, artifactAvailable        bool
	}{
		{"disabled-with-branch", "skipped", "skipped", true, false},
		{"quota-exceeded", "failure", "skipped", true, false},
		{"chart-upload-failed", "success", "failure", true, true},
		{"disabled-without-branch", "skipped", "skipped", false, false},
		{"uploads-and-publication-failed", "failure", "skipped", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, oversized := range []bool{false, true} {
				env, _, directory := publisherFixture(t)
				imageURL, reportURL := "", ""
				if test.published {
					imageURL = publishedURL(t, env)
					for _, line := range strings.Split(readFile(t, env["GITHUB_OUTPUT"]), "\n") {
						if value, ok := strings.CutPrefix(line, "report-url="); ok {
							reportURL = value
						}
					}
					if imageURL == "" || reportURL == "" {
						t.Fatal("missing published report links")
					}
				}
				if oversized {
					writeFile(t, filepath.Join(directory, "report", "summary.md"), strings.Repeat("author ", 150000)+"\n| TOTAL | | 1 | 2 | 3 | 5 |\n")
				}
				summaryPath := filepath.Join(directory, "job-summary")
				summaryEnv := map[string]string{
					"GH_REPORT_DIRECTORY": directory, "GH_REPORT_HEAD_SHA": strings.Repeat("a", 40),
					"GITHUB_STEP_SUMMARY":        summaryPath,
					"GH_REPORT_ARTIFACT_OUTCOME": test.artifactOutcome, "GH_REPORT_CHART_OUTCOME": test.chartOutcome,
					"GH_REPORT_CHART_IMAGE_URL": imageURL, "GH_REPORT_PUBLISHED_REPORT_URL": reportURL,
				}
				if test.artifactAvailable {
					summaryEnv["GH_REPORT_ARTIFACT_URL"] = "https://github.com/example/repo/actions/runs/1/artifacts/2"
				}
				if test.name == "uploads-and-publication-failed" {
					summaryEnv["GH_REPORT_CHART_PUBLISH_OUTCOME"] = "failure"
				}
				output, err := runScript(t, "summary.sh", summaryEnv)
				if err != nil {
					t.Fatalf("summary: %v\n%s", err, output)
				}
				summary := readFile(t, summaryPath)
				if !strings.Contains(summary, "| TOTAL | | 1 | 2 | 3 | 5 |") || len(summary) >= 1000000 {
					t.Fatal("summary must preserve totals within the size budget")
				}
				if test.published && (!strings.Contains(summary, "![贡献者提交统计](<"+imageURL+">)") || !strings.Contains(summary, reportURL)) {
					t.Fatal("missing published chart or report when artifacts are unavailable")
				}
				if strings.Contains(summary, "[下载完整报告") != test.artifactAvailable || strings.Contains(summary, "]()") {
					t.Fatal("summary must only link to available downloads")
				}
				failed := test.artifactOutcome == "failure" || test.chartOutcome == "failure"
				if strings.Contains(summary, "Artifact 上传未完成") != failed || strings.Contains(string(output), "::warning::") != failed {
					t.Fatal("upload failures must produce a warning without treating disabled uploads as failures")
				}
				if test.name == "uploads-and-publication-failed" && !strings.Contains(summary, "未能发布分支报告") {
					t.Fatal("publication failure must remain visible when uploads also fail")
				}
			}
		})
	}
}

func TestInvalidUploadArtifactsPublishesNoOutputs(t *testing.T) {
	directory := t.TempDir()
	outputPath := filepath.Join(directory, "outputs")
	writeFile(t, outputPath, "")
	output, err := runScript(t, "run.sh", map[string]string{
		"GITHUB_WORKSPACE": directory, "RUNNER_TEMP": directory, "GITHUB_OUTPUT": outputPath,
		"GH_REPORT_UPLOAD_ARTIFACTS": "yes",
	})
	if err == nil || !strings.Contains(string(output), "upload-artifacts") || readFile(t, outputPath) != "" {
		t.Fatalf("invalid upload option accepted: %v\n%s", err, output)
	}
}
