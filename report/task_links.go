package report

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/miclle/gh-report/github"
)

type closingIssueLister interface {
	ListClosingIssues(context.Context, string, string, int) ([]github.LinkedIssue, error)
}

// collectTaskLinks 仅为已采集 PR 和符合计划规则的 Project PR 补充关联。
// 关联 Issue 可以不在活动窗口内，但不能据此补造该用户的本期活动。
func collectTaskLinks(ctx context.Context, client closingIssueLister, reports []RepoReport, user string) error {
	plans := extractPlanItems(reports, user)
	type request struct {
		repoIndex int
		number    int
		issues    []github.LinkedIssue
		err       error
	}
	var requests []request
	for i := range reports {
		rr := &reports[i]
		numbers := make(map[int]bool)
		for _, pr := range rr.PullRequests {
			numbers[pr.GetNumber()] = true
		}
		for _, item := range plans {
			if item.Type == "pr" && item.Repo == rr.Owner+"/"+rr.Repo {
				numbers[item.Number] = true
			}
		}
		ordered := make([]int, 0, len(numbers))
		for number := range numbers {
			ordered = append(ordered, number)
		}
		sort.Ints(ordered)
		for _, number := range ordered {
			requests = append(requests, request{repoIndex: i, number: number})
		}
		rr.PullRequestIssues = make(map[int][]github.LinkedIssue)
	}

	// HTTP 客户端统一限制实际并发数，各请求独占结果槽位。
	var wg sync.WaitGroup
	for i := range requests {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := &requests[i]
			rr := &reports[req.repoIndex]
			req.issues, req.err = client.ListClosingIssues(ctx, rr.Owner, rr.Repo, req.number)
		}(i)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, req := range requests {
		rr := &reports[req.repoIndex]
		if req.err != nil {
			fmt.Fprintf(os.Stderr, "警告：无法获取 %s/%s#%d 的 Issue 关联，保留独立条目：%v\n", rr.Owner, rr.Repo, req.number, req.err)
			continue
		}
		rr.PullRequestIssues[req.number] = req.issues
	}
	return nil
}
