# 离线提交统计

`gh-report commits` 只读取本地 Git 历史，统计每个主作者的提交数、新增、删除和变更行数。无需 Token，不读取 YAML 配置，不查询 GitHub API 或下载对象；支持没有 remote、非 GitHub remote 的 Git 工作区。

```bash
# 从工作区或其子目录执行，读取本地保存的默认分支历史
gh-report commits

# 缺少默认分支缓存时，显式指定范围
gh-report commits --ref main
gh-report commits --ref HEAD

# 指定工作区，按变更行数降序导出
gh-report commits --path /path/to/repo --ref main --sort changes -f csv > commits.csv
gh-report commits --ref main -f json > commits.json

# 导出 Markdown，数值列右对齐
gh-report commits --ref main -f markdown > commits.md
```

## 参数与范围

| 参数 | 默认值 | 含义 |
|------|--------|------|
| `--path` | `.` | 本地 Git 工作区或其子目录 |
| `--ref` | 从本地 `origin/HEAD` 选择 | 一个本地分支、标签、SHA 或可解析为单个提交的引用，例如 `HEAD` |
| `--format` / `-f` | `table` | `table`、`markdown`、`csv`、`json` |
| `--sort` | `commits` | `commits`、`changes` 数字降序，或 `user` 邮箱升序 |
| `--show-rules` | `false` | 在表格或 Markdown 末尾显示统计口径；CSV / JSON 始终保留口径元数据 |

未指定 `--ref` 时读取本地符号引用 `refs/remotes/origin/HEAD`，以它保存的默认分支名称优先选择本地 `refs/heads/<branch>`，缺失时使用 `refs/remotes/origin/<branch>`。这与 `contributors --local` 的本地分支优先级一致，但不查询 GitHub：缓存可能过期，命令不会刷新它。缺少缓存或对应分支时报错，提示显式指定 `--ref`，不改用当前检出的功能分支。不使用 `main`、`master` 等名称猜测默认分支。

显式 `--ref HEAD` 表示当前 HEAD；与默认分支可以不同。命令固定所选引用的 SHA，统计其可达全历史，不切换分支或修改 refs。该历史中的未推送提交会计入，工作区及暂存区未提交内容不计入。不存在或尚无提交的引用报错。

拒绝浅克隆；部分克隆缺少所需对象时失败，不允许 Git 隐式联网下载。使用者需要在执行前自行准备完整的本地历史。需要本机安装 Git。

继承的工作报告参数（如 `--config`、`--token`、`--repos`、`--days`、`--user`、AI 参数）若被显式传入会报错。环境中的 Token 不会被读取。

## 统计口径

- 按主作者计数，排除 merge 和没有文件变更的空提交；不分配共同作者贡献。
- 文本新增、删除包括代码、文档、配置、锁文件等；变更行数为新增加删除，不是净增行数或当前文件归属。
- 开启重命名识别，二进制、纯重命名及文件模式变更仍计提交，文本变更行数可为零。
- 显式关闭 submodule 差异过滤、外部 diff、textconv、replace objects，并固定 Myers diff 算法，复用 `contributors --local` 的 NUL 分隔流式解析器。
- 邮箱去除首尾空格并转小写后聚合，同邮箱不同名称合并；同名不同邮箱不合并。无邮箱则按提交保留独立身份。不应用 `.mailmap`。
- 不关联 GitHub 账号，不依据名称猜测身份，不过滤 Bot；报告中的参与者表示 Git 作者邮箱身份。

数字同值时按邮箱、身份升序，输出稳定。不同数据源的行数对照应先确认 SHA 相同；本地 Git 与 GitHub 的 diff 结果可能有差异。

## 输出与失败行为

表格、Markdown、CSV 和 JSON 仅包含提交维度，不输出未采集的 Issue / PR 字段。JSON 包含 `local_path`、`source_ref`、`head_sha`、起止时间、排序、统计口径、`users` 和 `totals`。各用户及总计都有 `commits`、`additions`、`deletions`、`changes`。

表格和 Markdown 默认隐藏口径说明；需要查看时使用 `gh-report commits --show-rules`（也可同时指定 `--ref`）。CSV / JSON 中的 `rules` 始终保留，便于导出后核对。

Markdown 使用标准表格，作者及名称列左对齐，Commits、+Lines、-Lines、Changes 数值列右对齐，末尾含总计行。重定向 stdout 即可保存为 `.md` 文件；可加 `--show-rules` 附带统计口径。

CSV 每行携带统计元数据，`record_type=user` 表示作者行，末尾 `record_type=total` 表示总计；再次汇总时应只读取作者行，避免重复相加。无参与者时 JSON `users=[]`，CSV 仍输出总计及元数据。

采集成功后才输出报告到 stdout，采集或输出失败返回非零退出码，错误写入 stderr。支持 Ctrl-C 取消。需要联网采集 Issue / PR 和 GitHub 身份关联时，使用 [`contributors`](contributors.md)。
