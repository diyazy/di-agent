#!/usr/bin/env bash
# 05-peers.sh — discover VM IPs and register each VM as a peer on the others
#
# Peer topology (trust=0.8 for all pairs):
#   diag-1 ← diag-2, diag-3
#   diag-2 ← diag-1, diag-3
#   diag-3 ← diag-1, diag-2
#
# Usage: ./05-peers.sh [vm1 vm2 vm3]

set -euo pipefail

# ── colours ─────────────────────────────────────────────────────────────────
GREEN=$(tput setaf 2 2>/dev/null || echo "")
YELLOW=$(tput setaf 3 2>/dev/null || echo "")
RED=$(tput setaf 1 2>/dev/null || echo "")
RESET=$(tput sgr0 2>/dev/null || echo "")

info() { echo "${YELLOW}[peers] $*${RESET}"; }
ok()   { echo "${GREEN}[peers] $*${RESET}"; }
err()  { echo "${RED}[peers] $*${RESET}" >&2; }

# ── helpers ──────────────────────────────────────────────────────────────────
vm_ip() { multipass list | awk -v vm="$1" '$1 == vm {print $3}'; }

register_peer() {
    local target_vm="$1"   # VM whose agent receives the POST
    local target_ip="$2"   # IP of target_vm
    local peer_id="$3"     # ID of the peer being registered
    local peer_url="$4"    # URL of the peer
    local trust="${5:-0.8}"

    info "  $target_vm ← peer $peer_id ($peer_url, trust=$trust)"
    local response
    response=$(curl -sf -X POST \
        -H "Content-Type: application/json" \
        -d "{\"id\": \"${peer_id}\", \"url\": \"${peer_url}\", \"trust\": ${trust}}" \
        "http://${target_ip}:8080/peers" 2>&1) || {
        err "  Failed to POST /peers on $target_vm ($target_ip): $response"
        return 1
    }
    ok "  registered"
}

# ── args ─────────────────────────────────────────────────────────────────────
if [ "$#" -eq 0 ]; then
    VMS=(diag-1 diag-2 diag-3)
else
    VMS=("$@")
fi

# ── discover IPs ─────────────────────────────────────────────────────────────
declare -A IPS
for vm in "${VMS[@]}"; do
    ip=$(vm_ip "$vm")
    if [ -z "$ip" ]; then
        err "Could not find IP for $vm — is it running?"
        exit 1
    fi
    IPS["$vm"]="$ip"
    info "$vm → ${IPS[$vm]}"
done

# ── register peers ────────────────────────────────────────────────────────────
info ""
info "Registering peer relationships ..."

for target in "${VMS[@]}"; do
    target_ip="${IPS[$target]}"
    for peer in "${VMS[@]}"; do
        if [ "$peer" = "$target" ]; then
            continue
        fi
        peer_ip="${IPS[$peer]}"
        register_peer "$target" "$target_ip" "$peer" "http://${peer_ip}:8080" 0.8
    done
done

# ── verify ────────────────────────────────────────────────────────────────────
info ""
info "Verifying peer lists ..."
for vm in "${VMS[@]}"; do
    ip="${IPS[$vm]}"
    peer_count=$(curl -sf "http://${ip}:8080/peers" 2>/dev/null \
        | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d))" 2>/dev/null \
        || echo "?")
    ok "$vm: $peer_count peers registered"
done

echo ""
ok "Peer registration complete."
