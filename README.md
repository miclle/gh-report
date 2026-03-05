# gh-report

GitHub 仓库活动报告生成工具。通过 GitHub API 获取指定仓库（支持多个）的 Issue、Pull Request、评论、Review 以及 Projects v2 迭代信息，生成结构化的工作报告。支持日报、周报、月报、年报四种报告类型，提供 CSV 原始数据输出和 Summary 模式，并可通过 AI API（支持 Anthropic Claude 和 OpenAI）直接生成报告。

## 功能特性

- **多种报告类型** — 日报、周报、月报、年报，通过子命令切换
- **多仓库支持** — 一次运行可追踪多个仓库的活动
- **Issue 和 Pull Request** — 展示状态标签（`open`、`closed`、`merged`、`draft`）
- **评论汇总** — Issue 评论和 PR Review 评论，附内容预览
- **Review 摘要** — 每个 PR 的审查人及审查状态
- **Projects v2 迭代** — 展示当前迭代和下一迭代中的工作项
- **用户过滤** — 可选仅展示指定用户的活动
- **配置文件** — 支持 YAML 配置文件，避免重复输入参数
- **多种输出格式**：
  - `csv`（默认）— CSV 分段格式，展示原始活动数据
  - `summary` — 结构化的工作数据与计划数据及 Prompt 模板
- **AI 报告生成** — 通过 AI API（支持 Anthropic Claude 和 OpenAI）将活动数据自动整理为工作报告
- **友好的终端体验** — 彩色错误提示、多仓库并发进度条、Shell 补全支持

## 环境要求

- Go 1.25+
- GitHub Personal Access Token，所需权限：
  - `repo` — 访问仓库数据（Issue、PR、评论、Review）
  - `read:org` — 访问组织信息
  - `read:project` — 访问 GitHub Projects v2 数据（可选，缺少此权限时工具会打印警告并继续运行）

## 安装

```bash
go install github.com/miclle/gh-report@latest
```

或从源码构建：

```bash
git clone https://github.com/miclle/gh-report.git
cd gh-report
make build
```

## 配置

### GitHub Token

工具需要 GitHub Personal Access Token，按以下优先级解析：

1. `--token` 命令行参数
2. 配置文件中的 `token` 字段
3. `GITHUB_TOKEN` 环境变量

### AI API Key（可选，用于 AI 报告生成）

当使用 `-f summary --ai` 模式时，需要 AI API Key，按以下优先级解析：

1. `--ai-key` 命令行参数
2. 配置文件中的 `ai_key` 字段
3. `--anthropic-key` 命令行参数（已废弃）
4. 配置文件中的 `anthropic_key` 字段（已废弃）
5. 按 provider 查环境变量（`ANTHROPIC_API_KEY` 或 `OPENAI_API_KEY`）
6. `AI_API_KEY` 环境变量

AI API Base URL 按以下优先级解析（可选，各 provider 有默认值）：

1. `--ai-base-url` 命令行参数
2. 配置文件中的 `ai_base_url` 字段
3. `--anthropic-base-url` 命令行参数（已废弃）
4. 配置文件中的 `anthropic_base_url` 字段（已废弃）
5. 按 provider 查环境变量（`ANTHROPIC_BASE_URL` 或 `OPENAI_BASE_URL`）

### 配置文件

创建 YAML 配置文件以避免每次手动传参。参考 [config.example.yaml](config.example.yaml)：

```yaml
# GitHub Token（也可通过 GITHUB_TOKEN 环境变量设置）
# token: ghp_xxx

# 需要追踪的仓库列表
repos:
  - own/repo1
  - own/repo2

# 查看最近几天的活动（默认按报告类型：日报 1 天、周报 14 天、月报 60 天、年报 730 天）
days: 14

# 按用户过滤（可选，注释掉则显示所有用户）
# user: own

# 输出格式: csv（默认）或 summary
# format: summary

# 是否调用 AI API 直接生成报告（需配合 format: summary 使用）
# ai: true

# AI 服务提供商: anthropic（默认）或 openai
# ai_provider: anthropic

# AI API Key（也可通过 ANTHROPIC_API_KEY / OPENAI_API_KEY / AI_API_KEY 环境变量设置）
# ai_key: sk-xxx

# AI API Base URL（也可通过 ANTHROPIC_BASE_URL / OPENAI_BASE_URL 环境变量设置）
# ai_base_url: https://api.anthropic.com

# AI 模型名（默认: anthropic 为 claude-sonnet-4-20250514，openai 为 gpt-4o）
# model: claude-sonnet-4-20250514
```

### 命令行参数

| 参数 | 短选项 | 说明 | 默认值 |
|------|--------|------|--------|
| `--config` | `-c` | YAML 配置文件路径 | — |
| `--repos` | `-r` | 逗号分隔的仓库列表（`owner/repo` 格式） | — |
| `--days` | `-d` | 查看最近几天的活动 | 按报告类型 |
| `--user` | `-u` | 按 GitHub 用户名过滤 | —（显示所有用户） |
| `--token` | | GitHub Personal Access Token | — |
| `--format` | `-f` | 输出格式：`csv`（默认）或 `summary` | `csv` |
| `--ai` | | 调用 AI API 直接生成报告 | `false` |
| `--ai-provider` | | AI 服务提供商：`anthropic`（默认）或 `openai` | `anthropic` |
| `--ai-key` | | AI API Key | — |
| `--ai-base-url` | | AI API Base URL | — |
| `--model` | | AI 模型名 | 按 provider 默认 |
| `--anthropic-key` | | Anthropic API Key（已废弃，请使用 `--ai-key`） | — |
| `--anthropic-base-url` | | Anthropic API Base URL（已废弃，请使用 `--ai-base-url`） | — |

命令行参数优先级高于配置文件中的值。

## 使用方式

### 报告类型

通过子命令指定报告类型，不指定子命令时默认为日报：

```bash
gh-report -c config.yaml                # 日报（默认）
gh-report daily -c config.yaml          # 日报（显式指定）
gh-report weekly -c config.yaml         # 周报（默认拉取最近 14 天数据）
gh-report monthly -c config.yaml        # 月报（默认拉取最近 60 天数据）
gh-report yearly -c config.yaml         # 年报（默认拉取最近 730 天数据）
```

各报告类型的默认拉取天数可通过 `-d` 参数覆盖：

```bash
gh-report weekly -c config.yaml -d 21   # 周报，拉取最近 21 天数据
```

### 通过 Make 运行

```bash
# 使用默认配置（config.local.yaml）
make run

# 覆盖配置文件
make run CONFIG=config.yaml
```

### 通过配置文件运行

```bash
gh-report -c config.yaml
```

### 通过命令行参数运行

```bash
# 单仓库，最近 1 天
export GITHUB_TOKEN=ghp_xxx
gh-report -r own/repo1

# 多仓库，最近 7 天，按用户过滤
gh-report -r own/repo1,own/repo2 -d 7 -u alice

# 直接传入 Token
gh-report -r own/repo1 -d 3 --token ghp_xxx
```

### 配置文件 + 命令行参数混合使用

命令行参数会覆盖配置文件中的对应值：

```bash
gh-report -c config.yaml -d 7 -u alice
```

### Summary 模式

输出结构化的工作数据和计划数据，以及 Prompt 模板：

```bash
gh-report -c config.yaml -f summary              # 日报 summary
gh-report weekly -c config.yaml -f summary        # 周报 summary
```

### AI 报告生成

通过 AI API 直接生成工作报告（支持 Anthropic Claude 和 OpenAI）：

```bash
# 使用 Anthropic Claude（默认）生成日报
export ANTHROPIC_API_KEY=sk-ant-xxx
gh-report -c config.yaml -f summary --ai

# 生成周报
gh-report weekly -c config.yaml -f summary --ai

# 指定模型
gh-report -c config.yaml -f summary --ai --model claude-sonnet-4-20250514

# 使用 OpenAI
export OPENAI_API_KEY=sk-xxx
gh-report -c config.yaml -f summary --ai --ai-provider openai

# 使用 OpenAI 并指定模型
gh-report -c config.yaml -f summary --ai --ai-provider openai --ai-key sk-xxx --model gpt-4o
```

### 版本信息

```bash
gh-report version
```

### Shell 补全

生成 Shell 补全脚本：

```bash
# Bash
gh-report completion bash > /etc/bash_completion.d/gh-report

# Zsh
gh-report completion zsh > "${fpath[1]}/_gh-report"

# Fish
gh-report completion fish > ~/.config/fish/completions/gh-report.fish

# PowerShell
gh-report completion powershell | Out-String | Invoke-Expression
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

输出结构化的工作和计划数据，标题根据报告类型自动调整（日报为"今日工作/明日计划"，周报为"本周工作/下周计划"，以此类推）：

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
gh-report/
├── main.go                 # CLI 入口
├── config.example.yaml     # 配置文件示例
├── Makefile                # 构建和运行脚本
├── cmd/
│   ├── root.go             # 根命令定义、flags 注册、主逻辑
│   ├── daily.go            # daily 子命令
│   ├── weekly.go           # weekly 子命令
│   ├── monthly.go          # monthly 子命令
│   ├── yearly.go           # yearly 子命令
│   └── version.go          # version 子命令
├── ai/
│   ├── ai.go               # AI 客户端统一接口、工厂函数
│   ├── anthropic.go         # Anthropic Claude API 实现
│   └── openai.go            # OpenAI Chat Completions API 实现
├── github/
│   ├── client.go           # GitHub API 客户端（go-github REST + GraphQL）
│   ├── types.go            # Projects v2 相关数据结构
│   ├── issues.go           # Issue 和 Issue 评论获取
│   ├── pulls.go            # PR、Review、Review 评论获取
│   └── projects.go         # Projects v2 GraphQL 查询（迭代信息）
├── report/
│   ├── collector.go        # 按仓库收集和聚合数据
│   ├── printer.go          # CSV 格式化输出
│   └── summary.go          # Summary 模式（工作条目 + 计划条目 + Prompt）
└── docs/
    ├── report-rules.md     # 报告业务规则
    └── report-generation.md # 报告生成技术文档
```

详细的报告生成规则和技术文档请参考：
- [报告业务规则](docs/report-rules.md) — 报告类型、工作/计划条目的纳入排除规则、状态映射
- [报告生成技术文档](docs/report-generation.md) — 数据流、并发模型、过滤层次

## 许可证

[MIT](LICENSE)
