package cmd

import (
	"encoding/xml"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/miclle/gh-report/contribution"
)

func TestSaveCommitChartBundleAndPreserveExistingReport(t *testing.T) {
	counts := contribution.CommitCounts{Commits: 2, Additions: 3, Deletions: 1, Changes: 4}
	report := &contribution.CommitReport{Totals: counts, Users: []contribution.CommitUser{{
		Identity: "git:alice@example.com", Email: "alice@example.com", CommitCounts: counts,
		Weeks: []contribution.CommitWeek{{Week: "2026-01-05", CommitCounts: counts}},
	}}}
	directory := t.TempDir()
	output := filepath.Join(directory, "report-2026-11-16T10:00 (团队)#1.md")
	if err := saveCommitReport(report, "markdown", output, false, true); err != nil {
		t.Fatal(err)
	}
	document, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(document), "<details open>\n<summary>贡献图表</summary>\n\n") ||
		!strings.Contains(string(document), "<details>\n<summary>数据表格</summary>\n\n| User |") ||
		!strings.Contains(string(document), "| :--- | :--- | ---: | ---: | ---: | ---: |") ||
		strings.Count(string(document), "</details>") != 2 {
		t.Fatal("chart mode must include collapsible chart and aligned table")
	}
	_, image, found := strings.Cut(string(document), "![贡献者提交统计](<")
	if !found {
		t.Fatalf("missing image: %s", document)
	}
	image, _, _ = strings.Cut(image, ">)")
	ref, err := url.Parse(image)
	if err != nil || ref.Scheme != "" || ref.Host != "" || !strings.HasPrefix(image, "./") {
		t.Fatalf("bad relative image path: %s %v", image, err)
	}
	name := ref.Path
	if filepath.Base(name) != filepath.Clean(name) {
		t.Fatalf("image must be adjacent to the report: %s", name)
	}
	data, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil {
		t.Fatal(err)
	}
	var svg struct{ XMLName xml.Name }
	if err := xml.Unmarshal(data, &svg); err != nil || svg.XMLName.Local != "svg" {
		t.Fatalf("bad SVG: %v", err)
	}
	// 失败时保留旧 Markdown 及其图片，防止把缺少活动数据误导出成空趋势。
	report.Users[0].Weeks = nil
	if err := saveCommitReport(report, "markdown", output, false, true); err == nil {
		t.Fatal("missing activity should fail")
	}
	unchanged, err := os.ReadFile(output)
	if err != nil || string(unchanged) != string(document) {
		t.Fatal("failed export damaged old Markdown")
	}
	if _, err := os.Stat(filepath.Join(directory, name)); err != nil {
		t.Fatal("failed export damaged old SVG")
	}
	if err := saveCommitReport(report, "markdown", directory, false, true); err == nil {
		t.Fatal("directory output should fail")
	}
}
