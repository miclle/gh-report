# 离线提交统计

`gh-report commits` 只读取本地 Git 历史，统计每个主作者的提交数、新增、删除和变更行数。无需 Token，不读取 YAML 配置，普通报表不查询 GitHub API 或下载对象；支持没有 remote、非 GitHub remote 的 Git 工作区。图表模式默认联网获取并嵌入 GitHub 头像，使用 `--github-avatars=false` 可完全离线。

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

# 导出类似 Insights 的贡献者卡片和按周趋势（Markdown + SVG）
gh-report commits --ref main -f markdown --chart -o report.md

# 同一次采集另存用于 GitHub Job Summary 的数据表格
gh-report commits --ref main -f markdown --chart -o report.md --summary-output summary.md

# 完全离线导出图表，使用首字母头像
gh-report commits --ref main -f markdown --chart --github-avatars=false -o report.md

# 普通邮箱显式映射 GitHub 用户名，可重复指定
gh-report commits --ref main -f markdown --chart -o report.md \
  --avatar-user alice@example.com=alice \
  --avatar-user bob@example.com=bob
```

## 参数与范围

| 参数 | 默认值 | 含义 |
|------|--------|------|
| `--path` | `.` | 本地 Git 工作区或其子目录 |
| `--ref` | 从本地 `origin/HEAD` 选择 | 一个本地分支、标签、SHA 或可解析为单个提交的引用，例如 `HEAD` |
| `--format` / `-f` | `table` | `table`、`markdown`、`csv`、`json` |
| `--sort` | `commits` | `commits`、`changes` 数字降序，或 `user` 邮箱升序 |
| `--chart` | `false` | 用折叠区块同时展示贡献者图表和表格；需要 `-f markdown` 和 `--output` |
| `--github-avatars` | `true` | 仅图表模式联网获取并嵌入 GitHub 头像；设为 `false` 可完全离线，获取失败时保留首字母 |
| `--avatar-user` | 无 | `email=username` 映射，可重复指定；仅影响头像，需要图表模式且不能关闭头像获取 |
| `--output` / `-o` | stdout | 保存报告到指定文件；目标目录须已存在 |
| `--summary-output` | 无 | 另存 GitHub Job Summary 使用的 Markdown 数据表格；需要 `--chart`，不能与 `--output` 路径相同 |
| `--show-rules` | `false` | 在表格或 Markdown 末尾显示统计口径；CSV / JSON 始终保留口径元数据 |

未指定 `--ref` 时读取本地符号引用 `refs/remotes/origin/HEAD`，以它保存的默认分支名称优先选择本地 `refs/heads/<branch>`，缺失时使用 `refs/remotes/origin/<branch>`。这与 `contributors --local` 的本地分支优先级一致，但不查询 GitHub：缓存可能过期，命令不会刷新它。缺少缓存或对应分支时报错，提示显式指定 `--ref`，不改用当前检出的功能分支。不使用 `main`、`master` 等名称猜测默认分支。

显式 `--ref HEAD` 表示当前 HEAD；与默认分支可以不同。命令固定所选引用的 SHA，统计其可达全历史，不切换分支或修改 refs。该历史中的未推送提交会计入，工作区及暂存区未提交内容不计入。不存在或尚无提交的引用报错。

拒绝浅克隆；部分克隆缺少所需对象时失败，不允许 Git 隐式联网下载。使用者需要在执行前自行准备完整的本地历史。需要本机安装 Git。

继承的工作报告参数（如 `--config`、`--token`、`--repos`、`--days`、`--user`、AI 参数）若被显式传入会报错。所有模式均不读取环境中的 Token，也不调用 GitHub API。

## 统计口径

- 按主作者计数，排除 merge 和没有文件变更的空提交；不分配共同作者贡献。
- 文本新增、删除包括代码、文档、配置、锁文件等；变更行数为新增加删除，不是净增行数或当前文件归属。
- 开启重命名识别，二进制、纯重命名及文件模式变更仍计提交，文本变更行数可为零。
- 显式关闭 submodule 差异过滤、外部 diff、textconv、replace objects，并固定 Myers diff 算法，复用 `contributors --local` 的 NUL 分隔流式解析器。
- 邮箱去除首尾空格并转小写后聚合，同邮箱不同名称合并；同名不同邮箱不合并。无邮箱则按提交保留独立身份。不应用 `.mailmap`。
- 不按 GitHub 账号重新归并，不依据名称猜测身份，不过滤 Bot；报告中的参与者表示 Git 作者邮箱身份。可选头像获取只用于展示。

数字同值时按邮箱、身份升序，输出稳定。不同数据源的行数对照应先确认 SHA 相同；本地 Git 与 GitHub 的 diff 结果可能有差异。

## 输出与失败行为

表格、Markdown、CSV 和 JSON 仅包含提交维度，不输出未采集的 Issue / PR 字段。JSON 包含 `local_path`、`source_ref`、`head_sha`、起止时间、排序、统计口径、`users` 和 `totals`。各用户及总计都有 `commits`、`additions`、`deletions`、`changes`。

表格和 Markdown 默认隐藏口径说明；需要查看时使用 `gh-report commits --show-rules`（也可同时指定 `--ref`）。CSV / JSON 中的 `rules` 始终保留，便于导出后核对。

Markdown 使用标准表格，作者及名称列左对齐，Commits、+Lines、-Lines、Changes 数值列右对齐，末尾含总计行。重定向 stdout 即可保存为 `.md` 文件；可加 `--show-rules` 附带统计口径。

### 贡献者图表

`--chart` 输出每位贡献者的卡片，包含姓名、邮箱、提交总量、新增 / 删除行数和按周提交数面积图。按已有 `--sort` 顺序排列，展示全部贡献者；不包含时间范围选择条。无贡献者时展示空状态。

Markdown 使用 GitHub 支持的 `<details>` / `<summary>` 折叠区块，同时提供“贡献图表”和“数据表格”：默认展开图表、收起表格，可分别展开查看。表格包含相同参与者、总计和右对齐数值列；头部元数据及可选口径说明只输出一次。

活动时间使用提交的**作者时间**，转换为 UTC 后以周一 00:00 分周，沿用主作者身份聚合及 merge / 空提交排除规则。提交者时间不用于分周。所有卡片共享全历史时间范围和纵轴刻度，缺少活动的周按零绘制；只有一个活动周时，横轴前后各留一周空白。图表模式才额外保留周聚合数据，普通报表的采集与字段不变。

例如 `-o report.md` 会生成 `report.md` 和同目录的 `report-<内容指纹>.svg`。Markdown 通过相对路径引用 SVG，保存、提交到 GitHub 或分享时需带上这两个文件。SVG 无需 JavaScript、网络或外部字体下载。文件名包含内容指纹，确保新图表写入后、Markdown 保存失败时，旧报告引用的图片仍可用；重复导出产生的不再引用的旧 SVG 可自行清理。

图表模式需要指定 Markdown 文件路径，不能只重定向 stdout；其他格式及普通 Markdown 仍支持 stdout，也可通过 `--output` 保存。文件写入使用同目录临时文件后替换；失败会返回非零退出码，不覆盖尚未成功生成的新报告。

### GitHub Actions

使用 [`actions/commits`](../actions/commits/README.md) 可在其他仓库的 workflow 中运行统计：调用仓库先以 `fetch-depth: 0` checkout，Action 默认统计 `HEAD`。设置 `report-branch: gh-report` 和 `contents: write` 后，完整 Markdown 和配套 SVG 保存到报告分支，原版图表直接内嵌在 Job Summary 中；无需对象存储、图床或额外 PAT。

Summary 通过固定提交 SHA 的仓库图片地址展示原版贡献者卡片和逐周面积图，不依赖内联 SVG、base64、Artifact 相对路径或短期签名地址。新图表以内容指纹命名并追加提交，旧 Summary 保持原来的图表；相同图表复用已有文件。私有仓库的图片要求浏览器具有对应仓库的登录权限。`reports/<artifact-name>/` 直接展示最新完整报告，历史版本保留在 Git 提交中，Summary 也提供固定提交 SHA 的在线报告链接。报告分支内容会持续累积，不随 Artifact 过期而删除。

Action 同时保留独立 SVG 和完整报告的 Artifact 入口。PR 验证只复用已发布的相同图表；分支未配置、图表尚未发布或写入失败时，Summary 保留原图链接和数据表格。只需要 Artifact 入口时可省略 `report-branch`，继续使用 `contents: read`。

`--summary-output summary.md` 从同一份采集结果额外生成纯 Markdown 数据表格，不重复扫描历史；Action 发布图表后再补入内嵌图片和后备链接。下载报告中的 `report.md` 继续通过相对路径引用同目录 SVG，默认展开原版贡献图表并收起数据表格。超过 Summary 大小预算时保留全量总计、图表及链接。

### GitHub 头像

`--chart` 默认直接从 `avatars.githubusercontent.com` 下载并嵌入头像，不调用 GitHub API、不读取 Token，也不要求仓库存在 remote 或已推送。用 `--github-avatars=false` 关闭后完全离线。

头像账号按以下顺序确定：

1. 显式 `--avatar-user email=username` 映射优先，邮箱去除首尾空格并转小写；参数可重复，同一邮箱冲突映射时报错。
2. `数字ID+用户名@users.noreply.github.com` 使用数字 ID 对应的头像地址，避免账号更名后失效。
3. `用户名@users.noreply.github.com` 使用用户名对应的头像地址。
4. 普通邮箱未配置映射时保留首字母，不根据邮箱前缀、Git 作者姓名或仓库 owner 猜测账号，也不通过搜索或抓取页面反查邮箱。

例如你的作者邮箱是 `miclle.zheng@gmail.com`，可加 `--avatar-user miclle.zheng@gmail.com=miclle`。私有仓库、没有 remote 的仓库同样可用，因为头像获取与仓库访问权限无关；只需要能访问头像 CDN。

头像规范化为 80 × 80 的 PNG 后直接嵌入 SVG，无需增加头像文件。分享 Markdown 与 SVG 后，读者无需联网获取头像；图片是导出时的快照，不会随 GitHub 头像更新自动变化。同一图表中相同头像地址只下载一次；映射仅影响头像，不改变邮箱身份、姓名、排序和任何计数。

无可识别账号、图片不存在、网络失败或图片服务限流时自动使用首字母，继续输出完整本地统计；stderr 显示获取数量及首个下载错误的简短提示。失败的相同头像地址也会缓存，避免重复请求。头像下载不占用 GitHub REST API 的请求额度，但仍依赖图片服务可用性。Ctrl-C 会取消头像请求及本次导出。

CSV 每行携带统计元数据，`record_type=user` 表示作者行，末尾 `record_type=total` 表示总计；再次汇总时应只读取作者行，避免重复相加。无参与者时 JSON `users=[]`，CSV 仍输出总计及元数据。

采集成功后才输出报告到 stdout，采集或输出失败返回非零退出码，错误写入 stderr。支持 Ctrl-C 取消。需要联网采集 Issue / PR 和 GitHub 身份关联时，使用 [`contributors`](contributors.md)。
