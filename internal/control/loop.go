// Package control runs the 1 Hz supervisory loop from
// docs/project/features/control.md. One goroutine owns all active
// dispatch, device writes and operating state; device reads run
// concurrently and snapshots are published nonblockingly.
package control

import (
	"context"
	"sync"
	"time"

	"voltix/internal/device"
	"voltix/internal/dispatch"
	"voltix/internal/policy"
)

// Endpoints abstract the device adapters so tests can fake them.
type Inverter interface {
	ReadInverter() (device.InverterReading, error)
	SetPowerTargetW(watts int32) error
}
type Meter interface {
	ReadMeter() (device.MeterReading, error)
}

// Snapshot is one cycle's diagnostic record for the status surface.
type Snapshot struct {
	At            time.Time
	State         policy.State
	Reasons       []policy.Reason
	RequestedW    int64  // accepted intent target
	ConstrainedW  int64  // policy output this cycle
	Acknowledged  *int64 // last write accepted by the device; nil = uncertain
	MeasuredInvW  int64
	MeasuredSiteW int64
	ReadDuration  time.Duration // concurrent read phase
	WriteDuration time.Duration // serialised write phase
	InverterOK    bool
	MeterOK       bool
}

// Session is the dispatch lease surface the loop needs — narrow enough
// to fake in tests.
type Session interface {
	Active() (dispatch.Command, bool)
	Drop()
	Mono() time.Duration
}

// LoopConfig wires the loop's dependencies and bounds.
type LoopConfig struct {
	Inverter  Inverter
	Meter     Meter
	Policy    *policy.Supervisor
	Session   Session
	Period    time.Duration // nominal 1 Hz
	Now       func() time.Time
	Snapshots chan Snapshot // bounded; drops when full
}

type Loop struct {
	cfg      LoopConfig
	lastAck  *int64 // last device-acknowledged target; nil = uncertain
	writeErr error

	mu      sync.Mutex
	last    Snapshot
	hasLast bool
}

func New(cfg LoopConfig) *Loop {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Period <= 0 {
		cfg.Period = time.Second
	}
	return &Loop{cfg: cfg}
}

// Run steps at the configured period until ctx is cancelled, then issues
// one bounded zero request before returning.
func (l *Loop) Run(ctx context.Context) {
	tick := time.NewTicker(l.cfg.Period)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			l.shutdownWrite()
			return
		case now := <-tick.C:
			l.Step(now)
		}
	}
}

// step performs one read-policy-write cycle. Reads run concurrently; the
// write is serialised through this single owner. Exported as Step for
// deterministic tests that drive time themselves.
func (l *Loop) Step(now time.Time) Snapshot {
	r0 := time.Now()
	type invRes struct {
		r   device.InverterReading
		err error
	}
	type metRes struct {
		r   device.MeterReading
		err error
	}
	invCh := make(chan invRes, 1)
	metCh := make(chan metRes, 1)
	go func() { r, err := l.cfg.Inverter.ReadInverter(); invCh <- invRes{r, err} }()
	go func() { r, err := l.cfg.Meter.ReadMeter(); metCh <- metRes{r, err} }()
	inv, met := <-invCh, <-metCh
	readDur := time.Since(r0)

	in := policy.Inputs{
		Now:        now,
		MeasuredAt: r0, // staleness == this cycle's read latency
		InverterW:  inv.r.Watts,
		InverterOK: inv.err == nil,
		SoCPct:     inv.r.SoCPct,
		MeterW:     met.r.Watts,
		MeterOK:    met.err == nil,
		// Evt1 bit0 = device watchdog tripped (informational), bit1 = fault.
		DeviceFault: inv.err == nil && inv.r.Evt1&2 != 0,
	}
	if cmd, ok := l.cfg.Session.Active(); ok {
		// Map the session's monotonic deadline onto this cycle's Now.
		remaining := cmd.DeadlineMono - l.cfg.Session.Mono()
		in.Intent = &policy.Intent{
			TargetW:   int64(cmd.TargetKW * 1000),
			ExpiresAt: now.Add(remaining),
		}
	}

	d := l.cfg.Policy.Step(in)
	if d.Invalidate {
		l.cfg.Session.Drop()
	}

	w0 := time.Now()
	if err := l.cfg.Inverter.SetPowerTargetW(int32(d.TargetW)); err != nil {
		l.lastAck = nil // acknowledged output uncertain; no stale retry
		l.writeErr = err
	} else {
		l.lastAck = &d.TargetW
		l.writeErr = nil
	}
	writeDur := time.Since(w0)

	snap := Snapshot{
		At: now, State: d.State, Reasons: d.Reasons,
		ConstrainedW:  d.TargetW,
		Acknowledged:  l.lastAck,
		MeasuredInvW:  inv.r.Watts,
		MeasuredSiteW: met.r.Watts,
		ReadDuration:  readDur, WriteDuration: writeDur,
		InverterOK: inv.err == nil, MeterOK: met.err == nil,
	}
	if in.Intent != nil {
		snap.RequestedW = in.Intent.TargetW
	}
	l.publish(snap)
	l.mu.Lock()
	l.last, l.hasLast = snap, true
	l.mu.Unlock()
	return snap
}

// Latest returns the most recent cycle's snapshot for the status surface.
func (l *Loop) Latest() (Snapshot, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.last, l.hasLast
}

// publish is nonblocking and bounded: a slow consumer costs snapshots,
// never loop time.
func (l *Loop) publish(s Snapshot) {
	if l.cfg.Snapshots == nil {
		return
	}
	select {
	case l.cfg.Snapshots <- s:
	default:
	}
}

// shutdownWrite attempts one bounded zero request; failures are
// intentionally ignored — bounded shutdown beats a last-word write.
func (l *Loop) shutdownWrite() {
	_ = l.cfg.Inverter.SetPowerTargetW(0)
}

// mono reports the session's monotonic clock — retained for tests that
// need deadline arithmetic against the same clock the session uses.
func (l *Loop) mono() time.Duration {
	return l.cfg.Session.Mono()
}
