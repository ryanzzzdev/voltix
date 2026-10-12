// Package syncer implements bounded upload and acknowledgement handling
// from docs/project/features/telemetry.md: batches of up to 100 events
// flushed at least every 5 s, one in-flight upload per stream, jittered
// bounded retry on transient failures, and pruning of only acknowledged
// rows — database transactions are never held over the network.
package syncer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"sync/atomic"
	"time"

	"voltix/internal/store"
	"voltix/internal/upstream"
)

var ErrAckMismatch = errors.New("syncer: acknowledgement does not match batch")

// Config bounds the uploader; zero values use contract defaults.
type Config struct {
	URL         string
	Token       string
	BatchMax    int           // default 100
	FlushWait   time.Duration // default 5s
	HTTPTimeout time.Duration // default 3s
	BackoffInit time.Duration // default 1s
	BackoffCap  time.Duration // default 30s
	Recheck     time.Duration // config-fault recheck interval (default 30s)
	Rand        *rand.Rand    // jitter source; nil → seeded
}

func (c Config) withDefaults() Config {
	if c.BatchMax <= 0 {
		c.BatchMax = 100
	}
	if c.FlushWait <= 0 {
		c.FlushWait = 5 * time.Second
	}
	if c.HTTPTimeout <= 0 {
		c.HTTPTimeout = 3 * time.Second
	}
	if c.BackoffInit <= 0 {
		c.BackoffInit = time.Second
	}
	if c.BackoffCap <= 0 {
		c.BackoffCap = 30 * time.Second
	}
	if c.Recheck <= 0 {
		c.Recheck = 30 * time.Second
	}
	if c.Rand == nil {
		c.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return c
}

// Stats is a consistent point-in-time view of uploader health.
type Stats struct {
	Uploaded    int64
	Failed      int64
	ConfigFault bool
	LastError   string
}

// Uploader runs one drain loop for a single store stream.
type Uploader struct {
	store  *store.Store
	cfg    Config
	client *http.Client

	uploaded    atomic.Int64
	failed      atomic.Int64
	configFault atomic.Bool
	lastErr     atomic.Pointer[string]
}

func New(s *store.Store, cfg Config) *Uploader {
	cfg = cfg.withDefaults()
	return &Uploader{
		store: s, cfg: cfg,
		client: &http.Client{Timeout: cfg.HTTPTimeout},
	}
}

// Stats returns uploader health; safe to call concurrently with Run.
func (u *Uploader) Stats() Stats {
	st := Stats{
		Uploaded:    u.uploaded.Load(),
		Failed:      u.failed.Load(),
		ConfigFault: u.configFault.Load(),
	}
	if p := u.lastErr.Load(); p != nil {
		st.LastError = *p
	}
	return st
}

func (u *Uploader) setErr(err error) {
	msg := err.Error()
	u.lastErr.Store(&msg)
	u.failed.Add(1)
}

// Run uploads until ctx ends. Backoff/flush waits are ctx-aware so
// shutdown is prompt and bounded by the HTTP deadline.
func (u *Uploader) Run(ctx context.Context) {
	backoff := u.cfg.BackoffInit
	for {
		sent, err := u.round(ctx)
		switch {
		case err == nil:
			backoff = u.cfg.BackoffInit
			u.configFault.Store(false)
			if sent < u.cfg.BatchMax {
				if !sleep(ctx, u.cfg.FlushWait) {
					return
				}
			} // full batch → immediately fetch the next
		case errors.Is(err, ErrAckMismatch) || isClientFault(err):
			// Auth/schema problems are configuration faults: visible,
			// slow bounded rechecks — no hot loop, no data deletion.
			u.configFault.Store(true)
			u.setErr(err)
			if !sleep(ctx, u.cfg.Recheck) {
				return
			}
		case errors.Is(err, context.Canceled):
			return
		default: // transient: jittered bounded backoff, reset on success
			u.setErr(err)
			if !sleep(ctx, jitter(u.cfg.Rand, backoff)) {
				return
			}
			if backoff < u.cfg.BackoffCap {
				backoff = min(backoff*2, u.cfg.BackoffCap)
			}
		}
	}
}

// round takes one batch, posts it, and prunes acknowledged rows only.
func (u *Uploader) round(ctx context.Context) (int, error) {
	bctx, cancel := context.WithTimeout(ctx, u.cfg.HTTPTimeout+5*time.Second)
	defer cancel()
	events, err := u.store.Batch(bctx, u.cfg.BatchMax)
	if err != nil || len(events) == 0 {
		return 0, err
	}
	body, _ := json.Marshal(upstream.Batch{
		SchemaVersion: 1, StreamID: u.store.StreamID,
		Events: toWire(events),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.cfg.URL,
		bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+u.cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := u.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, &httpError{code: resp.StatusCode}
	}
	var ack struct {
		StreamID    string  `json:"stream_id"`
		AckedSeqIDs []int64 `json:"acked_seq_ids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ack); err != nil {
		return 0, fmt.Errorf("%w: undecodable response", ErrAckMismatch)
	}
	if ack.StreamID != u.store.StreamID {
		return 0, fmt.Errorf("%w: foreign stream %q", ErrAckMismatch, ack.StreamID)
	}
	sent := make(map[int64]bool, len(events))
	for _, e := range events {
		sent[e.SeqID] = true
	}
	for _, id := range ack.AckedSeqIDs {
		if !sent[id] {
			return 0, fmt.Errorf("%w: acked unsent seq %d", ErrAckMismatch, id)
		}
	}
	// Prune only acknowledged rows; eviction gaps/missing IDs are ignored.
	if err := u.store.DeleteAcked(bctx, ack.AckedSeqIDs); err != nil {
		return 0, err
	}
	u.uploaded.Add(int64(len(ack.AckedSeqIDs)))
	return len(events), nil
}

func toWire(evs []store.Event) []upstream.Event {
	out := make([]upstream.Event, len(evs))
	for i, e := range evs {
		out[i] = upstream.Event{
			SeqID: e.SeqID, Tier: int(e.Tier), ControllerID: e.ControllerID,
			ObservedAt: e.ObservedAt, Payload: e.Payload,
		}
	}
	return out
}

type httpError struct{ code int }

func (e *httpError) Error() string { return fmt.Sprintf("HTTP %d", e.code) }

// isClientFault reports auth/schema rejections: configuration faults.
func isClientFault(err error) bool {
	var he *httpError
	if errors.As(err, &he) {
		return he.code == 400 || he.code == 401 || he.code == 403 || he.code == 422
	}
	return false
}

// jitter returns ±25% of d.
func jitter(r *rand.Rand, d time.Duration) time.Duration {
	return time.Duration(float64(d) * (0.75 + r.Float64()*0.5))
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
