package upstream

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const tok = "recv-token-0123456789"

var t0 = time.Unix(1_700_000_000, 0).UTC()

func batch(stream string, evs ...Event) []byte {
	b, _ := json.Marshal(Batch{SchemaVersion: 1, StreamID: stream, Events: evs})
	return b
}

func ev(seq int64, payload string) Event {
	return Event{SeqID: seq, Tier: 0, ControllerID: "c1", ObservedAt: t0,
		Payload: []byte(payload)}
}

func post(t *testing.T, srv *httptest.Server, body []byte, token string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/telemetry",
		bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return -1, nil
	}
	defer resp.Body.Close()
	var v map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&v)
	return resp.StatusCode, v
}

func TestCommitAckDedupAndConflict(t *testing.T) {
	r, err := OpenReceiver(t.TempDir(), tok)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	srv := httptest.NewServer(r)
	defer srv.Close()

	code, v := post(t, srv, batch("s1", ev(1, `{"a":1}`), ev(2, `{"a":2}`)), tok)
	if code != 200 {
		t.Fatalf("commit: %d %v", code, v)
	}
	if got := v["acked_seq_ids"]; got == nil {
		t.Fatalf("no acked ids: %v", v)
	}
	// Identical replay: no-op success, same acks.
	code, _ = post(t, srv, batch("s1", ev(1, `{"a":1}`)), tok)
	if code != 200 {
		t.Fatalf("identical replay: %d", code)
	}
	// Conflicting content for an existing seq: rejected, nothing added.
	code, _ = post(t, srv, batch("s1", ev(1, `{"a":DIFFERENT}`)), tok)
	if code != 409 {
		t.Fatalf("conflict: want 409, got %d", code)
	}
	code, v = apiGet(t, srv, "/control/count")
	if v["count"].(float64) != 2 {
		t.Fatalf("count %v after conflict, want 2", v["count"])
	}
}

func apiGet(t *testing.T, srv *httptest.Server, path string) (int, map[string]any) {
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
	return resp.StatusCode, v
}

func TestRestartPersistence(t *testing.T) {
	dir := t.TempDir()
	r, _ := OpenReceiver(dir, tok)
	srv := httptest.NewServer(r)
	post(t, srv, batch("s1", ev(1, `{"x":1}`)), tok)
	srv.Close()
	r.Close()

	r2, err := OpenReceiver(dir, tok)
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()
	srv2 := httptest.NewServer(r2)
	defer srv2.Close()
	_, v := apiGet(t, srv2, "/control/count")
	if v["count"].(float64) != 1 {
		t.Fatalf("restart lost data: %v", v)
	}
	// Replayed seq=1 after restart: identical → no-op, conflicting → 409.
	if code, _ := post(t, srv2, batch("s1", ev(1, `{"x":1}`)), tok); code != 200 {
		t.Fatalf("identical post-restart replay: %d", code)
	}
}

func TestAuthSchemaAndBounds(t *testing.T) {
	r, _ := OpenReceiver(t.TempDir(), tok)
	defer r.Close()
	srv := httptest.NewServer(r)
	defer srv.Close()

	if code, _ := post(t, srv, batch("s1", ev(1, `{}`)), "wrong"); code != 401 {
		t.Fatalf("auth: %d", code)
	}
	// Oversized body >256KiB → 400 (payload field padded past the limit).
	big := `{"schema_version":1,"stream_id":"s","events":[` +
		`{"seq_id":1,"tier":0,"controller_id":"c","observed_at":"` +
		t0.Format(time.RFC3339Nano) + `","payload":"` + strings.Repeat("A", 300000) + `"}]}`
	if code, _ := post(t, srv, []byte(big), tok); code != 400 {
		t.Fatalf("oversize: %d", code)
	}
	// >100 events → 422.
	evs := make([]Event, 101)
	for i := range evs {
		evs[i] = ev(int64(i+1), `{}`)
	}
	if code, _ := post(t, srv, batch("s1", evs...), tok); code != 422 {
		t.Fatalf("101 events: %d", code)
	}
	// Unknown field → 400; wrong schema_version → 422.
	bad, _ := json.Marshal(map[string]any{"schema_version": 1, "stream_id": "s",
		"events": []Event{ev(1, `{}`)}, "extra": 1})
	if code, _ := post(t, srv, bad, tok); code != 400 {
		t.Fatalf("unknown field: %d", code)
	}
	bad, _ = json.Marshal(map[string]any{"schema_version": 2, "stream_id": "s",
		"events": []Event{ev(1, `{}`)}})
	if code, _ := post(t, srv, bad, tok); code != 422 {
		t.Fatalf("schema_version: %d", code)
	}
}

func TestCommitThenDropAckLoss(t *testing.T) {
	r, _ := OpenReceiver(t.TempDir(), tok)
	defer r.Close()
	srv := httptest.NewServer(r)
	defer srv.Close()

	apiGet(t, srv, "/control/drop-after-commit")
	// The write commits but the client sees a broken connection.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/telemetry",
		bytes.NewReader(batch("s1", ev(1, `{"y":1}`))))
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected connection drop, got response")
	}
	_, v := apiGet(t, srv, "/control/count")
	if v["count"].(float64) != 1 {
		t.Fatalf("commit-then-drop lost the row: %v", v)
	}
	// Idempotent retry of the same batch: no-op success.
	r.mu.Lock()
	r.dropAfterWrite = false
	r.mu.Unlock()
	code, v := post(t, srv, batch("s1", ev(1, `{"y":1}`)), tok)
	if code != 200 {
		t.Fatalf("post-drop retry: %d %v", code, v)
	}
}

func TestPauseAndReset(t *testing.T) {
	r, _ := OpenReceiver(t.TempDir(), tok)
	defer r.Close()
	srv := httptest.NewServer(r)
	defer srv.Close()

	apiGet(t, srv, "/control/pause")
	if code, _ := post(t, srv, batch("s1", ev(1, `{}`)), tok); code != 503 {
		t.Fatalf("paused: %d", code)
	}
	apiGet(t, srv, "/control/resume")
	post(t, srv, batch("s1", ev(1, `{}`)), tok)
	apiGet(t, srv, "/control/reset")
	_, v := apiGet(t, srv, "/control/count")
	if v["count"].(float64) != 0 {
		t.Fatalf("reset: %v", v)
	}
}
