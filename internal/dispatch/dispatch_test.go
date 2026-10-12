package dispatch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeClock lets tests shift wall and monotonic time independently.
type fakeClock struct {
	mu   sync.Mutex
	wall time.Time
	mono time.Duration
}

func newFakeClock() *fakeClock {
	return &fakeClock{wall: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Clock() Clock {
	return Clock{
		Wall: func() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.wall },
		Mono: func() time.Duration { c.mu.Lock(); defer c.mu.Unlock(); return c.mono },
	}
}
func (c *fakeClock) advance(wall, mono time.Duration) {
	c.mu.Lock()
	c.wall = c.wall.Add(wall)
	c.mono += mono
	c.mu.Unlock()
}

const (
	ctlID = "voltix-1"
	token = "test-token-0123456789abcdef"
	uuid1 = "11111111-2222-3333-4444-555555555555"
	uuid2 = "66666666-7777-8888-9999-aaaaaaaaaaaa"
)

func rig(t *testing.T) (*fakeClock, *Server, *Session) {
	t.Helper()
	fc := newFakeClock()
	sess := NewSession(ctlID, fc.Clock())
	return fc, NewServer(sess, token), sess
}

func reqBody(t *testing.T, fc *fakeClock, sess *Session, rev int64, id string,
	lifeSec float64, target float64) []byte {
	t.Helper()
	issued := fc.wall
	b, err := json.Marshal(Request{
		ControllerID: ctlID, BootSessionID: sess.bootID,
		CommandID: id, Revision: rev,
		IssuedAt:   issued,
		ValidUntil: issued.Add(time.Duration(lifeSec * float64(time.Second))),
		TargetKW:   target,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func do(t *testing.T, srv *Server, body []byte, tok string) (int, map[string]any) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/dispatch", bytes.NewReader(body))
	if tok != "" {
		r.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	var v map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	return w.Code, v
}

func getStatus(t *testing.T, srv *Server, tok string) (int, map[string]any) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	if tok != "" {
		r.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	var v map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	return w.Code, v
}

func TestAuthRequired(t *testing.T) {
	fc, srv, sess := rig(t)
	for _, tok := range []string{"", "wrong", token + "x"} {
		if code, _ := do(t, srv, reqBody(t, fc, sess, 1, uuid1, 30, 100), tok); code != 401 {
			t.Fatalf("dispatch token %q: want 401, got %d", tok, code)
		}
	}
	if code, _ := getStatus(t, srv, "bad"); code != 401 {
		t.Fatalf("status: want 401, got %d", code)
	}
}

func TestMalformedAndOversized(t *testing.T) {
	fc, srv, sess := rig(t)
	cases := [][]byte{
		[]byte(`{not json`),
		[]byte(`{"command_id":` + `"` + uuid1 + `","extra":1}`),        // unknown field
		[]byte(fmt.Sprintf(`{"pad":"%s"}`, strings.Repeat("x", 5000))), // >4KiB
	}
	for i, b := range cases {
		if code, _ := do(t, srv, b, token); code != 400 {
			t.Fatalf("case %d: want 400, got %d", i, code)
		}
	}
	// A valid body must still work after rejections.
	if code, _ := do(t, srv, reqBody(t, fc, sess, 1, uuid1, 30, 100), token); code != 202 {
		t.Fatalf("post-rejection accept: want 202, got %d", code)
	}
}

func TestAcceptAndStatusRedacted(t *testing.T) {
	fc, srv, sess := rig(t)
	code, v := do(t, srv, reqBody(t, fc, sess, 1, uuid1, 30, 120.5), token)
	if code != 202 || v["command_id"] != uuid1 {
		t.Fatalf("accept: %d %v", code, v)
	}
	code, st := getStatus(t, srv, token)
	if code != 200 || st["dispatch_active"] != true ||
		st["boot_session_id"] != sess.bootID || st["requested_target"] != 120.5 {
		t.Fatalf("status: %d %v", code, st)
	}
	if strings.Contains(fmt.Sprint(st), token) {
		t.Fatal("status leaks token")
	}
}

func TestIdempotentRetryPreservesDeadline(t *testing.T) {
	fc, srv, sess := rig(t)
	body := reqBody(t, fc, sess, 1, uuid1, 30, 100)
	code, r1 := do(t, srv, body, token)
	fc.advance(10*time.Second, 10*time.Second)
	code, r2 := do(t, srv, body, token)
	if code != 202 || r1["deadline_seconds"] != r2["deadline_seconds"] {
		t.Fatalf("retry must preserve deadline: %v vs %v", r1, r2)
	}
}

func TestRevisionOrderingAndConflicts(t *testing.T) {
	fc, srv, sess := rig(t)
	do(t, srv, reqBody(t, fc, sess, 5, uuid1, 30, 100), token)

	// Older revision rejected even after the active command expires.
	fc.advance(40*time.Second, 40*time.Second)
	if code, _ := do(t, srv, reqBody(t, fc, sess, 4, uuid2, 30, 50), token); code != 409 {
		t.Fatalf("old revision: want 409, got %d", code)
	}
	// Same revision with a different command -> conflict.
	if code, _ := do(t, srv, reqBody(t, fc, sess, 5, uuid2, 30, 50), token); code != 409 {
		t.Fatalf("same rev different cmd: want 409, got %d", code)
	}
	// Higher revision accepted.
	if code, _ := do(t, srv, reqBody(t, fc, sess, 6, uuid2, 30, 50), token); code != 202 {
		t.Fatalf("new revision: want 202, got %d", code)
	}
}

func TestSessionAndExpiryValidation(t *testing.T) {
	fc, srv, sess := rig(t)

	// Wrong controller / stale boot session.
	b := reqBody(t, fc, sess, 1, uuid1, 30, 100)
	var req Request
	_ = json.Unmarshal(b, &req)
	req.ControllerID = "other"
	bad, _ := json.Marshal(req)
	if code, _ := do(t, srv, bad, token); code != 409 {
		t.Fatalf("controller: want 409, got %d", code)
	}
	req.ControllerID, req.BootSessionID = ctlID, "deadbeef"
	bad, _ = json.Marshal(req)
	if code, _ := do(t, srv, bad, token); code != 409 {
		t.Fatalf("boot session: want 409, got %d", code)
	}

	// Expired and overlong/negative lifetimes.
	for _, life := range []float64{-1, 0, 30.001, 60} {
		if code, _ := do(t, srv, reqBody(t, fc, sess, 2, uuid1, life, 100), token); code != 422 {
			t.Fatalf("life %v: want 422, got %d", life, code)
		}
	}
	// Not yet valid (issued in the future) and already expired.
	req = Request{ControllerID: ctlID, BootSessionID: sess.bootID,
		CommandID: uuid1, Revision: 2, TargetKW: 100,
		IssuedAt: fc.wall.Add(time.Hour), ValidUntil: fc.wall.Add(2 * time.Hour)}
	bad, _ = json.Marshal(req)
	if code, _ := do(t, srv, bad, token); code != 422 {
		t.Fatalf("future issued_at: want 422, got %d", code)
	}
	// Non-UUID command id.
	req.CommandID, req.IssuedAt = "nope", fc.wall
	req.ValidUntil = fc.wall.Add(30 * time.Second)
	bad, _ = json.Marshal(req)
	if code, _ := do(t, srv, bad, token); code != 422 {
		t.Fatalf("uuid: want 422, got %d", code)
	}
}

func TestClockShifts(t *testing.T) {
	fc, srv, sess := rig(t)
	do(t, srv, reqBody(t, fc, sess, 1, uuid1, 30, 100), token)

	// Backward wall step must not extend the lease: -60 s wall, +5 s mono.
	fc.advance(-60*time.Second, 5*time.Second)
	if _, ok := sess.Active(); !ok {
		t.Fatal("backward wall step must not alter the mono lease")
	}
	// Forward wall jump past valid_until expires conservatively even though
	// the monotonic deadline has not been reached (mono is only 10 s in).
	fc.advance(95*time.Second, 5*time.Second)
	if _, ok := sess.Active(); ok {
		t.Fatal("forward wall jump past valid_until must expire the intent")
	}
}

func TestRestartRejectsOldSession(t *testing.T) {
	fc, srv, sess := rig(t)
	body := reqBody(t, fc, sess, 1, uuid1, 30, 100)
	if code, _ := do(t, srv, body, token); code != 202 {
		t.Fatal("baseline accept failed")
	}
	// Simulate restart: same clock, new session/boot id.
	srv2 := NewServer(NewSession(ctlID, fc.Clock()), token)
	if code, _ := do(t, srv2, body, token); code != 409 {
		t.Fatalf("replayed old-session command: want 409, got %d", code)
	}
}

func TestHeartbeatDoesNotExtend(t *testing.T) {
	fc, srv, sess := rig(t)
	do(t, srv, reqBody(t, fc, sess, 1, uuid1, 30, 100), token)
	// Poll status repeatedly across the window; deadline must not move.
	for i := 0; i < 5; i++ {
		getStatus(t, srv, token)
		fc.advance(5*time.Second, 5*time.Second)
	}
	fc.advance(6*time.Second, 6*time.Second) // 31 s total
	if _, ok := sess.Active(); ok {
		t.Fatal("status polls must not extend the dispatch deadline")
	}
}

func TestConcurrentAccepts(t *testing.T) {
	fc, srv, sess := rig(t)
	var wg sync.WaitGroup
	accepted := make(chan int, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(rev int64) {
			defer wg.Done()
			id := fmt.Sprintf("11111111-2222-3333-4444-%012d", rev)
			code, _ := do(t, srv, reqBody(t, fc, sess, rev, id, 30, 100), token)
			accepted <- code
		}(int64(i + 1))
	}
	wg.Wait()
	close(accepted)
	any202 := false
	for c := range accepted {
		if c == 202 {
			any202 = true
		}
	}
	if !any202 {
		t.Fatal("no request accepted under concurrency")
	}
}
