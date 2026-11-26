#!/usr/bin/env bash
# Nightly API 延迟基线：验证「除大模型外主操作 ≤ 3 秒」。
#
# 口径（docs/project/12-ai-writing-design.md）：逐项记录**最大值**与**超时次数**，
# 最大值 > 预算或超时 > 0 即失败；P50 仅作参考，不作为判定依据。
#
# 只测确定性接口：`/latex/wordcount` 依赖外部编译服务（CLSI 服务已删除），故不纳入；
# `compile/browser` 只代表「提交耗时」，不冒充编译完成，也不纳入主操作判定。
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:5478}"
API="$BASE_URL/api/v1"
ACCOUNT="${PERF_ACCOUNT:-e2euser}"
PASSWORD="${PERF_PASSWORD:-e2epass1}"
PROJECT_ID="${PERF_PROJECT_ID:-22222222-2222-2222-2222-222222222222}"
SAMPLES="${PERF_SAMPLES:-20}"
BUDGET_MS="${PERF_BUDGET_MS:-3000}"
OUTPUT="${PERF_OUTPUT:-perf-results.json}"

# 前端提交前做一次 SHA-256，后端再 bcrypt；这里必须照做，否则登录 401。
password_hash="$(printf '%s' "$PASSWORD" | sha256sum | cut -d' ' -f1)"

token="$(curl -sf -X POST "$API/auth/login" \
  --form-string "account=$ACCOUNT" \
  --form-string "password=$password_hash" |
  python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["accessToken"])')"
if [[ -z "$token" ]]; then
  echo "::error::登录失败，无法建立性能基线"
  exit 1
fi

rows_file="$(mktemp)"
trap 'rm -f "$rows_file"' EXIT

# measure <名称> <方法> <URL> <none|json|form> [请求体]
measure() {
  local name="$1" method="$2" url="$3" mode="$4" body="${5:-}"
  local times=() timeouts=0 status=""

  for ((i = 0; i < SAMPLES; i++)); do
    local args=(-sS -o /dev/null -w '%{http_code} %{time_total}' --max-time 3 -X "$method")
    case "$mode" in
      json) args+=(-H 'Content-Type: application/json' -d "$body") ;;
      form) args+=(--form-string "account=$ACCOUNT" --form-string "password=$password_hash") ;;
    esac
    local out
    if ! out="$(curl "${args[@]}" -H "Authorization: Bearer $token" "$url" 2>/dev/null)"; then
      timeouts=$((timeouts + 1))
      continue
    fi
    status="${out%% *}"
    times+=("${out##* }")
  done

  local csv
  csv="$(IFS=,; echo "${times[*]:-}")"
  local summary
  summary="$(TIMES_CSV="$csv" python3 -c '
import os
raw = [float(x) for x in os.environ["TIMES_CSV"].split(",") if x]
if not raw:
    print("0 0")
else:
    raw.sort()
    print(f"{max(raw) * 1000:.1f} {raw[len(raw) // 2] * 1000:.1f}")
')"
  local max_ms="${summary%% *}" p50_ms="${summary##* }" passed=true
  [[ "$timeouts" -gt 0 ]] && passed=false
  python3 -c "import sys; sys.exit(0 if float('$max_ms') <= $BUDGET_MS else 1)" || passed=false

  printf '%-24s status=%-3s max=%8s ms p50=%8s ms timeouts=%s %s\n' \
    "$name" "${status:--}" "$max_ms" "$p50_ms" "$timeouts" \
    "$([[ "$passed" == true ]] && echo PASS || echo FAIL)"

  python3 -c "
import json
print(json.dumps({
    'operation': '''$name''',
    'method': '''$method''',
    'path': '''${url#"$BASE_URL"}''',
    'status': '''$status''',
    'samples': $SAMPLES,
    'maxMs': float('''$max_ms'''),
    'p50Ms': float('''$p50_ms'''),
    'timeouts': $timeouts,
    'budgetMs': $BUDGET_MS,
    'passed': $([[ "$passed" == true ]] && echo True || echo False),
}, ensure_ascii=False))
" >>"$rows_file"
}

echo "── API 延迟基线（预算 ${BUDGET_MS} ms，每项 ${SAMPLES} 次）──"
measure "health" GET "$BASE_URL/health" none
measure "login" POST "$API/auth/login" form
measure "projects.list" GET "$API/projects" none
measure "projects.get" GET "$API/projects/$PROJECT_ID" none
measure "documents.list" GET "$API/projects/$PROJECT_ID/documents" none
measure "latex-files.list" GET "$API/projects/$PROJECT_ID/latex-files" none
measure "latex-files.put" PUT "$API/projects/$PROJECT_ID/latex-files/main.tex" json \
  '{"content":"\\documentclass{article}\\begin{document}perf baseline\\end{document}"}'

python3 - "$OUTPUT" "$BUDGET_MS" "$rows_file" <<'PY'
import json
import os
import sys
from datetime import datetime, timezone

output, budget, rows_path = sys.argv[1], int(sys.argv[2]), sys.argv[3]
with open(rows_path, encoding="utf-8") as handle:
    rows = [json.loads(line) for line in handle if line.strip()]

report = {
    "generatedAt": datetime.now(timezone.utc).isoformat(),
    "budgetMs": budget,
    "passed": all(row["passed"] for row in rows),
    "results": rows,
}
with open(output, "w", encoding="utf-8") as handle:
    json.dump(report, handle, ensure_ascii=False, indent=2)

lines = [
    "### Nightly API 延迟基线",
    "",
    f"预算：单次 ≤ {budget} ms；任一项最大值超标或出现超时即失败。",
    "",
    "| 操作 | 方法 | 状态 | 最大值 (ms) | P50 (ms) | 超时 | 结果 |",
    "|---|---|---|---:|---:|---:|---|",
]
for row in rows:
    lines.append(
        f"| `{row['operation']}` | {row['method']} | {row['status'] or '-'} | "
        f"{row['maxMs']:.1f} | {row['p50Ms']:.1f} | {row['timeouts']} | "
        f"{'✅' if row['passed'] else '❌'} |"
    )
markdown = "\n".join(lines) + "\n"
summary_path = os.environ.get("GITHUB_STEP_SUMMARY")
if summary_path:
    with open(summary_path, "a", encoding="utf-8") as handle:
        handle.write(markdown)
else:
    print(markdown)
print("wrote", output)
sys.exit(0 if report["passed"] else 1)
PY
