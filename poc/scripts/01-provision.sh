#!/usr/bin/env bash
# 01-provision.sh — launch Multipass VMs for the di-agent PoC
#
# Usage: ./01-provision.sh [vm1 vm2 vm3]
#   Defaults to diag-1 diag-2 diag-3

set -euo pipefail

# ── colours ────────────────────────────────────────────────────────────────
GREEN=$(tput setaf 2 2>/dev/null || echo "")
YELLOW=$(tput setaf 3 2>/dev/null || echo "")
RED=$(tput setaf 1 2>/dev/null || echo "")
RESET=$(tput sgr0 2>/dev/null || echo "")

info()  { echo "${YELLOW}[provision] $*${RESET}"; }
ok()    { echo "${GREEN}[provision] $*${RESET}"; }
err()   { echo "${RED}[provision] $*${RESET}" >&2; }

# ── helpers ─────────────────────────────────────────────────────────────────
vm_ip() { multipass list | awk -v vm="$1" '$1 == vm {print $3}'; }

vm_exists() {
    multipass list 2>/dev/null | awk '{print $1}' | grep -qx "$1"
}

# ── args ────────────────────────────────────────────────────────────────────
VMS=("${@:-diag-1 diag-2 diag-3}")
if [ "$#" -eq 0 ]; then
    VMS=(diag-1 diag-2 diag-3)
else
    VMS=("$@")
fi

# ── provision ───────────────────────────────────────────────────────────────
for vm in "${VMS[@]}"; do
    if vm_exists "$vm"; then
        info "$vm already exists — skipping launch"
    else
        info "Launching $vm (2 vCPU, 2G RAM, 10G disk, Ubuntu 22.04) ..."
        multipass launch 22.04 \
            --name "$vm" \
            --cpus 2 \
            --memory 2G \
            --disk 10G
        ok "$vm launched"
    fi
done

# ── wait for cloud-init ──────────────────────────────────────────────────────
for vm in "${VMS[@]}"; do
    info "Waiting for cloud-init on $vm ..."
    multipass exec "$vm" -- cloud-init status --wait
    ok "$vm cloud-init done"
done

# ── print IPs ───────────────────────────────────────────────────────────────
echo ""
ok "VM inventory:"
for vm in "${VMS[@]}"; do
    ip=$(vm_ip "$vm")
    echo "  $vm  →  $ip"
done
echo ""
ok "Provisioning complete."
