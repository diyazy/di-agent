#!/usr/bin/env bash
# 03-netdata.sh — install and start Netdata on each VM
#
# Tries apt first; falls back to the kickstart script if the apt version
# is too old (< 1.35) or unavailable.
#
# Usage: ./03-netdata.sh [vm1 vm2 vm3]

set -euo pipefail

# ── colours ─────────────────────────────────────────────────────────────────
GREEN=$(tput setaf 2 2>/dev/null || echo "")
YELLOW=$(tput setaf 3 2>/dev/null || echo "")
RED=$(tput setaf 1 2>/dev/null || echo "")
RESET=$(tput sgr0 2>/dev/null || echo "")

info() { echo "${YELLOW}[netdata] $*${RESET}"; }
ok()   { echo "${GREEN}[netdata] $*${RESET}"; }
err()  { echo "${RED}[netdata] $*${RESET}" >&2; }

# ── helpers ──────────────────────────────────────────────────────────────────
vm_ip() { multipass list | awk -v vm="$1" '$1 == vm {print $3}'; }

netdata_version_ok() {
    # Returns 0 if installed Netdata version is >= 1.35
    local vm="$1"
    local ver
    ver=$(multipass exec "$vm" -- bash -c \
        "netdata -v 2>/dev/null | grep -oP '(?<=v)\d+\.\d+' | head -1" 2>/dev/null || echo "0.0")
    local major minor
    major=$(echo "$ver" | cut -d. -f1)
    minor=$(echo "$ver" | cut -d. -f2)
    [ "${major:-0}" -gt 1 ] || { [ "${major:-0}" -eq 1 ] && [ "${minor:-0}" -ge 35 ]; }
}

install_netdata_apt() {
    local vm="$1"
    info "$vm: installing Netdata via apt ..."
    multipass exec "$vm" -- bash -c "
        sudo apt-get update -qq
        sudo apt-get install -y netdata
    "
}

install_netdata_kickstart() {
    local vm="$1"
    info "$vm: apt version too old or unavailable — using kickstart script ..."
    multipass exec "$vm" -- bash -c "
        curl -sSL https://my-netdata.io/kickstart.sh -o /tmp/netdata-kickstart.sh
        sudo bash /tmp/netdata-kickstart.sh --non-interactive --stable-channel \
            --disable-telemetry 2>&1 | tail -20
    "
}

verify_netdata() {
    local vm="$1"
    info "$vm: verifying Netdata API ..."
    local attempts=0
    while [ "$attempts" -lt 12 ]; do
        if multipass exec "$vm" -- bash -c \
            "curl -sf http://localhost:19999/api/v1/info > /dev/null 2>&1"; then
            ok "$vm: Netdata API responding at localhost:19999"
            return 0
        fi
        sleep 5
        attempts=$((attempts + 1))
    done
    err "$vm: Netdata did not respond within 60s"
    return 1
}

# ── args ─────────────────────────────────────────────────────────────────────
if [ "$#" -eq 0 ]; then
    VMS=(diag-1 diag-2 diag-3)
else
    VMS=("$@")
fi

# ── install + start ──────────────────────────────────────────────────────────
for vm in "${VMS[@]}"; do
    # Check if already installed
    if multipass exec "$vm" -- bash -c "command -v netdata >/dev/null 2>&1"; then
        info "$vm: netdata already installed"
        if ! netdata_version_ok "$vm"; then
            info "$vm: installed version is old, upgrading via kickstart ..."
            install_netdata_kickstart "$vm"
        fi
    else
        install_netdata_apt "$vm" || install_netdata_kickstart "$vm"
    fi

    info "$vm: enabling and starting netdata service ..."
    multipass exec "$vm" -- bash -c "
        sudo systemctl enable netdata 2>/dev/null || true
        sudo systemctl start netdata 2>/dev/null || sudo service netdata start
    "

    verify_netdata "$vm"
done

echo ""
ok "Netdata running on all VMs."
