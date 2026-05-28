"""
Publication constants from P1 and P2 (ESOCC 2025).

These are values reported in the papers that are not directly computable
from the available CSV files (security scores, pod-startup latency from
k-bench control-plane runs, throughput ops/sec, setup time, recovery time,
offline autonomy classification).

Each constant block includes the paper source and table/figure reference.
"""

# ── P2 paper: CIS benchmark security scores ───────────────────────────────
# Source: Table 3 in P2. CIS benchmark compliance % (weighted scoring).
SECURITY_SCORES: dict[str, float] = {
    "k8s":       0.55,
    "kubeEdge":  0.55,
    "openYurt":  0.55,
    "k0s":       0.2369,
    "k3s":       0.0721,
}

# ── P1 paper: Pod-startup latency under heavy load (ms) ──────────────────
# Source: k-bench CP heavy 12-client results, Table 2 in P1.
# Lower is better. kubeEdge degrades catastrophically at 120 pods (>30 000ms).
POD_STARTUP_LATENCY_MS: dict[str, float] = {
    "k3s":       6_800,
    "k0s":       7_200,
    "k8s":       7_500,
    "openYurt":  9_100,
    "kubeEdge":  30_500,
}

# ── P1 paper: Data-plane throughput (ops/sec) ─────────────────────────────
# Source: memtier benchmark, Table 2 in P1. Higher is better.
DP_THROUGHPUT_OPS: dict[str, float] = {
    "k8s":       19_000,
    "k3s":       16_500,
    "k0s":       17_000,
    "openYurt":  14_500,
    "kubeEdge":   4_750,   # ~75% lower than k8s
}

# ── P2 paper: Setup time (hours) ─────────────────────────────────────────
# Source: Section 4.3 Maintainability in P2. Lower is better.
SETUP_TIME_HOURS: dict[str, float] = {
    "k3s":       2.5,
    "k0s":       2.5,
    "k8s":       6.0,
    "kubeEdge":  14.0,
    "openYurt":  14.0,
}

# ── P2 paper: Recovery time after network partition (minutes) ─────────────
# Source: Section 4.2 Resilience in P2. Lower is better for recovery speed.
RECOVERY_TIME_MIN: dict[str, float] = {
    "k3s":       3.5,
    "k0s":       4.0,
    "k8s":       5.5,
    "kubeEdge":  8.0,
    "openYurt":  8.5,
}

# ── P2 paper: Offline message preservation (binary) ──────────────────────
# Source: Section 4.2 in P2. 1 = messages preserved during outage, 0 = lost.
OFFLINE_PRESERVATION: dict[str, float] = {
    "kubeEdge":  1.0,
    "openYurt":  1.0,
    "k3s":       0.0,
    "k0s":       0.0,
    "k8s":       0.0,
}

# ── Community / Ecosystem proxy: GitHub stars (approximate, May 2026) ────
# Used as ecosystem strength proxy. Normalised to [0,1] in calibration.
GITHUB_STARS: dict[str, float] = {
    "k8s":       110_000,
    "k3s":        29_000,
    "k0s":        3_800,
    "kubeEdge":   9_500,
    "openYurt":   1_800,
}
