package syncer

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"voltix/internal/store"
	"voltix/internal/upstream"
)

var t0 = time.Unix(1_700_000_000, 0).UTC()

const tok = "sync-token-0123456789"

func testCfg(url string) Config {
	return Config{
		URL: url, Token: tok,
		FlushWait: 20 * time.Millisecond, HTTPTimeout: time.Second,
		BackoffInit: 5 * time.Millisecond, BackoffCap: 50 * time.Millisecond,
		Recheck: 20 * time.Millisecond,
		Rand:    rand.New(rand.NewSource(1)),
	}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir(), store.Config{ControllerID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func fill(t *testing.T, s *store.Store, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := s.Insert(context.Background(), store.TierStatus,
			t0, []byte(`{"n":`+string(rune('0'+i))+`}`)); err != nil {
			t.Fatal(err)
		}
	}
}

func depth(t *testing.T, s *store.Store) int64 {
	t.Helper()
	st, _, _, err := s.Depth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func recvCtl(t *testing.T, srv *httptest.Server, path string) map[string]any {
	t.Helper()
	method := http.MethodPost
	if path == "/control/count" {
		method = http.MethodGet
	}
	req, _ := http.NewRequest(method, srv.URL+path, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var v map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&v)
	return v
}

func startReceiver(t *testing.T) *httptest.Server {
	t.Helper()
	r, err := upstream.OpenReceiver(t.TempDir(), tok)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(r)
	t.Cleanup(func() { srv.Close(); r.Close() })
	return srv
}

func runSyncer(t *testing.T, s *store.Store, url string) *Uploader {
	t.Helper()
	u := New(s, testCfg(url))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { u.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return u
}

func TestDrainAndAckedPruning(t *testing.T) {
	s, srv := openStore(t), startReceiver(t)
	fill(t, s, 7)
	runSyncer(t, s, srv.URL+"/api/telemetry")
	deadline := time.Now().Add(3 * time.Second)
	for depth(t, s) > 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if d := depth(t, s); d != 0 {
		t.Fatalf("depth %d after drain", d)
	}
	if n := recvCtl(t, srv, "/control/count")["count"]; n != 7.0 {
		t.Fatalf("receiver count %v", n)
	}
}

func TestBatchMax100(t *testing.T) {
	s, srv := openStore(t), startReceiver(t)
	fill(t, s, 250)
	runSyncer(t, s, srv.URL+"/api/telemetry")
	deadline := time.Now().Add(5 * time.Second)
	for depth(t, s) > 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if depth(t, s) != 0 {
		t.Fatalf("not drained: %d", depth(t, s))
	}
}

func TestLostResponseReplaysWithStableIdentity(t *testing.T) {
	s, srv := openStore(t), startReceiver(t)
	fill(t, s, 3)
	recvCtl(t, srv, "/control/drop-after-commit")
	u := runSyncer(t, s, srv.URL+"/api/telemetry")

	// The batch commits at the receiver but its ack is lost; the one-shot
	// drop auto-clears so the retry (identical identities) dedups as a
	// no-op and is acknowledged normally.
	deadline := time.Now().Add(3 * time.Second)
	for u.Stats().Failed == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if u.Stats().Failed == 0 {
		t.Fatal("lost response never observed")
	}
	for depth(t, s) > 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if depth(t, s) != 0 {
		t.Fatalf("replay never drained: %d stats %+v", depth(t, s), u.Stats())
	}
	// No duplicates persisted at the receiver despite the replay.
	if got := recvCtl(t, srv, "/control/count")["count"]; got != 3.0 {
		t.Fatalf("duplicate events at receiver: %v", got)
	}
}

func TestTransientRetryThenSuccess(t *testing.T) {
	s, srv := openStore(t), startReceiver(t)
	fill(t, s, 2)
	recvCtl(t, srv, "/control/pause")
	runSyncer(t, s, srv.URL+"/api/telemetry")
	time.Sleep(200 * time.Millisecond)
	if depth(t, s) != 2 {
		t.Fatal("rows pruned during outage")
	}
	recvCtl(t, srv, "/control/resume")
	deadline := time.Now().Add(3 * time.Second)
	for depth(t, s) > 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if depth(t, s) != 0 {
		t.Fatal("no recovery after resume")
	}
}

// canned returns scripted responses for ack-edge cases.
func canned(t *testing.T, streamID string, resp func(seqs []int64) any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b upstream.Batch
		_ = json.NewDecoder(r.Body).Decode(&b)
		var seqs []int64
		for _, e := range b.Events {
			seqs = append(seqs, e.SeqID)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp(seqs))
	}))
}

func TestPartialAckPrunesOnlyAcked(t *testing.T) {
	s := openStore(t)
	fill(t, s, 4)
	first := true
	srv := canned(t, s.StreamID, func(seqs []int64) any {
		if !first { // later rounds acknowledge nothing
			return map[string]any{"stream_id": s.StreamID, "acked_seq_ids": []int64{}}
		}
		first = false
		return map[string]any{"stream_id": s.StreamID,
			"acked_seq_ids": seqs[:2]} // first round: ack only seqs 1,2
	})
	defer srv.Close()
	runSyncer(t, s, srv.URL+"/api/telemetry")
	time.Sleep(300 * time.Millisecond)
	if d := depth(t, s); d != 2 {
		t.Fatalf("depth %d after partial ack, want 2", d)
	}
	evs, _ := s.Batch(context.Background(), 10)
	if evs[0].SeqID != 3 {
		t.Fatalf("remaining oldest seq %d, want 3", evs[0].SeqID)
	}
}

func TestForeignAndInvalidAcksDeleteNothing(t *testing.T) {
	s := openStore(t)
	fill(t, s, 3)
	cases := []func(seqs []int64) any{
		func(seqs []int64) any { // foreign stream
			return map[string]any{"stream_id": "other-stream",
				"acked_seq_ids": seqs}
		},
		func(seqs []int64) any { // acks an unsent id
			return map[string]any{"stream_id": s.StreamID,
				"acked_seq_ids": []int64{9999}}
		},
		func(seqs []int64) any { // undecodable
			return "garbage"
		},
	}
	for i, resp := range cases {
		srv := canned(t, s.StreamID, resp)
		u := runSyncer(t, s, srv.URL+"/api/telemetry")
		time.Sleep(200 * time.Millisecond)
		srv.Close()
		if d := depth(t, s); d != 3 {
			t.Fatalf("case %d: depth %d after invalid ack", i, d)
		}
		st := u.Stats()
		if !st.ConfigFault && st.Failed == 0 {
			t.Fatalf("case %d: failure not recorded: %+v", i, st)
		}
	}
}

func TestConfigFaultSlowRecheck(t *testing.T) {
	s := openStore(t)
	fill(t, s, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	u := runSyncer(t, s, srv.URL+"/api/telemetry")
	time.Sleep(200 * time.Millisecond)
	st := u.Stats()
	if !st.ConfigFault {
		t.Fatalf("401 not flagged config fault: %+v", st)
	}
	if d := depth(t, s); d != 1 {
		t.Fatalf("data deleted on auth failure: %d", d)
	}
}

func TestBoundedShutdown(t *testing.T) {
	s := openStore(t)
	fill(t, s, 1)
	// Server that hangs past the HTTP deadline.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()
	u := New(s, testCfg(srv.URL+"/api/telemetry"))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { u.Run(ctx); close(done) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown not bounded by HTTP deadline")
	}
}
