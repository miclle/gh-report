package contribution

import (
	"fmt"
	"html"
	"io"
	"math"
	"strings"
	"time"
)

// GitHub Summary 过滤内联 SVG、data URL 和样式。使用官方静态绿方块的显示尺寸绘制柱状图，URL 不携带报告数据。
const summaryBarImage = "https://github.githubassets.com/images/icons/emoji/unicode/1f7e9.png?v8"

// PrintCommitsJobSummary 输出 GitHub Job Summary 可直接渲染的贡献者卡片、活动柱状图和可折叠表格。
// 所有卡片共享 UTC 时间范围、分组宽度及纵轴；长历史按连续周分组求和，最多展示 52 列。
func PrintCommitsJobSummary(w io.Writer, report *CommitReport, avatarUsers map[string]string, showRules bool) error {
	series, first, last, _, err := commitChartSeries(report)
	if err != nil {
		return err
	}
	var document strings.Builder
	if err := printCommitsMarkdownHeader(&document, report); err != nil {
		return err
	}
	document.WriteString("<details open>\n<summary>贡献图表</summary>\n\n")
	if len(report.Users) == 0 {
		document.WriteString("没有符合统计口径的提交。\n")
	} else {
		weeks := int((last.Unix()-first.Unix())/int64(chartWeek/time.Second)) + 1
		weeksPerColumn := (weeks + 51) / 52
		columns := (weeks + weeksPerColumn - 1) / weeksPerColumn
		values := make([][]int64, len(series))
		var maximum int64
		for i, points := range series {
			values[i] = make([]int64, columns)
			for _, point := range points {
				week := int((point.date.Unix() - first.Unix()) / int64(chartWeek/time.Second))
				values[i][week/weeksPerColumn] += point.commits
			}
			for _, count := range values[i] {
				if count > maximum {
					maximum = count
				}
			}
		}
		fmt.Fprintf(&document, "%s — %s · UTC · 每列 %d 周提交数合计 · 共享纵轴 0–%d\n\n", first.Format(time.DateOnly), last.Format(time.DateOnly), weeksPerColumn, maximum)
		document.WriteString("<table><tbody>\n")
		for i, user := range report.Users {
			if i%2 == 0 {
				document.WriteString("<tr>")
			}
			name, identity := user.Name, user.Email
			if identity == "" {
				identity = user.Identity
			}
			if name == "" {
				name = identity
			}
			document.WriteString(`<td valign="top">`)
			if avatarURL := githubAvatarURL(user.Email, avatarUsers); len(user.avatarPNG) > 0 && avatarURL != "" {
				fmt.Fprintf(&document, `<img src="%s?s=64" width="32" height="32" alt=""> `, html.EscapeString(avatarURL))
			} else {
				fmt.Fprintf(&document, "<strong>[%s]</strong> ", html.EscapeString(chartInitials(name)))
			}
			fmt.Fprintf(&document, "<strong>#%d %s</strong><br><sub>%s</sub><br><strong>%d commits</strong> · +%d / −%d lines<br>", i+1, html.EscapeString(chartShortText(name, 40)), html.EscapeString(identity), user.Commits, user.Additions, user.Deletions)
			// 一像素纵轴固定绘图区高度；零值柱保留列宽，显示高度为零。
			fmt.Fprintf(&document, `<img src="%s" width="1" height="72" alt="">`, summaryBarImage)
			for column, count := range values[i] {
				height := 0
				if count > 0 && maximum > 0 {
					height = int(math.Ceil(float64(count) / float64(maximum) * 72))
				}
				start := first.AddDate(0, 0, column*weeksPerColumn*7)
				end := start.AddDate(0, 0, weeksPerColumn*7-1)
				lastDay := last.AddDate(0, 0, 6)
				if end.After(lastDay) {
					end = lastDay
				}
				fmt.Fprintf(&document, `<img src="%s" width="5" height="%d" alt="" title="%s — %s: %d commits">`, summaryBarImage, height, start.Format(time.DateOnly), end.Format(time.DateOnly), count)
			}
			fmt.Fprintf(&document, "<br><sub>%s — %s</sub></td>", first.Format(time.DateOnly), last.Format(time.DateOnly))
			if i%2 == 1 || i == len(report.Users)-1 {
				document.WriteString("</tr>\n")
			}
		}
		document.WriteString("</tbody></table>\n")
	}
	document.WriteString("\n</details>\n\n<details>\n<summary>数据表格</summary>\n\n")
	if err := printCommitsMarkdownTable(&document, report); err != nil {
		return err
	}
	document.WriteString("\n</details>\n")
	if err := printCommitsMarkdownRules(&document, report, showRules); err != nil {
		return err
	}
	_, err = io.WriteString(w, document.String())
	return err
}
