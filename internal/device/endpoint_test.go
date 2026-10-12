package device

import (
	"errors"
	"testing"
	"time"
)

func testEndpoint(t *testing.T, s *fakeServer) *Endpoint {
	t.Helper()
	e := NewEndpoint(s.addr(), 200*time.Millisecond)
	t.Cleanup(e.Close)
	return e
}

func TestDiscoverAndReadInverter(t *testing.T) {
	s := newFakeServer(t, inverterRegs(2500, 2, 5000, 0xFFFE, 4, 0, 0))
	e := testEndpoint(t, s)

	models, err := e.Discover(InverterModels)
	if err != nil {
		s.mu.Lock()
		t.Fatalf("%v — server log %v", err, s.log)
	}
	if len(models) != 4 || models[1] != 103 || models[3] != 704 {
		t.Fatalf("unexpected model chain %v", models)
	}

	r, err := e.ReadInverter()
	if err != nil {
		t.Fatal(err)
	}
	if r.Watts != 250000 || r.SoCPct != 50.0 || r.Status != 4 || r.Evt1 != 0 {
		t.Fatalf("bad reading %+v", r)
	}
}

func TestDiscoverRejectsMissingModel(t *testing.T) {
	regs := inverterRegs(0, 2, 0, 0xFFFE, 4, 0, 0)
	regs[40148], regs[40149] = EndMarkerID, 0 // drop model 704
	s := newFakeServer(t, regs)
	e := testEndpoint(t, s)
	if _, err := e.Discover(InverterModels); !errors.Is(err, ErrDiscovery) {
		t.Fatalf("want ErrDiscovery, got %v", err)
	}
}

func TestReadInverterSentinels(t *testing.T) {
	for name, regs := range map[string]map[uint16]uint16{
		"W sentinel":    inverterRegs(Int16Sentinel, 2, 5000, 0xFFFE, 4, 0, 0),
		"W_SF sentinel": inverterRegs(2500, SunSSFSentinel, 5000, 0xFFFE, 4, 0, 0),
		"SoC sentinel":  inverterRegs(2500, 2, Uint16Sentinel, 0xFFFE, 4, 0, 0),
		"Evt1 sentinel": inverterRegs(2500, 2, 5000, 0xFFFE, 4, 0xFFFF, 0xFFFF),
	} {
		t.Run(name, func(t *testing.T) {
			s := newFakeServer(t, regs)
			e := testEndpoint(t, s)
			if _, err := e.ReadInverter(); !errors.Is(err, ErrInvalidData) {
				t.Fatalf("want ErrInvalidData, got %v", err)
			}
		})
	}
}

func TestReadMeterNegativeImport(t *testing.T) {
	s := newFakeServer(t, map[uint16]uint16{
		AddrMetW: 0xFB50, AddrMetWSF: 2, // -1200 x 10^2
	})
	e := testEndpoint(t, s)
	r, err := e.ReadMeter()
	if err != nil {
		t.Fatal(err)
	}
	if r.Watts != -120000 {
		t.Fatalf("want -120000 W import, got %d", r.Watts)
	}
}

func TestSetPowerTargetWireFormat(t *testing.T) {
	s := newFakeServer(t, map[uint16]uint16{})
	e := testEndpoint(t, s)
	if err := e.SetPowerTargetW(-250000); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	want := map[uint16]uint16{AddrCtlWSet: 1, AddrCtlWSet + 1: 1,
		AddrCtlWSet + 2: 0xFFFC, AddrCtlWSet + 3: 0x2F70}
	for a, v := range want {
		if s.regs[a] != v {
			t.Fatalf("reg %d: want %#x got %#x", a, v, s.regs[a])
		}
	}
}

func TestRequestTimeoutIsBounded(t *testing.T) {
	s := newFakeServer(t, map[uint16]uint16{AddrMetW: 100, AddrMetWSF: 2})
	s.set(func(f *fakeServer) { f.delay = 500 * time.Millisecond })
	e := NewEndpoint(s.addr(), 80*time.Millisecond)
	t.Cleanup(e.Close)

	start := time.Now()
	if _, err := e.ReadMeter(); !errors.Is(err, ErrTimeout) {
		t.Fatalf("want ErrTimeout, got %v", err)
	}
	if el := time.Since(start); el > 2*80*time.Millisecond+100*time.Millisecond {
		t.Fatalf("unbounded request: %v", el)
	}
}

func TestMalformedResponse(t *testing.T) {
	s := newFakeServer(t, map[uint16]uint16{AddrMetW: 100, AddrMetWSF: 2})
	s.set(func(f *fakeServer) { f.garbage = true })
	e := NewEndpoint(s.addr(), 100*time.Millisecond)
	t.Cleanup(e.Close)

	// Unparsable bytes cannot be decoded as a frame, so the request ends at
	// the socket deadline — a bounded failure either way.
	start := time.Now()
	if _, err := e.ReadMeter(); err == nil {
		t.Fatal("malformed response must not decode silently")
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("malformed response was not bounded")
	}
}

func TestReconnectAfterHangup(t *testing.T) {
	s := newFakeServer(t, map[uint16]uint16{AddrMetW: 100, AddrMetWSF: 2})
	e := testEndpoint(t, s)

	s.set(func(f *fakeServer) { f.hangup = true })
	if _, err := e.ReadMeter(); err == nil {
		t.Fatal("want error on dropped connection")
	}
	s.set(func(f *fakeServer) { f.hangup = false })
	r, err := e.ReadMeter()
	if err != nil {
		t.Fatalf("reconnect failed: %v", err)
	}
	if r.Watts != 10000 {
		t.Fatalf("want 10000 W, got %d", r.Watts)
	}
}

func TestWriteTimeoutIsUncertain(t *testing.T) {
	s := newFakeServer(t, map[uint16]uint16{})
	s.set(func(f *fakeServer) { f.delay = 300 * time.Millisecond })
	e := NewEndpoint(s.addr(), 60*time.Millisecond)
	t.Cleanup(e.Close)

	start := time.Now()
	err := e.SetPowerTargetW(1000)
	if !errors.Is(err, ErrWriteUncertain) {
		t.Fatalf("want ErrWriteUncertain, got %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("uncertain write was not bounded")
	}
}
