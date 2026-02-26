// Package github 提供 GitHub REST 和 GraphQL API 客户端。
package github

import "time"

// ProjectIteration 表示 GitHub Projects v2 迭代字段中的单个迭代。
type ProjectIteration struct {
	ID        string `json:"id"`        // 迭代 ID
	Title     string `json:"title"`     // 迭代标题
	StartDate string `json:"startDate"` // 开始日期（格式: 2006-01-02）
	Duration  int    `json:"duration"`  // 持续天数
}

// ProjectItem 表示 GitHub Projects v2 项目中的一个工作项。
type ProjectItem struct {
	Title     string // 标题
	Number    int    // Issue 或 PR 编号
	URL       string // 链接地址
	State     string // 状态（OPEN、CLOSED、MERGED）
	Type      string // 类型（"Issue" 或 "PullRequest"）
	Iteration string // 所属迭代标题
	Status    string // 状态字段值（如 "Done"、"In Progress"）
}

// Project 表示一个 GitHub Projects v2 项目，包含迭代信息。
type Project struct {
	Title      string             // 项目标题
	Number     int                // 项目编号
	Iterations []ProjectIteration // 所有迭代
	Items      []ProjectItem      // 所有工作项
}

// IterationCategory 表示迭代相对于当前日期的分类。
type IterationCategory string

const (
	// IterationPrevious 表示已过去的迭代。
	IterationPrevious IterationCategory = "previous"
	// IterationCurrent 表示当前正在进行的迭代。
	IterationCurrent IterationCategory = "current"
	// IterationNext 表示下一个即将开始的迭代。
	IterationNext IterationCategory = "next"
)

// ClassifyIteration 根据给定的参考时间，判断迭代属于过去、当前还是未来。
func ClassifyIteration(iter ProjectIteration, now time.Time) IterationCategory {
	start, err := time.Parse("2006-01-02", iter.StartDate)
	if err != nil {
		return IterationPrevious
	}
	end := start.AddDate(0, 0, iter.Duration)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	if today.Before(start) {
		return IterationNext
	}
	if !today.Before(end) {
		return IterationPrevious
	}
	return IterationCurrent
}

// RelevantIterations 保存与当前时间最相关的三个迭代（各最多一个）。
type RelevantIterations struct {
	Previous *ProjectIteration // 最近的已结束迭代（可能为 nil）
	Current  *ProjectIteration // 当前正在进行的迭代（可能为 nil）
	Next     *ProjectIteration // 最近的未来迭代（可能为 nil）
}

// FindRelevantIterations 从迭代列表中找出最相关的三个迭代：
// 最近结束的一个 previous、当前进行中的 current、最近将开始的 next。
func FindRelevantIterations(iterations []ProjectIteration, now time.Time) RelevantIterations {
	var result RelevantIterations
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	for i := range iterations {
		iter := &iterations[i]
		start, err := time.Parse("2006-01-02", iter.StartDate)
		if err != nil {
			continue
		}
		end := start.AddDate(0, 0, iter.Duration)

		switch {
		case today.Before(start):
			// 未来迭代：选择开始日期最早（最近）的
			if result.Next == nil {
				result.Next = iter
			} else {
				nextStart, _ := time.Parse("2006-01-02", result.Next.StartDate)
				if start.Before(nextStart) {
					result.Next = iter
				}
			}
		case !today.Before(end):
			// 已结束迭代：选择结束日期最晚（最近结束）的
			if result.Previous == nil {
				result.Previous = iter
			} else {
				prevStart, _ := time.Parse("2006-01-02", result.Previous.StartDate)
				prevEnd := prevStart.AddDate(0, 0, result.Previous.Duration)
				if end.After(prevEnd) {
					result.Previous = iter
				}
			}
		default:
			// 当前迭代
			result.Current = iter
		}
	}

	return result
}
