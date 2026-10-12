// Package store implements the bounded durable event queue from
// docs/project/features/telemetry.md: a per-store random stream identity,
// a monotonic sequence that is never reused, two bounded eviction tiers
// with persistent loss accounting, and a bounded nonblocking writer
// handoff that keeps the control path independent of storage health.
//
// Durability boundary: an event is durable only after Commit returns nil.
// The driver is modernc.org/sqlite (pure Go, BSD-3) — no CGO toolchain is
// required, which resolves the build implication half of BLK-002.
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// Tier selects which bounded class an event belongs to.
type Tier int

const (
	TierStatus Tier = iota // ordinary periodic snapshots
	TierAlarm              // transitions/faults — isolated capacity
)

const schemaVersion = 1

// Event is one queued record; stream+seq is the deduplication key.
type Event struct {
	StreamID     string
	SeqID        int64
	Tier         Tier
	ControllerID string
	ObservedAt   time.Time // UTC
	Payload      []byte    // ≤ MaxPayload
}

// Config bounds the store; zero values fall back to contract defaults.
type Config struct {
	ControllerID string
	MaxStatus    int64 // persistent status tier bound (default 90000)
	MaxAlarm     int64 // persistent alarm tier bound (default 10000)
	MaxPayload   int   // encoded payload bound bytes (default 1024)
	Handoff      int   // per-tier in-memory handoff slots (default 900/100)
}

func (c Config) withDefaults() Config {
	if c.MaxStatus <= 0 {
		c.MaxStatus = 90_000
	}
	if c.MaxAlarm <= 0 {
		c.MaxAlarm = 10_000
	}
	if c.MaxPayload <= 0 {
		c.MaxPayload = 1024
	}
	if c.Handoff <= 0 {
		c.Handoff = 900
	}
	return c
}

var (
	ErrPayloadTooLarge = errors.New("store: payload exceeds bound")
	ErrUnhealthy       = errors.New("store: storage unhealthy, event dropped")
	ErrDropped         = errors.New("store: handoff full, event dropped")
	ErrIncompatible    = errors.New("store: incompatible schema version")
)

// Store owns the single SQLite writer connection. Reads and ack deletes
// share it under short bounded transactions.
type Store struct {
	db       *sql.DB
	cfg      Config
	StreamID string

	mu        sync.Mutex
	volLost   [2]int64 // volatile drops since last persistence point
	handoffs  [2]chan queued
	accepting atomic.Bool
}

type queued struct {
	tier Tier
	at   time.Time
	body []byte
}

// Open creates or reopens the store at dir/events.db. A missing directory
// must already exist; incompatible schema or a corrupt file fails without
// touching it — never a silent reset.
func Open(dir string, cfg Config) (*Store, error) {
	cfg = cfg.withDefaults()
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(2000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)",
		filepath.Join(dir, "events.db"))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one writer owns the store
	s := &Store{db: db, cfg: cfg}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) init() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var ver int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&ver); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if ver != 0 && ver != schemaVersion {
		return fmt.Errorf("%w: user_version=%d", ErrIncompatible, ver)
	}
	if ver == 0 {
		// Fresh store: new stream identity + version stamp.
		if _, err := s.db.ExecContext(ctx, `
			CREATE TABLE IF NOT EXISTS meta (k TEXT PRIMARY KEY, v TEXT NOT NULL);
			CREATE TABLE IF NOT EXISTS counters (
				tier INTEGER PRIMARY KEY, evicted INTEGER NOT NULL DEFAULT 0,
				volatile_lost INTEGER NOT NULL DEFAULT 0);
			CREATE TABLE IF NOT EXISTS events (
				stream_id TEXT NOT NULL, seq_id INTEGER NOT NULL,
				tier INTEGER NOT NULL, controller_id TEXT NOT NULL,
				observed_at TEXT NOT NULL, payload BLOB NOT NULL,
				PRIMARY KEY (stream_id, seq_id));
			PRAGMA user_version = 1`); err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return err
		}
		s.StreamID = hex.EncodeToString(b)
		if _, err := s.db.ExecContext(ctx,
			"INSERT INTO meta(k,v) VALUES('stream_id',?),('next_seq','1')",
			s.StreamID); err != nil {
			return fmt.Errorf("seed meta: %w", err)
		}
	} else {
		if err := s.db.QueryRowContext(ctx,
			"SELECT v FROM meta WHERE k='stream_id'").Scan(&s.StreamID); err != nil {
			return fmt.Errorf("read stream identity: %w", err)
		}
	}
	// Pragmas verified on the actual connection — BLK-002 sub-gate.
	var jm string
	var syncMode int
	if err := s.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&jm); err != nil {
		return err
	}
	if jm != "wal" {
		return fmt.Errorf("journal_mode=%s, want wal", jm)
	}
	if err := s.db.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&syncMode); err != nil {
		return err
	}
	if syncMode != 2 { // FULL
		return fmt.Errorf("synchronous=%d, want FULL(2)", syncMode)
	}
	s.accepting.Store(true)
	return nil
}

// HandoffChans returns the bounded per-tier producer channels; RunWriter
// drains them. Sending more than capacity drops — Offer handles that.
func (s *Store) startHandoff() {
	if s.handoffs[0] == nil {
		s.handoffs[TierStatus] = make(chan queued, s.cfg.Handoff)
		s.handoffs[TierAlarm] = make(chan queued, max(1, s.cfg.Handoff/9))
	}
}

// Offer submits an event for async persistence; it never blocks the
// caller. Full handoff or unhealthy storage counts a volatile drop.
func (s *Store) Offer(tier Tier, observedAt time.Time, payload []byte) error {
	if len(payload) > s.cfg.MaxPayload {
		return ErrPayloadTooLarge
	}
	if !s.accepting.Load() {
		s.countVolatile(tier)
		return ErrUnhealthy
	}
	s.mu.Lock()
	s.startHandoff()
	ch := s.handoffs[tier]
	s.mu.Unlock()
	select {
	case ch <- queued{tier: tier, at: observedAt, body: payload}:
		return nil
	default:
		s.countVolatile(tier)
		return ErrDropped
	}
}

// RunWriter drains handoff channels into the store until ctx ends, then
// flushes what remains. It is the only producer-side goroutine.
func (s *Store) RunWriter(ctx context.Context) {
	s.mu.Lock()
	s.startHandoff()
	st, al := s.handoffs[TierStatus], s.handoffs[TierAlarm]
	s.mu.Unlock()
	for {
		select {
		case <-ctx.Done():
			s.drain(st, al)
			return
		case q := <-al: // alarms first — they are scarce and important
			s.commit(q)
		case q := <-st:
			s.commit(q)
		}
	}
}

func (s *Store) drain(chs ...chan queued) {
	for _, ch := range chs {
		for done := false; !done; {
			select {
			case q := <-ch:
				s.commit(q)
			default:
				done = true
			}
		}
	}
	s.persistVolatile()
}

func (s *Store) commit(q queued) {
	if _, err := s.Insert(context.Background(), q.tier, q.at, q.body); err != nil {
		s.markUnhealthy()
		s.countVolatile(q.tier)
	}
}

// Insert allocates the next sequence number, applies tier eviction and
// records the event in one short transaction. This is the durability
// boundary: after nil return the event is committed.
func (s *Store) Insert(ctx context.Context, tier Tier, at time.Time, payload []byte) (int64, error) {
	if len(payload) > s.cfg.MaxPayload {
		return 0, ErrPayloadTooLarge
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var seq int64
	if err := tx.QueryRowContext(ctx,
		"SELECT CAST(v AS INTEGER) FROM meta WHERE k='next_seq'").Scan(&seq); err != nil {
		return 0, fmt.Errorf("seq alloc: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE meta SET v=? WHERE k='next_seq'", seq+1); err != nil {
		return 0, err
	}
	var n int64
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM events WHERE stream_id=? AND tier=?",
		s.StreamID, int(tier)).Scan(&n); err != nil {
		return 0, err
	}
	if cap := s.tierCap(tier); n >= cap {
		// Atomic evict-oldest + persistent accounting in the same tx.
		if _, err := tx.ExecContext(ctx, `DELETE FROM events
			WHERE stream_id=? AND tier=? AND seq_id=(
				SELECT MIN(seq_id) FROM events WHERE stream_id=? AND tier=?)`,
			s.StreamID, int(tier), s.StreamID, int(tier)); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO counters(tier,evicted,volatile_lost)
			VALUES(?,1,0) ON CONFLICT(tier) DO UPDATE SET evicted=evicted+1`,
			int(tier)); err != nil {
			return 0, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO events
		(stream_id,seq_id,tier,controller_id,observed_at,payload)
		VALUES(?,?,?,?,?,?)`,
		s.StreamID, seq, int(tier), s.cfg.ControllerID,
		at.UTC().Format(time.RFC3339Nano), payload); err != nil {
		return 0, err
	}
	return seq, tx.Commit()
}

// Batch returns up to n oldest queued events for the syncer — a short
// read transaction, never held over network I/O.
func (s *Store) Batch(ctx context.Context, n int) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT seq_id,tier,controller_id,observed_at,payload
		FROM events WHERE stream_id=? ORDER BY seq_id LIMIT ?`, s.StreamID, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var at string
		if err := rows.Scan(&e.SeqID, &e.Tier, &e.ControllerID, &at, &e.Payload); err != nil {
			return nil, err
		}
		e.StreamID = s.StreamID
		e.ObservedAt, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteAcked removes only the given sequence numbers of this store's
// stream — eviction gaps are legal, missing IDs are ignored.
func (s *Store) DeleteAcked(ctx context.Context, seqs []int64) error {
	if len(seqs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.PrepareContext(ctx,
		"DELETE FROM events WHERE stream_id=? AND seq_id=?")
	if err != nil {
		return err
	}
	defer st.Close()
	for _, q := range seqs {
		if _, err := st.ExecContext(ctx, s.StreamID, q); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Depth reports queued rows per tier plus the oldest event age.
func (s *Store) Depth(ctx context.Context) (status, alarm int64, oldest time.Duration, err error) {
	if err = s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM events WHERE stream_id=? AND tier=0",
		s.StreamID).Scan(&status); err != nil {
		return
	}
	if err = s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM events WHERE stream_id=? AND tier=1",
		s.StreamID).Scan(&alarm); err != nil {
		return
	}
	var at *string
	_ = s.db.QueryRowContext(ctx,
		"SELECT MIN(observed_at) FROM events WHERE stream_id=?",
		s.StreamID).Scan(&at)
	if at != nil {
		if t, perr := time.Parse(time.RFC3339Nano, *at); perr == nil {
			oldest = time.Since(t)
		}
	}
	return
}

// Counters reports persistent evictions and accumulated losses.
func (s *Store) Counters(ctx context.Context) (evicted, volatileLost [2]int64, err error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT tier,evicted,volatile_lost FROM counters")
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var t, ev, vl int64
		if err = rows.Scan(&t, &ev, &vl); err != nil {
			return
		}
		if t == 0 || t == 1 {
			evicted[t], volatileLost[t] = ev, vl
		}
	}
	s.mu.Lock()
	for i := range volatileLost {
		volatileLost[i] += s.volLost[i]
	}
	s.mu.Unlock()
	return evicted, volatileLost, rows.Err()
}

// Healthy reports whether storage is currently accepting events.
func (s *Store) Healthy() bool { return s.accepting.Load() }

// Checkpoint runs a bounded WAL checkpoint and reports whether it was
// busy — a busy checkpoint is observable and retried, never blocking.
func (s *Store) Checkpoint(ctx context.Context) (busy bool, err error) {
	var b, log, ckpt int
	err = s.db.QueryRowContext(ctx,
		"PRAGMA wal_checkpoint(PASSIVE)").Scan(&b, &log, &ckpt)
	return b != 0, err
}

// persistVolatile flushes accumulated volatile loss counters into the
// durable counters table when storage is healthy.
func (s *Store) persistVolatile() {
	s.mu.Lock()
	pending := s.volLost
	s.volLost = [2]int64{}
	s.mu.Unlock()
	for tier, n := range pending {
		if n == 0 {
			continue
		}
		if _, err := s.db.ExecContext(context.Background(), `INSERT INTO
			counters(tier,evicted,volatile_lost) VALUES(?,0,?)
			ON CONFLICT(tier) DO UPDATE SET volatile_lost=volatile_lost+?`,
			tier, n, n); err != nil {
			s.mu.Lock()
			s.volLost[tier] += n // keep in memory if persistence fails
			s.mu.Unlock()
		}
	}
}

func (s *Store) countVolatile(t Tier) {
	s.mu.Lock()
	s.volLost[t]++
	s.mu.Unlock()
}

func (s *Store) markUnhealthy() { s.accepting.Store(false) }

// MarkHealthy is called after a successful probe write during recovery.
func (s *Store) MarkHealthy() { s.accepting.Store(true) }

func (s *Store) tierCap(t Tier) int64 {
	if t == TierAlarm {
		return s.cfg.MaxAlarm
	}
	return s.cfg.MaxStatus
}

// Close flushes volatile counters then closes the connection.
func (s *Store) Close() {
	s.persistVolatile()
	_ = s.db.Close()
}
