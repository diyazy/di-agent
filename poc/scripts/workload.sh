#!/usr/bin/env bash
# workload.sh — apply or remove synthetic CPU/memory load on a VM
#
# Usage: ./workload.sh <vm-name> <heavy|light|stop>
#
#   heavy  — 2 CPU workers + 512 MiB VM stress for 300s
#   light  — 1 CPU worker + 128 MiB VM stress for 300s
#   stop   — kill any running stress-ng process

set -euo pipefail

# ── colours ─────────────────────────────────────────────────────────────────
GREEN=$(tput setaf 2 2>/dev/null || echo "")
YELLOW=$(tput setaf 3 2>/dev/null || echo "")
RED=$(tput setaf 1 2>/dev/null || echo "")
RESET=$(tput sgr0 2>/dev/null || echo "")

info() { echo "${YELLOW}[workload] $*${RESET}"; }
ok()   { echo "${GREEN}[workload] $*${RESET}"; }
err()  { echo "${RED}[workload] $*${RESET}" >&2; }

# ── args ─────────────────────────────────────────────────────────────────────
if [ "$#" -lt 2 ]; then
    err "Usage: $0 <vm-name> <heavy|light|stop>"
    exit 1
fi

VM="$1"
INTENSITY="$2"

# ── ensure stress-ng is available ────────────────────────────────────────────
ensure_stress_ng() {
    if ! multipass exec "$VM" -- bash -c "command -v stress-ng >/dev/null 2>&1"; then
        info "$VM: installing stress-ng ..."
        multipass exec "$VM" -- bash -c "sudo apt-get install -y stress-ng -qq"
        ok "$VM: stress-ng installed"
    fi
}

# ── dispatch ─────────────────────────────────────────────────────────────────
case "$INTENSITY" in
    heavy)
        ensure_stress_ng
        info "$VM: starting heavy workload (2 CPU workers, 512 MiB VM, 300s) ..."
        multipass exec "$VM" -- bash -c \
            "nohup stress-ng --cpu 2 --vm 1 --vm-bytes 512M --timeout 300s \
             > /tmp/stress.log 2>&1 &"
        ok "$VM: heavy workload started (PID logged to /tmp/stress.log)"
        ;;
    light)
        ensure_stress_ng
        info "$VM: starting light workload (1 CPU worker, 128 MiB VM, 300s) ..."
        multipass exec "$VM" -- bash -c \
            "nohup stress-ng --cpu 1 --vm 1 --vm-bytes 128M --timeout 300s \
             > /tmp/stress.log 2>&1 &"
        ok "$VM: light workload started (PID logged to /tmp/stress.log)"
        ;;
    stop)
        info "$VM: stopping stress-ng ..."
        multipass exec "$VM" -- bash -c "sudo pkill -f stress-ng || true"
        ok "$VM: stress-ng stopped (or was not running)"
        ;;
    *)
        err "Unknown intensity '$INTENSITY'. Use: heavy | light | stop"
        exit 1
        ;;
esac
