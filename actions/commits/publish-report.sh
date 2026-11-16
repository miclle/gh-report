#!/usr/bin/env bash
set -euo pipefail

: "${GH_REPORT_REPORT_DIRECTORY:?}"
: "${GH_REPORT_REPORT_BRANCH:?}"
: "${GITHUB_SERVER_URL:?}"
: "${GITHUB_REPOSITORY:?}"
: "${GITHUB_OUTPUT:?}"
: "${RUNNER_TEMP:?}"

branch="$GH_REPORT_REPORT_BRANCH"
name="${GH_REPORT_ARTIFACT_NAME:-commit-report}"
case "$name" in
  .|..|*/*|*\\*|*$'\n'*|*$'\r'*) echo '报告名称必须是单个目录名。' >&2; exit 1 ;;
esac
test -f "$GH_REPORT_REPORT_DIRECTORY/report.md"
git check-ref-format "refs/heads/$branch"
directory="$(mktemp -d "$RUNNER_TEMP/gh-report-publish.XXXXXX")"
trap 'rm -rf "$directory"' EXIT

# 隔离调用仓库及本机 Git 配置；凭据只在子进程环境中传递，不写入配置或 URL。
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 GIT_TERMINAL_PROMPT=0
export GIT_NO_REPLACE_OBJECTS=1
export GIT_CONFIG_COUNT=1
export GIT_CONFIG_KEY_0="http.${GITHUB_SERVER_URL}/.extraheader"
export GIT_CONFIG_VALUE_0=''
if [[ -n "${GH_REPORT_GITHUB_TOKEN:-}" ]]; then
  GIT_CONFIG_VALUE_0="AUTHORIZATION: basic $(printf 'x-access-token:%s' "$GH_REPORT_GITHUB_TOKEN" | base64 | tr -d '\n')"
fi

git init -q "$directory"
git -C "$directory" config user.name 'github-actions[bot]'
git -C "$directory" config user.email '41898282+github-actions[bot]@users.noreply.github.com'
git -C "$directory" remote add origin "${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}.git"
image_path=''
if [[ -n "${GH_REPORT_CHART_FILE:-}" ]]; then
  digest="$(git -C "$directory" hash-object --no-filters "$GH_REPORT_CHART_FILE")"
  image_path="charts/$digest.svg"
fi
report_path="reports/$name"
marker='gh-report report branch v1'

# 对报告名称逐字节编码，确保空格、中文等合法 Artifact 名称可用于 GitHub URL。
url_component() {
  local value="$1" encoded='' character code index LC_ALL=C
  for ((index=0; index<${#value}; index++)); do
    character="${value:index:1}"
    case "$character" in
      [a-zA-Z0-9.~_-]) encoded+="$character" ;;
      *) printf -v code '%%%02X' "'$character"; encoded+="$code" ;;
    esac
  done
  printf '%s' "$encoded"
}

output_image() {
  if [[ -n "$image_path" ]]; then
    printf 'chart-image-url=%s/%s/raw/%s/%s\n' "$GITHUB_SERVER_URL" "$GITHUB_REPOSITORY" "$1" "$image_path" >> "$GITHUB_OUTPUT"
  fi
}

for attempt in 1 2 3; do
  head="$(git -C "$directory" ls-remote --heads origin "refs/heads/$branch")"
  if [[ -n "$head" ]]; then
    git -C "$directory" fetch -q --depth=1 origin "refs/heads/$branch"
    git -C "$directory" checkout -q -B report FETCH_HEAD
    if [[ ! -f "$directory/.gh-report" ]] || [[ "$(cat "$directory/.gh-report")" != "$marker" ]]; then
      echo '目标分支不是 gh-report 报告分支，拒绝写入。' >&2
      exit 1
    fi
  elif [[ "${GH_REPORT_READ_ONLY:-false}" == true ]]; then
    echo 'PR 验证不创建报告分支；请先在受信任的工作流中发布图表。' >&2
    exit 1
  else
    # 新分支从空树开始，不复制源仓库历史、代码或凭据。
    git -C "$directory" checkout -q --orphan "report-$attempt"
    printf '%s\n' "$marker" > "$directory/.gh-report"
    printf '# gh-report\n\n[查看完整报告](reports/)\n\nreports/ 中各目录展示最新的 Markdown、数据表格和 SVG 报告，历史版本保留在 Git 提交中。Summary 引用固定提交 SHA 下的图表和完整报告，不随后续发布变化。\n' > "$directory/README.md"
  fi

  if [[ -L "$directory/charts" ]] || [[ -L "$directory/reports" ]] || [[ -L "$directory/$report_path" ]] || [[ -n "$image_path" && -L "$directory/$image_path" ]]; then
    echo '报告分支的发布路径不能是符号链接。' >&2
    exit 1
  fi
  if [[ -n "$image_path" && -f "$directory/$image_path" ]]; then
    if ! cmp -s "$GH_REPORT_CHART_FILE" "$directory/$image_path"; then
      echo '报告分支中的图表内容与指纹不符，拒绝复用。' >&2
      exit 1
    fi
  fi
  if [[ "${GH_REPORT_READ_ONLY:-false}" == true ]]; then
    if [[ -n "$image_path" && -f "$directory/$image_path" ]]; then
      output_image "$(git -C "$directory" rev-parse HEAD)"
      exit 0
    fi
    echo 'PR 验证只复用已发布的图表；本次报告不写入分支，保留 Artifact 入口。' >&2
    exit 1
  fi

  if [[ -n "$image_path" ]]; then
    mkdir -p "$directory/charts"
    cp "$GH_REPORT_CHART_FILE" "$directory/$image_path"
    git -C "$directory" add -- "$image_path"
  fi
  # 只替换当前名称的报告；其他任务的报告及历史 SVG 保留。
  rm -rf "${directory:?}/${report_path:?}"
  mkdir -p "$directory/$report_path"
  cp -R "$GH_REPORT_REPORT_DIRECTORY/." "$directory/$report_path/"
  cp "$GH_REPORT_REPORT_DIRECTORY/report.md" "$directory/$report_path/README.md"
  git -C "$directory" add -- .gh-report README.md "$report_path"
  if ! git -C "$directory" diff --cached --quiet; then
    git -C "$directory" -c commit.gpgsign=false commit -q -s -m 'docs(report): 发布完整提交贡献报告'
    # 普通 push 保留并发任务的结果；冲突后重新读取远端，最多尝试三次。
    if ! git -C "$directory" push -q origin "HEAD:refs/heads/$branch"; then
      if [[ "$attempt" == 3 ]]; then
        echo '报告分支发布失败，保留 Artifact 入口。' >&2
        exit 1
      fi
      continue
    fi
  fi

  commit="$(git -C "$directory" rev-parse HEAD)"
  output_image "$commit"
  printf 'report-url=%s/%s/blob/%s/reports/%s/README.md\n' "$GITHUB_SERVER_URL" "$GITHUB_REPOSITORY" "$commit" "$(url_component "$name")" >> "$GITHUB_OUTPUT"
  exit 0
done
