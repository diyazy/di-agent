package minimal

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/DiyazY/di-agent/pkg/types"
)

// NetdataCollector is a CollectorContract implementation that polls the
// Netdata HTTP API v1 for live node metrics (system.cpu, system.ram,
// system.net) and normalizes them into MetricSamples.
//
// HTTP errors and non-200 responses from individual charts are treated as
// transient unavailability — the chart is skipped and other charts continue.
// Collect() only returns a non-nil error when an unexpected internal failure
// occurs; the caller must treat (nil, nil) as "no data available right now".
type NetdataCollector struct {
	nodeID     string
	baseURL    string
	httpClient *http.Client
	sid        string
}

var netdataAvailMetrics = []types.MetricType{
	types.CPUUtilization,
	types.MemoryUtilization,
	types.NetworkRxBps,
	types.NetworkTxBps,
}

// NewNetdataCollector creates a collector that polls the Netdata daemon at
// baseURL (e.g. "http://localhost:19999") for the given node.
//
// If httpClient is nil, a client with a 5-second timeout is used.
func NewNetdataCollector(nodeID, baseURL string, httpClient *http.Client) *NetdataCollector {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &NetdataCollector{
		nodeID:     nodeID,
		baseURL:    baseURL,
		httpClient: httpClient,
		sid:        "netdata:" + nodeID,
	}
}

func (n *NetdataCollector) SourceID() string                    { return n.sid }
func (n *NetdataCollector) AvailableMetrics() []types.MetricType { return netdataAvailMetrics }

// Collect polls all configured Netdata charts and returns normalized samples.
// Charts that are unavailable or return errors are silently skipped.
func (n *NetdataCollector) Collect() ([]*types.MetricSample, error) {
	var out []*types.MetricSample

	// ── system.cpu ───────────────────────────────────────────────────────────
	if resp, ok := n.fetchChart("system.cpu"); ok {
		if val, ts, found := dimValue(resp, "idle"); found {
			util := 1.0 - val/100.0
			if util < 0 {
				util = 0
			}
			if util > 1 {
				util = 1
			}
			out = append(out, n.sample(types.CPUUtilization, util, ts))
		}
	}

	// ── system.ram ───────────────────────────────────────────────────────────
	if resp, ok := n.fetchChart("system.ram"); ok {
		used, _, uOk := dimValue(resp, "used")
		free, _, fOk := dimValue(resp, "free")
		cached, _, cOk := dimValue(resp, "cached")
		buffers, ts, bOk := dimValue(resp, "buffers")
		if uOk && fOk && cOk && bOk {
			sum := used + free + cached + buffers
			if sum > 0 {
				util := used / sum
				if util < 0 {
					util = 0
				}
				if util > 1 {
					util = 1
				}
				out = append(out, n.sample(types.MemoryUtilization, util, ts))
			}
		}
	}

	// ── system.net ───────────────────────────────────────────────────────────
	if resp, ok := n.fetchChart("system.net"); ok {
		if inOctets, ts, found := dimValue(resp, "InOctets"); found {
			rx := inOctets * 125.0
			if rx < 0 {
				rx = 0
			}
			out = append(out, n.sample(types.NetworkRxBps, rx, ts))
		}
		if outOctets, ts, found := dimValue(resp, "OutOctets"); found {
			tx := outOctets
			if tx < 0 {
				tx = -tx
			}
			tx *= 125.0
			out = append(out, n.sample(types.NetworkTxBps, tx, ts))
		}
	}

	return out, nil
}

// fetchChart calls GET <baseURL>/api/v1/data?chart=CHART&points=1&after=-30&format=json.
// Returns (parsed response, true) on success, (nil, false) on any error or non-200 status.
func (n *NetdataCollector) fetchChart(chart string) (*netdataResponse, bool) {
	url := fmt.Sprintf("%s/api/v1/data?chart=%s&points=1&after=-30&format=json", n.baseURL, chart)
	resp, err := n.httpClient.Get(url)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	var nd netdataResponse
	if err := json.NewDecoder(resp.Body).Decode(&nd); err != nil {
		return nil, false
	}
	return &nd, true
}

// sample builds a MetricSample with a deterministic EventID.
// The key includes the source ID, nodeID, metric type, and the Netdata
// timestamp — so the same physical observation always produces the same EventID.
func (n *NetdataCollector) sample(mt types.MetricType, value float64, ts int64) *types.MetricSample {
	key := fmt.Sprintf("%s:%s:%s:%d", n.sid, n.nodeID, string(mt), ts)
	h := sha256.Sum256([]byte(key))
	eid := fmt.Sprintf("%x", h[:8])

	return &types.MetricSample{
		NodeID:        n.nodeID,
		MetricType:    mt,
		Value:         value,
		TimestampUnix: ts,
		EventID:       eid,
	}
}

// ── Netdata API response types ────────────────────────────────────────────────

type netdataResponse struct {
	Result struct {
		Labels []string    `json:"labels"`
		Data   [][]float64 `json:"data"`
	} `json:"result"`
}

// dimValue extracts the value and Unix timestamp for the named dimension from
// a Netdata v1 data response. labels[0] is always "time" and is skipped.
// Returns (value, timestamp, true) on success; (0, 0, false) if not found.
func dimValue(resp *netdataResponse, name string) (float64, int64, bool) {
	for i, label := range resp.Result.Labels {
		if i == 0 {
			continue // skip "time"
		}
		if label == name {
			if len(resp.Result.Data) == 0 || i >= len(resp.Result.Data[0]) {
				return 0, 0, false
			}
			ts := int64(resp.Result.Data[0][0])
			return resp.Result.Data[0][i], ts, true
		}
	}
	return 0, 0, false
}
