package contribution

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

func commitValues(c CommitCounts) []string {
	return []string{strconv.FormatInt(c.Commits, 10), strconv.FormatInt(c.Additions, 10), strconv.FormatInt(c.Deletions, 10), strconv.FormatInt(c.Changes, 10)}
}

// PrintCommits 将离线提交报告输出为 table、markdown、csv 或 json；showRules 控制表格和 Markdown 中的口径说明。
func PrintCommits(w io.Writer, report *CommitReport, format string, showRules bool) error {
	switch format {
	case "markdown":
		return printCommitsMarkdown(w, report, showRules)
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	case "csv":
		cw := csv.NewWriter(w)
		if err := cw.Write([]string{"record_type", "local_path", "source_ref", "head_sha", "started_at", "collected_at", "sort", "rules", "identity", "name", "email", "commits", "additions", "deletions", "changes"}); err != nil {
			return err
		}
		metadata := []string{report.LocalPath, report.SourceRef, report.HeadSHA, report.StartedAt.Format(time.RFC3339), report.CollectedAt.Format(time.RFC3339), report.Sort, strings.Join(report.Rules, "；")}
		for _, u := range report.Users {
			row := append([]string{"user"}, metadata...)
			row = append(row, u.Identity, u.Name, u.Email)
			if err := cw.Write(append(row, commitValues(u.CommitCounts)...)); err != nil {
				return err
			}
		}
		total := append([]string{"total"}, metadata...)
		total = append(total, "", "", "")
		if err := cw.Write(append(total, commitValues(report.Totals)...)); err != nil {
			return err
		}
		cw.Flush()
		return cw.Error()
	case "table":
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		if _, err := fmt.Fprintf(tw, "工作区：%s\n本地 ref：%s\nHEAD：%s\n采集时间：%s → %s\n排序：%s\n\nUser\tName\tCommits\t+Lines\t-Lines\tChanges\n", report.LocalPath, report.SourceRef, report.HeadSHA, report.StartedAt.Format(time.RFC3339), report.CollectedAt.Format(time.RFC3339), report.Sort); err != nil {
			return err
		}
		for _, u := range report.Users {
			label := u.Email
			if label == "" {
				label = u.Identity
			}
			if _, err := fmt.Fprintln(tw, strings.Join(append([]string{label, u.Name}, commitValues(u.CommitCounts)...), "\t")); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(tw, strings.Join(append([]string{"TOTAL", ""}, commitValues(report.Totals)...), "\t")); err != nil {
			return err
		}
		if showRules {
			for _, rule := range report.Rules {
				if _, err := fmt.Fprintln(tw, "口径："+rule); err != nil {
					return err
				}
			}
		}
		return tw.Flush()
	default:
		return fmt.Errorf("不支持提交统计输出格式 %q（可用 table、markdown、csv、json）", format)
	}
}

// markdownText 防止作者、路径等原始文本破坏表格结构或被解释为 Markdown / HTML。
func markdownText(value string) string {
	return strings.NewReplacer(
		"\\", "\\\\", "|", "&#124;", "`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "~", "\\~",
		"\r\n", "<br>", "\r", "<br>", "\n", "<br>",
	).Replace(html.EscapeString(value))
}

func printCommitsMarkdown(w io.Writer, report *CommitReport, showRules bool) error {
	if err := printCommitsMarkdownHeader(w, report); err != nil {
		return err
	}
	if err := printCommitsMarkdownTable(w, report); err != nil {
		return err
	}
	return printCommitsMarkdownRules(w, report, showRules)
}

func printCommitsMarkdownTable(w io.Writer, report *CommitReport) error {
	if _, err := fmt.Fprint(w, "| User | Name | Commits | +Lines | -Lines | Changes |\n| :--- | :--- | ---: | ---: | ---: | ---: |\n"); err != nil {
		return err
	}
	for _, u := range report.Users {
		label := u.Email
		if label == "" {
			label = u.Identity
		}
		row := append([]string{markdownText(label), markdownText(u.Name)}, commitValues(u.CommitCounts)...)
		if _, err := fmt.Fprintln(w, "| "+strings.Join(row, " | ")+" |"); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "| TOTAL | | "+strings.Join(commitValues(report.Totals), " | ")+" |"); err != nil {
		return err
	}
	return nil
}

func printCommitsMarkdownHeader(w io.Writer, report *CommitReport) error {
	_, err := fmt.Fprintf(w, "# 提交统计\n\n- 工作区：%s\n- 本地 ref：%s\n- HEAD：%s\n- 采集时间：%s → %s\n- 排序：%s\n\n",
		markdownText(report.LocalPath), markdownText(report.SourceRef), markdownText(report.HeadSHA), report.StartedAt.Format(time.RFC3339), report.CollectedAt.Format(time.RFC3339), markdownText(report.Sort))
	return err
}

func printCommitsMarkdownRules(w io.Writer, report *CommitReport, showRules bool) error {
	if showRules {
		if _, err := fmt.Fprint(w, "\n统计口径：\n\n"); err != nil {
			return err
		}
		for _, rule := range report.Rules {
			if _, err := fmt.Fprintln(w, "- "+markdownText(rule)); err != nil {
				return err
			}
		}
	}
	return nil
}
