#!/usr/bin/env bash
# 依赖许可证门禁：拒绝强传染性许可证（AGPL / GPL / SSPL / OSL / EUPL）。
#
# 范围只到依赖树，不含 vendored 源码：
# - Go 只扫 server/ 模块。
# - 前端只扫生产依赖（--prod）：devDependencies 不进产物。
#
# shortcut: go-licenses v2 在 Go 1.27 下无法为标准库包定位模块信息，
# 会把 ~120 个 stdlib 包判成 "does not have module info" 直接失败；
# 这里显式把 `go list std` 全部加入 --ignore。升级 go-licenses 后可去掉。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DENY_RE='^(A?GPL|SSPL|OSL-|EUPL)'
status=0

echo "── Go 依赖（server/）──"
cd "$ROOT/server"
mapfile -t STDLIB_IGNORES < <(go list std | sed 's|^|--ignore=|')
go_csv="$(go-licenses csv "${STDLIB_IGNORES[@]}" \
  --ignore github.com/0verL1nk/GraduateProject ./... 2>/dev/null)"

go_denied="$(awk -F, -v re="$DENY_RE" 'NF >= 3 && $3 ~ re { print $1 " -> " $3 }' <<<"$go_csv")"
go_unknown="$(awk -F, 'NF >= 3 && $3 == "Unknown" { print $1 }' <<<"$go_csv")"

awk -F, 'NF >= 3 { print $3 }' <<<"$go_csv" | sort | uniq -c | sort -rn | head -12

if [[ -n "$go_unknown" ]]; then
  # 上游子模块没有独立 LICENSE 文件时 go-licenses 只能报 Unknown；
  # 不当作失败，但必须可见，避免悄悄放过。
  echo "::warning::以下 Go 依赖未能识别许可证，需人工确认："
  echo "$go_unknown" | sed 's/^/  - /'
fi

if [[ -n "$go_denied" ]]; then
  echo "::error::Go 依赖出现强传染性许可证："
  echo "$go_denied" | sed 's/^/  - /'
  status=1
fi

echo "── 前端生产依赖（frontend/）──"
cd "$ROOT/frontend"
frontend_denied="$(pnpm licenses list --prod --json | node -e '
let raw = "";
process.stdin.on("data", (chunk) => (raw += chunk));
process.stdin.on("end", () => {
  const deny = /^(A?GPL|SSPL|OSL-|EUPL)/;
  for (const license of Object.keys(JSON.parse(raw))) {
    if (deny.test(license)) console.log(license);
  }
});')"

if [[ -n "$frontend_denied" ]]; then
  echo "::error::前端生产依赖出现强传染性许可证："
  echo "$frontend_denied" | sed 's/^/  - /'
  status=1
else
  echo "未发现强传染性许可证。"
fi

exit "$status"
