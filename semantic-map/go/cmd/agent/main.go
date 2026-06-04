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
//	      -priors /path/to/prior_weights.json -kd k0s \
//	      -collect-interval 10s -cgroup-root /sys/fs/cgroup \
//	      -peers http://node_1:8080,http://node_2:8080
//
// The -kd flag selects per-distribution edge weights from prior_weights.json
// when set. Omit it (or pass an empty string) to use the global Di-Select
// proposition strengths.
//
// The autonomous collection loop ticks at -collect-interval, calls
// CollectorContract.Collect on the profile's collector, and runs each sample
// through the Bridge → Updater pipe. Setting -collect-interval=0 or
// -cgroup-root="" disables it (the manual POST /ingest path still works).
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/DiyazY/di-agent/pkg/contracts"
	"github.com/DiyazY/di-agent/pkg/profiles"
	"github.com/DiyazY/di-agent/pkg/semmap"
)

// Version is the daemon's reported semver. Returned by GET /version.
// Phase 1 ships 0.1.0 (HTTP expansion). Bump for future control-surface work.
const Version = "0.1.0"

// BuildCommit is the short git SHA the binary was built from. Empty when the
// binary is built without `-ldflags "-X main.BuildCommit=…"`.
var BuildCommit = ""

func main() {
	profileName     := flag.String("profile", "edge-minimal", "deployment profile")
	addr            := flag.String("addr", ":8080", "HTTP listen address")
	alpha           := flag.Float64("alpha", 0.2, "EMA decay factor (0 < alpha < 1)")
	convergence     := flag.Float64("convergence", 500, "observations for confidence=1.0")
	minTrust        := flag.Float64("min-trust", 0.5, "minimum peer trust score")
	priorsPath      := flag.String("priors", "", "path to prior_weights.json from initialization pipeline")
	kd              := flag.String("kd", "", "Kubernetes distribution running on this node "+
		"(k3s|k0s|k8s|kubeEdge|openYurt); selects per-KD edge weights from -priors when set")
	collectInterval := flag.Duration("collect-interval", 10*time.Second,
		"how often the collection loop ticks the Collector; 0 disables the loop")
	cgroupRoot      := flag.String("cgroup-root", "/sys/fs/cgroup",
		"filesystem root the cgroup collector reads from; empty string disables the loop")
	nodeID          := flag.String("node-id", "",
		"identifier this agent uses in MetricSamples; empty falls back to os.Hostname()")
	peersFlag       := flag.String("peers", "",
		"comma-separated peer agent URLs to register at startup "+
			"(e.g. http://node_1:8080,http://node_2:8080). RecommendPeer ranks "+
			"these by trust-weighted savings. Additional peers can be added at "+
			"runtime via POST /peers.")
	var useProposer bool
	flag.BoolVar(&useProposer, "proposer", true, "enable MI correlation proposer (disable for low-CPU devices)")
	flag.Parse()

	if *nodeID == "" {
		if h, err := os.Hostname(); err == nil {
			*nodeID = h
		}
	}

	peerURLs := parsePeerURLs(*peersFlag)

	cfg := profiles.Config{
		EMAAlpha:             *alpha,
		ConvergenceThreshold: *convergence,
		MinTrustScore:        *minTrust,
		PriorWeightsPath:     *priorsPath,
		KD:                   *kd,
		NodeID:               *nodeID,
		CgroupRoot:           *cgroupRoot,
		CollectInterval:      *collectInterval,
		PeerURLs:             peerURLs,
		UseProposer:          useProposer,
	}

	sm, collector, err := profiles.Build(*profileName, cfg)
	if err != nil {
		log.Fatalf("failed to build profile %q: %v", *profileName, err)
	}
	if len(peerURLs) > 0 {
		log.Printf("registered %d peers: %s", len(peerURLs), strings.Join(peerURLs, ", "))
	}

	mux := http.NewServeMux()
	registerRoutes(mux, sm)

	srv := &http.Server{Addr: *addr, Handler: mux}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		log.Printf("semantic-map agent starting profile=%s addr=%s", *profileName, *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Start the autonomous collection loop if the profile produced a
	// collector AND the interval is positive. Both must hold — a configured
	// collector with interval=0 is a deliberately disabled loop (useful for
	// tests and for nodes that only accept manual POST /ingest).
	startCollectionLoop(ctx, sm, collector, *collectInterval)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down")
	cancel()
}

// parsePeerURLs splits the --peers comma-separated value into a clean slice.
// Empty entries (e.g. trailing commas, all-whitespace tokens) are dropped so
// the registry never sees a "" URL. Returns nil when the input is empty so
// callers can branch on len() instead of inspecting both flag and slice.
func parsePeerURLs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// startCollectionLoop launches the autonomous tick goroutine. It is a no-op
// (with a single explanatory log line) when either the collector is nil or
// the interval is zero — both cases are treated as "operator intentionally
// disabled the loop" rather than as errors.
func startCollectionLoop(
	ctx context.Context,
	sm *semmap.SemanticMap,
	collector contracts.CollectorContract,
	interval time.Duration,
) {
	switch {
	case collector == nil:
		log.Printf("collection loop disabled: no collector for this profile/configuration")
		return
	case interval <= 0:
		log.Printf("collection loop disabled: -collect-interval=%s", interval)
		return
	}

	log.Printf("collection loop started: source=%s interval=%s", collector.SourceID(), interval)
	go runCollectionLoop(ctx, sm, collector, interval)
}

// runCollectionLoop is the body of the scheduler goroutine. Errors from the
// collector or from any individual sample are logged but never stop the loop —
// transient failures (a missing cgroup file, an unknown construct) must not
// disable the agent. Shutdown is via ctx cancellation; the function returns
// promptly once Done is closed.
func runCollectionLoop(
	ctx context.Context,
	sm *semmap.SemanticMap,
	collector contracts.CollectorContract,
	interval time.Duration,
) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Printf("collection loop exiting: %v", ctx.Err())
			return
		case <-t.C:
			samples, err := collector.Collect()
			if err != nil {
				log.Printf("collection loop: Collect error: %v", err)
				continue
			}
			for _, s := range samples {
				if s == nil {
					continue
				}
				if err := sm.IngestSample(s); err != nil {
					log.Printf("collection loop: IngestSample error metric=%s node=%s: %v",
						s.MetricType, s.NodeID, err)
				}
			}
		}
	}
}
