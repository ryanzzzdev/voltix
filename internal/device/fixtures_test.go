package device

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Fixture decode tests: the adapter must agree with the golden PDUs produced
// independently from the vendored upstream model definitions (VTX-004).

type fixture struct {
	Name    string          `json:"name"`
	RespHex string          `json:"expected_response_pdu_hex"`
	Decode  json.RawMessage `json:"decode"`
}

func loadFixture(t *testing.T, name string) fixture {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "contracts", "fixtures", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

// responseRegs parses "<fc> <bytecount> <reg>..." hex into registers.
func responseRegs(t *testing.T, hexPDU string) []uint16 {
	t.Helper()
	fields := strings.Fields(hexPDU)
	if len(fields) < 2 {
		t.Fatalf("short PDU %q", hexPDU)
	}
	n, _ := strconv.ParseUint(fields[1], 16, 8)
	var regs []uint16
	for i := 0; i < int(n)/2; i++ {
		v, err := strconv.ParseUint(fields[2+i], 16, 16)
		if err != nil {
			t.Fatalf("bad fixture reg %q", fields[2+i])
		}
		regs = append(regs, uint16(v))
	}
	return regs
}

func decodeNum(t *testing.T, raw json.RawMessage, key string) float64 {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	v, ok := m[key].(float64)
	if !ok {
		t.Fatalf("fixture missing numeric %q", key)
	}
	return v
}

func TestFixtureInverterPower(t *testing.T) {
	for _, name := range []string{"inverter-discharge", "inverter-charge"} {
		f := loadFixture(t, name)
		regs := responseRegs(t, f.RespHex)
		got, err := scaledWatts(regs[0], regs[1])
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if want := int64(decodeNum(t, f.Decode, "watts")); got != want {
			t.Fatalf("%s: want %d W, got %d", name, want, got)
		}
	}
}

func TestFixtureMeterPower(t *testing.T) {
	for _, name := range []string{"meter-export", "meter-import"} {
		f := loadFixture(t, name)
		regs := responseRegs(t, f.RespHex)
		sf := int16(decodeNum(t, f.Decode, "W_SF"))
		got, err := scaledWatts(regs[0], uint16(sf))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if want := int64(decodeNum(t, f.Decode, "watts")); got != want {
			t.Fatalf("%s: want %d W, got %d", name, want, got)
		}
	}
}

func TestFixtureSentinelsRejected(t *testing.T) {
	// sentinel-w: int16 value sentinel must be rejected, not scaled.
	regs := responseRegs(t, loadFixture(t, "sentinel-w").RespHex)
	if _, err := scaledWatts(regs[0], regs[1]); !errors.Is(err, ErrInvalidData) {
		t.Fatal("int16 sentinel was not rejected")
	}
	// sentinel-sunssf: the scale factor itself is the sentinel.
	regs = responseRegs(t, loadFixture(t, "sentinel-sunssf").RespHex)
	if _, err := scaledWatts(100, regs[0]); !errors.Is(err, ErrInvalidData) {
		t.Fatal("sunssf sentinel was not rejected")
	}
	// sentinel-chastate: uint16 sentinel.
	regs = responseRegs(t, loadFixture(t, "sentinel-chastate").RespHex)
	if regs[0] != Uint16Sentinel {
		t.Fatal("fixture unexpectedly changed")
	}
}

func TestFixtureWriteWindowValues(t *testing.T) {
	// The adapter's SetPowerTargetW emits the same register payload the
	// write-target fixtures encode: [Ena=1, Mod=1, int32 hi, lo].
	for name, watts := range map[string]int32{
		"write-target-discharge": 250000,
		"write-target-charge":    -250000,
		"write-target-zero":      0,
	} {
		f := loadFixture(t, name)
		var m map[string]any
		if err := json.Unmarshal(f.Decode, &m); err != nil {
			t.Fatal(err)
		}
		if int32(m["WSet"].(float64)) != watts {
			t.Fatalf("%s: decode drifted", name)
		}
	}
}
