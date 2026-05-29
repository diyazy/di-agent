package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DiyazY/di-agent/pkg/profiles"
	"github.com/DiyazY/di-agent/pkg/semmap"
)

// newTestAgent wires a SemanticMap via the edge-minimal profile and returns
// the base URL of an httptest server bound to that map. Tests use it to
// avoid duplicating the agent boot sequence.
func newTestAgent(t *testing.T) (baseURL string, sm *semmap.SemanticMap, cleanup func()) {
	t.Helper()
	sm, err := profiles.Build("edge-minimal", profiles.Config{
		EMAAlpha:             0.2,
		ConvergenceThreshold: 500,
		MinTrustScore:        0.5,
	})
	if err != nil {
		t.Fatalf("profiles.Build: %v", err)
	}
	mux := http.NewServeMux()
	registerRoutes(mux, sm)
	srv := httptest.NewServer(mux)
	return srv.URL, sm, srv.Close
}

// getJSON is a tiny helper that does a GET, asserts 200, and decodes JSON.
func getJSON(t *testing.T, url string, out any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s: status=%d body=%s", url, resp.StatusCode, string(body))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
}

func TestGetGraph_ReturnsSevenConstructsAndFifteenPropositions(t *testing.T) {
	base, _, cleanup := newTestAgent(t)
	defer cleanup()
	var snap GraphSnapshot
	getJSON(t, base+"/graph", &snap)
	if len(snap.Constructs) != 7 {
		t.Errorf("constructs: got %d, want 7", len(snap.Constructs))
	}
	if len(snap.Propositions) != 15 {
		t.Errorf("propositions: got %d, want 15", len(snap.Propositions))
	}
	if len(snap.Edges) != 15 {
		t.Errorf("edges: got %d, want 15", len(snap.Edges))
	}
}

func TestGetGraph_EncodesDirectionAsPlusMinus(t *testing.T) {
	base, _, cleanup := newTestAgent(t)
	defer cleanup()
	var snap GraphSnapshot
	getJSON(t, base+"/graph", &snap)
	for _, p := range snap.Propositions {
		if p.Direction != "+" && p.Direction != "-" {
			t.Errorf("proposition %s: direction=%q, want + or -", p.PropositionID, p.Direction)
		}
	}
	for _, e := range snap.Edges {
		if e.Direction != "+" && e.Direction != "-" {
			t.Errorf("edge %s: direction=%q, want + or -", e.PropositionID, e.Direction)
		}
	}
}

func TestGetEdges_NoFilterReturnsFifteen(t *testing.T) {
	base, _, cleanup := newTestAgent(t)
	defer cleanup()
	var edges []EdgeDTO
	getJSON(t, base+"/edges", &edges)
	if len(edges) != 15 {
		t.Errorf("/edges: got %d, want 15", len(edges))
	}
}

func TestGetEdges_FilterByFromTo_RC_PS_ReturnsBothP2AndP3(t *testing.T) {
	base, _, cleanup := newTestAgent(t)
	defer cleanup()
	var edges []EdgeDTO
	getJSON(t, base+"/edges?from=RC&to=PS", &edges)
	if len(edges) != 2 {
		t.Fatalf("RC→PS pair: got %d edges, want 2 (P2 and P3)", len(edges))
	}
	seen := map[string]bool{}
	for _, e := range edges {
		seen[e.PropositionID] = true
	}
	if !seen["P2"] || !seen["P3"] {
		t.Errorf("expected P2 and P3 in RC→PS pair; got %v", seen)
	}
}

func TestGetHistory_RespectsRFC3339Since(t *testing.T) {
	base, sm, cleanup := newTestAgent(t)
	defer cleanup()
	// Future timestamp should return zero events.
	future := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	var events []OntologyEventDTO
	getJSON(t, base+"/history?since="+future, &events)
	if len(events) != 0 {
		t.Errorf("future since: got %d events, want 0", len(events))
	}
	// Trigger a mutation, then query with zero time — must include it.
	if err := sm.SetPropositionStrength("P1", 0.77); err != nil {
		t.Fatal(err)
	}
	getJSON(t, base+"/history", &events)
	if len(events) != 1 {
		t.Errorf("after one mutation: got %d events, want 1", len(events))
	}
}

func TestGetHistory_RespectsDurationSince(t *testing.T) {
	base, sm, cleanup := newTestAgent(t)
	defer cleanup()
	if err := sm.SetPropositionStrength("P1", 0.5); err != nil {
		t.Fatal(err)
	}
	var events []OntologyEventDTO
	getJSON(t, base+"/history?since=1h", &events)
	if len(events) != 1 {
		t.Errorf("since=1h: got %d events, want 1", len(events))
	}
}

func TestHealthz_OK(t *testing.T) {
	base, _, cleanup := newTestAgent(t)
	defer cleanup()
	var h HealthResponse
	getJSON(t, base+"/healthz", &h)
	if !h.OK {
		t.Error("/healthz returned ok=false")
	}
}

func TestVersion_ReturnsStructWithCounts(t *testing.T) {
	base, _, cleanup := newTestAgent(t)
	defer cleanup()
	var v VersionResponse
	getJSON(t, base+"/version", &v)
	if v.AgentVersion == "" {
		t.Error("agent_version empty")
	}
	if v.GoVersion == "" {
		t.Error("go_version empty")
	}
	if v.SemmapConstructs != 7 {
		t.Errorf("semmap_constructs: got %d, want 7", v.SemmapConstructs)
	}
	if v.SemmapPropositions != 15 {
		t.Errorf("semmap_propositions: got %d, want 15", v.SemmapPropositions)
	}
}

func TestGetNeighbors_ReturnsTargetConstructs(t *testing.T) {
	base, _, cleanup := newTestAgent(t)
	defer cleanup()
	// SC has propositions P1 (→RC), P4 (→RR), P12 (→MU).
	var neighbors []string
	getJSON(t, base+"/neighbors?node=SC", &neighbors)
	got := map[string]bool{}
	for _, n := range neighbors {
		got[n] = true
	}
	for _, want := range []string{"RC", "RR", "MU"} {
		if !got[want] {
			t.Errorf("SC neighbors missing %s; got %v", want, neighbors)
		}
	}
}

// ── shared post helper (used by mutation tests added in step 1.7) ────────────

// postJSON does a POST with Content-Type: application/json and returns the
// response. Tests assert status and decode the body themselves.
func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// postRaw posts a raw body without setting Content-Type. Used to exercise
// requireJSON's CSRF guard.
func postRaw(t *testing.T, url, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", url, strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// Pretty-print helper used in failure messages.
func body(resp *http.Response) string {
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return fmt.Sprintf("status=%d body=%s", resp.StatusCode, string(b))
}
