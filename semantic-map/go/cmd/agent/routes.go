package main

// HTTP surface served by the agent daemon.
//
// Endpoint                          Method  Body / Path                    Status on success
// ─────────────────────────────────────────────────────────────────────────────────────────
// /ingest                           POST    {from_id,to_id,observation,…}  204
// /cost                             GET     ?task=&node=                   200 ActionCost
// /recommend                        POST    OffloadContext                 200 PeerRecommendation
// /simulate                         POST    {context,target_node_id}       200 OutcomeSimulation
// /candidates                       GET     —                              200 []CandidateEdge
// ─────────────────────────────────────────────────────────────────────────────────────────
// /graph                            GET     —                              200 GraphSnapshot
// /edges                            GET     ?from=&to=                     200 []EdgeDTO
// /constructs                       GET     —                              200 []ConstructDTO
// /propositions                     GET     —                              200 []PropositionDTO
// /history                          GET     ?since=                        200 []OntologyEventDTO
// /neighbors                        GET     ?node=                         200 []string
// /healthz                          GET     —                              200 HealthResponse
// /version                          GET     —                              200 VersionResponse
// /ontology/strength                POST    SetStrengthRequest             204
// /ontology/deprecate               POST    DeprecateRequest               204
// /ontology/construct               POST    AddConstructRequest            204
// /ontology/proposition             POST    AddPropositionRequest          204
// /agent/reset                      POST    ResetRequest                   204
// /candidates/{id}/confirm          POST    —                              204
// /candidates/{id}/reject           POST    —                              204
// /candidates/{id}/defer            POST    —                              204
// /ui/...                           GET     —                              200 (embedded HTML)
//
// Endpoints above the divider are pre-existing and keep their original
// plain-text http.Error format. Endpoints below were added in the Phase 1
// HTTP-API expansion and emit JSON errors via writeError; their POST
// handlers gate on requireJSON for CSRF mitigation (path-only candidate
// endpoints excepted).

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/DiyazY/di-agent/pkg/semmap"
	"github.com/DiyazY/di-agent/pkg/types"
)

// registerRoutes wires every HTTP handler onto mux. It is the single entry
// point for the daemon's URL surface; main only constructs the mux, the
// SemanticMap, and the http.Server.
//
// Convention: the EXISTING endpoints (/ingest, /cost, /recommend, /simulate,
// /candidates) keep their original http.Error plain-text error format to
// minimize diff. Every NEW endpoint added in this expansion uses
// writeError to emit a JSON {"error":"..."} body, and every new POST
// handler calls requireJSON at the top as a lightweight CSRF mitigation.
func registerRoutes(mux *http.ServeMux, sm *semmap.SemanticMap) {
	registerExistingRoutes(mux, sm)
	registerReadRoutes(mux, sm)
	registerMutationRoutes(mux, sm)
	registerStaticRoutes(mux)
}

// registerStaticRoutes serves the embedded UI under /ui/.
//
// http.FileServer serves index.html for the directory root automatically
// when the URL ends in "/". An explicit "/ui/{$}" → "/ui/index.html"
// redirect would loop because http.FileServer canonicalizes URLs ending
// in /index.html back to "./" — so we rely on the default behavior.
func registerStaticRoutes(mux *http.ServeMux) {
	mux.Handle("GET /ui/", staticHandler())
}

// registerExistingRoutes preserves the original five endpoints unchanged.
// They are kept in their own function so the diff against pre-expansion
// behavior is obvious to reviewers.
func registerExistingRoutes(mux *http.ServeMux, sm *semmap.SemanticMap) {
	// POST /ingest  {"from_id":"SC","to_id":"RC","observation":0.7,"event_id":"evt-1"}
	mux.HandleFunc("POST /ingest", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			FromID      string  `json:"from_id"`
			ToID        string  `json:"to_id"`
			Observation float64 `json:"observation"`
			EventID     string  `json:"event_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := sm.Ingest(req.FromID, req.ToID, req.Observation, req.EventID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /cost?task=pod-scheduling&node=node_1
	mux.HandleFunc("GET /cost", func(w http.ResponseWriter, r *http.Request) {
		task := r.URL.Query().Get("task")
		node := r.URL.Query().Get("node")
		result, err := sm.CostOfAction(task, node)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, result)
	})

	// POST /recommend  {"task_type":"...","source_node_id":"...","data_size_bytes":1024,"latency_budget_ms":500}
	mux.HandleFunc("POST /recommend", func(w http.ResponseWriter, r *http.Request) {
		var ctx types.OffloadContext
		if err := json.NewDecoder(r.Body).Decode(&ctx); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result, err := sm.RecommendPeer(&ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, result)
	})

	// POST /simulate  {"context":{...},"target_node_id":"node_2"}
	mux.HandleFunc("POST /simulate", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Context      types.OffloadContext `json:"context"`
			TargetNodeID string               `json:"target_node_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result, err := sm.SimulateOutcome(&req.Context, req.TargetNodeID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, result)
	})

	// GET /candidates  — pending graph extension proposals
	mux.HandleFunc("GET /candidates", func(w http.ResponseWriter, r *http.Request) {
		candidates, err := sm.PendingCandidates()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, candidates)
	})
}

// ── helpers ───────────────────────────────────────────────────────────────────

// writeJSON serializes v as JSON with a 200 OK header. It is shared by both
// old and new endpoints because the success-path content type is the same.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON error: %v", err)
	}
}

// writeError emits a JSON-encoded error response with the given status code.
// Used by every new endpoint added in the HTTP expansion; existing endpoints
// keep plain-text http.Error for diff minimization.
func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(ErrorResponse{Error: msg}); err != nil {
		log.Printf("writeError encoding failure: %v", err)
	}
}

// registerReadRoutes wires the introspection endpoints: /graph, /edges,
// /constructs, /propositions, /history, /neighbors, /healthz, /version.
// These read-only endpoints never mutate state and emit JSON on both
// success and error paths.
func registerReadRoutes(mux *http.ServeMux, sm *semmap.SemanticMap) {
	mux.HandleFunc("GET /graph", func(w http.ResponseWriter, r *http.Request) {
		constructs, err := sm.Constructs()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		propositions, err := sm.Propositions()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		edges, err := sm.AllEdges()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		snap := GraphSnapshot{
			Constructs:   make([]ConstructDTO, 0, len(constructs)),
			Propositions: make([]PropositionDTO, 0, len(propositions)),
			Edges:        make([]EdgeDTO, 0, len(edges)),
		}
		for _, c := range constructs {
			snap.Constructs = append(snap.Constructs, constructToDTO(c))
		}
		for _, p := range propositions {
			snap.Propositions = append(snap.Propositions, propositionToDTO(p))
		}
		for _, e := range edges {
			snap.Edges = append(snap.Edges, edgeToDTO(e))
		}
		writeJSON(w, snap)
	})

	mux.HandleFunc("GET /edges", func(w http.ResponseWriter, r *http.Request) {
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")
		var (
			edges []*types.EdgeDescriptor
			err   error
		)
		switch {
		case from != "" && to != "":
			edges, err = sm.EdgesByPair(from, to)
		default:
			// Empty filter on either side -> return all and filter in-process
			// for the half-specified case. AllEdges is the common path.
			edges, err = sm.AllEdges()
			if err == nil && (from != "" || to != "") {
				filtered := edges[:0]
				for _, e := range edges {
					if from != "" && e.FromID != from {
						continue
					}
					if to != "" && e.ToID != to {
						continue
					}
					filtered = append(filtered, e)
				}
				edges = filtered
			}
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out := make([]EdgeDTO, 0, len(edges))
		for _, e := range edges {
			out = append(out, edgeToDTO(e))
		}
		writeJSON(w, out)
	})

	mux.HandleFunc("GET /constructs", func(w http.ResponseWriter, r *http.Request) {
		constructs, err := sm.Constructs()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out := make([]ConstructDTO, 0, len(constructs))
		for _, c := range constructs {
			out = append(out, constructToDTO(c))
		}
		writeJSON(w, out)
	})

	mux.HandleFunc("GET /propositions", func(w http.ResponseWriter, r *http.Request) {
		propositions, err := sm.Propositions()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out := make([]PropositionDTO, 0, len(propositions))
		for _, p := range propositions {
			out = append(out, propositionToDTO(p))
		}
		writeJSON(w, out)
	})

	mux.HandleFunc("GET /history", func(w http.ResponseWriter, r *http.Request) {
		since, err := parseSince(r.URL.Query().Get("since"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		events, err := sm.History(since)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out := make([]OntologyEventDTO, 0, len(events))
		for _, e := range events {
			out = append(out, eventToDTO(e))
		}
		writeJSON(w, out)
	})

	mux.HandleFunc("GET /neighbors", func(w http.ResponseWriter, r *http.Request) {
		node := r.URL.Query().Get("node")
		if node == "" {
			writeError(w, http.StatusBadRequest, "missing required query parameter: node")
			return
		}
		neighbors, err := sm.Neighbors(node)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if neighbors == nil {
			neighbors = []string{}
		}
		writeJSON(w, neighbors)
	})

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, HealthResponse{OK: true})
	})

	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		// Count constructs/propositions for operator visibility. Errors here
		// shouldn't fail /version — fall back to zero counts.
		var nC, nP int
		if cs, err := sm.Constructs(); err == nil {
			nC = len(cs)
		}
		if ps, err := sm.Propositions(); err == nil {
			nP = len(ps)
		}
		writeJSON(w, VersionResponse{
			AgentVersion:       Version,
			GoVersion:          runtime.Version(),
			BuildCommit:        BuildCommit,
			SemmapConstructs:   nC,
			SemmapPropositions: nP,
		})
	})
}

// registerMutationRoutes wires the ontology and edge mutation endpoints.
//
// Every body-bearing handler:
//  1. Calls requireJSON (CSRF mitigation: rejects non-application/json bodies)
//  2. Decodes the typed DTO from the body
//  3. Calls the facade and returns 204 No Content on success
//  4. Emits writeError on any failure
//
// The path-only candidate endpoints intentionally skip requireJSON because
// they take no body — the CSRF concern there is satisfied by the path
// parameter being non-guessable in practice (UUID-shaped candidate IDs).
func registerMutationRoutes(mux *http.ServeMux, sm *semmap.SemanticMap) {
	mux.HandleFunc("POST /ontology/strength", func(w http.ResponseWriter, r *http.Request) {
		if err := requireJSON(r); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		var req SetStrengthRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := sm.SetPropositionStrength(req.PropositionID, req.Strength); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /ontology/deprecate", func(w http.ResponseWriter, r *http.Request) {
		if err := requireJSON(r); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		var req DeprecateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := sm.Deprecate(req.PropositionID, req.Reason); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /ontology/construct", func(w http.ResponseWriter, r *http.Request) {
		if err := requireJSON(r); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		var req AddConstructRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		c := &types.Construct{
			ConstructID: req.ConstructID,
			Name:        req.Name,
			Description: req.Description,
		}
		if err := sm.AddConstruct(c); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /ontology/proposition", func(w http.ResponseWriter, r *http.Request) {
		if err := requireJSON(r); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		var req AddPropositionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		dir, ok := directionFromString(req.Direction)
		if !ok {
			writeError(w, http.StatusBadRequest, "direction must be \"+\" or \"-\"")
			return
		}
		p := &types.Proposition{
			PropositionID: req.PropositionID,
			FromConstruct: req.From,
			ToConstruct:   req.To,
			Direction:     dir,
			PriorStrength: req.PriorStrength,
		}
		if err := sm.AddValidatedProposition(p); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /agent/reset", func(w http.ResponseWriter, r *http.Request) {
		if err := requireJSON(r); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		var req ResetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := sm.ResetEdge(req.From, req.To); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Path-only candidate review actions. No body, no requireJSON.
	mux.HandleFunc("POST /candidates/{id}/confirm", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := sm.ConfirmCandidate(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /candidates/{id}/reject", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := sm.RejectCandidate(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /candidates/{id}/defer", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := sm.DeferCandidate(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// parseSince accepts an empty string (returns zero time), an RFC3339
// timestamp, or a Go duration (subtracted from time.Now). It is the shared
// parser for the ?since= query parameter on /history.
func parseSince(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		return time.Now().Add(-d), nil
	}
	return time.Time{}, errors.New("since must be RFC3339 timestamp or Go duration (e.g. 1h, 30m)")
}

// requireJSON returns an error when the request's Content-Type is not
// application/json. Called at the top of every new POST handler as a CSRF
// mitigation: browsers will not send Content-Type: application/json on
// simple cross-origin form submissions, so requiring it blocks naive CSRF.
// Path-only mutation endpoints (e.g. /candidates/{id}/confirm) skip this.
func requireJSON(r *http.Request) error {
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		return errors.New("Content-Type must be application/json")
	}
	return nil
}
