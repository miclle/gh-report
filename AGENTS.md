# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## Build & Run

```bash
make build          # go build with ldflags -o gh-report .
make run            # build + run with config.local.yaml
make install        # go install with ldflags
make clean          # rm gh-report
go vet ./...        # lint check
```

使用 `go test ./...` 运行测试，使用 `go test -race ./...` 检测竞态。尚未配置 golangci-lint 等独立 linter。

## Architecture

CLI tool that fetches GitHub activity (Issues, PRs, Comments, Reviews, Projects v2) and outputs structured reports.

**Data flow**: `main.go` → `cmd.Execute()` (Cobra CLI) → `report.Collect` (API calls, with mpb progress bars) → output formatter (csv/summary/ai)

**Packages**:
- `main` — Entry point, calls `cmd.Execute()`
- `cmd` — Cobra CLI commands (`root.go`: root command + flags + config loading + mpb progress bars; `daily.go`/`weekly.go`/`monthly.go`/`yearly.go`: 报告类型子命令; `version.go`: version subcommand)
- `ai` — 统一 AI 客户端接口，支持 Anthropic Codex 和 OpenAI（手工 HTTP 实现，无 SDK 依赖）
- `github` — GitHub REST API (via `go-github/v69`) + custom GraphQL client for Projects v2
- `report` — Data collection (`collector.go` with `Progress` interface for UI feedback), CSV output (`printer.go`), summary/prompt generation (`summary.go`)
- `contribution` — 单仓库默认分支全历史贡献采集、身份聚合和 Table / CSV / JSON 输出，由 `contributors` 子命令调用；`commits` 子命令复用本地解析器，完全离线按邮箱统计提交和增删行数，不读取配置或 Token；`--local` 从本地 Git 读取 commits，Issue / PR 及身份关联仍走 API，独立于工作报告过滤与任务归并

**Concurrency**: `report.Collect` uses three-level concurrency — org Projects vs repo data in parallel (separate WaitGroups), 4 API calls per repo in parallel, PR reviews in parallel. Progress is reported via the `Progress` interface (implemented by `cmd.progressBars` using mpb).

**Config resolution priority**: CLI flag > YAML config file > environment variable > hardcoded default

**Key env vars**: `GITHUB_TOKEN`, `AI_API_KEY`, `ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL`, `OPENAI_API_KEY`, `OPENAI_BASE_URL`

**Detailed docs**:
- [docs/report-rules.md](docs/report-rules.md) — 报告业务规则（报告类型、工作/计划条目的纳入排除规则、状态映射）
- [docs/report-generation.md](docs/report-generation.md) — 报告生成技术文档（数据流、并发模型、过滤层次）
- [docs/contributors.md](docs/contributors.md) — 仓库贡献统计口径、身份归并、输出及失败行为
- [docs/commits.md](docs/commits.md) — 完全离线的提交统计、默认 ref 选择和作者邮箱聚合

## Conventions

- All comments and documentation are in **Chinese**; code identifiers are in English
- Exported functions/types must have Go-style doc comments starting with the identifier name (in Chinese)
- Commit messages follow simplified Angular convention: `feat`, `fix`, `refactor`, `chore`, `docs`, `test`, `perf` — written in Chinese
- `config.local.yaml` is git-ignored for local development; `config.example.yaml` is the tracked template
