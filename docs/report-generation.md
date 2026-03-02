# 报告生成逻辑

本文档详细说明 gh-report 从数据获取到报告输出的完整流程，包括过滤规则、并发模型和输出格式。

## 整体数据流

```
CLI 输入 (flags / YAML 配置 / 环境变量)
    ↓
配置解析与合并（CLI > YAML > 环境变量 > 默认值）
    ↓
创建 GitHub 客户端 + 初始化进度条
    ↓
report.Collect()  ─── 并发获取数据 ──→ []RepoReport
    ↓
根据输出格式分发：
├─ csv     → report.Print()              → CSV 分段输出
├─ summary → report.PrintSummaryData()   → 结构化文本 + Prompt 模板
└─ ai      → report.BuildSummaryPrompt() → Claude API → 日报文本
```

## 时间参数

### 两个关键时间基准

| 名称 | 计算方式 | 用途 |
|------|----------|------|
| `since` | `now - days` | 控制 **数据获取范围**，传给 GitHub API |
| `today` | 当天 00:00:00 | 控制 **今日工作过滤**，在 summary 层使用 |

`days` 参数决定从 GitHub API 拉取多少天的数据，默认值为 1。但无论 `days` 设为多少，"今日工作"始终只展示**当天**的活动。`days` 较大时的作用是为"明日计划"提供更完整的上下文（如当前迭代的 Project Items）。

## 数据获取 (report.Collect)

### 三层并发模型

```
第一层：组织级并发
├─ [WaitGroup A] 各组织的 Projects v2  ──→ orgProjects map
└─ [WaitGroup B] 各仓库的活动数据      ──→ reports slice
    │
    ├─ 第二层：仓库内 4 个 API 并发
    │  ├─ ListIssues(since)
    │  ├─ ListPullRequests(since)
    │  ├─ ListIssueComments(since)
    │  └─ ListReviewComments(since)
    │
    └─ 第三层：PR Review 并发
       └─ 对每个 PR 并发调用 ListReviews()
```

HTTP 并发数由 `--concurrency` 控制（默认 8），通过 `semaphoreTransport` 限流。

### GitHub API 调用细节

#### Issues (github/issues.go)

- 接口：`Issues.ListByRepo()`
- 参数：`State: "all"`, `Since: since`, `Sort: "updated"`
- 行为：返回 `updated_at >= since` 的所有 Issue（包括被 bot 更新的）
- 注意：结果包含 PR（GitHub API 特性），collector 通过 `IsPullRequest()` 排除

#### Pull Requests (github/pulls.go)

- 接口：`PullRequests.List()`
- 参数：`State: "all"`, `Sort: "updated"`, `Direction: "desc"`
- 行为：按更新时间降序排列，遇到 `updated_at < since` 的 PR 即停止翻页（早退优化）
- 注意：PR API 不支持 `Since` 参数，靠客户端判断截断

#### Issue 评论 (github/issues.go)

- 接口：`Issues.ListComments()`
- 参数：`Since: &since`, `Sort: "updated"`
- 行为：返回 `updated_at >= since` 的评论

#### Review 评论 (github/pulls.go)

- 接口：`PullRequests.ListComments()`
- 参数：`Since: since`, `Sort: "updated"`
- 行为：返回 `updated_at >= since` 的 review 评论

#### PR Reviews (github/pulls.go)

- 接口：`PullRequests.ListReviews()`
- 行为：返回指定 PR 的所有 review（无时间过滤）

#### Projects v2 (github/projects.go)

- 接口：自定义 GraphQL 查询
- 两阶段获取：
  1. 获取组织下所有项目及迭代元数据
  2. 对每个项目并发获取所有 item（标题、编号、URL、状态、迭代、Assignees）
- 无时间过滤，获取全量数据

### Collector 层过滤

获取原始数据后，collector 做第一轮过滤：

#### 用户过滤

当指定 `--user` 时：
- Issues：只保留用户创建的
- PRs：只保留用户创建的
- 评论：只保留用户发表的
- Review 评论：只保留用户发表的

#### PR 活动过滤 (prHasActivitySince)

避免因 GitHub 自动更新 `updated_at`（如 bot 评论、标签变更）导致旧 PR 被误收录：

```
PR 纳入条件（满足任一）：
├─ 状态为 open（进行中的工作，明日计划需要）
├─ created_at >= since
├─ merged_at >= since
└─ closed_at >= since
```

### 数据结构

```go
RepoReport {
    Owner          string                             // 仓库所有者
    Repo           string                             // 仓库名称
    Issues         []*gh.Issue                        // 过滤后的 Issue 列表
    PullRequests   []*gh.PullRequest                  // 过滤后的 PR 列表
    IssueComments  []*gh.IssueComment                 // 过滤后的 Issue 评论
    ReviewComments []*gh.PullRequestComment            // 过滤后的 Review 评论
    Reviews        map[int][]*gh.PullRequestReview    // PR 编号 → Review 列表
    Projects       []github.Project                   // 关联的 Projects v2
}
```

## 报告生成 — Summary 模式

### 今日工作 (extractWorkItems)

以**当天零点**为基准，从 collector 已过滤的数据中进一步筛选今天的工作：

#### PR 纳入规则 (prWorkedSince)

```
PR 纳入今日工作的条件（满足任一）：
├─ 状态为 open/draft → 视为进行中的工作，始终纳入
├─ created_at >= today
├─ merged_at >= today
└─ closed_at >= today

即：已合并或已关闭的 PR 仅在当天操作时纳入，避免旧 PR 出现在今日工作中。
```

#### Issue 纳入规则 (issueWorkedSince)

```
Issue 纳入今日工作的条件（满足任一）：
├─ 状态为 open → 视为进行中的工作，始终纳入
├─ created_at >= today
└─ closed_at >= today

即：已关闭的 Issue 仅在当天关闭时纳入。
```

#### Issue 评论纳入规则

```
评论纳入今日工作的条件（全部满足）：
├─ 评论者是指定用户
├─ created_at >= today（只保留今天的评论）
├─ 用户不是该 Issue/PR 的作者（用户自己 PR 上的评论不单独列出）
└─ 同一 Issue 只记一条（按 Issue 分组去重）
```

#### Review 评论纳入规则

```
Review 纳入今日工作的条件（全部满足）：
├─ 评论者是指定用户
├─ created_at >= today（只保留今天的 review）
├─ 用户不是该 PR 的作者（自己 PR 上的 review 不单独列出）
└─ 同一 PR 只记一条（按 PR 分组去重）
```

#### Assignee 过滤

- Issue 有 Assignees 但不包含当前用户时跳过（属于别人的任务）
- 无 Assignees 的 Issue 不受此规则影响

### 明日计划 (extractPlanItems)

明日计划不按当天日期过滤，而是基于**任务状态**和**迭代归属**：

#### 来源 1：未完成的 PR

```
PR 纳入明日计划的条件（全部满足）：
├─ 用户是 PR 作者
├─ PR 状态不是 merged 或 closed
├─ （如有 Assignees）包含当前用户
└─ 按 owner/repo#number 去重
```

#### 来源 2：当前迭代中未完成的项目

```
Project Item 纳入明日计划的条件（全部满足）：
├─ 属于当前迭代（通过 FindRelevantIterations 确定）
├─ Item URL 匹配当前仓库
├─ Assignees 包含当前用户
├─ 状态不是 DONE、CLOSED 或 MERGED
└─ 按 owner/repo#number 去重（与来源 1 合并）
```

如果某个 Item 已从来源 1（open PR）添加，则从来源 2 补充其 `status` 信息。

#### 迭代分类 (ClassifyIteration)

```
判断逻辑（基于 time.Now()）：
├─ today < start_date       → Next（下一迭代）
├─ today >= end_date         → Previous（上一迭代）
└─ start_date <= today < end → Current（当前迭代）
```

`FindRelevantIterations` 从所有迭代中找出最近的 Previous、Current 和 Next。

### 去重策略

| 类型 | 去重键 | 规则 |
|------|--------|------|
| Issue 评论 | `owner/repo#issue_number` | 同一 Issue 每天只记一条 |
| Review 评论 | `owner/repo#pr_number` | 同一 PR 每天只记一条 |
| 明日计划 | `owner/repo#number` | open_pr 与 project_item 合并去重 |

### 用户自身 PR 评论去重

`prAuthorKeys` 收集用户作为作者的**所有 PR 编号**（不限日期），确保用户在自己 PR 上的评论和 review 不会单独列为"讨论"条目。只有在**他人 PR** 上的评论才会作为 comment/review 类型列出。

## 报告生成 — CSV 模式

CSV 模式 (`report.Print`) 直接输出 collector 层的数据，不做 today 过滤。分为五个段：

| 段 | 列 |
|----|----|
| Issues | Repo, Number, Title, State, User, Date |
| Pull Requests | Repo, Number, Title, State, User, Date, Reviews |
| Issue Comments | Repo, Issue Number, User, Date, Body(截断80字符) |
| Review Comments | Repo, PR Number, User, Date, Path, Body(截断80字符) |
| Project Items | Repo, Project, Iteration, Category, Number, Title, State, Status |

空段自动跳过不输出。

## 报告生成 — AI 模式

AI 模式在 Summary 模式基础上，将结构化数据 + Prompt 模板发送给 Claude API：

```
Prompt 结构：
├─ 指令模板（日期范围、用户、输出格式要求）
├─ 今日工作数据（格式化的 WorkItem 列表）
└─ 明日计划数据（格式化的 PlanItem 列表）
```

Claude API 调用参数：
- 端点：`POST {base_url}/v1/messages`
- 模型：默认 `claude-sonnet-4-20250514`，可通过 `--model` 覆盖
- `max_tokens`: 4096

## 进度条

每个仓库一个进度条，总步数动态计算：

```
总步数 = 4 (Issues + PRs + Comments + Reviews)
       + N (每个 PR 的 Review 获取)
       + 1 (完成步)
       + 1 (Projects 关联步)

初始默认 6 步（4 + 2），获取到 PR 列表后通过 SetTotal 更新。
```

## 过滤层次总结

```
                      ┌─────────────────────────────────────┐
                      │        GitHub API 层                 │
                      │  Since 参数: now - days              │
                      │  返回 updated_at >= since 的数据     │
                      └──────────────┬──────────────────────┘
                                     ↓
                      ┌─────────────────────────────────────┐
                      │        Collector 层                  │
                      │  用户过滤: 按 author 匹配            │
                      │  PR 活动过滤: prHasActivitySince     │
                      │  (open 放行 / created/merged/closed) │
                      └──────────────┬──────────────────────┘
                                     ↓
              ┌──────────────────────┴──────────────────────┐
              ↓                                             ↓
┌──────────────────────────┐              ┌──────────────────────────┐
│     今日工作               │              │     明日计划               │
│  时间基准: today 零点       │              │  不按日期过滤              │
│  PR: open 纳入 /           │              │  来源 1: open/draft PR    │
│      merged/closed 当天    │              │  来源 2: 当前迭代未完成项  │
│  Issue: open 纳入 /        │              │  去重合并两个来源          │
│         closed 当天        │              └──────────────────────────┘
│  评论: created_at >= today │
│  Review: created_at >= today│
│  去重: 自身 PR 评论不列出   │
└──────────────────────────┘
```
