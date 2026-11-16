# 仓库贡献统计

`contributors` 统计单个仓库所有可识别参与者的贡献，默认输出终端表格。它独立于日报、周报等工作报告，不调用 AI，也不采集评论、Review 或 Projects。

仅需本地提交数及增删行数、不需要 Issue / PR 时，可使用完全离线、无需 Token 的 [`commits`](commits.md) 命令。`contributors --local` 仍保留联网的完整贡献统计。

## 使用方式

```bash
export GITHUB_TOKEN=你的令牌

# 默认分支全历史，按提交数降序
gh-report contributors -r owner/repo

# 按变更行数排序，排除 Bot
gh-report contributors -r owner/repo --sort changes --exclude-bots

# 导出 CSV / JSON
gh-report contributors -r owner/repo -f csv > contributors.csv
gh-report contributors -r owner/repo -f json > contributors.json

# 复用配置文件中的 token 和单个 repos 条目
gh-report contributors -c config.yaml -r owner/repo

# 在本地 Git 工作区中统计本地默认分支的 commits，仍联网获取 Issue / PR
gh-report contributors --local
gh-report contributors --local --sort changes -f json > contributors-local.json
```

Token 优先级沿用 `--token` > 配置文件 `token` > `GITHUB_TOKEN`。需有目标仓库读取权限；细粒度令牌需要仓库 Contents、Issues、Pull requests 的读取权限。此子命令不需要 Projects 权限。

| 参数 | 默认值 | 含义 |
|------|--------|------|
| `--repos` / `-r` | 配置文件 `repos` | 恰好一个 `owner/repo`，不接受仓库 URL 或多个仓库 |
| `--format` / `-f` | `table` | `table`、`csv`、`json` |
| `--sort` | `commits` | `commits`、`changes`、`issues`、`issues-completed`、`prs`、`prs-merged`、`user` |
| `--exclude-bots` | `false` | 排除 Bot 账号及用户名以 `[bot]` 结尾的参与者 |
| `--local` | `false` | 从当前 Git 工作区保存的默认分支读取提交；Issue / PR 仍联网获取 |

数字排序按降序；同值按用户标识升序，保证输出稳定。`user` 按用户名或未关联作者的邮箱升序。总计只包含过滤后展示的参与者。

配置文件只读取 `token` 和 `repos`；已有工作报告的 `days`、`user`、`format` 和 AI 配置不影响此子命令。`--local` 忽略配置文件的 `repos`，改用当前工作区 remote；显式 `-r` 必须匹配其中一个 GitHub remote。显式传入 `--days`、`--user` 或 AI 参数会报错，而不是静默忽略。统计固定为全历史、全部用户、默认分支，不支持时间范围或指定其他分支。

## 本地提交模式

`--local` 需要本机安装 Git，可从工作区根目录或子目录执行。它仅更换提交数据源，仍需 Token 和网络获取默认分支名称、Issue / PR 及主作者的 GitHub 身份。GitHub API 模式不需要本机 Git。

仓库自动识别规则：优先选择 GitHub `origin`；没有匹配的 `origin` 时，若所有可用 GitHub remote 指向同一仓库，则使用该仓库；存在多个不同仓库时需用 `-r owner/repo` 选择。显式 `-r` 必须与本地 remote 匹配，避免混用不同仓库的提交与 Issue / PR。目前支持 github.com 的 HTTPS、SSH 和 Git 协议 remote。

默认分支名称以 GitHub 当前信息为准。例如默认分支为 `main`：

1. 优先读取本地 `refs/heads/main`。
2. 没有该本地分支时，读取所选 remote 的 `refs/remotes/<remote>/main`。
3. 两者都不存在则报错，不改用当前检出的功能分支。

采集固定到所选 ref 当时的 SHA。默认分支上已提交但未推送的提交会计入；其他未合并分支、暂存区和工作区未提交修改不计入。命令不 checkout、不 fetch，不更新 refs 或工作区文件。本地跟踪分支可能落后于 GitHub；如需最新统计，应由使用者先更新本地历史。

浅克隆会报错，避免把截断历史当作全历史。部分克隆不允许隐式下载对象，缺少 diff 所需对象时会失败；请先补全本地历史与对象。GitHub 尚无默认分支且本地已有提交时也会报错，不能猜测应统计哪条分支；双方都为空时输出零提交，并继续统计 Issue / PR。

本地使用 `git log --numstat -z` 流式读取全部可达历史，排除 merge 和空提交，开启重命名识别并禁用外部 diff、textconv 和 replace objects。显式设置 `--ignore-submodules=none`，确保用户的 submodule 忽略配置不会导致提交或 gitlink 变更行数漏计。纯二进制、重命名和文件模式变更仍计为提交，文本行数可为零。NUL 分隔保证文件名中的换行和制表符不会破坏解析。

身份归并先按邮箱聚合本地计数，再为每个非空邮箱选一个较早的代表提交，批量查询 GitHub 主作者身份（每批最多 50 个），不会逐个下载提交或 diff。能确认身份时与该账号的 Issue / PR 计数合并；代表提交不在 GitHub 或没有关联账号时，保留 Git 作者身份，不凭名称猜测。仅有本地未推送提交的新作者可能无法关联到 GitHub 账号。

本地 Git 与 GitHub 的 diff 算法、重命名识别限制可能存在差异。对照两种模式时，应先确认使用相同 SHA；每份报告明确记录实际数据源与统计范围。

## 统计口径

| 表格列 | JSON / CSV 字段 | 口径 |
|--------|-----------------|------|
| `Commits` | `commits` | 固定默认分支 HEAD 可达的历史提交，按主作者归属，排除 merge 和空提交 |
| `+Lines` | `additions` | 纳入提交的新增文本行数之和 |
| `-Lines` | `deletions` | 纳入提交的删除文本行数之和 |
| `Changes` | `changes` | 新增行数加删除行数 |
| `Issues` | `issues_created` | 用户创建的 Issue 数，不含 PR |
| `IssuesDone` | `issues_authored_completed` | 该用户创建的 Issue 中，当前状态为 `closed` 且关闭原因为 `completed` 的数量 |
| `IssuesUnknown` | `issues_closed_unknown_reason` | 该用户创建的 Issue 中，已关闭但未返回关闭原因的数量 |
| `PRs` | `prs_created` | 用户创建的全部 PR 数，包含 draft 和未合并关闭的 PR |
| `PRsMerged` | `prs_merged` | 用户创建的 PR 中，`merged_at` 非空的数量 |

Issue 完成数表示“作者提出的问题中已有多少完成”，不表示作者解决了这些问题。关闭操作人、Assignee 和关联 PR 的作者不用于重新分配此计数。`not_planned`、`duplicate` 和关闭原因缺失的 Issue 均不计入已完成；重开后的 Issue 按当前状态统计。PR 的合并操作人不获得作者的 PR 贡献，未合并关闭不算完成。

Issue 与 PR 是独立维度：一个 Issue 关联多个 PR 时，各自计数，不使用工作报告中的任务归并逻辑，也不相加为“任务总数”。这些数量和行数不合成为综合贡献分。

代码行数包含文档、配置、锁文件和生成文件等文本，不代表纯业务代码量。重复修改同一行会重复计数，不是当前文件归属或净增行数。API 模式采用 GitHub 的计算结果，`--local` 采用本地 Git 的 numstat；交叉核对需开启重命名识别（如 `git diff-tree -M`）。二进制提交计入提交数，但不产生文本行数；API 模式通过根树变化识别空提交，本地模式根据是否存在文件变更识别，避免误排纯二进制、重命名或文件模式变更。

Squash 后按默认分支上实际保留的提交统计，不恢复被压缩的原始提交。共同作者不重复分配主作者的整份变更量；merge commit 内独有的冲突解决变更随该 merge commit 一起排除。

## 身份与覆盖范围

- GitHub 账号使用稳定的全局 ID 聚合，提交、Issue、PR 三类参与者取并集；没有代码提交的 Issue / PR 作者也会展示。
- 未关联 GitHub 账号的 Git 作者按去除首尾空格、转小写的邮箱聚合；同名但不同邮箱不合并。无邮箱时每个提交保留独立身份，避免将同名作者误认为同一人。
- 已关联账号和未关联邮箱之间不猜测关联；无法识别的 Issue / PR 作者归入 `unavailable`。GitHub 的 ghost 等替代账号无法还原被删除账号的原始归属。
- Bot 判定依据账号类型 `Bot` 或 GitHub 用户名后缀 `[bot]`；无法关联账号的 Git 作者不能可靠判定是否为 Bot。
- 不限制前 100 名。范围包含所选默认分支 SHA 可达的提交和当前账号可访问的 Issue / PR；不包含仅存在于其他未合并分支的提交。API 模式无法读取已删除或不可访问的远程对象，本地模式以本地保留的默认分支历史为准。

## 输出与失败行为

表格展示仓库、默认分支、完整 HEAD SHA、采集起止时间、排序、Bot 过滤条件和统计口径，并附总计。

JSON / CSV 的 `commit_source` 为 `github_api` 或 `local_git`；本地模式另含 `local_path` 和 `source_ref`，表格也展示本地工作区及实际 ref。`head_sha` 在本地模式中表示所选本地 ref 的 SHA，不是远程实时 HEAD。

JSON 包含同样的元数据、`rules`、`users` 和 `totals`。无参与者时 `users` 为 `[]`。

CSV 第一行为字段名；每个参与者一行，`record_type=user`；末尾附 `record_type=total` 的总计行。各行重复携带仓库、HEAD、采集时间、过滤条件和口径，因此空仓库也有元数据。再次汇总 CSV 时应筛选 `record_type=user`，避免重复计入总计行。

采集提示和错误只写 stderr，报告写 stdout。任意提交、Issue 或 PR 分页失败时返回非零退出码，不输出部分报告。重定向时 shell 仍可能创建空文件，调用方应同时检查退出码。输出设备写入失败也会返回错误。支持 Ctrl-C 取消请求。

提交历史固定到采集开始时的 HEAD SHA。Issue 和 PR 逐页读取采集期间的当前状态，不是事务快照；期间的创建、状态变更或删除可能影响结果。全历史报告中的“已完成”是当前状态数量，不是历史关闭事件次数。

## 实现与性能

1. API 模式通过 GraphQL 获取默认分支和 HEAD SHA；本地模式先定位工作区、匹配 GitHub remote 并检查浅克隆，再用 API 确认默认分支名称。
2. API 模式通过 GraphQL `Commit.history` 每页读取 50 个提交，同时获取主作者、增删行数、根树和父提交信息；本地模式固定本地默认分支 SHA，流式读取 Git numstat，再批量关联主作者。双方都没有提交时仍获取 Issue / PR。
3. REST 分页获取 `state=all` 的全部 Issue 和 PR，每页最多 100 条；Issue 排除 PR，分别按对象 ID 去重。不继承工作报告的近期时间过滤。
4. 全部成功后过滤 Bot、排序、计算总计并输出。

API 模式请求量约为提交数除以 50 加上 Issue / PR 列表页数；本地模式的提交网络请求仅用于按邮箱代表提交批量关联身份，不下载提交历史或 diff。本地 Git 计算时间仍随历史规模和 diff 大小增长。复用现有认证、并发上限和请求重试机制，不新增 SDK；仅 `--local` 依赖本机 Git。当前 GraphQL 包装仅对请求传输错误重试，服务端计算错误和 HTTP 错误会中止报告，可稍后重跑。

没有使用 Contributors Stats API 作为代码数据源：该接口对达到 10,000 commits 的仓库将新增、删除行数返回为 0，且仅提供按周聚合的统计。

参考：[GitHub Contributors 规则](https://docs.github.com/en/repositories/viewing-activity-and-data-for-your-repository/viewing-a-projects-contributors)、[统计 API 限制](https://docs.github.com/en/rest/metrics/statistics#get-all-contributor-commit-activity)、[GraphQL Commit 字段](https://docs.github.com/en/graphql/reference/commits#commit)。
