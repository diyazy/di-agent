#!/usr/bin/env bash
#
# dev.sh — Semantic Map local development helper.
#
# Dev-only. Hides the build/run/kill/curl boilerplate behind named
# subcommands so the daily loop is one keystroke. NOT for production —
# the daemon is meant to be run directly from the built binary on a real
# host. This script just makes the inner-loop fast.
#
# Usage:
#   ./dev.sh <command> [args]
#
# See ./dev.sh help.

set -euo pipefail

# ── Configuration (override via env) ────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

PORT="${PORT:-8080}"
PROFILE="${PROFILE:-edge-minimal}"
KD="${KD:-}"
PRIORS="${PRIORS:-}"
ALPHA="${ALPHA:-0.2}"
CONVERGENCE="${CONVERGENCE:-500}"

AGENT_BIN="/tmp/semantic-map-agent"
MAPCTL_BIN="/tmp/semantic-map-mapctl"
LOG_FILE="/tmp/semantic-map-agent.log"

# ── Output helpers ──────────────────────────────────────────────────────────

if [[ -t 1 ]] && command -v tput >/dev/null 2>&1; then
    BOLD=$(tput bold)
    DIM=$(tput dim)
    GREEN=$(tput setaf 2)
    RED=$(tput setaf 1)
    YELLOW=$(tput setaf 3)
    BLUE=$(tput setaf 4)
    RESET=$(tput sgr0)
else
    BOLD=""; DIM=""; GREEN=""; RED=""; YELLOW=""; BLUE=""; RESET=""
fi

info() { printf "%s→%s %s\n" "${BOLD}${GREEN}" "${RESET}" "$*"; }
warn() { printf "%s!%s %s\n" "${BOLD}${YELLOW}" "${RESET}" "$*"; }
fail() { printf "%s✗%s %s\n" "${BOLD}${RED}" "${RESET}" "$*" >&2; exit 1; }
step() { printf "%s%s%s\n" "${DIM}" "$*" "${RESET}"; }

# ── Process helpers ─────────────────────────────────────────────────────────

running_pid() {
    lsof -ti ":$PORT" 2>/dev/null || true
}

ensure_jq() {
    command -v jq >/dev/null 2>&1 || warn "jq not installed; some output will be raw JSON"
}

# ── Commands ────────────────────────────────────────────────────────────────

cmd_build() {
    step "go build -o $AGENT_BIN ./cmd/agent"
    go build -o "$AGENT_BIN" ./cmd/agent
    step "go build -o $MAPCTL_BIN ./cmd/mapctl"
    go build -o "$MAPCTL_BIN" ./cmd/mapctl
    info "built: $AGENT_BIN, $MAPCTL_BIN"
}

cmd_start() {
    local pid
    pid="$(running_pid)"
    if [[ -n "$pid" ]]; then
        warn "agent already running on :$PORT (PID $pid). Use 'restart' to swap, 'stop' to kill."
        return 0
    fi

    if [[ ! -x "$AGENT_BIN" ]]; then
        warn "no agent binary at $AGENT_BIN — building first"
        cmd_build
    fi

    local args=(-profile "$PROFILE" -addr ":$PORT" -alpha "$ALPHA" -convergence "$CONVERGENCE")
    [[ -n "$KD" ]]     && args+=(-kd "$KD")
    [[ -n "$PRIORS" ]] && args+=(-priors "$PRIORS")

    step "$AGENT_BIN ${args[*]}"
    nohup "$AGENT_BIN" "${args[@]}" > "$LOG_FILE" 2>&1 &
    disown
    sleep 1

    pid="$(running_pid)"
    if [[ -z "$pid" ]]; then
        fail "agent failed to start. Check $LOG_FILE"
    fi
    info "started: PID $pid, :$PORT, log $LOG_FILE"
}

cmd_stop() {
    local pid
    pid="$(running_pid)"
    if [[ -z "$pid" ]]; then
        warn "no agent running on :$PORT"
        return 0
    fi
    step "kill -9 $pid"
    kill -9 "$pid" 2>/dev/null || true
    sleep 1
    info "stopped"
}

cmd_restart() {
    cmd_stop
    cmd_build
    cmd_start
}

cmd_status() {
    local pid
    pid="$(running_pid)"
    if [[ -z "$pid" ]]; then
        warn "agent NOT running on :$PORT"
        return 1
    fi
    info "running: PID $pid, :$PORT"
    if curl -sf "http://localhost:$PORT/healthz" >/dev/null 2>&1; then
        ensure_jq
        if command -v jq >/dev/null 2>&1; then
            curl -s "http://localhost:$PORT/version" | jq .
        else
            curl -s "http://localhost:$PORT/version"
            echo
        fi
    else
        warn "process alive but /healthz not responding yet"
    fi
}

cmd_logs() {
    if [[ ! -f "$LOG_FILE" ]]; then
        fail "no log file at $LOG_FILE (start the agent first)"
    fi
    tail -f "$LOG_FILE"
}

cmd_ui() {
    if ! cmd_status >/dev/null 2>&1; then
        warn "agent not running — starting first"
        cmd_start
    fi
    info "opening http://localhost:$PORT/ui/"
    if command -v open >/dev/null 2>&1; then
        open "http://localhost:$PORT/ui/"        # macOS
    elif command -v xdg-open >/dev/null 2>&1; then
        xdg-open "http://localhost:$PORT/ui/"    # Linux
    else
        echo "http://localhost:$PORT/ui/"        # fallback: print URL
    fi
}

cmd_cli() {
    if [[ ! -x "$MAPCTL_BIN" ]]; then
        warn "no mapctl binary — building first"
        cmd_build
    fi
    "$MAPCTL_BIN" --addr "http://localhost:$PORT" "$@"
}

cmd_test() {
    step "go test ./..."
    go test ./...
    step "go vet ./..."
    go vet ./...
    info "tests + vet OK"
}

cmd_smoke() {
    if ! cmd_status >/dev/null 2>&1; then
        cmd_start
    fi
    ensure_jq
    local jq_or_cat
    if command -v jq >/dev/null 2>&1; then jq_or_cat="jq ."; else jq_or_cat="cat"; fi

    echo
    info "GET /healthz"; curl -s "localhost:$PORT/healthz" | $jq_or_cat
    echo
    info "GET /graph (counts)"
    if command -v jq >/dev/null 2>&1; then
        curl -s "localhost:$PORT/graph" | jq '{constructs:(.constructs|length),propositions:(.propositions|length),edges:(.edges|length)}'
    else
        curl -s "localhost:$PORT/graph"
        echo
    fi
    echo
    info "GET /edges?from=RC&to=PS (expect 2 — conflict pair)"
    if command -v jq >/dev/null 2>&1; then
        curl -s "localhost:$PORT/edges?from=RC&to=PS" | jq 'length'
    else
        curl -s "localhost:$PORT/edges?from=RC&to=PS"
        echo
    fi
    echo
    info "mapctl deprecate P11 'smoke from dev.sh'"
    cmd_cli deprecate P11 "smoke from dev.sh" || true
    echo
    info "GET /history (last entry)"
    if command -v jq >/dev/null 2>&1; then
        curl -s "localhost:$PORT/history" | jq '.[-1]'
    else
        curl -s "localhost:$PORT/history"
        echo
    fi
}

cmd_clean() {
    cmd_stop
    step "rm -f $AGENT_BIN $MAPCTL_BIN $LOG_FILE"
    rm -f "$AGENT_BIN" "$MAPCTL_BIN" "$LOG_FILE"
    step "go clean -cache"
    go clean -cache
    info "cleaned"
}

cmd_help() {
    cat <<EOF
${BOLD}dev.sh${RESET} — Semantic Map development helper (dev-only)

${BOLD}Usage:${RESET}
  ./dev.sh <command> [args]

${BOLD}Commands:${RESET}
  ${BOLD}build${RESET}      Build agent + mapctl into /tmp
  ${BOLD}start${RESET}      Start the daemon (no-op if already running)
  ${BOLD}stop${RESET}       Kill the running daemon
  ${BOLD}restart${RESET}    Stop → build → start
  ${BOLD}status${RESET}     Show PID + /healthz + /version
  ${BOLD}logs${RESET}       Tail $LOG_FILE
  ${BOLD}ui${RESET}         Open http://localhost:\$PORT/ui/ (starts daemon if needed)
  ${BOLD}cli${RESET} ...    Run mapctl with --addr already set, e.g. ./dev.sh cli graph
  ${BOLD}test${RESET}       go test ./... + go vet ./...
  ${BOLD}smoke${RESET}      End-to-end smoke (curl + mapctl) against the running daemon
  ${BOLD}clean${RESET}      Stop, remove binaries, clear go build cache
  ${BOLD}help${RESET}       This help

${BOLD}Environment overrides:${RESET}
  PORT          ${BLUE}${PORT}${RESET}            HTTP port
  PROFILE       ${BLUE}${PROFILE}${RESET}   Deployment profile
  KD            ${BLUE}${KD:-(none)}${RESET}         k3s|k0s|k8s|kubeEdge|openYurt
  PRIORS        ${BLUE}${PRIORS:-(none)}${RESET}         path to prior_weights.json
  ALPHA         ${BLUE}${ALPHA}${RESET}            EMA decay factor
  CONVERGENCE   ${BLUE}${CONVERGENCE}${RESET}            observations until confidence=1.0

${BOLD}Examples:${RESET}
  ./dev.sh restart                 # rebuild + restart in one command
  ./dev.sh cli graph               # mapctl --addr http://localhost:8080 graph
  ./dev.sh cli deprecate P1 "test"
  ./dev.sh ui                      # browser viewer (starts daemon if needed)
  KD=k0s ./dev.sh restart          # use per-KD priors
  PORT=9000 ./dev.sh start         # custom port

${BOLD}Note:${RESET} dev-only. Binaries land in /tmp; not for production deployment.
EOF
}

# ── Dispatch ────────────────────────────────────────────────────────────────

CMD="${1:-help}"
shift || true

case "$CMD" in
    build)          cmd_build ;;
    start)          cmd_start ;;
    stop)           cmd_stop ;;
    restart)        cmd_restart ;;
    status)         cmd_status ;;
    logs)           cmd_logs ;;
    ui)             cmd_ui ;;
    cli)            cmd_cli "$@" ;;
    test)           cmd_test ;;
    smoke)          cmd_smoke ;;
    clean)          cmd_clean ;;
    help|--help|-h) cmd_help ;;
    *)              fail "unknown command: $CMD (try './dev.sh help')" ;;
esac
