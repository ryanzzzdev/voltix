// Package dispatch implements the authenticated dispatch API and lease
// semantics from docs/project/features/control.md: a random boot session,
// strictly increasing command revisions, and monotonic-deadline conversion
// at acceptance so wall-clock steps cannot extend or prematurely expire
// intent.
package dispatch

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"
)

// Clock abstracts wall and monotonic time so tests can shift either
// independently. Mono is only ever compared to itself.
type Clock struct {
	Wall func() time.Time
	Mono func() time.Duration
}

func RealClock() Clock {
	start := time.Now()
	return Clock{
		Wall: time.Now,
		Mono: func() time.Duration { return time.Since(start) },
	}
}

const maxCommandLifetime = 30 * time.Second

// Request is the strict /api/dispatch payload schema.
type Request struct {
	ControllerID  string    `json:"controller_id"`
	BootSessionID string    `json:"boot_session_id"`
	CommandID     string    `json:"command_id"`
	Revision      int64     `json:"revision"`
	IssuedAt      time.Time `json:"issued_at"`
	ValidUntil    time.Time `json:"valid_until"`
	TargetKW      float64   `json:"target_kw"`
}

// Command is an accepted dispatch with a monotonic deadline.
type Command struct {
	Request
	DeadlineMono time.Duration
}

// Session tracks the boot session and the latest accepted command.
type Session struct {
	controllerID string
	bootID       string
	clock        Clock

	mu      sync.Mutex
	lastRev int64
	lastCmd *Command
}

var errClock = errors.New("dispatch: nil clock")

func NewSession(controllerID string, clock Clock) *Session {
	if clock.Wall == nil || clock.Mono == nil {
		panic(errClock)
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return &Session{controllerID: controllerID, bootID: hex.EncodeToString(b), clock: clock}
}

func (s *Session) BootSessionID() string { return s.bootID }

// Mono reports the session clock's monotonic reading, letting callers map
// a command's DeadlineMono onto their own cycle time.
func (s *Session) Mono() time.Duration { return s.clock.Mono() }

// Active returns the current intent and its monotonic deadline; ok=false
// when nothing is active or it has expired. Expiry is checked on BOTH the
// monotonic deadline and the wall valid_until, so a forward wall-clock
// jump expires conservatively while a backward step cannot extend intent.
func (s *Session) Active() (cmd Command, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.lastCmd
	if c == nil || s.clock.Mono() >= c.DeadlineMono ||
		!s.clock.Wall().Before(c.ValidUntil) {
		return Command{}, false
	}
	return *c, true
}

// accept validates a decoded request and, when valid, installs it.
// Duplicate identical retries return the stored command unchanged.
func (s *Session) accept(req Request) (Command, int, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if req.ControllerID != s.controllerID {
		return Command{}, http.StatusConflict, "controller_id mismatch"
	}
	if req.BootSessionID != s.bootID {
		return Command{}, http.StatusConflict, "stale boot session"
	}
	if !validUUID(req.CommandID) {
		return Command{}, http.StatusUnprocessableEntity, "command_id not a UUID"
	}

	// Exact retry of the active command is idempotent while still valid.
	if s.lastCmd != nil && req.CommandID == s.lastCmd.CommandID &&
		req.Revision == s.lastCmd.Revision {
		if req == s.lastCmd.Request {
			return *s.lastCmd, http.StatusAccepted, ""
		}
		return Command{}, http.StatusConflict, "same revision with changed fields"
	}
	if req.Revision <= s.lastRev {
		return Command{}, http.StatusConflict, "revision not increasing"
	}

	now := s.clock.Wall()
	if req.IssuedAt.After(now) || !now.Before(req.ValidUntil) {
		return Command{}, http.StatusUnprocessableEntity, "command expired or not yet valid"
	}
	life := req.ValidUntil.Sub(req.IssuedAt)
	if life <= 0 || life > maxCommandLifetime {
		return Command{}, http.StatusUnprocessableEntity,
			fmt.Sprintf("lifetime %v exceeds %v bound", life, maxCommandLifetime)
	}
	if math.IsNaN(req.TargetKW) || math.IsInf(req.TargetKW, 0) {
		return Command{}, http.StatusUnprocessableEntity, "non-finite target_kw"
	}

	// Freeze remaining lifetime as a monotonic deadline at acceptance.
	s.lastRev = req.Revision
	s.lastCmd = &Command{Request: req,
		DeadlineMono: s.clock.Mono() + req.ValidUntil.Sub(now)}
	return *s.lastCmd, http.StatusAccepted, ""
}

// Drop invalidates the active intent (called by the control loop when the
// policy signals expiry or degraded inputs). Revision state persists so
// old revisions stay rejected.
func (s *Session) Drop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastCmd = nil
}

// Expired reports whether the active intent has passed its deadline.
func (s *Session) Expired() bool {
	_, ok := s.Active()
	return !ok
}

// validUUID checks the 8-4-4-4-12 hyphenated hex form without pulling a dep.
func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'):
			return false
		}
	}
	return true
}

// decodeRequest enforces the 4 KiB bound and the strict field set.
func decodeRequest(w http.ResponseWriter, r *http.Request) (Request, error) {
	var req Request
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return req, err
	}
	return req, nil
}
