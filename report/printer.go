package report

import (
	"encoding/csv"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	gh "github.com/google/go-github/v69/github"

	"github.com/miclle/report/github"
)

// Print 将完整的活动报告以 CSV 分段格式写入 writer。
// 按 5 个类别分段输出：Issues、Pull Requests、Issue Comments、Review Comments、Project Items。
// 每段格式为：段标题行 → CSV 表头行 → 数据行，段之间空行分隔。跳过没有数据的段。
func Print(w io.Writer, reports []RepoReport, since, until time.Time) {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	// 收集所有仓库的 5 类数据
	var (
		issueRows         [][]string
		prRows            [][]string
		issueCommentRows  [][]string
		reviewCommentRows [][]string
		projectItemRows   [][]string
	)

	now := time.Now()

	for _, rr := range reports {
		fullRepo := rr.Owner + "/" + rr.Repo

		// Issues
		for _, issue := range rr.Issues {
			issueRows = append(issueRows, []string{
				fullRepo,
				strconv.Itoa(issue.GetNumber()),
				issue.GetTitle(),
				issue.GetState(),
				issue.GetUser().GetLogin(),
				issue.GetUpdatedAt().Format("2006-01-02"),
			})
		}

		// Pull Requests
		for _, pr := range rr.PullRequests {
			reviews := buildReviewSummary(rr.Reviews[pr.GetNumber()])
			prRows = append(prRows, []string{
				fullRepo,
				strconv.Itoa(pr.GetNumber()),
				pr.GetTitle(),
				prDisplayState(pr),
				pr.GetUser().GetLogin(),
				pr.GetUpdatedAt().Format("2006-01-02"),
				reviews,
			})
		}

		// Issue Comments
		for _, c := range rr.IssueComments {
			num := extractNumber(c.GetIssueURL())
			body := truncate(strings.TrimSpace(c.GetBody()), 80)
			issueCommentRows = append(issueCommentRows, []string{
				fullRepo,
				num,
				c.GetUser().GetLogin(),
				c.GetCreatedAt().Format("2006-01-02"),
				body,
			})
		}

		// Review Comments
		for _, c := range rr.ReviewComments {
			num := extractNumber(c.GetPullRequestURL())
			body := truncate(strings.TrimSpace(c.GetBody()), 80)
			reviewCommentRows = append(reviewCommentRows, []string{
				fullRepo,
				num,
				c.GetUser().GetLogin(),
				c.GetCreatedAt().Format("2006-01-02"),
				c.GetPath(),
				body,
			})
		}

		// Project Items
		for _, project := range rr.Projects {
			// 检查该项目是否有与当前仓库相关的工作项
			hasRepoItems := false
			for _, item := range project.Items {
				if strings.Contains(item.URL, fullRepo) {
					hasRepoItems = true
					break
				}
			}
			if !hasRepoItems {
				continue
			}

			relevant := github.FindRelevantIterations(project.Iterations, now)

			type iterEntry struct {
				category  string
				iteration *github.ProjectIteration
			}
			entries := []iterEntry{
				{"Previous", relevant.Previous},
				{"Current", relevant.Current},
				{"Next", relevant.Next},
			}

			for _, entry := range entries {
				if entry.iteration == nil {
					continue
				}
				for _, item := range project.Items {
					if item.Iteration != entry.iteration.Title || !strings.Contains(item.URL, fullRepo) {
						continue
					}
					projectItemRows = append(projectItemRows, []string{
						fullRepo,
						project.Title,
						entry.iteration.Title,
						entry.category,
						strconv.Itoa(item.Number),
						item.Title,
						item.State,
						item.Status,
					})
				}
			}
		}
	}

	// 分段写入
	first := true

	if len(issueRows) > 0 {
		writeSection(cw, w, &first, "Issues",
			[]string{"Repo", "Number", "Title", "State", "User", "Date"},
			issueRows)
	}

	if len(prRows) > 0 {
		writeSection(cw, w, &first, "Pull Requests",
			[]string{"Repo", "Number", "Title", "State", "User", "Date", "Reviews"},
			prRows)
	}

	if len(issueCommentRows) > 0 {
		writeSection(cw, w, &first, "Issue Comments",
			[]string{"Repo", "Issue Number", "User", "Date", "Body"},
			issueCommentRows)
	}

	if len(reviewCommentRows) > 0 {
		writeSection(cw, w, &first, "Review Comments",
			[]string{"Repo", "PR Number", "User", "Date", "Path", "Body"},
			reviewCommentRows)
	}

	if len(projectItemRows) > 0 {
		writeSection(cw, w, &first, "Project Items",
			[]string{"Repo", "Project", "Iteration", "Category", "Number", "Title", "State", "Status"},
			projectItemRows)
	}
}

// writeSection 写入一个 CSV 分段：段标题、表头、数据行，段之间用空行分隔。
func writeSection(cw *csv.Writer, w io.Writer, first *bool, title string, header []string, rows [][]string) {
	// 先刷新之前的 CSV 缓冲
	cw.Flush()

	// 段之间插入空行
	if !*first {
		fmt.Fprintln(w)
	}
	*first = false

	// 段标题行（非 CSV 格式，直接写入）
	fmt.Fprintln(w, title)

	// 写入表头和数据行
	cw.Write(header)
	for _, row := range rows {
		cw.Write(row)
	}
	cw.Flush()
}

// buildReviewSummary 构建 PR Review 的摘要字符串，格式如 "@user1 COMMENTED; @user2 APPROVED"。
func buildReviewSummary(reviews []*gh.PullRequestReview) string {
	if len(reviews) == 0 {
		return ""
	}
	var parts []string
	seen := make(map[string]bool)
	for _, r := range reviews {
		if r.GetState() == "PENDING" {
			continue
		}
		key := r.GetUser().GetLogin() + ":" + r.GetState()
		if seen[key] {
			continue
		}
		seen[key] = true
		parts = append(parts, fmt.Sprintf("@%s %s", r.GetUser().GetLogin(), r.GetState()))
	}
	return strings.Join(parts, "; ")
}

// prDisplayState 返回 Pull Request 的可读状态。
func prDisplayState(pr *gh.PullRequest) string {
	if pr.MergedAt != nil {
		return "merged"
	}
	if pr.GetDraft() {
		return "draft"
	}
	return pr.GetState()
}

// issueNumberRe 用于从 URL 中提取 Issue/PR 编号。
var issueNumberRe = regexp.MustCompile(`/(\d+)$`)

// extractNumber 从 URL 路径中提取末尾的数字编号。
func extractNumber(url string) string {
	matches := issueNumberRe.FindStringSubmatch(url)
	if len(matches) >= 2 {
		return matches[1]
	}
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		if _, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			return parts[len(parts)-1]
		}
	}
	return "?"
}

// truncate 将字符串截断到指定的最大长度，超出部分用 "..." 替代。
func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
