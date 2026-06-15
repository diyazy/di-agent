package minimal_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DiyazY/di-agent/internal/minimal"
	"github.com/DiyazY/di-agent/pkg/types"
)

// buildNetdataTestServer returns a test server that responds with canned
// Netdata v1 JSON for system.cpu, system.ram, and system.net.
func buildNetdataTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("chart") {
		case "system.cpu":
			fmt.Fprint(w, `{"labels":["time","user","system","idle"],"data":[[1703123456,2.5,0.5,96.9]]}`)
		case "system.ram":
			fmt.Fprint(w, `{"labels":["time","free","used","cached","buffers"],"data":[[1703123456,4096.0,2048.0,1024.0,512.0]]}`)
		case "system.net":
			fmt.Fprint(w, `{"labels":["time","InOctets","OutOctets"],"data":[[1703123456,8.0,-6.0]]}`)
		default:
			http.NotFound(w, r)
		}
	})
	return httptest.NewServer(mux)
}

func TestNetdataCollector_SourceID(t *testing.T) {
	srv := buildNetdataTestServer(t)
	defer srv.Close()

	c := minimal.NewNetdataCollector("test-node", srv.URL, nil)
	if got := c.SourceID(); got != "netdata:test-node" {
		t.Errorf("SourceID() = %q; want %q", got, "netdata:test-node")
	}
}

func TestNetdataCollector_AvailableMetrics(t *testing.T) {
	c := minimal.NewNetdataCollector("test-node", "http://unused", nil)
	metrics := c.AvailableMetrics()
	want := []types.MetricType{
		types.CPUUtilization,
		types.MemoryUtilization,
		types.NetworkRxBps,
		types.NetworkTxBps,
	}
	if len(metrics) != len(want) {
		t.Fatalf("AvailableMetrics() len = %d; want %d", len(metrics), len(want))
	}
	for i, mt := range want {
		if metrics[i] != mt {
			t.Errorf("AvailableMetrics()[%d] = %q; want %q", i, metrics[i], mt)
		}
	}
}

func TestNetdataCollector_CPU(t *testing.T) {
	srv := buildNetdataTestServer(t)
	defer srv.Close()

	c := minimal.NewNetdataCollector("test-node", srv.URL, nil)
	samples, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect() error: %v", err)
	}

	var cpuSample *types.MetricSample
	for _, s := range samples {
		if s.MetricType == types.CPUUtilization {
			cpuSample = s
			break
		}
	}
	if cpuSample == nil {
		t.Fatal("no CPUUtilization sample returned")
	}
	// idle=96.9 → util = 1 - 96.9/100 = 0.031
	want := 1.0 - 96.9/100.0
	const eps = 1e-9
	if diff := cpuSample.Value - want; diff > eps || diff < -eps {
		t.Errorf("CPUUtilization = %.6f; want %.6f (idle=96.9)", cpuSample.Value, want)
	}
}

func TestNetdataCollector_RAM(t *testing.T) {
	srv := buildNetdataTestServer(t)
	defer srv.Close()

	c := minimal.NewNetdataCollector("test-node", srv.URL, nil)
	samples, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect() error: %v", err)
	}

	var ramSample *types.MetricSample
	for _, s := range samples {
		if s.MetricType == types.MemoryUtilization {
			ramSample = s
			break
		}
	}
	if ramSample == nil {
		t.Fatal("no MemoryUtilization sample returned")
	}
	// used=2048, free=4096, cached=1024, buffers=512 → sum=7680, util=2048/7680≈0.2667
	sum := 2048.0 + 4096.0 + 1024.0 + 512.0
	want := 2048.0 / sum
	const eps = 1e-9
	if diff := ramSample.Value - want; diff > eps || diff < -eps {
		t.Errorf("MemoryUtilization = %.6f; want %.6f", ramSample.Value, want)
	}
}

func TestNetdataCollector_Network(t *testing.T) {
	srv := buildNetdataTestServer(t)
	defer srv.Close()

	c := minimal.NewNetdataCollector("test-node", srv.URL, nil)
	samples, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect() error: %v", err)
	}

	var rxSample, txSample *types.MetricSample
	for _, s := range samples {
		switch s.MetricType {
		case types.NetworkRxBps:
			rxSample = s
		case types.NetworkTxBps:
			txSample = s
		}
	}

	if rxSample == nil {
		t.Fatal("no NetworkRxBps sample returned")
	}
	// InOctets=8.0 kilobits/s → 8.0 * 125 / 125_000_000 = 0.000008 (normalized [0,1])
	wantRx := 8.0 * 125.0 / 125_000_000.0
	if rxSample.Value != wantRx {
		t.Errorf("NetworkRxBps = %v; want %v", rxSample.Value, wantRx)
	}

	if txSample == nil {
		t.Fatal("no NetworkTxBps sample returned")
	}
	// OutOctets=-6.0 kilobits/s → abs(-6.0) * 125 / 125_000_000 = 0.000006
	wantTx := 6.0 * 125.0 / 125_000_000.0
	if txSample.Value != wantTx {
		t.Errorf("NetworkTxBps = %v; want %v", txSample.Value, wantTx)
	}
}

func TestNetdataCollector_HTTPError(t *testing.T) {
	// Server that always returns 500.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := minimal.NewNetdataCollector("test-node", srv.URL, nil)
	samples, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect() must not return an error on HTTP 500; got: %v", err)
	}
	if len(samples) != 0 {
		t.Errorf("expected empty slice on HTTP 500; got %d samples", len(samples))
	}
}

func TestNetdataCollector_EventIDDeterminism(t *testing.T) {
	srv := buildNetdataTestServer(t)
	defer srv.Close()

	c := minimal.NewNetdataCollector("test-node", srv.URL, nil)

	first, err := c.Collect()
	if err != nil {
		t.Fatalf("first Collect() error: %v", err)
	}
	second, err := c.Collect()
	if err != nil {
		t.Fatalf("second Collect() error: %v", err)
	}

	// Build maps by MetricType → EventID from both calls.
	firstMap := make(map[types.MetricType]string)
	for _, s := range first {
		firstMap[s.MetricType] = s.EventID
	}
	secondMap := make(map[types.MetricType]string)
	for _, s := range second {
		secondMap[s.MetricType] = s.EventID
	}

	for mt, eid := range firstMap {
		if eid2, ok := secondMap[mt]; ok {
			if eid != eid2 {
				t.Errorf("EventID for %s changed between calls: %q → %q", mt, eid, eid2)
			}
		}
	}
}
