"""
Loaders for computed result CSVs from energy-analysis/ and overhead-decomposition/.

All paths are relative to the mega-research root. Call with an explicit
root_dir if running from a different working directory.
"""

import csv
import os
from pathlib import Path
from typing import Optional


def _repo_root(root_dir: Optional[str] = None) -> Path:
    if root_dir:
        return Path(root_dir)
    # Walk upward from this file to find the repo root (contains CLAUDE.md).
    p = Path(__file__).resolve()
    for parent in p.parents:
        if (parent / "CLAUDE.md").exists():
            return parent
    raise FileNotFoundError("Cannot locate repo root (no CLAUDE.md found in parents)")


def load_energy_efficiency(root_dir: Optional[str] = None) -> dict[str, dict]:
    """
    Returns per-KD energy efficiency metrics from P4.

    Keys per distribution:
      energy_per_pod_j   – Joules per pod creation (CP test)
      cp_overhead_w      – CPU power overhead during CP test (W above idle)
      energy_per_op_mj   – milli-Joules per data-plane operation (DP test)
      cp_duration_s      – Seconds to create 120 pods (proxy for scalability)
      idle_power_w       – Idle power consumption (W)
    """
    path = _repo_root(root_dir) / "energy-analysis" / "results" / "energy_efficiency.csv"
    result: dict[str, dict] = {}
    with open(path, newline="") as f:
        for row in csv.DictReader(f):
            kd = row["kd"].strip()
            result[kd] = {
                "energy_per_pod_j":  float(row["energy_per_pod_j"]) if row["energy_per_pod_j"].strip() else None,
                "cp_overhead_w":     float(row["cp_overhead_w"]) if row["cp_overhead_w"].strip() else None,
                "energy_per_op_mj":  float(row["energy_per_op_mj"]) if row["energy_per_op_mj"].strip() else None,
                "cp_duration_s":     float(row["cp_duration_s_mean"]) if row["cp_duration_s_mean"].strip() else None,
                "idle_power_w":      float(row["idle_power_w_mean"]) if row["idle_power_w_mean"].strip() else None,
            }
    return result


def load_interrupt_amplification(root_dir: Optional[str] = None) -> dict[str, dict]:
    """
    Returns per-KD interrupt amplification factors from P4.

    Keys per distribution (using cp_heavy_12client test type):
      cp_amplification   – IRQ rate amplification factor under CP load
      dp_amplification   – IRQ rate amplification factor under DP load
    """
    path = _repo_root(root_dir) / "energy-analysis" / "results" / "interrupt_amplification.csv"
    result: dict[str, dict] = {}
    with open(path, newline="") as f:
        for row in csv.DictReader(f):
            kd = row["kd"].strip()
            test = row["test_type"].strip()
            if kd not in result:
                result[kd] = {}
            amp = float(row["amplification_factor"])
            if test == "cp_heavy_12client":
                result[kd]["cp_amplification"] = amp
            elif test == "dp_redis_density":
                result[kd]["dp_amplification"] = amp
    return result


def load_k0s_orchestration_tax(root_dir: Optional[str] = None) -> dict[str, dict]:
    """
    Returns k0s per-test-type orchestration tax from P5.

    Keys per test type:
      cpu_tax_pct   – System pod CPU overhead as % of total node capacity
      mem_tax_pct   – System pod memory overhead as % of total node capacity
    """
    path = _repo_root(root_dir) / "overhead-decomposition" / "results" / "k0s_orchestration_tax.csv"
    result: dict[str, dict] = {}
    with open(path, newline="") as f:
        for row in csv.DictReader(f):
            test = row["test_type"].strip()
            metric = row["metric"].strip()
            val = float(row["orchestration_tax_pct"])
            if test not in result:
                result[test] = {}
            if metric == "cpu_pct":
                result[test]["cpu_tax_pct"] = val
            elif metric == "mem_mib":
                result[test]["mem_tax_pct"] = val
    return result
