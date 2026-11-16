#!/usr/bin/env bash
set -euo pipefail

: "${GH_REPORT_DIRECTORY:?}"
: "${GH_REPORT_HEAD_SHA:?}"
: "${GH_REPORT_ARTIFACT_URL:?}"
: "${GITHUB_STEP_SUMMARY:?}"

source_file="$GH_REPORT_DIRECTORY/report/summary.md"
summary_file="$GH_REPORT_DIRECTORY/job-summary.md"
footer_file="$GH_REPORT_DIRECTORY/footer.md"
printf '\n[下载完整报告（图表及数据表格）](%s)\n' "$GH_REPORT_ARTIFACT_URL" > "$footer_file"

# 为每个 step 的 1 MiB 限制预留空间，超限时仍保留全量总计和下载链接。
if (( $(wc -c < "$source_file") + $(wc -c < "$footer_file") < 1000000 )); then
  cat "$source_file" "$footer_file" > "$summary_file"
else
  {
    printf '# 提交统计\n\n- HEAD：%s\n\n' "$GH_REPORT_HEAD_SHA"
    printf '完整报告超过 Job Summary 大小限制，下面仅展示全量总计；请下载完整报告查看全部作者及图表。\n\n'
    printf '| User | Name | Commits | +Lines | -Lines | Changes |\n'
    printf '| :--- | :--- | ---: | ---: | ---: | ---: |\n'
    awk '/^\| TOTAL \|/ { print }' "$source_file"
    cat "$footer_file"
  } > "$summary_file"
fi
cat "$summary_file" >> "$GITHUB_STEP_SUMMARY"
