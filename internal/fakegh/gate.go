package fakegh

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A gate is the one piece of fakegh that can tell whether two downloads were
// in flight at the same moment, which is what `block sync` running tools in
// parallel looks like from the server. A request under
//
//	/gate/<key>/<n>/<route>
//
// is held until n requests are waiting at that gate together, and only then
// served as /<route> would be. Once a gate has opened it stays open, so a
// later request passes straight through. A gate that never fills — a client
// that downloads one artifact at a time — times out and answers 503 rather
// than hanging, so a sync that has gone serial fails rather than stalls. The
// key keeps the gates of different scenarios apart, since one server serves
// the whole suite.
//
//	/gate/<key>/stats
//
// reports what the gate saw as JSON: how many requests arrived, the most that
// were in flight at once, and whether it opened.
type gate struct {
	n        int
	arrived  int
	inflight int
	// maxInflight is the most requests that were inside the gate at one time,
	// counting from arrival until the response was written.
	maxInflight int
	opened      bool
	timeout     time.Duration
	cond        *sync.Cond
}

// DefaultGateTimeout is how long a request waits for company before the gate
// gives up on it. Every fellow request arrives within milliseconds when the
// client is concurrent; this is only here so a serial client fails rather
// than hangs. [Server.GateTimeout] overrides it.
const DefaultGateTimeout = 10 * time.Second

// gateStats is the JSON /gate/<key>/stats answers with.
type gateStats struct {
	Arrived     int  `json:"arrived"`
	MaxInflight int  `json:"max_inflight"`
	Opened      bool `json:"opened"`
}

func (s *Server) gateFor(key string, n int) *gate {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gates == nil {
		s.gates = map[string]*gate{}
	}
	g, ok := s.gates[key]
	if !ok {
		timeout := s.GateTimeout
		if timeout <= 0 {
			timeout = DefaultGateTimeout
		}
		g = &gate{n: n, timeout: timeout, cond: sync.NewCond(&sync.Mutex{})}
		s.gates[key] = g
	}
	return g
}

// serveGate handles everything under /gate/.
func (s *Server) serveGate(w http.ResponseWriter, r *http.Request, rest string) {
	key, tail, ok := strings.Cut(rest, "/")
	if !ok || key == "" {
		http.NotFound(w, r)
		return
	}
	if tail == "stats" {
		s.mu.Lock()
		g := s.gates[key]
		s.mu.Unlock()
		var stats gateStats
		if g != nil {
			g.cond.L.Lock()
			stats = gateStats{Arrived: g.arrived, MaxInflight: g.maxInflight, Opened: g.opened}
			g.cond.L.Unlock()
		}
		writeJSON(w, http.StatusOK, stats)
		return
	}
	count, route, ok := strings.Cut(tail, "/")
	n, err := strconv.Atoi(count)
	if !ok || err != nil || n < 1 {
		http.NotFound(w, r)
		return
	}
	g := s.gateFor(key, n)
	if !g.enter(r.Context()) {
		g.leave()
		http.Error(w, "gate "+key+": fewer than "+count+" downloads were in flight together", http.StatusServiceUnavailable)
		return
	}
	defer g.leave()
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/" + route
	s.ServeHTTP(w, r2)
}

// enter waits until the gate has opened, and reports whether it did. A
// request whose client has gone — a download block cancelled because another
// failed — stops waiting at once rather than holding the connection open
// until the timeout.
func (g *gate) enter(ctx context.Context) bool {
	g.cond.L.Lock()
	defer g.cond.L.Unlock()
	g.arrived++
	g.inflight++
	if g.inflight > g.maxInflight {
		g.maxInflight = g.inflight
	}
	if g.inflight >= g.n {
		g.opened = true
		g.cond.Broadcast()
	}
	if !g.opened {
		timer := time.AfterFunc(g.timeout, g.cond.Broadcast)
		defer timer.Stop()
		stop := context.AfterFunc(ctx, g.cond.Broadcast)
		defer stop()
		deadline := time.Now().Add(g.timeout)
		for !g.opened && ctx.Err() == nil && time.Now().Before(deadline) {
			g.cond.Wait()
		}
	}
	return g.opened
}

func (g *gate) leave() {
	g.cond.L.Lock()
	g.inflight--
	g.cond.L.Unlock()
}
