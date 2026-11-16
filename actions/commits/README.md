# 在 GitHub Actions 中展示提交统计

此 Composite Action 从自身源码构建 `gh-report`，读取调用仓库的完整 Git 历史，将完整 Markdown 和配套 SVG 保存到仓库的报告分支，在 workflow 的 Job Summary 内嵌贡献者卡片和逐周面积图，并上传 Markdown 报告和配套 SVG。无需对象存储、图床或额外 PAT，统计本身不调用 GitHub API。

## 接入

```yaml
name: Commit Report

on:
  workflow_dispatch:
  schedule:
    - cron: '0 3 * * 1'

permissions:
  contents: write

jobs:
  report:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
        with:
          ref: main
          fetch-depth: 0
          persist-credentials: false

      - uses: miclle/gh-report/actions/commits@main
        with:
          ref: HEAD
          report-branch: gh-report
```

将 checkout 的 `ref` 改为目标仓库的分支，例如 LAS 使用 `develop`。Action 默认统计 checkout 的 `HEAD`，不会推断默认分支或主动 fetch；在 PR workflow 中直接统计 `HEAD` 可能统计到 GitHub 生成的合并提交历史，因此全仓贡献报告建议使用独立的手动或定时 workflow。

示例使用 `@main`；正式使用时应固定为包含此 Action 的经过验证的完整提交 SHA。调用仓库须允许使用 `miclle/gh-report` 及其依赖的官方 Actions。

首版支持 GitHub.com 上具备 Bash、Git 和 Go 工具链下载网络的 Linux / macOS runner，使用 `actions/setup-go@v6` 按本项目的 `go.mod` 安装 Go，再从 **Action 自身目录**构建，不使用调用仓库的 Go 模块或工作区。自托管 runner 须满足官方 Actions 的 Node.js 24 运行要求。Artifact 使用 `actions/upload-artifact@v7`（Node.js 24），不支持 GitHub Enterprise Server。

## 输入

| 输入 | 默认值 | 含义 |
| :--- | :--- | :--- |
| `path` | `.` | 已 checkout 的仓库路径；相对路径基于 `github.workspace`，也可使用绝对路径 |
| `ref` | `HEAD` | 本地 Git 分支、标签或 SHA；报告生成前固定为单个 SHA |
| `sort` | `commits` | `commits`、`changes` 或 `user` |
| `chart` | `'true'` | 是否生成原版贡献者卡片和按周面积图 SVG，并在 Summary 提供原图入口 |
| `report-branch` | 空 | 完整报告分支，例如 `gh-report`；开启后在 Summary 内嵌图表并提供在线报告入口，需要 `contents: write`。留空时保留只读模式及 Artifact 链接 |
| `github-avatars` | `'true'` | 图表是否直接下载并嵌入 GitHub 头像；设为 `'false'` 可关闭，下载失败使用首字母 |
| `avatar-users` | 空 | 多行 `email=username` 头像映射；需要开启图表和头像 |
| `artifact-name` | `commit-report` | 完整报告 Artifact 名称；原图使用该名称加 `.svg`。同一 workflow 中所有 Artifact 名称须唯一，例如包含 matrix 值；不能包含路径分隔符或换行 |

```yaml
- uses: miclle/gh-report/actions/commits@main
  with:
    path: target-repo
    ref: HEAD
    sort: changes
    report-branch: gh-report
    avatar-users: |
      alice@example.com=alice
      bob@example.com=bob
    artifact-name: target-commit-report
```

`chart` 和 `github-avatars` 仅接受 `'true'` 或 `'false'`。默认不输出统计口径，统计范围和作者聚合规则沿用 [`commits`](../../docs/commits.md)。Action 固定 SHA 后只采集一次，Summary 和下载报告复用同一份数据。

## 展示与下载

- 图表使用原版 SVG：贡献者卡片、姓名 / 邮箱 / 头像、提交总数、增删行数和逐周面积图；所有卡片共享 UTC 时间范围及纵轴。
- 配置 `report-branch: gh-report` 后，Job Summary 顶部直接内嵌 SVG 卡片和面积图，保留“查看 SVG 原图”后备入口，下方展示全部作者的数据表格，数值列右对齐，包含总计、统计 SHA 和采集时间。`chart: 'false'` 时只展示数据表格及完整报告下载链接。
- SVG 原图以 `<artifact-name>.svg` 单独上传，使用 `archive: false`，无需下载 ZIP 后解压；它与完整报告中的 SVG 内容完全相同。
- 完整报告 Artifact 包含 `summary.md`、`report.md`，图表开启时另含 `report-<内容指纹>.svg`。下载解压后通过支持 Markdown 相对图片的查看器打开 `report.md`；提交到 GitHub 展示时需同时保留 SVG。
- Summary 使用 `https://github.com/<owner>/<repo>/raw/<commit-sha>/charts/<内容指纹>.svg` 作为图片地址，不使用内联 SVG、base64 或短期签名地址。图片仍是同一份原版 SVG，不另外生成柱状图。私有仓库的图片需要浏览器具有相应的 GitHub 登录权限。
- 超过 Summary 大小预算时仅展示全量总计，同时保留内嵌图表、原图和完整报告链接；不会将部分作者的计数作为全量结果。
- 两个 Artifact 均继承仓库访问控制和默认保留期；访问者需要登录并具有仓库读取权限，过期后链接失效。报告分支中的文件遵循仓库访问权限，不会随 Artifact 过期而删除。
- `reports/<artifact-name>/README.md` 在 GitHub 分支目录中直接展示完整报告，旁边保留原始 `report.md`、`summary.md` 和配套 SVG。Summary 的“在线查看完整报告”链接固定到本次发布的提交。

## 报告分支与权限

`gh-report` 是独立的报告发布分支。首次发布从空树创建，包含用途标记 `.gh-report`、说明、`charts/` 中的 SVG 和 `reports/` 中的完整报告，不复制源代码历史。已有分支必须带有匹配的用途标记，防止误写业务分支。图表以内容指纹命名，相同内容复用已有文件；`reports/<artifact-name>/` 展示该名称的最新完整报告，历史版本保留在 Git 提交中。每次报告更新追加普通提交，不强推，遇到并发冲突重新读取远端后重试。

每份 Summary 引用固定提交 SHA 下的 SVG 和完整报告，后续更新不会改变旧 Summary 对应的内容。不同报告名称使用独立目录，并发发布不会覆盖其他名称的报告。不要删除报告分支或重写其历史；报告图片及历史会持续累积。

启用报告分支需为 workflow 设置 `contents: write`，使用 Actions 自带的 `GITHUB_TOKEN`，无需配置额外 Secret。`pull_request` 事件只读取、复用已经发布的相同图表，不写入报告分支；缺少对应图表时，Summary 明确提示并保留 Artifact 入口。完整报告和新图表由受信任的手动、定时或 push workflow 发布；PR 不将本次完整报告写入分支，不向 fork PR 提供写凭据。

分支权限不足、规则限制或发布失败时，统计和 Artifact 仍保留，Summary 提示使用原图链接。若不需要内嵌图片，可省略 `report-branch` 并保留 `contents: read`。

## 输出

| 输出 | 含义 |
| :--- | :--- |
| `report-directory` | runner 上的报告目录，供后续步骤使用 |
| `head-sha` | 本次统计固定的 SHA |
| `artifact-url` | 完整报告的 Artifact 下载链接 |
| `chart-url` | SVG 原图的 Artifact 链接；关闭图表时为空 |
| `chart-image-url` | 固定提交 SHA 的内嵌 SVG 图片地址；未配置或发布失败时为空 |
| `report-url` | 固定提交 SHA 的完整报告在线查看地址；未发布时为空 |

目标仓库必须有完整历史：`fetch-depth: 0`，不设置部分克隆的 `filter`。浅克隆、缺少对象、无效引用、编译或统计失败都会使 Action 失败，不上传部分报告。下载工具链和 Go 模块、上传 Artifact 需要网络；Git 提交统计本身不联网，头像获取也无需 Token。私有仓库 checkout 使用 Actions 自带的 `GITHUB_TOKEN`，无需额外 PAT。
