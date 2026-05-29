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

// ── Mutation tests ───────────────────────────────────────────────────────────

func TestSetStrength_UpdatesAndAppearsInHistory(t *testing.T) {
	base, _, cleanup := newTestAgent(t)
	defer cleanup()
	resp := postJSON(t, base+"/ontology/strength", SetStrengthRequest{
		PropositionID: "P1",
		Strength:      0.95,
	})
	if resp.StatusCode != 204 {
		t.Fatalf("set strength: %s", body(resp))
	}
	resp.Body.Close()

	var props []PropositionDTO
	getJSON(t, base+"/propositions", &props)
	for _, p := range props {
		if p.PropositionID == "P1" {
			if p.PriorStrength != 0.95 {
				t.Errorf("P1 strength after set: got %.3f, want 0.95", p.PriorStrength)
			}
		}
	}

	var events []OntologyEventDTO
	getJSON(t, base+"/history", &events)
	if len(events) != 1 {
		t.Fatalf("history: got %d events, want 1", len(events))
	}
	if events[0].Kind != "proposition_strength_set" {
		t.Errorf("event kind: got %q, want proposition_strength_set", events[0].Kind)
	}
	if events[0].TargetID != "P1" {
		t.Errorf("event target: got %q, want P1", events[0].TargetID)
	}
}

func TestDeprecate_FlagsPropositionAndReasonerSkipsIt(t *testing.T) {
	base, sm, cleanup := newTestAgent(t)
	defer cleanup()

	before, err := sm.CostOfAction("pod-scheduling", "node_1")
	if err != nil {
		t.Fatal(err)
	}

	resp := postJSON(t, base+"/ontology/deprecate", DeprecateRequest{
		PropositionID: "P1",
		Reason:        "http test",
	})
	if resp.StatusCode != 204 {
		t.Fatalf("deprecate: %s", body(resp))
	}
	resp.Body.Close()

	var props []PropositionDTO
	getJSON(t, base+"/propositions", &props)
	var p1 *PropositionDTO
	for i := range props {
		if props[i].PropositionID == "P1" {
			p1 = &props[i]
		}
	}
	if p1 == nil {
		t.Fatal("P1 disappeared from /propositions after deprecate (must be soft-delete)")
	}
	if !p1.Deprecated {
		t.Error("P1 not flagged deprecated after POST /ontology/deprecate")
	}
	if p1.DeprecatedReason != "http test" {
		t.Errorf("P1 deprecated_reason: got %q, want \"http test\"", p1.DeprecatedReason)
	}

	after, err := sm.CostOfAction("pod-scheduling", "node_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(after.GraphPathUsed) != len(before.GraphPathUsed)-1 {
		t.Errorf("graph path should shrink by 1 after deprecate; got %d → %d",
			len(before.GraphPathUsed), len(after.GraphPathUsed))
	}
}

func TestAddConstruct_AppearsInConstructs(t *testing.T) {
	base, _, cleanup := newTestAgent(t)
	defer cleanup()
	resp := postJSON(t, base+"/ontology/construct", AddConstructRequest{
		ConstructID: "PR",
		Name:        "Privacy & Data Sovereignty",
		Description: "added by http test",
	})
	if resp.StatusCode != 204 {
		t.Fatalf("add construct: %s", body(resp))
	}
	resp.Body.Close()

	var constructs []ConstructDTO
	getJSON(t, base+"/constructs", &constructs)
	found := false
	for _, c := range constructs {
		if c.ConstructID == "PR" {
			found = true
			if c.Name != "Privacy & Data Sovereignty" {
				t.Errorf("PR name: got %q", c.Name)
			}
		}
	}
	if !found {
		t.Error("PR construct not found after POST /ontology/construct")
	}
}

func TestAddValidatedProposition_AppearsInPropositions(t *testing.T) {
	base, _, cleanup := newTestAgent(t)
	defer cleanup()
	// PS→SC is not in the Di-Select bootstrap; safe to add.
	resp := postJSON(t, base+"/ontology/proposition", AddPropositionRequest{
		PropositionID: "P-http",
		From:          "PS",
		To:            "SC",
		Direction:     "+",
		PriorStrength: 0.42,
	})
	if resp.StatusCode != 204 {
		t.Fatalf("add proposition: %s", body(resp))
	}
	resp.Body.Close()

	var props []PropositionDTO
	getJSON(t, base+"/propositions", &props)
	for _, p := range props {
		if p.PropositionID == "P-http" {
			if p.FromConstruct != "PS" || p.ToConstruct != "SC" {
				t.Errorf("P-http endpoints: from=%s to=%s", p.FromConstruct, p.ToConstruct)
			}
			if p.Direction != "+" {
				t.Errorf("P-http direction: got %q, want +", p.Direction)
			}
			return
		}
	}
	t.Error("P-http proposition not found after POST /ontology/proposition")
}

func TestResetEdge_RestoresPriorAfterUpdates(t *testing.T) {
	base, sm, cleanup := newTestAgent(t)
	defer cleanup()

	// Stream a few observations into PS→RC to move EMA off prior.
	for i := 0; i < 50; i++ {
		if err := sm.Ingest("PS", "RC", 0.9, fmt.Sprintf("evt-%d", i)); err != nil {
			t.Fatal(err)
		}
	}

	// Reset via HTTP.
	resp := postJSON(t, base+"/agent/reset", ResetRequest{From: "PS", To: "RC"})
	if resp.StatusCode != 204 {
		t.Fatalf("reset: %s", body(resp))
	}
	resp.Body.Close()

	var edges []EdgeDTO
	getJSON(t, base+"/edges?from=PS&to=RC", &edges)
	if len(edges) == 0 {
		t.Fatal("no PS→RC edges returned after reset")
	}
	for _, e := range edges {
		if e.EMAWeight != e.PriorWeight {
			t.Errorf("edge %s: EMA=%.3f != prior=%.3f after reset", e.PropositionID, e.EMAWeight, e.PriorWeight)
		}
		if e.NObservations != 0 {
			t.Errorf("edge %s: n_observations=%d after reset; want 0", e.PropositionID, e.NObservations)
		}
		if e.Confidence != 0.0 {
			t.Errorf("edge %s: confidence=%.3f after reset; want 0.0", e.PropositionID, e.Confidence)
		}
	}
}

func TestPostWithoutJSONContentType_Returns400(t *testing.T) {
	base, _, cleanup := newTestAgent(t)
	defer cleanup()
	resp := postRaw(t, base+"/ontology/strength", `{"proposition_id":"P1","strength":0.5}`)
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("missing Content-Type: got %d, want 400", resp.StatusCode)
	}
	var er ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if !strings.Contains(er.Error, "application/json") {
		t.Errorf("error message %q should mention application/json", er.Error)
	}
}

func TestStaticUI_ServesPlaceholder(t *testing.T) {
	base, _, cleanup := newTestAgent(t)
	defer cleanup()
	resp, err := http.Get(base + "/ui/placeholder.html")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("placeholder: status %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), "replaced in Phase 2B") {
		t.Errorf("placeholder body should contain marker; got %q", string(b))
	}
}

func TestStaticUI_RootServesIndex(t *testing.T) {
	// http.FileServer serves the directory's index.html for "/" requests
	// directly (200 with body), without a redirect. We do NOT install an
	// explicit /ui/{$} → /ui/index.html redirect because that would loop
	// with the stdlib's /index.html → ./ canonicalization.
	base, _, cleanup := newTestAgent(t)
	defer cleanup()
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(base + "/ui/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("/ui/: status %d, want 200", resp.StatusCode)
	}
}

func TestSetStrength_UnknownProposition_Returns500WithErrorJSON(t *testing.T) {
	base, _, cleanup := newTestAgent(t)
	defer cleanup()
	resp := postJSON(t, base+"/ontology/strength", SetStrengthRequest{
		PropositionID: "P-nonexistent",
		Strength:      0.5,
	})
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Errorf("unknown prop: got %d, want 500", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("error Content-Type: got %q, want application/json", ct)
	}
	var er ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if er.Error == "" {
		t.Error("error body empty")
	}
}
