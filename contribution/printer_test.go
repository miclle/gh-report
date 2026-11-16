package contribution

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

var errOutput = errors.New("output failed")

type failedWriter struct{}

func (failedWriter) Write(p []byte) (int, error) { return 0, errOutput }

func TestPrintFormatsAndWriteFailures(t *testing.T) {
	report := &Report{Repository: "team/repo", HeadSHA: "fixed", DefaultBranch: "main", Rules: statisticsRules(), Users: []UserStats{{Identity: "git:a@example.com", Email: "a@example.com", Name: "名字,引号\"", Kind: "git", Counts: Counts{Commits: 1, Additions: 10, Deletions: 3, Changes: 13}}}}
	report.Totals = report.Users[0].Counts
	for _, format := range []string{"table", "csv", "json"} {
		t.Run(format, func(t *testing.T) {
			var out bytes.Buffer
			if err := Print(&out, report, format); err != nil {
				t.Fatal(err)
			}
			switch format {
			case "json":
				var decoded Report
				if err := json.Unmarshal(out.Bytes(), &decoded); err != nil || decoded.Users[0].Changes != 13 || len(decoded.Rules) == 0 {
					t.Fatalf("JSON did not preserve schema/metadata: %v %s", err, out.String())
				}
			case "csv":
				rows, err := csv.NewReader(strings.NewReader(out.String())).ReadAll()
				if err != nil || len(rows) != 3 || rows[1][0] != "user" || rows[1][1] != "team/repo" || rows[2][0] != "total" || !strings.Contains(out.String(), "issues_authored_completed") || !strings.Contains(out.String(), "名字") {
					t.Fatalf("invalid CSV: %v %s", err, out.String())
				}
			case "table":
				if !strings.Contains(out.String(), "a@example.com") || !strings.Contains(out.String(), "TOTAL") || !strings.Contains(out.String(), "不代表解决者贡献") {
					t.Fatalf("table missing identity/totals/rules: %s", out.String())
				}
			}
			if err := Print(failedWriter{}, report, format); !errors.Is(err, errOutput) {
				t.Errorf("write error lost: %v", err)
			}
		})
	}
}
