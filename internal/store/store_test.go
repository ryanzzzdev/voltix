package store

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

var t0 = time.Unix(1_700_000_000, 0).UTC()

func smallCfg() Config {
	return Config{
		ControllerID: "voltix-test",
		MaxStatus:    5, MaxAlarm: 3, MaxPayload: 1024, Handoff: 4,
	}
}

func open(t *testing.T, cfg Config) *Store {
	t.Helper()
	s, err := Open(t.TempDir(), cfg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestPragmasOnRealConnection(t *testing.T) {
	s := open(t, Config{ControllerID: "x"})
	var jm string
	var sy int
	if err := s.db.QueryRow("PRAGMA journal_mode").Scan(&jm); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("PRAGMA synchronous").Scan(&sy); err != nil {
		t.Fatal(err)
	}
	if jm != "wal" || sy != 2 {
		t.Fatalf("journal_mode=%s synchronous=%d, want wal/2", jm, sy)
	}
}

func TestSeqNeverReusedAfterFullDeleteAndReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, smallCfg())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seq1, _ := s.Insert(ctx, TierStatus, t0, []byte(`{"a":1}`))
	seq2, _ := s.Insert(ctx, TierStatus, t0, []byte(`{"a":2}`))
	if err := s.DeleteAcked(ctx, []int64{seq1, seq2}); err != nil {
		t.Fatal(err)
	}
	sid := s.StreamID
	s.Close()

	s2, err := Open(dir, smallCfg()) // reopen: all rows deleted
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if s2.StreamID != sid {
		t.Fatal("stream identity changed across reopen")
	}
	seq3, _ := s2.Insert(ctx, TierStatus, t0, []byte(`{"a":3}`))
	if seq3 <= seq2 {
		t.Fatalf("sequence reused: %d after %d", seq3, seq2)
	}
}

func TestAtomicEvictionPerTier(t *testing.T) {
	s := open(t, smallCfg())
	ctx := context.Background()
	// Fill status tier (cap 5); sixth insert must evict the oldest.
	for i := 0; i < 6; i++ {
		if _, err := s.Insert(ctx, TierStatus, t0.Add(time.Duration(i)), []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	st, al, _, err := s.Depth(ctx)
	if err != nil || st != 5 || al != 0 {
		t.Fatalf("depth: %d/%d %v", st, al, err)
	}
	ev, _, _ := s.Counters(ctx)
	if ev[TierStatus] != 1 || ev[TierAlarm] != 0 {
		t.Fatalf("evictions: %v", ev)
	}
	evs, _ := s.Batch(ctx, 10)
	if evs[0].SeqID != 2 { // seq1 evicted, oldest remaining is 2
		t.Fatalf("oldest retained seq %d, want 2", evs[0].SeqID)
	}
}

func TestAlarmSaturationKeepsTierIsolation(t *testing.T) {
	s := open(t, smallCfg())
	ctx := context.Background()
	for i := 0; i < 10; i++ { // cap is 3
		if _, err := s.Insert(ctx, TierAlarm, t0, []byte(`{"alarm":1}`)); err != nil {
			t.Fatal(err)
		}
	}
	_, al, _, _ := s.Depth(ctx)
	if al != 3 {
		t.Fatalf("alarm depth %d, want 3", al)
	}
	ev, _, _ := s.Counters(ctx)
	if ev[TierAlarm] != 7 {
		t.Fatalf("alarm evictions %d, want 7", ev[TierAlarm])
	}
	if _, err := s.Insert(ctx, TierStatus, t0, []byte(`{}`)); err != nil {
		t.Fatal("status insert blocked by saturated alarms")
	}
}

func TestPayloadBound(t *testing.T) {
	s := open(t, smallCfg())
	if _, err := s.Insert(context.Background(), TierStatus, t0,
		make([]byte, 2000)); err != ErrPayloadTooLarge {
		t.Fatalf("oversize: %v", err)
	}
	if err := s.Offer(TierStatus, t0, make([]byte, 2000)); err != ErrPayloadTooLarge {
		t.Fatalf("offer oversize: %v", err)
	}
}

func TestHandoffOverflowCountsVolatileDrops(t *testing.T) {
	s := open(t, smallCfg()) // handoff cap 4
	for i := 0; i < 4; i++ {
		if err := s.Offer(TierStatus, t0, []byte(`{}`)); err != nil {
			t.Fatalf("offer %d: %v", i, err)
		}
	}
	if err := s.Offer(TierStatus, t0, []byte(`{}`)); err != ErrDropped {
		t.Fatalf("want ErrDropped, got %v", err)
	}
	_, vl, _ := s.Counters(context.Background())
	if vl[TierStatus] != 1 {
		t.Fatalf("volatile lost %d, want 1", vl[TierStatus])
	}
}

func TestWriterPersistsAndShutdownDrains(t *testing.T) {
	s := open(t, smallCfg())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.RunWriter(ctx); close(done) }()
	for i := 0; i < 3; i++ {
		s.Offer(TierStatus, t0, []byte(`{}`))
	}
	cancel()
	<-done
	st, _, _, _ := s.Depth(context.Background())
	if st != 3 {
		t.Fatalf("depth %d after drain, want 3", st)
	}
}

func TestCorruptStoreNeverSilentlyReset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.db")
	if err := os.WriteFile(path, []byte("definitely not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir, smallCfg()); err == nil {
		t.Fatal("corrupt file must fail Open")
	}
	b, _ := os.ReadFile(path) // untouched — no silent delete/reset
	if string(b) != "definitely not sqlite" {
		t.Fatal("corrupt store was modified")
	}
}

func TestIncompatibleVersionRejected(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, smallCfg())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("PRAGMA user_version = 99"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, err := Open(dir, smallCfg()); err == nil ||
		!strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("want incompatible error, got %v", err)
	}
}

func TestDeleteAckedIgnoresGapsAndForeign(t *testing.T) {
	s := open(t, smallCfg())
	ctx := context.Background()
	q1, _ := s.Insert(ctx, TierStatus, t0, []byte(`{}`))
	q2, _ := s.Insert(ctx, TierStatus, t0, []byte(`{}`))
	// Ack a nonexistent seq and only q2: q1 must remain.
	if err := s.DeleteAcked(ctx, []int64{q2, 9999}); err != nil {
		t.Fatal(err)
	}
	evs, _ := s.Batch(ctx, 10)
	if len(evs) != 1 || evs[0].SeqID != q1 {
		t.Fatalf("remaining %v, want seq %d", evs, q1)
	}
}

func TestCheckpointIsBounded(t *testing.T) {
	s := open(t, smallCfg())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := s.Checkpoint(ctx); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
}

// Measurement for evidence/telemetry/findings.md — run:
//
//	go test ./internal/store/ -run TestMeasureCommitThroughput -v
func TestMeasureCommitThroughput(t *testing.T) {
	s := open(t, Config{ControllerID: "bench", MaxStatus: 1e9})
	ctx := context.Background()
	payload := make([]byte, 320) // representative snapshot size
	const n = 2000
	start := time.Now()
	for i := 0; i < n; i++ {
		if _, err := s.Insert(ctx, TierStatus, t0, payload); err != nil {
			t.Fatal(err)
		}
	}
	d := time.Since(start)
	t.Logf("MEASURE: %d commits in %v = %.0f events/s (%.1f µs/commit) incl. seq alloc + cap check",
		n, d, float64(n)/d.Seconds(), float64(d.Microseconds())/n)
}

// Audit R4: the store's disk-full handling path under a REAL
// kernel-enforced write failure. A genuine filesystem ENOSPC needs a
// bounded mount (unavailable without privileges), so the harness re-execs
// this test in a subprocess with RLIMIT_FSIZE and ignored SIGXFSZ: file
// writes then fail with EFBIG — the same class of storage-write error the
// commit path must handle. The test proves: the error is surfaced (not
// lost), the store stops accepting writes, prior rows stay readable, and
// raising the limit restores writes without corruption.
func TestStorageWriteFailureSurfacedAndRecoverable(t *testing.T) {
	if os.Getenv("VTX_FS_LIMIT") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestStorageWriteFailureSurfacedAndRecoverable$", "-test.v")
		cmd.Env = append(os.Environ(), "VTX_FS_LIMIT=1")
		out, err := cmd.CombinedOutput()
		t.Logf("subprocess:\n%s", out)
		if err != nil {
			t.Fatalf("limited subprocess failed: %v", err)
		}
		return
	}

	signal.Ignore(syscall.SIGXFSZ)
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
		t.Skipf("rlimit unavailable: %v", err)
	}
	lim.Cur = 64 * 1024
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
		t.Skipf("cannot set RLIMIT_FSIZE: %v", err)
	}
	dir, err := os.MkdirTemp("", "voltix-fslimit")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	s, err := Open(dir, smallCfg())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	seedSeq, err := s.Insert(ctx, TierStatus, t0, []byte(`{"seed":1}`))
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Write until the kernel refuses — must surface an error, never panic
	// or silently succeed.
	var failErr error
	for i := 0; i < 100; i++ {
		if _, err := s.Insert(ctx, TierStatus, t0, []byte(strings.Repeat("x", 900))); err != nil {
			failErr = err
			break
		}
	}
	if failErr == nil {
		t.Fatal("no storage-write failure observed under RLIMIT_FSIZE")
	}
	t.Logf("write failure surfaced: %v", failErr)

	// The async commit path treats it the same way: unhealthy + counted.
	s.MarkHealthy()
	s.commit(queued{tier: TierStatus, at: t0, body: []byte(`{"v":1}`)})
	if s.Healthy() {
		t.Fatal("failed commit must mark store unhealthy")
	}
	if err := s.Offer(TierStatus, t0, []byte(`{"v":2}`)); err != ErrUnhealthy {
		t.Fatalf("offer while unhealthy: %v", err)
	}
	_, vol, err := s.Counters(ctx)
	if err != nil || vol[TierStatus] < 2 {
		t.Fatalf("volatile drops not counted: %v %v", vol, err)
	}

	// Prior committed rows remain readable and intact.
	evs, err := s.Batch(ctx, 5)
	if err != nil || len(evs) < 1 || evs[0].SeqID != seedSeq {
		t.Fatalf("reads degraded after write failure: %v %+v", err, evs)
	}

	// Raising the soft limit again restores writes — same on-disk file,
	// no corruption, no reset.
	lim.Cur = lim.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
		t.Fatalf("raise limit: %v", err)
	}
	s.MarkHealthy()
	if _, err := s.Insert(ctx, TierStatus, t0, []byte(`{"after":1}`)); err != nil {
		t.Fatalf("insert after limit raised: %v", err)
	}
}
