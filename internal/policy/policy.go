// Package policy implements the pure supervisory operating rules from
// docs/project/features/control.md. No sockets, HTTP or storage; all time
// is injected so behaviour is deterministic under test.
package policy

import (
	"encoding/json"
	"math"
	"time"
)

// State is the controller operating state.
type State int

const (
	Standby  State = iota // no fresh dispatch; request fallback zero
	Active                // fresh intent + valid measurements; constrained target
	Degraded              // expired/stale/gap/comms issue; invalidate and ramp to zero
	Fault                 // device-reported fault; inhibit nonzero commands
)

func (s State) String() string {
	return [...]string{"STANDBY", "ACTIVE", "DEGRADED", "FAULT"}[s]
}

func (s State) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// Reason codes recorded alongside a decision.
type Reason string

const (
	ReasonOK           Reason = "OK"
	ReasonNoDispatch   Reason = "NO_DISPATCH"
	ReasonExpired      Reason = "DISPATCH_EXPIRED"
	ReasonInvalidInput Reason = "INVALID_INPUT"
	ReasonStaleInput   Reason = "STALE_INPUT"
	ReasonGap          Reason = "TIMING_GAP"
	ReasonDeviceFault  Reason = "DEVICE_FAULT"
	ReasonSoCFloor     Reason = "SOC_DISCHARGE_FLOOR"
	ReasonSoCCeil      Reason = "SOC_CHARGE_CEILING"
	ReasonExportLimit  Reason = "SITE_EXPORT_LIMIT"
	ReasonInfeasible   Reason = "SITE_LIMIT_INFEASIBLE"
	ReasonSlew         Reason = "SLEW_LIMIT"
	ReasonHardBounds   Reason = "HARD_BOUNDS_REAPPLIED"
)

// Config holds the site's operating limits (watts; charge negative).
type Config struct {
	DischargeLimitW int64         // max discharge power, positive
	ChargeLimitW    int64         // max charge power magnitude, positive
	ExportLimitW    int64         // site export upper bound
	SoCFloorPct     float64       // discharge prohibited at/below
	SoCCeilPct      float64       // charge prohibited at/above
	SlewWPS         int64         // normal slew rate W/s
	MaxSnapshotAge  time.Duration // reject measurements older than this
	MaxGap          time.Duration // cycle gap that invalidates dispatch
}

// Intent is an active, unexpired dispatch command.
type Intent struct {
	TargetW   int64
	ExpiresAt time.Time // monotonic deadline
}

// Inputs are one cycle's validated readings and command state.
type Inputs struct {
	Now         time.Time
	InverterW   int64 // measured battery power, + discharge
	InverterOK  bool
	SoCPct      float64
	MeterW      int64 // measured site power, + export
	MeterOK     bool
	MeasuredAt  time.Time // acquisition start of the freshest reading
	DecisionAt  time.Time // post-read decision time; zero falls back to Now
	DeviceFault bool
	Intent      *Intent // nil = no dispatch
}

// decisionAt returns the staleness reference time: DecisionAt when the
// caller recorded it (post-read), otherwise Now.
func (in Inputs) decisionAt() time.Time {
	if in.DecisionAt.IsZero() {
		return in.Now
	}
	return in.DecisionAt
}

// Decision is the supervisor's per-cycle outcome.
type Decision struct {
	State      State
	TargetW    int64 // device target to request this cycle
	Invalidate bool  // caller must drop the active intent
	Reasons    []Reason
}

// Supervisor carries cross-cycle control state only; it never performs I/O.
type Supervisor struct {
	cfg        Config
	lastAt     time.Time
	prevTarget int64
	recovering bool // after a gap: ramp from measured, ≤1 s allowance
}

func New(cfg Config) *Supervisor { return &Supervisor{cfg: cfg} }

// Step evaluates one control cycle. dt semantics: the first call uses a
// one-second allowance with the measured output as ramp baseline; gaps over
// MaxGap invalidate the intent without multiplying by the full elapsed time.
func (s *Supervisor) Step(in Inputs) Decision {
	c := s.cfg
	first := s.lastAt.IsZero()
	dt := in.Now.Sub(s.lastAt)
	s.lastAt = in.Now

	d := Decision{Reasons: []Reason{}}
	reason := func(r Reason) { d.Reasons = append(d.Reasons, r) }

	gap := !first && dt > c.MaxGap
	if gap {
		reason(ReasonGap)
		s.recovering = true
	}

	switch {
	case in.DeviceFault:
		// Contract: fault inhibits nonzero commands — an immediate zero
		// request, not a slew toward zero — AND invalidates the intent;
		// recovery requires clear status plus a fresh dispatch.
		reason(ReasonDeviceFault)
		d.State, d.Invalidate, d.TargetW = Fault, true, 0
	case !in.InverterOK || !in.MeterOK || math.IsNaN(in.SoCPct) || math.IsInf(in.SoCPct, 0):
		reason(ReasonInvalidInput)
		d.State, d.Invalidate, d.TargetW = Degraded, true, s.rampTo(in, 0, first, gap)
	case in.decisionAt().Sub(in.MeasuredAt) > c.MaxSnapshotAge:
		reason(ReasonStaleInput)
		d.State, d.Invalidate, d.TargetW = Degraded, true, s.rampTo(in, 0, first, gap)
	case gap:
		d.State, d.Invalidate, d.TargetW = Degraded, true, s.rampTo(in, 0, first, true)
	case in.Intent == nil:
		reason(ReasonNoDispatch)
		d.State, d.TargetW = Standby, s.rampTo(in, 0, first, false)
	case !in.Now.Before(in.Intent.ExpiresAt):
		reason(ReasonExpired)
		d.State, d.Invalidate, d.TargetW = Degraded, true, s.rampTo(in, 0, first, false)
	default:
		d.State = Active
		d.TargetW = s.activeTarget(in, &d, first, dt)
	}

	s.prevTarget = d.TargetW
	s.recovering = d.State == Degraded
	if len(d.Reasons) == 0 {
		d.Reasons = []Reason{ReasonOK}
	}
	return d
}

// activeTarget computes the constrained target for a healthy ACTIVE cycle:
// hard interval first, then slew, then hard bounds reapplied on top.
func (s *Supervisor) activeTarget(in Inputs, d *Decision, first bool, dt time.Duration) int64 {
	lo, hi := s.interval(in, d)
	if lo > hi {
		d.Reasons = append(d.Reasons, ReasonInfeasible)
		return s.rampTo(in, 0, first, s.recovering)
	}
	clamped := clamp(in.Intent.TargetW, lo, hi)

	base := s.prevTarget
	if first || s.recovering {
		base = in.InverterW // fresh measured baseline at startup/recovery
	}
	allow := s.cfg.SlewWPS
	if !first && dt < time.Second {
		allow = allow * int64(dt) / int64(time.Second)
	}
	target := base + clamp(clamped-base, -allow, allow)
	if target != clamped {
		d.Reasons = append(d.Reasons, ReasonSlew)
	}
	if hb := clamp(target, lo, hi); hb != target {
		target = hb
		d.Reasons = append(d.Reasons, ReasonHardBounds)
	}
	return target
}

// interval returns the hard [charge, discharge] window for this cycle.
func (s *Supervisor) interval(in Inputs, d *Decision) (lo, hi int64) {
	lo, hi = -s.cfg.ChargeLimitW, s.cfg.DischargeLimitW
	if in.SoCPct <= s.cfg.SoCFloorPct {
		hi = min(hi, 0)
		d.Reasons = append(d.Reasons, ReasonSoCFloor)
	}
	if in.SoCPct >= s.cfg.SoCCeilPct {
		lo = max(lo, 0)
		d.Reasons = append(d.Reasons, ReasonSoCCeil)
	}
	baseline := in.MeterW - in.InverterW // uncontrollable site net export
	lim := s.cfg.ExportLimitW - baseline
	// Flag only when the bound actually clamps the achievable interval
	// below what the request could otherwise reach.
	if lim < hi && in.Intent != nil && in.Intent.TargetW > lim {
		d.Reasons = append(d.Reasons, ReasonExportLimit)
	}
	if lim < hi {
		hi = lim
	}
	return lo, hi
}

// rampTo moves the request toward goal with at most one second of slew
// allowance; the baseline is measured output on first/recovery cycles.
func (s *Supervisor) rampTo(in Inputs, goal int64, first, useMeasured bool) int64 {
	base := s.prevTarget
	if first || useMeasured {
		base = in.InverterW
	}
	return base + clamp(goal-base, -s.cfg.SlewWPS, s.cfg.SlewWPS)
}

// WriteFailed records that the last requested target was not
// acknowledged. Contract: slew is based on the previous *acknowledged*
// target — after an uncertain write the next cycle ramps from fresh
// measured output rather than an unconfirmed request.
func (s *Supervisor) WriteFailed() { s.recovering = true }

func clamp(v, lo, hi int64) int64 { return min(max(v, lo), hi) }
