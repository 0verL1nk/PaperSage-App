#!/usr/bin/env bash
set -euo pipefail

# ─── PaperSage CLSI Dev Server Startup ───
# Starts the vendored CLSI service in host mode.
# Usage: ./scripts/clsi/dev.sh  or  make clsi-dev

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
CLSI_APP="${PROJECT_ROOT}/latex_render/services/clsi/app.js"

# Load environment
# shellcheck source=env.sh
source "${SCRIPT_DIR}/env.sh"

# Verify entry point exists
if [[ ! -f "${CLSI_APP}" ]]; then
    echo "[dev] ERROR: CLSI app not found at ${CLSI_APP}" >&2
    exit 1
fi

# Verify Node is available
if ! command -v node &>/dev/null; then
    echo "[dev] ERROR: node not found. Run 'make install-clsi' first." >&2
    exit 1
fi

echo "[dev] Starting CLSI service on http://${HOST}:${PORT}"
echo "[dev] Press Ctrl+C to stop"
echo ""

exec node "${CLSI_APP}"
