package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DiyazY/di-agent/cmd/mapctl/client"
)

// fakePeerAgent stands in for a real daemon serving /peers and the related
// endpoints. Holds a tiny mutable peer table so the cobra subcommand tests
// can exercise both reads and writes against a single fixture.
type fakePeerAgent struct {
	mu    sync.Mutex
	peers map[string]client.PeerDTO
}

func newFakePeerAgent() *fakePeerAgent {
	return &fakePeerAgent{peers: map[string]client.PeerDTO{}}
}

func (f *fakePeerAgent) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /peers", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		out := make([]client.PeerDTO, 0, len(f.peers))
		for _, p := range f.peers {
			out = append(out, p)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})

	mux.HandleFunc("POST /peers", func(w http.ResponseWriter, r *http.Request) {
		var req client.AddPeerRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		defer f.mu.Unlock()
		id := "fake-" + req.URL
		d := client.PeerDTO{
			ID:    id,
			URL:   req.URL,
			Trust: 0.5,
			Note:  req.Note,
		}
		f.peers[id] = d
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(d)
	})

	mux.HandleFunc("DELETE /peers/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		f.mu.Lock()
		defer f.mu.Unlock()
		if _, ok := f.peers[id]; !ok {
			http.Error(w, "not found", 404)
			return
		}
		delete(f.peers, id)
		w.WriteHeader(204)
	})

	mux.HandleFunc("POST /peers/{id}/trust", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req client.SetTrustRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		defer f.mu.Unlock()
		d, ok := f.peers[id]
		if !ok {
			http.Error(w, "not found", 404)
			return
		}
		d.Trust = req.Value
		d.NObserved++
		d.LastSeen = time.Now()
		f.peers[id] = d
		w.WriteHeader(204)
	})

	return mux
}

func runMapctl(t *testing.T, args []string) string {
	t.Helper()
	root := NewRootCmd()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetContext(context.Background())
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
	return buf.String()
}

func TestMapctl_PeersList_Empty(t *testing.T) {
	srv := httptest.NewServer(newFakePeerAgent().handler())
	defer srv.Close()
	out := runMapctl(t, []string{"--addr", srv.URL, "peers", "list"})
	if !strings.Contains(out, "no peers registered") {
		t.Errorf("expected empty-list message; got %q", out)
	}
}

func TestMapctl_PeersAddListRemove(t *testing.T) {
	fake := newFakePeerAgent()
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	addOut := runMapctl(t, []string{"--addr", srv.URL,
		"peers", "add", "http://node_1:8080", "--note", "rpi"})
	if !strings.Contains(addOut, "registered peer") {
		t.Errorf("add output: %q", addOut)
	}

	listOut := runMapctl(t, []string{"--addr", srv.URL, "peers", "list"})
	if !strings.Contains(listOut, "http://node_1:8080") {
		t.Errorf("list missing url: %q", listOut)
	}

	removeOut := runMapctl(t, []string{"--addr", srv.URL,
		"peers", "remove", "fake-http://node_1:8080"})
	if !strings.Contains(removeOut, "removed peer") {
		t.Errorf("remove output: %q", removeOut)
	}

	listOut = runMapctl(t, []string{"--addr", srv.URL, "peers", "list"})
	if !strings.Contains(listOut, "no peers registered") {
		t.Errorf("after remove, list should be empty; got %q", listOut)
	}
}

func TestMapctl_PeersTrust(t *testing.T) {
	fake := newFakePeerAgent()
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	runMapctl(t, []string{"--addr", srv.URL, "peers", "add", "http://node_X:8080"})
	out := runMapctl(t, []string{"--addr", srv.URL,
		"peers", "trust", "fake-http://node_X:8080", "0.9"})
	if !strings.Contains(out, "set trust") {
		t.Errorf("trust output: %q", out)
	}

	listOut := runMapctl(t, []string{"--addr", srv.URL, "peers", "list"})
	if !strings.Contains(listOut, "0.900") {
		t.Errorf("expected trust 0.900 in list; got %q", listOut)
	}
}

func TestMapctl_PeersListJSON(t *testing.T) {
	fake := newFakePeerAgent()
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	runMapctl(t, []string{"--addr", srv.URL, "peers", "add", "http://x:8080"})
	out := runMapctl(t, []string{"--addr", srv.URL, "--json", "peers", "list"})
	var peers []client.PeerDTO
	if err := json.Unmarshal([]byte(out), &peers); err != nil {
		t.Fatalf("--json output not valid JSON: %v\noutput=%q", err, out)
	}
	if len(peers) != 1 {
		t.Errorf("expected 1 peer in json output; got %d", len(peers))
	}
}
