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
report.Collect()  ─── 并发获取数据 + 可选任务关联 ──→ []RepoReport
    ↓
根据输出格式分发：
├─ csv     → report.Print()              → CSV 分段输出
├─ summary → report.PrintSummaryData()   → 结构化文本 + Prompt 模板
└─ ai      → report.BuildSummaryPrompt() → AI API → 报告文本
```

## 时间参数

### 两个关键时间基准

| 名称 | 计算方式 | 用途 |
|------|----------|------|
| `since` | `now - days` | 控制 **数据获取范围**，传给 GitHub API |
| `cutoff` | 由报告类型决定 | 控制 **工作条目过滤**，在 summary 层使用 |

### 报告类型与 cutoff 计算

| 报告类型 | 子命令 | 默认 days | cutoff 计算 |
|----------|--------|-----------|-------------|
| 日报 | `daily`（默认） | 1 | 当天 00:00:00 |
| 周报 | `weekly` | 14 | 本周一 00:00:00 |
| 月报 | `monthly` | 60 | 本月一号 00:00:00 |
| 年报 | `yearly` | 730 | 今年一月一号 00:00:00 |

`days` 参数决定从 GitHub API 拉取多少天的数据。但无论 `days` 设为多少，工作条目始终只展示 **cutoff 之后** 的活动。`days` 较大时的作用是为计划条目提供更完整的上下文（如当前迭代的 Project Items）。

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

### Issue–PR 关联（Summary / AI）

CLI 在 `format=summary` 时设置 `Options.IncludeTaskLinks`；CSV 不增加关联查询。

Projects 关联到仓库后，`collectTaskLinks` 为已采集的 PR，以及当前迭代中通过计划过滤的 Project PR，调用 `github.ListClosingIssues`。同仓库同编号只查询一次，各请求复用现有 HTTP 并发限制。完成此阶段后才推进仓库进度的最后一步。

- GraphQL 字段：`pullRequest.closingIssuesReferences(first: 100, after: cursor, excludeUserLinked: false)`。
- 获取每个 Issue 的仓库全名、编号、标题、URL、状态，按游标读取全部分页。
- 任意一页失败、PR 不可访问或关联数据不完整：丢弃该 PR 的全部关联结果，向 stderr 输出警告，报告继续保留独立条目；上下文取消则返回错误。
- 关联结果存入 `RepoReport.PullRequestIssues`，不改写原始 Issues、PullRequests 或 Projects。

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
    PullRequestIssues map[int][]github.LinkedIssue     // PR → 明确关联 Issue
}
```

## 报告生成 — Summary 模式

### 工作条目 (extractWorkItems)

以报告类型对应的 **cutoff** 时间为基准，从 collector 已过滤的数据中进一步筛选工作条目：

#### PR 纳入规则 (prWorkedSince)

```
PR 纳入工作条目的条件（满足任一）：
├─ 状态为 open/draft → 视为进行中的工作，始终纳入
├─ created_at >= cutoff
├─ merged_at >= cutoff
└─ closed_at >= cutoff

即：已合并或已关闭的 PR 仅在报告时间范围内操作时纳入。
```

#### Issue 纳入规则 (issueWorkedSince)

```
Issue 纳入工作条目的条件（满足任一）：
├─ 状态为 open → 视为进行中的工作，始终纳入
├─ created_at >= cutoff
└─ closed_at >= cutoff

即：已关闭的 Issue 仅在报告时间范围内关闭时纳入。
```

#### Issue 评论纳入规则

```
评论纳入工作条目的条件（全部满足）：
├─ 评论者是指定用户
├─ created_at >= cutoff（只保留时间范围内的评论）
├─ 用户不是该 Issue/PR 的作者（用户自己 PR 上的评论不单独列出）
└─ 同一 Issue 只记一条（按 Issue 分组去重）
```

#### Review 评论纳入规则

```
Review 纳入工作条目的条件（全部满足）：
├─ 评论者是指定用户
├─ created_at >= cutoff（只保留时间范围内的 review）
├─ 用户不是该 PR 的作者（自己 PR 上的 review 不单独列出）
└─ 同一 PR 只记一条（按 PR 分组去重）
```

#### Assignee 过滤

- Issue 有 Assignees 但不包含当前用户时跳过（属于别人的任务）
- 无 Assignees 的 Issue 不受此规则影响

### 计划条目 (extractPlanItems)

计划条目不按当天日期过滤，而是基于**任务状态**和**迭代归属**：

#### 来源 1：未完成的 PR

```
PR 纳入计划条目的条件（全部满足）：
├─ 用户是 PR 作者
├─ PR 状态不是 merged 或 closed
├─ （如有 Assignees）包含当前用户
└─ 按 owner/repo#number 去重
```

#### 来源 2：当前迭代中未完成的项目

```
Project Item 纳入计划条目的条件（全部满足）：
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
| Issue 评论 | `owner/repo#issue_number` | 同一 Issue 在时间范围内只记一条 |
| Review 评论 | `owner/repo#pr_number` | 同一 PR 在时间范围内只记一条 |
| 计划条目 | `owner/repo#number` | open_pr 与 project_item 合并去重 |

### 任务归并层（report/tasks.go）

`extractWorkItems` 和 `extractPlanItems` 仍负责活动筛选；筛选后分别经过 `groupTaskItems` / `groupPlanItems`，生成 `reportTask`（任务标题 + 多个 `WorkItem` 来源）。Summary 和 AI Prompt 使用同一份归并和格式化逻辑。

以不区分大小写的完整 `owner/repo#number` 标识 Issue 任务。归并只沿 PR→Issue 单向关系进行，不用连通分量把共享 PR 的多个 Issue 合成一个任务。每个任务内同一来源只保留一次；没有关系的 PR、Issue、评论和 Review 保持独立。

输入中已有的 Issue 保留原始活动日期和状态。关联 Issue 不在筛选结果中时，以 `ContextOnly=true` 补充背景，不携带活动日期，也不增加本期贡献。未通过筛选的 PR 不作为来源补入。

`formatWorkData` 为每个来源直接输出完整 URL 并分别展示状态；多 PR 任务保留实现标题和 Review 摘要，非日报保留各来源日期。`formatPlanData` 保留全部链接，但不输出状态或优先级。URL 保留完整仓库路径和评论锚点，与相邻文字用空格分隔，不添加 Markdown 包装。

Project Item 的归属按 URL 中的完整仓库路径匹配，避免仓库名前缀相同导致误归并。

### 用户自身 PR 评论去重

`prAuthorKeys` 收集用户作为作者的**所有 PR 编号**（不限日期），确保用户在自己 PR 上的评论和 review 不会单独列为"讨论"条目。只有在**他人 PR** 上的评论才会作为 comment/review 类型列出。

## 报告生成 — CSV 模式

CSV 模式 (`report.Print`) 直接输出 collector 层的数据，不做 cutoff 过滤。分为五个段：

| 段 | 列 |
|----|----|
| Issues | Repo, Number, Title, State, User, Date |
| Pull Requests | Repo, Number, Title, State, User, Date, Reviews |
| Issue Comments | Repo, Issue Number, User, Date, Body(截断80字符) |
| Review Comments | Repo, PR Number, User, Date, Path, Body(截断80字符) |
| Project Items | Repo, Project, Iteration, Category, Number, Title, State, Status |

空段自动跳过不输出。

## 报告生成 — AI 模式

AI 模式在 Summary 模式基础上，将结构化数据 + Prompt 模板发送给 AI API（支持 Anthropic Claude 和 OpenAI）：

```
Prompt 结构：
├─ 指令模板（日期范围、用户、报告类型对应的输出格式要求）
├─ 工作任务数据（归并后的 reportTask 列表，含所有来源链接和状态）
└─ 计划任务数据（归并后的 reportTask 列表，含所有来源链接）
```

Prompt 模板中的标题和角色根据报告类型自动调整：
- 日报：工作日报助手、今日工作、明日计划
- 周报：工作周报助手、本周工作、下周计划
- 月报：工作月报助手、本月工作、下月计划
- 年报：年度总结助手、年度工作、下年计划

Prompt 要求每个任务一行、保留全部完整 URL 并直接展示，不使用 Markdown 链接、尖括号或反引号包装，不按相似标题再次归并，不把关联背景算成本期工作，也不从 PR 合并推断发布或验收完成。

AI API 调用参数：
- Anthropic 端点：`POST {base_url}/v1/messages`
- OpenAI 端点：`POST {base_url}/v1/chat/completions`

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
│     工作条目               │              │     计划条目               │
│  时间基准: cutoff          │              │  不按日期过滤              │
│  (按报告类型计算)          │              │  来源 1: open/draft PR    │
│  PR: open 纳入 /           │              │  来源 2: 当前迭代未完成项  │
│      merged/closed >=cutoff│              │  去重合并两个来源          │
│  Issue: open 纳入 /        │              └──────────────────────────┘
│         closed >=cutoff    │
│  评论: created_at >= cutoff│
│  Review: created_at >=cutoff│
│  去重: 自身 PR 评论不列出   │
└──────────────────────────┘
```
