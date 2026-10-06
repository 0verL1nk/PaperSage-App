#!/usr/bin/env bash
set -euo pipefail

# ─── PaperSage CLSI Environment Variables ───
# Source this file before running the vendored CLSI service.
# Usage: source scripts/clsi/env.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# ─── Network ───
# 变量名必须与 vendored CLSI 读取的一致（config/settings.defaults.cjs）：
# CLSI_HOST / CLSI_COMPILES_PATH / CLSI_OUTPUT_PATH / CLSI_CACHE_PATH / CLSI_UPLOAD_PATH。
# 此前这里用的是 HOST / COMPILES_DIR …，CLSI 根本不认，目录一直回落到
# latex_render/services/clsi/ 下的默认值。
export CLSI_HOST="${CLSI_HOST:-0.0.0.0}"
export PORT="${PORT:-3013}"

# ─── Paths ───
export CLSI_COMPILES_PATH="${PROJECT_ROOT}/.clsi/compiles"
export CLSI_OUTPUT_PATH="${PROJECT_ROOT}/.clsi/output"
export CLSI_CACHE_PATH="${PROJECT_ROOT}/.clsi/cache"
export CLSI_UPLOAD_PATH="${PROJECT_ROOT}/.clsi/uploads"
export TEXMFVAR="${PROJECT_ROOT}/.clsi/texmf-var"

# ─── Mode ───
# Host mode: no sandboxing, uses host filesystem directly
export SANDBOXED_COMPILES="false"

# ─── Ensure directories exist ───
mkdir -p "${CLSI_COMPILES_PATH}" "${CLSI_OUTPUT_PATH}" "${CLSI_CACHE_PATH}" "${CLSI_UPLOAD_PATH}" "${TEXMFVAR}"

echo "[env] CLSI host-mode environment loaded"
echo "[env]   CLSI_HOST=${CLSI_HOST}"
echo "[env]   PORT=${PORT}"
echo "[env]   CLSI_COMPILES_PATH=${CLSI_COMPILES_PATH}"
