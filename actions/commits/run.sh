#!/usr/bin/env bash
set -euo pipefail

: "${GH_REPORT_ACTION_PATH:?}"
: "${GITHUB_WORKSPACE:?}"
: "${RUNNER_TEMP:?}"
: "${GITHUB_OUTPUT:?}"

chart="${GH_REPORT_CHART:-true}"
avatars="${GH_REPORT_AVATARS:-true}"
for value in "$chart" "$avatars"; do
  case "$value" in
    true|false) ;;
    *) echo 'chart 和 github-avatars 只能为 true 或 false。' >&2; exit 1 ;;
  esac
done

target="${GH_REPORT_PATH:-.}"
if [[ "$target" != /* ]]; then
  target="$GITHUB_WORKSPACE/$target"
fi
target="$(cd "$target" && pwd -P)"
ref="${GH_REPORT_REF:-HEAD}"
sha="$(GIT_NO_REPLACE_OBJECTS=1 GIT_NO_LAZY_FETCH=1 git -C "$target" rev-parse --verify --end-of-options "${ref}^{commit}")"

directory="$(mktemp -d "$RUNNER_TEMP/gh-report-commits.XXXXXX")"
mkdir "$directory/report"
(
  # 从 Action 自身源码构建，避免使用调用仓库的 go.mod 或 go.work。
  cd "$GH_REPORT_ACTION_PATH/../.."
  GOWORK=off go build -o "$directory/gh-report" .
)

args=(commits --path "$target" --ref "$sha" --sort "${GH_REPORT_SORT:-commits}" -f markdown)
"$directory/gh-report" "${args[@]}" -o "$directory/report/summary.md"
if [[ "$chart" == true ]]; then
  avatar_args=("--github-avatars=$avatars")
  while IFS= read -r entry || [[ -n "$entry" ]]; do
    entry="${entry%$'\r'}"
    [[ -z "$entry" ]] || avatar_args+=(--avatar-user "$entry")
  done <<< "${GH_REPORT_AVATAR_USERS:-}"
  "$directory/gh-report" "${args[@]}" --chart "${avatar_args[@]}" -o "$directory/report/report.md"
else
  if [[ -n "${GH_REPORT_AVATAR_USERS:-}" ]]; then
    echo 'avatar-users 需要 chart=true。' >&2
    exit 1
  fi
  cp "$directory/report/summary.md" "$directory/report/report.md"
fi

# 仅在全部报告成功后发布路径，不把部分统计作为成功结果。
printf 'directory=%s\nhead-sha=%s\n' "$directory" "$sha" >> "$GITHUB_OUTPUT"
