package fakegh

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// A gate opens when as many requests as it asks for are waiting together, and
// refuses when they never come — so it can tell a concurrent client from a
// serial one without any clock but its own timeout.
func TestGateOpensForConcurrentRequestsAndRefusesASerialOne(t *testing.T) {
	t.Parallel()
	s := New(Fixtures())
	s.GateTimeout = 300 * time.Millisecond
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	s.SetBase(srv.URL)

	// A public asset reached through the gate: the same bytes as without it.
	asset := "/download/foundry-rs/foundry/v1.7.4/foundry_v1.7.4_linux_amd64.tar.gz"
	direct, err := http.Get(srv.URL + asset) //nolint:noctx // test
	if err != nil {
		t.Fatal(err)
	}
	want, _ := io.ReadAll(direct.Body)
	_ = direct.Body.Close()

	var wg sync.WaitGroup
	codes := make([]int, 2)
	bodies := make([][]byte, 2)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(srv.URL + "/gate/pair/2" + asset) //nolint:noctx // test
			if err != nil {
				t.Error(err)
				return
			}
			defer resp.Body.Close() //nolint:errcheck // test
			codes[i] = resp.StatusCode
			bodies[i], _ = io.ReadAll(resp.Body)
		}()
	}
	wg.Wait()
	for i := range codes {
		if codes[i] != http.StatusOK || string(bodies[i]) != string(want) {
			t.Errorf("gated request %d = %d, %d bytes; want 200 and the asset", i, codes[i], len(bodies[i]))
		}
	}
	stats := gateStatsOf(t, srv.URL+"/gate/pair/stats")
	if stats.Arrived != 2 || stats.MaxInflight != 2 || !stats.Opened {
		t.Errorf("stats = %+v, want 2 arrived, 2 in flight, opened", stats)
	}
	// Once open, a request on its own passes.
	resp, err := http.Get(srv.URL + "/gate/pair/2" + asset) //nolint:noctx // test
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("request after the gate opened = %d", resp.StatusCode)
	}

	// A gate nobody else reaches refuses rather than hangs.
	resp, err = http.Get(srv.URL + "/gate/alone/2" + asset) //nolint:noctx // test
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || string(body) != "gate alone: fewer than 2 downloads were in flight together\n" {
		t.Errorf("lone request = %d, %q", resp.StatusCode, body)
	}
	if stats := gateStatsOf(t, srv.URL+"/gate/alone/stats"); stats.Opened || stats.MaxInflight != 1 {
		t.Errorf("stats = %+v, want unopened with 1 in flight", stats)
	}
	// A gate that was never used reports nothing, and a malformed one is 404.
	if stats := gateStatsOf(t, srv.URL+"/gate/never/stats"); stats != (gateStats{}) {
		t.Errorf("unused gate stats = %+v", stats)
	}
	for _, bad := range []string{"/gate/", "/gate/x", "/gate/x/zero/download", "/gate/x/0/download"} {
		resp, err := http.Get(srv.URL + bad) //nolint:noctx // test
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", bad, resp.StatusCode)
		}
	}
}

func gateStatsOf(t *testing.T, url string) gateStats {
	t.Helper()
	resp, err := http.Get(url) //nolint:noctx // test
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck // test
	var stats gateStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	return stats
}
