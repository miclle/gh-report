package contribution

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// Print 将完整报告输出为 table、csv 或 json，并向调用方返回写入错误。
func Print(w io.Writer, report *Report, format string) error {
	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	case "csv":
		return printCSV(w, report)
	case "table":
		return printTable(w, report)
	default:
		return fmt.Errorf("不支持贡献统计输出格式 %q（可用 table、csv、json）", format)
	}
}

func countValues(c Counts) []string {
	values := []int64{c.Commits, c.Additions, c.Deletions, c.Changes, c.IssuesCreated, c.IssuesAuthoredCompleted, c.IssuesClosedUnknownReason, c.PRsCreated, c.PRsMerged}
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = strconv.FormatInt(value, 10)
	}
	return result
}

func printTable(w io.Writer, report *Report) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	branch, sha := report.DefaultBranch, report.HeadSHA
	if sha == "" {
		branch, sha = "（空仓库）", "—"
	}
	if _, err := fmt.Fprintf(tw, "仓库：%s\n默认分支：%s\nHEAD：%s\n采集时间：%s → %s\n范围：全历史；排除 Bot：%t；排序：%s\n\n",
		report.Repository, branch, sha, report.StartedAt.Format(time.RFC3339), report.CollectedAt.Format(time.RFC3339), report.ExcludeBots, report.Sort); err != nil {
		return err
	}
	if report.CommitSource == "local_git" {
		if _, err := fmt.Fprintf(tw, "提交来源：本地 Git\n工作区：%s\n本地 ref：%s\n\n", report.LocalPath, report.SourceRef); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(tw, "User\tType\tCommits\t+Lines\t-Lines\tChanges\tIssues\tIssuesDone\tIssuesUnknown\tPRs\tPRsMerged"); err != nil {
		return err
	}
	for _, user := range report.Users {
		kind := user.Kind
		if user.Bot {
			kind = "bot"
		}
		if _, err := fmt.Fprintln(tw, strings.Join(append([]string{user.label(), kind}, countValues(user.Counts)...), "\t")); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(tw, strings.Join(append([]string{"TOTAL", ""}, countValues(report.Totals)...), "\t")); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(tw, "\nIssuesDone：作者创建的 Issue 中已完成数；IssuesUnknown：已关闭但关闭原因缺失数。"); err != nil {
		return err
	}
	for _, rule := range report.Rules {
		if _, err := fmt.Fprintln(tw, "口径："+rule); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func printCSV(w io.Writer, report *Report) error {
	cw := csv.NewWriter(w)
	header := []string{"record_type", "repository", "default_branch", "head_sha", "started_at", "collected_at", "scope", "exclude_bots", "sort", "rules", "commit_source", "local_path", "source_ref", "identity", "login", "name", "email", "kind", "bot",
		"commits", "additions", "deletions", "changes", "issues_created", "issues_authored_completed", "issues_closed_unknown_reason", "prs_created", "prs_merged"}
	if err := cw.Write(header); err != nil {
		return err
	}
	metadata := []string{report.Repository, report.DefaultBranch, report.HeadSHA, report.StartedAt.Format(time.RFC3339), report.CollectedAt.Format(time.RFC3339), report.Scope,
		strconv.FormatBool(report.ExcludeBots), report.Sort, strings.Join(report.Rules, "；"), report.CommitSource, report.LocalPath, report.SourceRef}
	for _, user := range report.Users {
		row := append([]string{"user"}, metadata...)
		row = append(row, user.Identity, user.Login, user.Name, user.Email, user.Kind, strconv.FormatBool(user.Bot))
		row = append(row, countValues(user.Counts)...)
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	// 总计行使无参与者的 CSV 也保留仓库、HEAD 和统计口径。
	total := append([]string{"total"}, metadata...)
	total = append(total, "", "", "", "", "", "")
	total = append(total, countValues(report.Totals)...)
	if err := cw.Write(total); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}
