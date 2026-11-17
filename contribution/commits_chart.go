package contribution

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const chartWeek = 7 * 24 * time.Hour

type activityPoint struct {
	date    time.Time
	commits int64
}

// PrintCommitsChart 生成包含所有作者卡片的独立 SVG，按周展示提交活动，不包含时间选择控件。
func PrintCommitsChart(w io.Writer, report *CommitReport) error {
	series := make([][]activityPoint, len(report.Users))
	var first, last time.Time
	var maximum int64
	for i, user := range report.Users {
		if len(user.Weeks) == 0 {
			return fmt.Errorf("作者 %q 缺少按周活动数据，请启用 Weekly 采集", user.Identity)
		}
		for _, week := range user.Weeks {
			date, err := time.Parse(time.DateOnly, week.Week)
			if err != nil {
				return fmt.Errorf("无法解析活动周 %q: %w", week.Week, err)
			}
			if first.IsZero() || date.Before(first) {
				first = date
			}
			if last.IsZero() || date.After(last) {
				last = date
			}
			if week.Commits > maximum {
				maximum = week.Commits
			}
			series[i] = append(series[i], activityPoint{date: date, commits: week.Commits})
		}
	}
	if !first.IsZero() && first.Equal(last) {
		first, last = first.Add(-chartWeek), last.Add(chartWeek)
	}
	const marginX, gapX, marginY, gapY, cardWidth, cardHeight, top = 8, 12, 24, 20, 546, 266, 100
	columns := 2
	if len(report.Users) < 2 {
		columns = 1
	}
	width := 2*marginX + columns*cardWidth + (columns-1)*gapX
	rows := (len(report.Users) + columns - 1) / columns
	height := top + rows*(cardHeight+gapY) + marginY
	if rows == 0 {
		height = 180
	}
	var svg strings.Builder
	fmt.Fprintf(&svg, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-labelledby="title description">`, width, height, width, height)
	svg.WriteString(`<title id="title">Contributors</title><desc id="description">贡献者提交总量、增删行数和按周提交趋势；所有卡片使用相同的 UTC 时间范围和纵轴刻度。</desc><style>text{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","Noto Sans CJK SC",sans-serif;fill:#1f2328}.muted{fill:#656d76}.small{font-size:11px}.axis{fill:#656d76;font-size:10px}</style>`)
	fmt.Fprintf(&svg, `<rect width="%d" height="%d" fill="#ffffff"/><text x="%d" y="32" font-size="22" font-weight="600">Contributors</text>`, width, height, marginX)
	fmt.Fprintf(&svg, `<text x="%d" y="56" font-size="13" class="muted">%d contributors · %d commits · +%d / −%d lines</text>`, marginX, len(report.Users), report.Totals.Commits, report.Totals.Additions, report.Totals.Deletions)
	if rows == 0 {
		fmt.Fprintf(&svg, `<text x="%d" y="118" font-size="14" class="muted">没有符合统计口径的提交</text>`, marginX)
	} else {
		fmt.Fprintf(&svg, `<text x="%d" y="77" class="small muted">%s — %s · Weekly commits · UTC · Shared scale</text>`, marginX, first.Format(time.DateOnly), last.Format(time.DateOnly))
	}
	scale := chartMaximum(maximum)
	for i, user := range report.Users {
		x, y := marginX+(i%2)*(cardWidth+gapX), top+(i/2)*(cardHeight+gapY)
		fmt.Fprintf(&svg, `<g transform="translate(%d %d)">`, x, y)
		fmt.Fprintf(&svg, `<rect width="%d" height="%d" rx="8" fill="#ffffff" stroke="#d1d9e0"/>`, cardWidth, cardHeight)
		name, email := user.Name, user.Email
		if name == "" {
			name = email
		}
		if name == "" {
			name = user.Identity
		}
		if email == "" {
			email = user.Identity
		}
		fmt.Fprintf(&svg, `<title>%s — %s</title><circle cx="32" cy="33" r="17" fill="#ddf4ff"/>`, svgText(name), svgText(email))
		if len(user.avatarPNG) > 0 {
			fmt.Fprintf(&svg, `<clipPath id="avatar-%d"><circle cx="32" cy="33" r="17"/></clipPath><image x="15" y="16" width="34" height="34" clip-path="url(#avatar-%d)" href="data:image/png;base64,%s"/>`, i, i, base64.StdEncoding.EncodeToString(user.avatarPNG))
		} else {
			fmt.Fprintf(&svg, `<text x="32" y="38" text-anchor="middle" font-size="13" font-weight="600">%s</text>`, svgText(chartInitials(name)))
		}
		fmt.Fprintf(&svg, `<text x="60" y="29" font-size="15" font-weight="600">%s</text><text x="60" y="48" class="small muted">%s</text><text x="524" y="29" text-anchor="end" font-size="12" class="muted">#%d</text>`, svgText(chartShortText(name, 28)), svgText(chartShortText(email, 58)), i+1)
		fmt.Fprintf(&svg, `<text x="20" y="80" font-size="13"><tspan font-weight="600">%d commits</tspan><tspan dx="16" fill="#1a7f37">%d ++</tspan><tspan dx="12" fill="#cf222e">%d --</tspan></text>`, user.Commits, user.Additions, user.Deletions)
		svg.WriteString(`<text x="20" y="101" class="small muted">Weekly commits</text>`)
		const left, right, plotTop, bottom = 44.0, 524.0, 117.0, 230.0
		for tick := 0; tick <= 2; tick++ {
			if scale == 1 && tick == 1 {
				continue
			}
			value := math.Floor(scale * float64(tick) / 2)
			py := bottom - (bottom-plotTop)*value/scale
			fmt.Fprintf(&svg, `<line x1="44" y1="%.2f" x2="524" y2="%.2f" stroke="#d8dee4" stroke-dasharray="3 3"/><text x="35" y="%.2f" text-anchor="end" class="axis">%s</text>`, py, py, py+3, chartTick(value))
		}
		points := chartPoints(series[i], first, last)
		var line strings.Builder
		for j, point := range points {
			px := left + float64(point.date.Unix()-first.Unix())/float64(last.Unix()-first.Unix())*(right-left)
			py := bottom - float64(point.commits)/scale*(bottom-plotTop)
			command := "L"
			if j == 0 {
				command = "M"
			}
			fmt.Fprintf(&line, "%s%.2f %.2f ", command, px, py)
		}
		fmt.Fprintf(&svg, `<path d="M44 230 %s L524 230 Z" fill="#daf4df"/><path d="%s" fill="none" stroke="#2da44e" stroke-width="1.8" stroke-linejoin="round"/>`, strings.Replace(line.String(), "M", "L", 1), line.String())
		fmt.Fprintf(&svg, `<text x="44" y="250" class="axis">%s</text><text x="524" y="250" text-anchor="end" class="axis">%s</text></g>`, first.Format(time.DateOnly), last.Format(time.DateOnly))
	}
	svg.WriteString("</svg>\n")
	_, err := io.WriteString(w, svg.String())
	return err
}

// chartPoints 只补活动周相邻的零值及边界，避免跨多年空白历史时为每个作者分配全部周。
func chartPoints(points []activityPoint, first, last time.Time) []activityPoint {
	values := map[int64]int64{first.Unix(): 0, last.Unix(): 0}
	for _, point := range points {
		for _, date := range []time.Time{point.date.Add(-chartWeek), point.date.Add(chartWeek)} {
			if !date.Before(first) && !date.After(last) {
				values[date.Unix()] = 0
			}
		}
	}
	for _, point := range points {
		values[point.date.Unix()] = point.commits
	}
	result := make([]activityPoint, 0, len(values))
	for timestamp, count := range values {
		result = append(result, activityPoint{date: time.Unix(timestamp, 0).UTC(), commits: count})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].date.Before(result[j].date) })
	return result
}

func chartMaximum(maximum int64) float64 {
	if maximum <= 1 {
		return 1
	}
	unit := math.Pow(10, math.Floor(math.Log10(float64(maximum))))
	for _, multiple := range []float64{1, 2, 5, 10} {
		if multiple*unit >= float64(maximum) {
			return multiple * unit
		}
	}
	return float64(maximum)
}

func chartTick(value float64) string {
	if value >= 1000000 {
		return fmt.Sprintf("%.3gM", value/1000000)
	}
	if value >= 1000 {
		return fmt.Sprintf("%.3gk", value/1000)
	}
	return fmt.Sprintf("%g", value)
}

func svgText(value string) string {
	var escaped strings.Builder
	// 写入内存缓冲区不会失败；EscapeText 同时处理 XML 不支持的控制字符。
	_ = xml.EscapeText(&escaped, []byte(value))
	return escaped.String()
}

func chartShortText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit-1]) + "…"
}

func chartInitials(value string) string {
	var initials []rune
	for _, field := range strings.Fields(value) {
		initial, _ := utf8.DecodeRuneInString(field)
		initials = append(initials, initial)
		if len(initials) == 2 {
			break
		}
	}
	return strings.ToUpper(string(initials))
}

// PrintCommitsChartMarkdown 在两个折叠区块中展示相邻 SVG 图表和数据表格，默认展开图表。
func PrintCommitsChartMarkdown(w io.Writer, report *CommitReport, imageName string, showRules bool) error {
	if err := printCommitsMarkdownHeader(w, report); err != nil {
		return err
	}
	// 显式相对路径避免文件名中的冒号被 URI 解析器识别为协议。
	if _, err := fmt.Fprintf(w, "<details open>\n<summary>贡献图表</summary>\n\n![贡献者提交统计](<./%s>)\n\n</details>\n\n<details>\n<summary>数据表格</summary>\n\n", url.PathEscape(imageName)); err != nil {
		return err
	}
	if err := printCommitsMarkdownTable(w, report); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, "\n</details>\n"); err != nil {
		return err
	}
	return printCommitsMarkdownRules(w, report, showRules)
}
