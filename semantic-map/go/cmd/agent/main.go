// Command agent is the edge-minimal Semantic Map daemon.
//
// It loads the configured profile, seeds the graph from Di-Select priors,
// and serves the agent queries plus the graph control surface over
// HTTP/JSON on :8080. Telemetry is accepted via POST /ingest. The graph
// introspection (/graph, /edges, /history, /constructs, /propositions,
// /neighbors), ontology mutation (/ontology/*), candidate review
// (/candidates/{id}/{confirm,reject,defer}), edge reset (/agent/reset),
// and operator meta (/healthz, /version, /ui/) routes are wired by
// registerRoutes in routes.go.
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
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/DiyazY/di-agent/pkg/profiles"
)

// Version is the daemon's reported semver. Returned by GET /version.
// Phase 1 ships 0.1.0 (HTTP expansion). Bump for future control-surface work.
const Version = "0.1.0"

// BuildCommit is the short git SHA the binary was built from. Empty when the
// binary is built without `-ldflags "-X main.BuildCommit=…"`.
var BuildCommit = ""

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
