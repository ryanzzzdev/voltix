package control

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"voltix/internal/device"
	"voltix/internal/dispatch"
	"voltix/internal/policy"
)

var pcfg = policy.Config{
	DischargeLimitW: 500_000, ChargeLimitW: 500_000, ExportLimitW: 400_000,
	SoCFloorPct: 5, SoCCeilPct: 95, SlewWPS: 50_000,
	MaxSnapshotAge: 2 * time.Second, MaxGap: 2500 * time.Millisecond,
}

type fakeInv struct {
	mu       sync.Mutex
	read     func() (device.InverterReading, error)
	writeErr error
	writes   []int32
}

func (f *fakeInv) ReadInverter() (device.InverterReading, error) {
	return f.read()
}
func (f *fakeInv) SetPowerTargetW(w int32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, w)
	return f.writeErr
}
func (f *fakeInv) written() []int32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int32{}, f.writes...)
}

type fakeMeter struct {
	read func() (device.MeterReading, error)
}

func (f *fakeMeter) ReadMeter() (device.MeterReading, error) { return f.read() }

type fakeSession struct {
	mu   sync.Mutex
	cmd  *dispatch.Command
	mono time.Duration
	drop int
}

func (s *fakeSession) Active() (dispatch.Command, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd == nil {
		return dispatch.Command{}, false
	}
	return *s.cmd, true
}
func (s *fakeSession) Drop() { s.mu.Lock(); s.drop++; s.mu.Unlock() }
func (s *fakeSession) Mono() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mono
}
func (s *fakeSession) drops() int { s.mu.Lock(); defer s.mu.Unlock(); return s.drop }

func invOK(watts int64, soc float64) func() (device.InverterReading, error) {
	return func() (device.InverterReading, error) {
		return device.InverterReading{Watts: watts, SoCPct: soc, Status: 4}, nil
	}
}
func metOK(watts int64) func() (device.MeterReading, error) {
	return func() (device.MeterReading, error) {
		return device.MeterReading{Watts: watts}, nil
	}
}

var t0 = time.Unix(0, 0)

func activeSession(targetKW float64, remaining time.Duration) *fakeSession {
	return &fakeSession{cmd: &dispatch.Command{
		Request:      dispatch.Request{TargetKW: targetKW},
		DeadlineMono: remaining,
	}}
}

func rig(inv *fakeInv, met *fakeMeter, sess *fakeSession, snaps chan Snapshot) *Loop {
	return New(LoopConfig{
		Inverter: inv, Meter: met, Policy: policy.New(pcfg),
		Session: sess, Snapshots: snaps,
	})
}

func TestActiveCycleWritesConstrainedTarget(t *testing.T) {
	inv := &fakeInv{read: invOK(0, 50)}
	sess := activeSession(300, 30*time.Second)
	l := rig(inv, &fakeMeter{read: metOK(0)}, sess, make(chan Snapshot, 4))

	snap := l.Step(t0)
	if snap.State != policy.Active || snap.RequestedW != 300_000 {
		t.Fatalf("want ACTIVE req=300k: %+v", snap)
	}
	if snap.ConstrainedW != 50_000 || *snap.Acknowledged != 50_000 {
		t.Fatalf("first slew step: %+v", snap)
	}
	w := inv.written()
	if len(w) != 1 || w[0] != 50_000 {
		t.Fatalf("write: %v", w)
	}
	if snap.ReadDuration <= 0 || snap.WriteDuration <= 0 {
		t.Fatalf("phase timings missing: %+v", snap)
	}
}

func TestStandbyRequestsZero(t *testing.T) {
	inv := &fakeInv{read: invOK(120_000, 50)}
	l := rig(inv, &fakeMeter{read: metOK(120_000)}, &fakeSession{}, nil)
	snap := l.Step(t0)
	if snap.State != policy.Standby || snap.ConstrainedW != 70_000 {
		t.Fatalf("standby fallback: %+v", snap)
	}
}

func TestReadFailureDegradesAndDrops(t *testing.T) {
	inv := &fakeInv{read: func() (device.InverterReading, error) {
		return device.InverterReading{}, errors.New("timeout")
	}}
	sess := activeSession(100, 30*time.Second)
	l := rig(inv, &fakeMeter{read: metOK(0)}, sess, nil)
	snap := l.Step(t0)
	if snap.State != policy.Degraded || !snap.InverterOK == false {
		t.Fatalf("degraded: %+v", snap)
	}
	if sess.drops() != 1 {
		t.Fatalf("intent not dropped: %d", sess.drops())
	}
}

func TestWriteFailureMarksUncertainNoStaleRetry(t *testing.T) {
	inv := &fakeInv{read: invOK(0, 50), writeErr: errors.New("conn reset")}
	sess := activeSession(300, 30*time.Second)
	l := rig(inv, &fakeMeter{read: metOK(0)}, sess, nil)

	s1 := l.Step(t0)
	if s1.Acknowledged != nil {
		t.Fatal("failed write must mark ack uncertain")
	}
	// Next cycle still issues its own fresh decision — not a replayed write.
	inv.writeErr = nil
	s2 := l.Step(t0.Add(time.Second))
	if s2.Acknowledged == nil || *s2.Acknowledged != 100_000 {
		t.Fatalf("fresh write: %+v", s2)
	}
	if w := inv.written(); len(w) != 2 {
		t.Fatalf("writes: %v", w)
	}
}

func TestSlowConsumerDoesNotStall(t *testing.T) {
	inv := &fakeInv{read: invOK(0, 50)}
	snaps := make(chan Snapshot, 1)
	l := rig(inv, &fakeMeter{read: metOK(0)}, &fakeSession{}, snaps)

	start := time.Now()
	for i := 0; i < 50; i++ {
		l.Step(t0.Add(time.Duration(i) * time.Second))
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("loop stalled on full snapshot channel: %v", d)
	}
	// Channel holds only the first; rest were dropped, not queued.
	if got := len(snaps); got != 1 {
		t.Fatalf("want 1 buffered snapshot, got %d", got)
	}
}

func TestGapInvalidatesIntent(t *testing.T) {
	inv := &fakeInv{read: invOK(0, 50)}
	sess := activeSession(300, time.Hour)
	l := rig(inv, &fakeMeter{read: metOK(0)}, sess, nil)

	l.Step(t0)
	l.Step(t0.Add(time.Second))
	snap := l.Step(t0.Add(5 * time.Second)) // 4 s gap > 2.5 s
	if snap.State != policy.Degraded || sess.drops() != 1 {
		t.Fatalf("gap: %+v drops=%d", snap, sess.drops())
	}
}

func TestIntentExpiryFallsBack(t *testing.T) {
	inv := &fakeInv{read: invOK(0, 50)}
	sess := activeSession(300, 500*time.Millisecond) // expires mid-second
	l := rig(inv, &fakeMeter{read: metOK(0)}, sess, nil)

	l.Step(t0)
	sess.mono = 600 * time.Millisecond // lease's mono deadline now passed
	snap := l.Step(t0.Add(time.Second))
	if snap.State != policy.Degraded || snap.RequestedW != 300_000 {
		t.Fatalf("expiry: %+v", snap)
	}
	if sess.drops() != 1 {
		t.Fatal("expired intent not dropped")
	}
}

func TestRunBoundedShutdownWritesZero(t *testing.T) {
	inv := &fakeInv{read: invOK(10_000, 50)}
	sess := &fakeSession{}
	l := New(LoopConfig{
		Inverter: inv, Meter: &fakeMeter{read: metOK(0)},
		Policy: policy.New(pcfg), Session: sess,
		Period: 5 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { l.Run(ctx); close(done) }()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not shut down bounded")
	}
	if w := inv.written(); len(w) == 0 || w[len(w)-1] != 0 {
		t.Fatalf("final write not zero: %v", w)
	}
}

func TestDeviceFaultInhibits(t *testing.T) {
	inv := &fakeInv{read: func() (device.InverterReading, error) {
		return device.InverterReading{Watts: 0, SoCPct: 50, Status: 4, Evt1: 2}, nil
	}}
	sess := activeSession(300, 30*time.Second)
	l := rig(inv, &fakeMeter{read: metOK(0)}, sess, nil)
	snap := l.Step(t0)
	if snap.State != policy.Fault {
		t.Fatalf("fault: %+v", snap)
	}
}
