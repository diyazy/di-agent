package compare

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
)

// FormatTable renders the human-readable per-edge × per-KD divergence
// table. The columns are: PropID, Edge (From→To with direction sign),
// Prior, one column per KD (Effective value), Range.
//
// Below the table we print:
//   - count of edges whose range > divergenceThreshold (default 0.05)
//   - per-KD convergence summary (edges at confidence >= 0.9)
//   - top 3 most divergent edges with full per-KD breakdowns
//   - bridge boundary count (edges with n_observations > 0 in every KD)
//
// Numbers are right-aligned with 3 decimal places using text/tabwriter.
func FormatTable(w io.Writer, r *Result) error {
	if r == nil {
		return fmt.Errorf("nil result")
	}

	runLabel := strconv.Itoa(r.Options.Run)
	if r.Options.Run == 0 {
		runLabel = "avg(1..5)"
	}
	fmt.Fprintf(w, "=== compare: test=%s, run=%s, %d KDs ===\n\n",
		r.Options.TestType, runLabel, len(r.Options.KDs))

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)

	// Header: left-justify PropID and Edge, right-justify numeric columns.
	header := []string{"PropID", "Edge", "Prior"}
	header = append(header, r.Options.KDs...)
	header = append(header, "Range")
	// First two columns are text — pad with a tab on each side; tabwriter
	// will infer left-alignment for the leading non-numeric columns.
	fmt.Fprintln(tw, strings.Join(header, "\t"))

	// Body sorted by PropositionID for stable left-to-right reading. The
	// Divergence slice is sorted by Range desc, so reindex by propID here.
	idx := map[string]*EdgeDivergence{}
	for _, d := range r.Divergence {
		idx[d.PropositionID] = d
	}
	// Iterate PropIDs in numeric order.
	ordered := append([]*EdgeDivergence(nil), r.Divergence...)
	// Re-sort ordered ascending by propID for body printout.
	sortByPropID(ordered)

	for _, d := range ordered {
		row := []string{
			d.PropositionID,
			fmt.Sprintf("%s→%s(%s)", d.From, d.To, d.Direction),
			fmt.Sprintf("%.3f", d.PriorWeight),
		}
		for _, eff := range d.Effective {
			row = append(row, fmt.Sprintf("%.3f", eff))
		}
		row = append(row, fmt.Sprintf("%.3f", d.Range))
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	// Summary lines.
	fmt.Fprintln(w)
	const divThreshold = 0.05
	diverged := 0
	for _, d := range r.Divergence {
		if d.Range > divThreshold {
			diverged++
		}
	}
	fmt.Fprintf(w, "Edges that diverged (range > %.2f):  %d of %d\n",
		divThreshold, diverged, len(r.Divergence))

	// Per-KD convergence summary: edges at confidence >= 0.9.
	fmt.Fprint(w, "Convergence summary (confidence >= 0.9):")
	for i, kd := range r.Options.KDs {
		converged := 0
		total := 0
		for _, d := range r.Divergence {
			if i < len(d.Confidence) {
				total++
				if d.Confidence[i] >= 0.9 {
					converged++
				}
			}
		}
		fmt.Fprintf(w, "  %s=%d/%d", kd, converged, total)
	}
	fmt.Fprintln(w)

	// Top N most divergent edges (max 3).
	top := TopN(r.Divergence, 3)
	if len(top) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Top 3 most divergent edges:")
		for _, d := range top {
			parts := make([]string, 0, len(d.Effective))
			for i, kd := range r.Options.KDs {
				if i < len(d.Effective) {
					parts = append(parts, fmt.Sprintf("%s=%.3f", kd, d.Effective[i]))
				}
			}
			fmt.Fprintf(w, "  %s  %s→%s(%s)  range=%.3f   %s\n",
				d.PropositionID, d.From, d.To, d.Direction, d.Range, strings.Join(parts, "  "))
		}
	}

	// Bridge boundary: edges with n_observations > 0 in every KD.
	allObserved := 0
	for _, d := range r.Divergence {
		allHit := true
		for _, n := range d.NObservations {
			if n == 0 {
				allHit = false
				break
			}
		}
		if allHit {
			allObserved++
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Bridge boundary: %d of %d edges had n_observations > 0 in all KDs.\n",
		allObserved, len(r.Divergence))
	return nil
}

// FormatJSON writes a stable JSON representation of the Result. The
// shape is the public artifact the dissertation cites; field names are
// snake_case for compatibility with downstream notebook tooling.
func FormatJSON(w io.Writer, r *Result) error {
	doc := jsonDoc{
		Options: jsonOptions{
			Test:             r.Options.TestType,
			Run:              r.Options.Run,
			KDs:              r.Options.KDs,
			DataDir:          r.Options.DataDir,
			PriorWeightsPath: r.Options.PriorWeightsPath,
			NodeFilter:       r.Options.NodeFilter,
		},
	}

	for _, p := range r.PerKD {
		edges := make([]jsonEdge, 0, len(p.Edges))
		for _, e := range p.Edges {
			edges = append(edges, jsonEdge{
				PropositionID: e.PropositionID,
				From:          e.FromID,
				To:            e.ToID,
				Direction:     directionString(e.Direction),
				PriorWeight:   e.PriorWeight,
				EMAWeight:     e.EMAWeight,
				Confidence:    e.Confidence,
				NObservations: e.NObservations,
			})
		}
		doc.PerKD = append(doc.PerKD, jsonPerKD{
			KD:             p.KD,
			SamplesSent:    p.SamplesSent,
			SamplesSkipped: p.SamplesSkipped,
			DurationMS:     p.DurationMS,
			Edges:          edges,
		})
	}

	for _, d := range r.Divergence {
		doc.Divergence = append(doc.Divergence, jsonDivergence{
			PropositionID: d.PropositionID,
			From:          d.From,
			To:            d.To,
			Direction:     d.Direction,
			PriorWeight:   d.PriorWeight,
			Effective:     d.Effective,
			EMA:           d.EMA,
			Confidence:    d.Confidence,
			NObservations: d.NObservations,
			Range:         d.Range,
			StdDev:        d.StdDev,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// FormatCSV writes a long-format CSV (one row per KD × edge) ready to
// import into pandas / R. Header row is fixed; column order matches the
// briefing.
func FormatCSV(w io.Writer, r *Result) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	if err := cw.Write([]string{
		"kd", "test", "run", "proposition_id", "from", "to", "direction",
		"prior_strength", "ema_weight", "confidence", "n_observations",
		"effective",
	}); err != nil {
		return err
	}

	runStr := strconv.Itoa(r.Options.Run)
	if r.Options.Run == 0 {
		runStr = "avg"
	}

	for i, p := range r.PerKD {
		_ = i
		for _, e := range p.Edges {
			eff := (1.0-e.Confidence)*e.PriorWeight + e.Confidence*e.EMAWeight
			row := []string{
				p.KD,
				r.Options.TestType,
				runStr,
				e.PropositionID,
				e.FromID,
				e.ToID,
				directionString(e.Direction),
				fmt.Sprintf("%.6f", e.PriorWeight),
				fmt.Sprintf("%.6f", e.EMAWeight),
				fmt.Sprintf("%.6f", e.Confidence),
				strconv.Itoa(e.NObservations),
				fmt.Sprintf("%.6f", eff),
			}
			if err := cw.Write(row); err != nil {
				return err
			}
		}
	}
	return cw.Error()
}

// sortByPropID orders divergence entries ascending by P1, P2, ..., P15.
func sortByPropID(div []*EdgeDivergence) {
	// Stable insertion-sort is fine here (n=15 in v1).
	for i := 1; i < len(div); i++ {
		for j := i; j > 0 && propIDLess(div[j].PropositionID, div[j-1].PropositionID); j-- {
			div[j], div[j-1] = div[j-1], div[j]
		}
	}
}

// ── JSON wire types ───────────────────────────────────────────────────────

type jsonDoc struct {
	Options    jsonOptions      `json:"options"`
	PerKD      []jsonPerKD      `json:"per_kd"`
	Divergence []jsonDivergence `json:"divergence"`
}

type jsonOptions struct {
	Test             string   `json:"test"`
	Run              int      `json:"run"`
	KDs              []string `json:"kds"`
	DataDir          string   `json:"data_dir"`
	PriorWeightsPath string   `json:"prior_weights_path,omitempty"`
	NodeFilter       []string `json:"node_filter,omitempty"`
}

type jsonPerKD struct {
	KD             string     `json:"kd"`
	SamplesSent    int        `json:"samples_sent"`
	SamplesSkipped int        `json:"samples_skipped"`
	DurationMS     int64      `json:"duration_ms"`
	Edges          []jsonEdge `json:"edges"`
}

type jsonEdge struct {
	PropositionID string  `json:"proposition_id"`
	From          string  `json:"from"`
	To            string  `json:"to"`
	Direction     string  `json:"direction"`
	PriorWeight   float64 `json:"prior_weight"`
	EMAWeight     float64 `json:"ema_weight"`
	Confidence    float64 `json:"confidence"`
	NObservations int     `json:"n_observations"`
}

type jsonDivergence struct {
	PropositionID string    `json:"proposition_id"`
	From          string    `json:"from"`
	To            string    `json:"to"`
	Direction     string    `json:"direction"`
	PriorWeight   float64   `json:"prior_weight"`
	Effective     []float64 `json:"effective"`
	EMA           []float64 `json:"ema"`
	Confidence    []float64 `json:"confidence"`
	NObservations []int     `json:"n_observations"`
	Range         float64   `json:"range"`
	StdDev        float64   `json:"std_dev"`
}
