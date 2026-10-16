package policy

import (
	"math"
	"testing"
	"time"
)

var cfg = Config{
	DischargeLimitW: 500_000,
	ChargeLimitW:    500_000,
	ExportLimitW:    400_000,
	SoCFloorPct:     5,
	SoCCeilPct:      95,
	SlewWPS:         50_000,
	MaxSnapshotAge:  2 * time.Second,
	MaxGap:          2500 * time.Millisecond,
}

var t0 = time.Unix(0, 0)

func at(sec float64) time.Time {
	return t0.Add(time.Duration(sec * float64(time.Second)))
}

// healthy returns a fully valid Inputs at t with the given measurements.
func healthy(t time.Time, invW, metW int64, soc float64, intent *Intent) Inputs {
	return Inputs{
		Now: t, InverterW: invW, InverterOK: true, SoCPct: soc,
		MeterW: metW, MeterOK: true, MeasuredAt: t, Intent: intent,
	}
}

func intent(t time.Time, targetW int64, validSec float64) *Intent {
	return &Intent{TargetW: targetW, ExpiresAt: t.Add(time.Duration(validSec * float64(time.Second)))}
}

func hasReason(d Decision, r Reason) bool {
	for _, x := range d.Reasons {
		if x == r {
			return true
		}
	}
	return false
}

func TestStandbyRequestsZero(t *testing.T) {
	s := New(cfg)
	d := s.Step(healthy(at(0), 120_000, 120_000, 50, nil))
	if d.State != Standby || !hasReason(d, ReasonNoDispatch) {
		t.Fatalf("want STANDBY/NO_DISPATCH, got %v %v", d.State, d.Reasons)
	}
	// Startup ramps from measured baseline at slew rate: 120 kW -> 70 kW.
	if d.TargetW != 70_000 {
		t.Fatalf("want 70000 fallback target, got %d", d.TargetW)
	}
}

func TestActiveSlewFromMeasured(t *testing.T) {
	s := New(cfg)
	d := s.Step(healthy(at(0), 0, 0, 50, intent(at(0), 300_000, 30)))
	if d.State != Active || d.TargetW != 50_000 {
		t.Fatalf("want ACTIVE ramp to 50k, got %v %d", d.State, d.TargetW)
	}
	if !hasReason(d, ReasonSlew) {
		t.Fatalf("want SLEW reason, got %v", d.Reasons)
	}
	// Next cycle continues from previous target.
	d = s.Step(healthy(at(1), 50_000, 50_000, 50, intent(at(1), 300_000, 30)))
	if d.TargetW != 100_000 {
		t.Fatalf("want 100k after another second, got %d", d.TargetW)
	}
}

func TestSoCBoundariesExact(t *testing.T) {
	// 5.0% floor: discharge prohibited, charge allowed.
	s := New(cfg)
	d := s.Step(healthy(at(0), 0, 0, 5.0, intent(at(0), 100_000, 30)))
	if !hasReason(d, ReasonSoCFloor) || d.TargetW != 0 {
		t.Fatalf("floor: want inhibit, got %d %v", d.TargetW, d.Reasons)
	}
	d = s.Step(healthy(at(1), 0, 0, 5.0, intent(at(1), -100_000, 30)))
	if d.TargetW != -50_000 {
		t.Fatalf("floor charge allowed: got %d", d.TargetW)
	}
	// 95.0% ceiling: charge prohibited, discharge allowed.
	d = s.Step(healthy(at(2), 0, 0, 95.0, intent(at(2), -100_000, 30)))
	if !hasReason(d, ReasonSoCCeil) || d.TargetW != 0 {
		t.Fatalf("ceil: want inhibit, got %d %v", d.TargetW, d.Reasons)
	}
	d = s.Step(healthy(at(3), 0, 0, 95.0, intent(at(3), 100_000, 30)))
	if d.TargetW != 50_000 {
		t.Fatalf("ceil discharge allowed: got %d", d.TargetW)
	}
}

func TestSiteExportBound(t *testing.T) {
	s := New(cfg)
	// Meter +300kW while battery at 0 -> baseline 300kW uncontrollable export.
	// Export limit 400kW -> battery headroom = 100kW.
	d := s.Step(healthy(at(0), 0, 300_000, 50, intent(at(0), 250_000, 30)))
	if !hasReason(d, ReasonExportLimit) {
		t.Fatalf("want EXPORT_LIMIT, got %v", d.Reasons)
	}
	if d.TargetW != 50_000 { // slew also binds: min(clamped 100k, 1s slew) = 50k
		t.Fatalf("want 50000, got %d", d.TargetW)
	}
	// Next cycle baseline still 300kW; slew allows 50k more, hard cap = 150k.
	d = s.Step(healthy(at(1), 50_000, 300_000, 50, intent(at(1), 250_000, 30)))
	if d.TargetW != 100_000 || !hasReason(d, ReasonSlew) {
		t.Fatalf("slew under export cap: got %d %v", d.TargetW, d.Reasons)
	}
}

func TestHardBoundsPrecedenceOverSlew(t *testing.T) {
	s := New(cfg)
	// Ramp up to 100 kW discharge...
	s.Step(healthy(at(0), 0, 0, 50, intent(at(0), 300_000, 30)))
	s.Step(healthy(at(1), 50_000, 50_000, 50, intent(at(1), 300_000, 30)))
	// ...then SoC hits the floor: hi=0. Slew alone would yield 50 kW; the
	// re-applied hard bound must clamp it to 0.
	d := s.Step(healthy(at(2), 100_000, 100_000, 5.0, intent(at(2), 300_000, 30)))
	if !hasReason(d, ReasonSoCFloor) || !hasReason(d, ReasonHardBounds) {
		t.Fatalf("want floor+hardbounds, got %v", d.Reasons)
	}
	if d.TargetW != 0 {
		t.Fatalf("hard bound must override ramp: got %d", d.TargetW)
	}
}

func TestInfeasibleInterval(t *testing.T) {
	s := New(cfg)
	// SoC=95 -> only discharge (lo=0). Baseline export 450kW > limit 400kW
	// -> hi = -50k. Empty interval: fallback zero + INFEASIBLE.
	d := s.Step(healthy(at(0), 0, 450_000, 95.0, intent(at(0), 100_000, 30)))
	if !hasReason(d, ReasonInfeasible) || d.TargetW != 0 {
		t.Fatalf("infeasible: want 0+INFEASIBLE, got %d %v", d.TargetW, d.Reasons)
	}
}

func TestExpiredIntentInvalidates(t *testing.T) {
	s := New(cfg)
	s.Step(healthy(at(0), 0, 0, 50, intent(at(0), 300_000, 0.5)))
	// t=1 is past the intent's 0.5 s expiry but within the 2.5 s gap limit.
	d := s.Step(healthy(at(1), 300_000, 300_000, 50, intent(at(0), 300_000, 0.5)))
	if d.State != Degraded || !d.Invalidate || !hasReason(d, ReasonExpired) {
		t.Fatalf("expiry: got %v inv=%v %v", d.State, d.Invalidate, d.Reasons)
	}
}

func TestStaleAndInvalidInputs(t *testing.T) {
	s := New(cfg)
	s.Step(healthy(at(0), 0, 0, 50, intent(at(0), 100_000, 30)))

	// Snapshot too old.
	in := healthy(at(1), 0, 0, 50, intent(at(1), 100_000, 30))
	in.MeasuredAt = at(1).Add(-3 * time.Second)
	d := s.Step(in)
	if d.State != Degraded || !hasReason(d, ReasonStaleInput) || !d.Invalidate {
		t.Fatalf("stale: %v %v", d.State, d.Reasons)
	}

	// Failed read.
	s2 := New(cfg)
	in2 := healthy(at(0), 0, 0, 50, intent(at(0), 100_000, 30))
	in2.InverterOK = false
	d = s2.Step(in2)
	if d.State != Degraded || !hasReason(d, ReasonInvalidInput) || !d.Invalidate {
		t.Fatalf("invalid: %v %v", d.State, d.Reasons)
	}

	// Non-finite SoC.
	s3 := New(cfg)
	in3 := healthy(at(0), 0, 0, math.NaN(), intent(at(0), 100_000, 30))
	d = s3.Step(in3)
	if d.State != Degraded || !hasReason(d, ReasonInvalidInput) {
		t.Fatalf("NaN: %v %v", d.State, d.Reasons)
	}
}

func TestGapInvalidatesAndRecoversFromMeasured(t *testing.T) {
	s := New(cfg)
	s.Step(healthy(at(0), 0, 0, 50, intent(at(0), 300_000, 30)))
	s.Step(healthy(at(1), 50_000, 50_000, 50, intent(at(1), 300_000, 30)))

	// 3 s pause (scheduled or wall) > 2.5 s: intent invalidated, fallback
	// ramps from measured output with ≤1 s allowance (not 3 s worth).
	d := s.Step(healthy(at(4), 100_000, 100_000, 50, intent(at(4), 300_000, 30)))
	if d.State != Degraded || !hasReason(d, ReasonGap) || !d.Invalidate {
		t.Fatalf("gap: %v inv=%v %v", d.State, d.Invalidate, d.Reasons)
	}
	if d.TargetW != 50_000 { // 100kW measured - 50kW/s x 1s allowance
		t.Fatalf("gap fallback: want 50000, got %d", d.TargetW)
	}
}

func TestDeviceFaultInhibits(t *testing.T) {
	s := New(cfg)
	s.Step(healthy(at(0), 0, 0, 50, intent(at(0), 300_000, 30)))
	in := healthy(at(1), 100_000, 100_000, 50, intent(at(1), 300_000, 30))
	in.DeviceFault = true
	d := s.Step(in)
	// Contract: inhibit nonzero commands — hard zero at nonzero measured
	// output (not a slew step), and the intent is invalidated.
	if d.State != Fault || !hasReason(d, ReasonDeviceFault) || d.TargetW != 0 || !d.Invalidate {
		t.Fatalf("fault: %v %d %v", d.State, d.TargetW, d.Reasons)
	}
}

func TestSignedTargetsBothDirections(t *testing.T) {
	s := New(cfg)
	d := s.Step(healthy(at(0), 0, 0, 50, intent(at(0), -400_000, 30)))
	if d.TargetW != -50_000 {
		t.Fatalf("charge ramp: got %d", d.TargetW)
	}
}
