#!/usr/bin/env bash
set -euo pipefail

# ─── PaperSage CLSI Environment Variables ───
# Source this file before running the vendored CLSI service.
# Usage: source scripts/clsi/env.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# ─── Network ───
export HOST="${CLSI_HOST:-0.0.0.0}"
export PORT="${CLSI_PORT:-3013}"

# ─── Paths ───
export COMPILES_DIR="${PROJECT_ROOT}/.clsi/compiles"
export OUTPUT_DIR="${PROJECT_ROOT}/.clsi/output"
export CACHE_DIR="${PROJECT_ROOT}/.clsi/cache"
export UPLOAD_DIR="${PROJECT_ROOT}/.clsi/uploads"
export TEXMFVAR="${PROJECT_ROOT}/.clsi/texmf-var"

# ─── Mode ───
# Host mode: no sandboxing, uses host filesystem directly
export SANDBOXED_COMPILES="false"

# ─── Ensure directories exist ───
mkdir -p "${COMPILES_DIR}" "${OUTPUT_DIR}" "${CACHE_DIR}" "${UPLOAD_DIR}" "${TEXMFVAR}"

echo "[env] CLSI host-mode environment loaded"
echo "[env]   HOST=${HOST}"
echo "[env]   PORT=${PORT}"
echo "[env]   COMPILES_DIR=${COMPILES_DIR}"
