"""
Prior initialization pipeline entry point.

Usage:
    python -m semantic-map.prior_init.pipeline [--root ROOT] [--out OUT]

    --root  Path to the mega-research repo root (default: auto-detected)
    --out   Output path for prior_weights.json (default: semantic-map/prior_weights.json)

Outputs prior_weights.json with:
  - version + metadata
  - propositions: calibrated PriorStrength λ per proposition
  - distribution_construct_scores: normalised [0,1] construct scores per KD
  - distribution_edge_weights: per-distribution edge priors for storage seeding
"""

from __future__ import annotations

import argparse
import json
import sys
from datetime import date
from pathlib import Path

from .calibration import compute_construct_scores, compute_proposition_strengths, PROPOSITIONS, KDS


def build_edge_weights(
    construct_scores: dict[str, dict[str, float]],
    proposition_strengths: dict[str, dict],
) -> dict[str, dict[str, dict]]:
    """
    Build per-distribution edge descriptors for storage seeding.

    For each (distribution, proposition) pair, produce an edge descriptor:
      from_id        – source construct ID
      to_id          – target construct ID
      proposition_id – P1..P15
      direction      – positive | negative
      prior_weight   – distribution-specific source construct score
                       (how strongly this KD exhibits the source construct)
      ema_weight     – same as prior_weight at T=0 (no observations yet)
    """
    edges: dict[str, dict[str, dict]] = {}

    for kd in KDS:
        edges[kd] = {}
        for prop_id, from_c, to_c, direction in PROPOSITIONS:
            # Edge weight = source construct score for this distribution,
            # modulated by the proposition strength.
            source_score = construct_scores[kd][from_c]
            strength     = proposition_strengths[prop_id]["prior_weight"] = \
                           proposition_strengths[prop_id]["prior_strength"]
            prior        = round(source_score * strength, 4)

            edge_key = f"{from_c}→{to_c}:{prop_id}"
            edges[kd][edge_key] = {
                "from_id":        from_c,
                "to_id":          to_c,
                "proposition_id": prop_id,
                "direction":      direction,
                "prior_weight":   prior,
                "ema_weight":     prior,   # identical at cold-start
            }

    return edges


def run(root_dir: str | None = None, out_path: str | None = None) -> dict:
    """Execute the pipeline and return the output document."""
    construct_scores  = compute_construct_scores(root_dir)
    prop_strengths    = compute_proposition_strengths(construct_scores)
    edge_weights      = build_edge_weights(construct_scores, prop_strengths)

    # Summarise reversed propositions.  Overridden ones are noted separately.
    warnings = []
    for pid, v in prop_strengths.items():
        if not v["sign_consistent"]:
            tag = "(domain_override)" if v["method"] == "domain_override" else "(proxy reversal — investigate)"
            warnings.append(f"{pid} {tag}: ρ={v['spearman_rho']:.3f}")

    output = {
        "version":        "1.0",
        "generated_at":   str(date.today()),
        "evidence_papers": ["P1", "P2", "P4", "P5"],
        "distributions":  KDS,
        "warnings":       warnings,
        "propositions":   prop_strengths,
        "distribution_construct_scores":  construct_scores,
        "distribution_edge_weights":      edge_weights,
    }

    # Write output
    repo_root = Path(__file__).resolve().parents[2]
    default_out = repo_root / "semantic-map" / "prior_weights.json"
    target = Path(out_path) if out_path else default_out
    target.parent.mkdir(parents=True, exist_ok=True)
    with open(target, "w") as f:
        json.dump(output, f, indent=2)
    print(f"Wrote {target}")

    if warnings:
        print(f"\nWARNINGS ({len(warnings)}):")
        for w in warnings:
            print(f"  ⚠  {w}")

    return output


def main() -> None:
    parser = argparse.ArgumentParser(description="Semantic Map prior initialization pipeline")
    parser.add_argument("--root", default=None, help="Repo root directory")
    parser.add_argument("--out",  default=None, help="Output path for prior_weights.json")
    args = parser.parse_args()
    run(root_dir=args.root, out_path=args.out)


if __name__ == "__main__":
    main()
