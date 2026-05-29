package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

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
