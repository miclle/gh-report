// Package contribution 提供仓库默认分支全历史贡献统计，支持 API 和本地 Git 提交采集。
package contribution

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	gh "github.com/google/go-github/v69/github"

	"github.com/miclle/gh-report/github"
)

// Options 指定单仓库贡献统计的范围和展示方式。
type Options struct {
	Repo        string
	Sort        string
	ExcludeBots bool
	LocalPath   string // 非空时从该工作区的本地默认分支读取提交。
}

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9_.-]+$`)

// Validate 在发起网络请求前验证仓库格式和排序字段。
func (o Options) Validate() error {
	if !(o.LocalPath != "" && o.Repo == "") && !o.validRepo() {
		return fmt.Errorf("请指定一个 owner/repo 格式的仓库（不支持逗号分隔的多个仓库）")
	}
	switch o.Sort {
	case "", "commits", "changes", "issues", "issues-completed", "prs", "prs-merged", "user":
		return nil
	default:
		return fmt.Errorf("不支持排序字段 %q（可用 commits、changes、issues、issues-completed、prs、prs-merged、user）", o.Sort)
	}
}

func (o Options) validRepo() bool {
	return repositoryPattern.MatchString(o.Repo) && !strings.HasSuffix(o.Repo, "/.") && !strings.HasSuffix(o.Repo, "/..")
}

// Counts 保存各维度独立计数；Issue 完成数只属于该 Issue 的作者。
type Counts struct {
	Commits                   int64 `json:"commits"`
	Additions                 int64 `json:"additions"`
	Deletions                 int64 `json:"deletions"`
	Changes                   int64 `json:"changes"`
	IssuesCreated             int64 `json:"issues_created"`
	IssuesAuthoredCompleted   int64 `json:"issues_authored_completed"`
	IssuesClosedUnknownReason int64 `json:"issues_closed_unknown_reason"`
	PRsCreated                int64 `json:"prs_created"`
	PRsMerged                 int64 `json:"prs_merged"`
}

func (c *Counts) add(other Counts) {
	c.Commits += other.Commits
	c.Additions += other.Additions
	c.Deletions += other.Deletions
	c.Changes += other.Changes
	c.IssuesCreated += other.IssuesCreated
	c.IssuesAuthoredCompleted += other.IssuesAuthoredCompleted
	c.IssuesClosedUnknownReason += other.IssuesClosedUnknownReason
	c.PRsCreated += other.PRsCreated
	c.PRsMerged += other.PRsMerged
}

// UserStats 保存一个 GitHub 账号或未关联的 Git 作者的贡献。
type UserStats struct {
	Identity string `json:"identity"`
	Login    string `json:"login,omitempty"`
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	Kind     string `json:"kind"`
	Bot      bool   `json:"bot"`
	Counts
}

func (u UserStats) label() string {
	if u.Login != "" {
		return u.Login
	}
	if u.Email != "" {
		return u.Email
	}
	return u.Identity
}

// Report 保存过滤后的用户统计、总计及采集元数据。
type Report struct {
	Repository    string      `json:"repository"`
	DefaultBranch string      `json:"default_branch"`
	HeadSHA       string      `json:"head_sha"`
	CommitSource  string      `json:"commit_source"`
	LocalPath     string      `json:"local_path,omitempty"`
	SourceRef     string      `json:"source_ref,omitempty"`
	StartedAt     time.Time   `json:"started_at"`
	CollectedAt   time.Time   `json:"collected_at"`
	Scope         string      `json:"scope"`
	Rules         []string    `json:"rules"`
	Sort          string      `json:"sort"`
	ExcludeBots   bool        `json:"exclude_bots"`
	Users         []UserStats `json:"users"`
	Totals        Counts      `json:"totals"`
}

func statisticsRules() []string {
	return []string{
		"提交取固定 HEAD 可达的默认分支全历史，仅计主作者，排除 merge 和空提交",
		"新增和删除包含文档、配置、锁文件等文本；变更行数为新增行数与删除行数之和，二进制变更不计文本行数",
		"Issue 按作者归属，已完成仅计当前 closed 且 state_reason=completed；原因缺失单列，不代表解决者贡献",
		"PR 按作者归属，完成仅计已合并；Issue 与 PR 分别计数，不归并任务",
		"GitHub 身份按稳定 ID 聚合；未关联作者按邮箱聚合，无邮箱按提交单列；缺失账号统一列为不可识别",
		"Issue 和 PR 反映采集期间可访问的当前状态，不是历史事件统计或事务快照",
	}
}

type accumulator struct {
	users map[string]*UserStats
}

func newAccumulator() *accumulator {
	return &accumulator{users: make(map[string]*UserStats)}
}

func (a *accumulator) user(identity UserStats) *UserStats {
	u, ok := a.users[identity.Identity]
	if !ok {
		u = &identity
		a.users[identity.Identity] = u
	}
	// REST 的账号类型补充 GitActor.user 未提供的 Bot 信息。
	u.Bot = u.Bot || identity.Bot
	if identity.Login != "" {
		u.Login = identity.Login
	}
	return u
}

func githubIdentity(id, login string) UserStats {
	return UserStats{Identity: "github:" + id, Login: login, Kind: "github", Bot: strings.HasSuffix(strings.ToLower(login), "[bot]")}
}

func restIdentity(user *gh.User) UserStats {
	if user == nil {
		return UserStats{Identity: "unavailable", Kind: "unavailable"}
	}
	id := user.GetNodeID()
	if id == "" {
		// REST 通常提供 node_id；不把缺失身份的不同账号按空 ID 合并。
		if user.GetID() == 0 {
			return UserStats{Identity: "unavailable", Kind: "unavailable"}
		}
		id = "database:" + strconv.FormatInt(user.GetID(), 10)
	}
	u := githubIdentity(id, user.GetLogin())
	u.Bot = u.Bot || strings.EqualFold(user.GetType(), "Bot")
	return u
}

func (a *accumulator) addCommit(commit github.CommitActivity) error {
	if commit.Merge || commit.Empty {
		return nil
	}
	if commit.Additions < 0 || commit.Deletions < 0 {
		return fmt.Errorf("提交 %s 的变更行数无效", commit.SHA)
	}
	var identity UserStats
	if user := commit.Author.User; user != nil && user.ID != "" {
		identity = githubIdentity(user.ID, user.Login)
	} else {
		email := strings.ToLower(strings.TrimSpace(commit.Author.Email))
		key := "git:" + email
		if email == "" {
			key = "git-commit:" + commit.SHA
		}
		identity = UserStats{Identity: key, Email: email, Name: commit.Author.Name, Kind: "git"}
	}
	u := a.user(identity)
	u.Commits++
	u.Additions += commit.Additions
	u.Deletions += commit.Deletions
	u.Changes += commit.Additions + commit.Deletions
	return nil
}

func (a *accumulator) addIssue(issue *gh.Issue) {
	if issue.IsPullRequest() {
		return
	}
	u := a.user(restIdentity(issue.GetUser()))
	u.IssuesCreated++
	if issue.GetState() == "closed" {
		switch issue.GetStateReason() {
		case "completed":
			u.IssuesAuthoredCompleted++
		case "":
			u.IssuesClosedUnknownReason++
		}
	}
}

func (a *accumulator) addPR(pr *gh.PullRequest) {
	u := a.user(restIdentity(pr.GetUser()))
	u.PRsCreated++
	if !pr.GetMergedAt().IsZero() {
		u.PRsMerged++
	}
}

func (a *accumulator) finish(report *Report) {
	report.Users = make([]UserStats, 0, len(a.users))
	for _, u := range a.users {
		if report.ExcludeBots && u.Bot {
			continue
		}
		report.Users = append(report.Users, *u)
		report.Totals.add(u.Counts)
	}
	metric := func(u UserStats) int64 {
		switch report.Sort {
		case "changes":
			return u.Changes
		case "issues":
			return u.IssuesCreated
		case "issues-completed":
			return u.IssuesAuthoredCompleted
		case "prs":
			return u.PRsCreated
		case "prs-merged":
			return u.PRsMerged
		case "user":
			return 0
		default:
			return u.Commits
		}
	}
	sort.Slice(report.Users, func(i, j int) bool {
		a, b := report.Users[i], report.Users[j]
		if metric(a) != metric(b) {
			return metric(a) > metric(b)
		}
		if strings.ToLower(a.label()) != strings.ToLower(b.label()) {
			return strings.ToLower(a.label()) < strings.ToLower(b.label())
		}
		return a.Identity < b.Identity
	})
}
