// Command agent is the edge-minimal Semantic Map daemon.
//
// It loads the configured profile, seeds the graph from Di-Select priors,
// and serves the three agent queries over HTTP/JSON on :8080.
// Telemetry is accepted via POST /ingest.
//
// Usage:
//
//	agent -profile edge-minimal -addr :8080 -alpha 0.2 -convergence 500 \
//	      -priors /path/to/prior_weights.json -kd k0s
//
// The -kd flag selects per-distribution edge weights from prior_weights.json
// when set. Omit it (or pass an empty string) to use the global Di-Select
// proposition strengths.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/DiyazY/di-agent/pkg/profiles"
	"github.com/DiyazY/di-agent/pkg/semmap"
	"github.com/DiyazY/di-agent/pkg/types"
)

func main() {
	profileName  := flag.String("profile", "edge-minimal", "deployment profile")
	addr         := flag.String("addr", ":8080", "HTTP listen address")
	alpha        := flag.Float64("alpha", 0.2, "EMA decay factor (0 < alpha < 1)")
	convergence  := flag.Float64("convergence", 500, "observations for confidence=1.0")
	minTrust     := flag.Float64("min-trust", 0.5, "minimum peer trust score")
	priorsPath   := flag.String("priors", "", "path to prior_weights.json from initialization pipeline")
	kd           := flag.String("kd", "", "Kubernetes distribution running on this node "+
		"(k3s|k0s|k8s|kubeEdge|openYurt); selects per-KD edge weights from -priors when set")
	flag.Parse()

	cfg := profiles.Config{
		EMAAlpha:             *alpha,
		ConvergenceThreshold: *convergence,
		MinTrustScore:        *minTrust,
		PriorWeightsPath:     *priorsPath,
		KD:                   *kd,
	}

	sm, err := profiles.Build(*profileName, cfg)
	if err != nil {
		log.Fatalf("failed to build profile %q: %v", *profileName, err)
	}

	mux := http.NewServeMux()
	registerRoutes(mux, sm)

	srv := &http.Server{Addr: *addr, Handler: mux}

	go func() {
		log.Printf("semantic-map agent starting profile=%s addr=%s", *profileName, *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down")
}

func registerRoutes(mux *http.ServeMux, sm *semmap.SemanticMap) {
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

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON error: %v", err)
	}
}
