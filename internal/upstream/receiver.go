// Package upstream provides the transactional mock receiver and dispatch
// client used for acceptance testing. The receiver persists telemetry
// batches to its own SQLite store (WAL + FULL, same driver as the event
// store) and deduplicates on (stream_id, seq_id); the dispatcher learns
// the controller's boot session and issues fresh revisions.
package upstream

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Batch is the POST /api/telemetry envelope.
type Batch struct {
	SchemaVersion int     `json:"schema_version"`
	StreamID      string  `json:"stream_id"`
	Events        []Event `json:"events"`
}

// Event mirrors the wire record. Content equality against a stored row
// decides duplicate no-op vs conflict.
type Event struct {
	SeqID        int64     `json:"seq_id"`
	Tier         int       `json:"tier"`
	ControllerID string    `json:"controller_id"`
	ObservedAt   time.Time `json:"observed_at"`
	Payload      []byte    `json:"payload"`
}

const (
	maxBatchEvents = 100
	maxBody        = 256 * 1024
)

// Receiver is the mock upstream: durable, transactional, idempotent.
type Receiver struct {
	db    *sql.DB
	token string
	mux   *http.ServeMux

	mu             sync.Mutex
	paused         bool
	dropAfterWrite bool // test mode: commit then close without response
}

// OpenReceiver creates/reopens the receiver store at dir (WAL+FULL).
func OpenReceiver(dir, token string) (*Receiver, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(2000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)",
		filepath.Join(dir, "receiver.db"))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS received (
		stream_id TEXT NOT NULL, seq_id INTEGER NOT NULL, tier INTEGER NOT NULL,
		controller_id TEXT NOT NULL, observed_at TEXT NOT NULL,
		payload BLOB NOT NULL, PRIMARY KEY(stream_id, seq_id))`); err != nil {
		db.Close()
		return nil, err
	}
	r := &Receiver{db: db, token: token, mux: http.NewServeMux()}
	r.mux.HandleFunc("POST /api/telemetry", r.handleTelemetry)
	// Test-only controls; documented as such, loopback-only in tests.
	r.mux.HandleFunc("POST /control/pause", r.handlePause)
	r.mux.HandleFunc("POST /control/resume", r.handleResume)
	r.mux.HandleFunc("POST /control/reset", r.handleReset)
	r.mux.HandleFunc("POST /control/drop-after-commit", r.handleDropMode)
	r.mux.HandleFunc("GET /control/count", r.handleCount)
	return r, nil
}

func (r *Receiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

func (r *Receiver) Close() { _ = r.db.Close() }

func (r *Receiver) handleTelemetry(w http.ResponseWriter, req *http.Request) {
	if subtle.ConstantTimeCompare(
		[]byte(req.Header.Get("Authorization")),
		[]byte("Bearer "+r.token)) != 1 {
		fail(w, http.StatusUnauthorized, "bad token")
		return
	}
	r.mu.Lock()
	paused, drop := r.paused, r.dropAfterWrite
	r.mu.Unlock()
	if paused {
		fail(w, http.StatusServiceUnavailable, "receiver paused")
		return
	}
	var b Batch
	dec := json.NewDecoder(http.MaxBytesReader(w, req.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		fail(w, http.StatusBadRequest, "malformed or oversized batch")
		return
	}
	if b.SchemaVersion != 1 || b.StreamID == "" ||
		len(b.Events) == 0 || len(b.Events) > maxBatchEvents {
		fail(w, http.StatusUnprocessableEntity, "invalid batch")
		return
	}
	for _, e := range b.Events {
		if e.SeqID <= 0 || e.Tier < 0 || e.Tier > 1 || len(e.Payload) > 1024 {
			fail(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("event seq=%d invalid", e.SeqID))
			return
		}
	}
	acked, err := r.commitBatch(req.Context(), b)
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	if drop {
		// Commit happened; simulate the response lost on the wire by
		// aborting the connection before any bytes are written.
		panic(http.ErrAbortHandler)
	}
	respond(w, http.StatusOK, map[string]any{
		"stream_id": b.StreamID, "acked_seq_ids": acked,
	})
}

// commitBatch validates events and inserts the full batch atomically.
// Identical duplicates are no-ops; conflicting content rejects the batch.
func (r *Receiver) commitBatch(ctx context.Context, b Batch) ([]int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	acked := make([]int64, 0, len(b.Events))
	for _, e := range b.Events {
		var stored []byte
		err := tx.QueryRowContext(ctx, `SELECT payload FROM received
			WHERE stream_id=? AND seq_id=?`, b.StreamID, e.SeqID).Scan(&stored)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(ctx, `INSERT INTO received
				(stream_id,seq_id,tier,controller_id,observed_at,payload)
				VALUES(?,?,?,?,?,?)`, b.StreamID, e.SeqID, e.Tier,
				e.ControllerID, e.ObservedAt.UTC().Format(time.RFC3339Nano),
				e.Payload); err != nil {
				return nil, err
			}
			acked = append(acked, e.SeqID)
		case err != nil:
			return nil, err
		case string(stored) != string(e.Payload):
			return nil, fmt.Errorf("conflicting content for seq=%d", e.SeqID)
		default: // identical duplicate: no-op, still acknowledged
			acked = append(acked, e.SeqID)
		}
	}
	return acked, tx.Commit()
}

// Test-only controls ---------------------------------------------------------

func (r *Receiver) handlePause(w http.ResponseWriter, _ *http.Request) {
	r.mu.Lock()
	r.paused = true
	r.mu.Unlock()
	respond(w, 200, map[string]any{"paused": true})
}
func (r *Receiver) handleResume(w http.ResponseWriter, _ *http.Request) {
	r.mu.Lock()
	r.paused = false
	r.mu.Unlock()
	respond(w, 200, map[string]any{"paused": false})
}
func (r *Receiver) handleReset(w http.ResponseWriter, req *http.Request) {
	// Test environments only: wipes received rows.
	if _, err := r.db.ExecContext(req.Context(), "DELETE FROM received"); err != nil {
		fail(w, 500, err.Error())
		return
	}
	respond(w, 200, map[string]any{"reset": true})
}
func (r *Receiver) handleDropMode(w http.ResponseWriter, _ *http.Request) {
	r.mu.Lock()
	r.dropAfterWrite = true
	r.mu.Unlock()
	respond(w, 200, map[string]any{"drop_after_commit": true})
}
func (r *Receiver) handleCount(w http.ResponseWriter, req *http.Request) {
	var n int
	_ = r.db.QueryRowContext(req.Context(), "SELECT COUNT(*) FROM received").Scan(&n)
	respond(w, 200, map[string]any{"count": n})
}

func respond(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, code int, msg string) {
	respond(w, code, map[string]any{"error": msg})
}
