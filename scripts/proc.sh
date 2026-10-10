#!/usr/bin/env bash
# ─── PaperSage Process Manager ───
# Manages graceful startup/shutdown of all development services.
# Usage: scripts/proc.sh <start|stop|status>
set -uo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PID_DIR="${PROJECT_ROOT}/.pids"
LOG_DIR="${PROJECT_ROOT}/logs"

# Service definitions: name|health_url|health_method|start_command
# Order matters: started top-to-bottom, stopped bottom-to-top
SERVICES=(
    # CLSI is temporarily parked while browser compilation becomes the active path.
    # Restore the service tuple below if the old backend compiler is re-enabled:
    # "clsi|http://localhost:3013/status|GET|source '${PROJECT_ROOT}/scripts/clsi/env.sh' && mkdir -p '${PROJECT_ROOT}/latex_render/services/clsi/compiles' '${PROJECT_ROOT}/latex_render/services/clsi/output' '${PROJECT_ROOT}/latex_render/services/clsi/cache'; node --watch '${PROJECT_ROOT}/latex_render/services/clsi/app.js'"
    "backend|http://localhost:5478/health|GET|cd '${PROJECT_ROOT}/server' && DATABASE_URL='sqlite:///./papersage.db' LATEX_CLSI_ENABLED=false go run ./cmd/papersage"
    "frontend|http://localhost:5173|TCP|cd '${PROJECT_ROOT}/frontend' && pnpm dev --host"
)

HEALTH_TIMEOUT=60
SHUTDOWN_GRACE=8

# ─── Colors ───
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${GREEN}[proc]${NC} $*"; }
warn()  { echo -e "${YELLOW}[proc]${NC} $*"; }
error() { echo -e "${RED}[proc]${NC} $*" >&2; }

# ─── PID file helpers ───
pid_file() { echo "${PID_DIR}/$1.pid"; }
log_file() { echo "${LOG_DIR}/$1.log"; }

get_pid() {
    local pf="$1"
    pf="$(pid_file "$pf")"
    [[ -f "$pf" ]] || return 1
    local pid
    pid=$(cat "$pf" 2>/dev/null) || return 1
    if kill -0 "$pid" 2>/dev/null; then
        echo "$pid"
        return 0
    fi
    rm -f "$pf"
    return 1
}

save_pid() {
    mkdir -p "$PID_DIR"
    echo "$2" > "$(pid_file "$1")"
}

remove_pid() { rm -f "$(pid_file "$1")"; }

# ─── Parse service entry ───
parse_service() {
    local entry="$1"
    IFS='|' read -r SVC_NAME SVC_URL SVC_METHOD SVC_CMD <<< "$entry"
}

# ─── Health check ───
wait_healthy() {
    local name="$1" url="$2" method="$3" pid="$4"
    local elapsed=0

    while (( elapsed < HEALTH_TIMEOUT )); do
        if ! kill -0 "$pid" 2>/dev/null; then
            error "$name (PID $pid) exited during startup"
            return 1
        fi

        case "$method" in
            GET)
                if curl -sf -o /dev/null --max-time 1 "$url" 2>/dev/null; then
                    info "$name is healthy"
                    return 0
                fi
                ;;
            TCP)
                if curl -sf -o /dev/null --max-time 1 "$url" 2>/dev/null; then
                    info "$name is healthy"
                    return 0
                fi
                ;;
        esac

        sleep 0.5
        ((elapsed++)) || true
    done

    warn "$name not ready after ${HEALTH_TIMEOUT}s (continuing anyway)"
    return 0
}

# ─── Start a single service ───
start_service() {
    local name="$1" cmd="$2"

    local existing_pid
    if existing_pid=$(get_pid "$name"); then
        warn "$name already running (PID $existing_pid), skipping"
        return 0
    fi

    # Kill any orphan process holding the port
    local port
    port=$(echo "$SVC_URL" | grep -oP ':\K\d+' | head -1) || true
    if [[ -n "$port" ]]; then
        local orphan
        orphan=$(lsof -i :"$port" -t 2>/dev/null) || true
        if [[ -n "$orphan" ]]; then
            warn "Port $port in use by orphan process(es): $orphan, killing..."
            for p in $orphan; do
                kill_tree "$p" "-TERM"
            done
            sleep 1
        fi
    fi

    mkdir -p "$LOG_DIR"
    local lf
    lf="$(log_file "$name")"

    info "Starting $name..."

    eval "$cmd" >> "$lf" 2>&1 &
    local pid=$!
    save_pid "$name" "$pid"

    info "$name started (PID $pid, log: ${lf#${PROJECT_ROOT}/})"
}

# Kill an entire process tree (children before parent)
kill_tree() {
    local pid="$1" sig="$2"
    local children
    children=$(pgrep -P "$pid" 2>/dev/null) || true
    for child in $children; do
        kill_tree "$child" "$sig"
    done
    kill "$sig" "$pid" 2>/dev/null || true
}

# ─── Stop a single service ───
stop_service() {
    local name="$1" pid

    pid=$(get_pid "$name") || return 0

    info "Stopping $name (PID $pid)..."
    kill_tree "$pid" "-TERM"

    local waited=0
    while (( waited < SHUTDOWN_GRACE )); do
        if ! kill -0 "$pid" 2>/dev/null; then
            info "$name stopped gracefully"
            remove_pid "$name"
            return 0
        fi
        sleep 1
        ((waited++)) || true
    done

    warn "$name did not exit within ${SHUTDOWN_GRACE}s, force killing"
    kill_tree "$pid" "-KILL"
    wait "$pid" 2>/dev/null || true
    remove_pid "$name"
    info "$name killed"
}

# ─── START command ───
cmd_start() {
    # Stop any previous instances first
    cmd_stop 2>/dev/null || true

    info "Starting PaperSage development environment..."

    local entry pid failed=()

    for entry in "${SERVICES[@]}"; do
        parse_service "$entry"

        if ! start_service "$SVC_NAME" "$SVC_CMD"; then
            error "Failed to start $SVC_NAME"
            failed+=("$SVC_NAME")
            continue
        fi

        pid=$(get_pid "$SVC_NAME") || true
        if [[ -n "$pid" ]]; then
            if ! wait_healthy "$SVC_NAME" "$SVC_URL" "$SVC_METHOD" "$pid"; then
                error "$SVC_NAME failed health check"
                failed+=("$SVC_NAME")
            fi
        fi
    done

    if [[ ${#failed[@]} -gt 0 ]]; then
        warn "Services with issues: ${failed[*]}"
    fi

    echo ""
    info "All services started!"
    echo -e "  ${BLUE}Frontend:${NC} http://localhost:5173"
    echo -e "  ${BLUE}Backend:${NC}  http://localhost:5478"
    echo -e "  ${BLUE}CLSI:${NC}     http://localhost:3013"
    echo -e "  ${BLUE}Logs:${NC}     ${LOG_DIR#${PROJECT_ROOT}/}/"
    echo ""
    info "Press Ctrl+C to stop all services"

    # Block: wait for any child to exit
    while true; do
        wait -n 2>/dev/null || sleep 1
        for entry in "${SERVICES[@]}"; do
            parse_service "$entry"
            pid=$(get_pid "$SVC_NAME") 2>/dev/null || true
            if [[ -z "$pid" ]]; then
                warn "$SVC_NAME has exited"
            fi
        done
    done
}

# ─── STOP command ───
cmd_stop() {
    info "Stopping all services (reverse order)..."

    local entry i
    for (( i=${#SERVICES[@]}-1; i>=0; i-- )); do
        parse_service "${SERVICES[$i]}"
        stop_service "$SVC_NAME"
    done

    info "All services stopped"
}

# ─── STATUS command ───
cmd_status() {
    info "Service status:"
    local entry pid
    for entry in "${SERVICES[@]}"; do
        parse_service "$entry"
        if pid=$(get_pid "$SVC_NAME"); then
            echo -e "  ${GREEN}●${NC} $SVC_NAME (PID $pid)"
        else
            echo -e "  ${RED}○${NC} $SVC_NAME (stopped)"
        fi
    done
}

# ─── Signal handler ───
shutdown() {
    echo ""
    info "Shutting down gracefully..."
    cmd_stop
    exit 0
}
trap shutdown SIGINT SIGTERM

# ─── Main ───
case "${1:-}" in
    start)  cmd_start  ;;
    stop)   cmd_stop   ;;
    status) cmd_status ;;
    *)
        echo "Usage: $0 {start|stop|status}"
        exit 1
        ;;
esac
