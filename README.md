# report

GitHub 仓库活动报告生成工具。通过 GitHub API 获取指定仓库（支持多个）的 Issue、Pull Request、评论、Review 以及 Projects v2 迭代信息，生成结构化的工作报告。支持 CSV 原始数据输出和 Summary 日报模式，并可通过 Claude API 直接生成"今日工作 + 明日计划"格式的日报。

## 功能特性

- **多仓库支持** — 一次运行可追踪多个仓库的活动
- **Issue 和 Pull Request** — 展示状态标签（`open`、`closed`、`merged`、`draft`）
- **评论汇总** — Issue 评论和 PR Review 评论，附内容预览
- **Review 摘要** — 每个 PR 的审查人及审查状态
- **Projects v2 迭代** — 展示当前迭代和下一迭代中的工作项
- **用户过滤** — 可选仅展示指定用户的活动
- **配置文件** — 支持 YAML 配置文件，避免重复输入参数
- **多种输出格式**：
  - `csv`（默认）— CSV 分段格式，展示原始活动数据
  - `summary` — 结构化的"今日工作 + 明日计划"数据及 Prompt 模板
- **AI 日报生成** — 通过 Claude API 将活动数据自动整理为工作日报

## 环境要求

- Go 1.25+
- GitHub Personal Access Token，所需权限：
  - `repo` — 访问仓库数据（Issue、PR、评论、Review）
  - `read:org` — 访问组织信息
  - `read:project` — 访问 GitHub Projects v2 数据（可选，缺少此权限时工具会打印警告并继续运行）

## 安装

```bash
go install github.com/miclle/report@latest
```

或从源码构建：

```bash
git clone https://github.com/miclle/report.git
cd report
make build
```

## 配置

### GitHub Token

工具需要 GitHub Personal Access Token，按以下优先级解析：

1. `-token` 命令行参数
2. 配置文件中的 `token` 字段
3. `GITHUB_TOKEN` 环境变量

### Anthropic API Key（可选，用于 AI 日报生成）

当使用 `-format summary -ai` 模式时，需要 Anthropic API Key，按以下优先级解析：

1. `-anthropic-key` 命令行参数
2. 配置文件中的 `anthropic_key` 字段
3. `ANTHROPIC_API_KEY` 环境变量

Anthropic API Base URL 按以下优先级解析（可选，默认为 `https://api.anthropic.com`）：

1. `-anthropic-base-url` 命令行参数
2. 配置文件中的 `anthropic_base_url` 字段
3. `ANTHROPIC_BASE_URL` 环境变量

### 配置文件

创建 YAML 配置文件以避免每次手动传参。参考 [config.example.yaml](config.example.yaml)：

```yaml
# GitHub Token（也可通过 GITHUB_TOKEN 环境变量设置）
# token: ghp_xxx

# 需要追踪的仓库列表
repos:
  - own/repo1
  - own/repo2

# 查看最近几天的活动（默认: 1）
days: 14

# 按用户过滤（可选，注释掉则显示所有用户）
# user: own

# 输出格式: csv（默认）或 summary
# format: summary

# 是否调用 Claude API 直接生成日报（需配合 format: summary 使用）
# ai: true

# Anthropic API Key（也可通过 ANTHROPIC_API_KEY 环境变量设置）
# anthropic_key: sk-ant-xxx

# Anthropic API Base URL（也可通过 ANTHROPIC_BASE_URL 环境变量设置）
# anthropic_base_url: https://api.anthropic.com

# Claude 模型名（默认: claude-sonnet-4-20250514）
# model: claude-sonnet-4-20250514
```

### 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-config` | YAML 配置文件路径 | — |
| `-repos` | 逗号分隔的仓库列表（`owner/repo` 格式） | — |
| `-days` | 查看最近几天的活动 | `1` |
| `-user` | 按 GitHub 用户名过滤 | —（显示所有用户） |
| `-token` | GitHub Personal Access Token | — |
| `-format` | 输出格式：`csv`（默认）或 `summary` | `csv` |
| `-ai` | 调用 Claude API 直接生成日报 | `false` |
| `-anthropic-key` | Anthropic API Key | — |
| `-anthropic-base-url` | Anthropic API Base URL | `https://api.anthropic.com` |
| `-model` | Claude 模型名 | `claude-sonnet-4-20250514` |

命令行参数优先级高于配置文件中的值。

## 使用方式

### 通过 Make 运行

```bash
# 使用默认配置（config.local.yaml）
make run

# 覆盖配置文件
make run CONFIG=config.yaml
```

### 通过配置文件运行

```bash
./gh-report -config config.yaml
```

### 通过命令行参数运行

```bash
# 单仓库，最近 1 天
export GITHUB_TOKEN=ghp_xxx
./gh-report -repos own/repo1

# 多仓库，最近 7 天，按用户过滤
./gh-report -repos own/repo1,own/repo2 -days 7 -user alice

# 直接传入 Token
./gh-report -repos own/repo1 -days 3 -token ghp_xxx
```

### 配置文件 + 命令行参数混合使用

命令行参数会覆盖配置文件中的对应值：

```bash
./gh-report -config config.yaml -days 7 -user alice
```

### Summary 模式

输出结构化的"今日工作 + 明日计划"数据和 Prompt 模板：

```bash
./gh-report -config config.yaml -format summary
```

### AI 日报生成

通过 Claude API 直接生成工作日报：

```bash
export ANTHROPIC_API_KEY=sk-ant-xxx
./gh-report -config config.yaml -format summary -ai

# 指定模型
./gh-report -config config.yaml -format summary -ai -model claude-sonnet-4-20250514
```

## 输出示例

### CSV 模式（默认）

输出按类别分段的 CSV 数据，每段包含标题行、表头行和数据行：

```
Issues
Repo,Number,Title,State,User,Date
own/repo1,101,Bug: 登录失败,open,alice,2026-02-25
own/repo1,98,deploy(frontend): 沙箱管理,closed,alice,2026-02-25

Pull Requests
Repo,Number,Title,State,User,Date,Reviews
own/repo1,120,feat: 新增沙箱管理功能,merged,alice,2026-02-25,@bob APPROVED
own/repo1,122,fix: 修复连接超时问题,open,alice,2026-02-26,@bob COMMENTED

Project Items
Repo,Project,Iteration,Category,Number,Title,State,Status
own/repo1,Sprint Board,Sprint 2026-W09,Current,101,Bug: 登录失败,OPEN,In Progress
own/repo1,Sprint Board,Sprint 2026-W09,Current,98,deploy(frontend): 沙箱管理,CLOSED,Done
```

### Summary 模式

输出结构化的今日工作和明日计划数据，并附上可直接粘贴给 AI 的 Prompt：

```
========== 今日工作 ==========
- [PR] own/repo1#120 feat: 新增沙箱管理功能 | 状态: merged | Review: @bob APPROVED | https://...
- [PR] own/repo1#122 fix: 修复连接超时问题 | 状态: open | Review: @bob COMMENTED | https://...

========== 明日计划 ==========
- [open_pr] own/repo1#122 fix: 修复连接超时问题 | https://...
- [project_item] own/repo1#101 Bug: 登录失败 | Status: In Progress | https://...

========== Prompt（复制以下内容粘贴给 AI）==========
...
```

## 项目结构

```
report/
├── main.go                 # CLI 入口，参数解析与执行
├── config.go               # YAML 配置文件加载
├── config.example.yaml     # 配置文件示例
├── Makefile                # 构建和运行脚本
├── anthropic/
│   └── client.go           # Anthropic Messages API 客户端
├── github/
│   ├── client.go           # GitHub API 客户端（go-github REST + GraphQL）
│   ├── types.go            # Projects v2 相关数据结构
│   ├── issues.go           # Issue 和 Issue 评论获取
│   ├── pulls.go            # PR、Review、Review 评论获取
│   └── projects.go         # Projects v2 GraphQL 查询（迭代信息）
└── report/
    ├── collector.go        # 按仓库收集和聚合数据
    ├── printer.go          # CSV 格式化输出
    └── summary.go          # Summary 模式（今日工作 + 明日计划 + Prompt）
```

## 许可证

[MIT](LICENSE)
