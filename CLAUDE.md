# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
make build          # go build with ldflags -o gh-report .
make run            # build + run with config.local.yaml
make install        # go install with ldflags
make clean          # rm gh-report
go vet ./...        # lint check
```

No tests exist yet. No linter config (e.g. golangci-lint) is set up.

## Architecture

CLI tool that fetches GitHub activity (Issues, PRs, Comments, Reviews, Projects v2) and outputs structured reports.

**Data flow**: `main.go` → `cmd.Execute()` (Cobra CLI) → `report.Collect` (API calls, with mpb progress bars) → output formatter (csv/summary/ai)

**Packages**:
- `main` — Entry point, calls `cmd.Execute()`
- `cmd` — Cobra CLI commands (`root.go`: root command + flags + config loading + mpb progress bars; `version.go`: version subcommand)
- `github` — GitHub REST API (via `go-github/v69`) + custom GraphQL client for Projects v2
- `report` — Data collection (`collector.go` with `Progress` interface for UI feedback), CSV output (`printer.go`), summary/prompt generation (`summary.go`)
- `anthropic` — Lightweight Anthropic Messages API client (raw `net/http`, no SDK)

**Concurrency**: `report.Collect` uses three-level concurrency — org Projects vs repo data in parallel (separate WaitGroups), 4 API calls per repo in parallel, PR reviews in parallel. Progress is reported via the `Progress` interface (implemented by `cmd.progressBars` using mpb).

**Config resolution priority**: CLI flag > YAML config file > environment variable > hardcoded default

**Key env vars**: `GITHUB_TOKEN`, `ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL`

**Detailed docs**:
- [docs/report-rules.md](docs/report-rules.md) — 报告业务规则（今日工作/明日计划的纳入排除规则、状态映射）
- [docs/report-generation.md](docs/report-generation.md) — 报告生成技术文档（数据流、并发模型、过滤层次）

## Conventions

- All comments and documentation are in **Chinese**; code identifiers are in English
- Exported functions/types must have Go-style doc comments starting with the identifier name (in Chinese)
- Commit messages follow simplified Angular convention: `feat`, `fix`, `refactor`, `chore`, `docs`, `test`, `perf` — written in Chinese
- `config.local.yaml` is git-ignored for local development; `config.example.yaml` is the tracked template
