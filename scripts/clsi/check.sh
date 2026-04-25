#!/usr/bin/env bash
set -euo pipefail

# ─── PaperSage CLSI Host-Mode Prerequisites Checker ───
# Verifies all required commands are available for running vendored CLSI.

PASS=0
FAIL=0

# ─── Helpers ───

ok()   { printf "\033[0;32m  PASS\033[0m  %s\n" "$*"; ((PASS++)); }
fail() { printf "\033[0;31m  FAIL\033[0m  %s\n" "$*" >&2; ((FAIL++)); }

# ─── Checks ───

check_cmd() {
    local cmd="$1"
    local label="${2:-$cmd}"
    if command -v "$cmd" &>/dev/null; then
        local ver
        ver=$("$cmd" --version 2>&1 | head -1 || echo "available")
        ok "${label}: ${ver}"
    else
        fail "${label}: not found"
    fi
}

check_node_env() {
    if [[ -d "${PROJECT_ROOT}/latex_render/node_modules" ]]; then
        ok "latex_render/node_modules present"
    else
        fail "latex_render/node_modules missing (run: make install-clsi)"
    fi
}

# ─── Main ───

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  PaperSage CLSI Host-Mode Prerequisites"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "── Runtime ───"
check_cmd node   "Node.js"
check_cmd npm    "npm"

echo ""
echo "── TeX Engines ───"
check_cmd pdflatex  "pdflatex"
check_cmd xelatex   "xelatex"
check_cmd lualatex  "lualatex"

echo ""
echo "── Build Tools ───"
check_cmd latexmk   "latexmk"
check_cmd biber     "biber"

echo ""
echo "── Utilities ───"
check_cmd qpdf      "qpdf"
check_cmd texcount  "texcount"
check_cmd inkscape  "inkscape (optional)"

echo ""
echo "── Workspace ───"
check_node_env

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
if (( FAIL == 0 )); then
    printf "\033[0;32m  All checks passed (%d/%d)\033[0m\n" "$PASS" "$((PASS + FAIL))"
else
    printf "\033[0;31m  %d check(s) failed (%d/%d passed)\033[0m\n" "$FAIL" "$PASS" "$((PASS + FAIL))"
    exit 1
fi
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
