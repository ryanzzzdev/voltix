package device

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/simonvetter/modbus"
)

// Typed readings decoded from the frozen wire profile.
type InverterReading struct {
	Watts  int64 // + discharging, - charging (battery convention)
	SoCPct float64
	Status uint16
	Evt1   uint32 // bit0 = device command-watchdog tripped
}

// MeterReading carries site power; positive = exporting (sim convention,
// normalised by callers at their boundary).
type MeterReading struct {
	Watts int64
}

// Discover validates the SunS marker and walks the model chain at AddrBase,
// returning the model IDs found. It fails if the chain does not terminate
// within the bounded model table or if a required model is absent.
func (e *Endpoint) Discover(ctx context.Context, required []uint16) ([]uint16, error) {
	var models []uint16
	err := e.doRead(ctx, "sunspec discovery", func(c *modbus.ModbusClient) error {
		marker, err := c.ReadRegisters(AddrBase, 2, modbus.HOLDING_REGISTER)
		if err != nil {
			return err
		}
		if marker[0] != 0x5375 || marker[1] != 0x6E53 {
			return fmt.Errorf("%w: bad marker %#x", ErrDiscovery, marker)
		}
		addr := uint16(AddrBase + 2)
		for len(models) < 16 {
			hdr, err := c.ReadRegisters(addr, 2, modbus.HOLDING_REGISTER)
			if err != nil {
				return err
			}
			if hdr[0] == EndMarkerID {
				return nil
			}
			if hdr[1] == 0 {
				return fmt.Errorf("%w: model %d zero length", ErrDiscovery, hdr[0])
			}
			models = append(models, hdr[0])
			addr += 2 + hdr[1]
		}
		return fmt.Errorf("%w: no end marker in 16 models", ErrDiscovery)
	})
	if err != nil {
		return nil, err
	}
	for _, want := range required {
		if !slices.Contains(models, want) {
			return nil, fmt.Errorf("%w: model %d missing (found %v)", ErrDiscovery, want, models)
		}
	}
	return models, nil
}

func (e *Endpoint) readRegs(ctx context.Context, addr, count uint16) ([]uint16, error) {
	var regs []uint16
	err := e.doRead(ctx, fmt.Sprintf("read %d+%d", addr, count), func(c *modbus.ModbusClient) error {
		r, err := c.ReadRegisters(addr, count, modbus.HOLDING_REGISTER)
		regs = r
		return err
	})
	return regs, err
}

// ReadInverter reads measured power, SoC, status and the watchdog event
// register with per-point sentinel checks.
func (e *Endpoint) ReadInverter(ctx context.Context) (InverterReading, error) {
	var r InverterReading

	pw, err := e.readRegs(ctx, AddrInvW, 2) // W, W_SF are adjacent in model 103
	if err != nil {
		return r, err
	}
	if r.Watts, err = scaledWatts(pw[0], pw[1]); err != nil {
		return r, fmt.Errorf("inverter W: %w", err)
	}

	soc, err := e.readRegs(ctx, AddrInvChaState, 1)
	if err != nil {
		return r, err
	}
	if soc[0] == Uint16Sentinel {
		return r, fmt.Errorf("ChaState: %w", ErrInvalidData)
	}
	sf, err := e.readRegs(ctx, AddrInvChaStateSF, 1)
	if err != nil {
		return r, err
	}
	if sf[0] == SunSSFSentinel {
		return r, fmt.Errorf("ChaState_SF: %w", ErrInvalidData)
	}
	r.SoCPct = float64(soc[0]) * math.Pow10(int(int16(sf[0])))

	st, err := e.readRegs(ctx, AddrInvSt, 1)
	if err != nil {
		return r, err
	}
	if st[0] == Uint16Sentinel {
		return r, fmt.Errorf("St: %w", ErrInvalidData)
	}
	r.Status = st[0]

	evt, err := e.readRegs(ctx, AddrInvEvt1, 2)
	if err != nil {
		return r, err
	}
	r.Evt1 = uint32(evt[0])<<16 | uint32(evt[1])
	if r.Evt1 == 0xFFFFFFFF {
		return r, fmt.Errorf("Evt1: %w", ErrInvalidData)
	}
	return r, nil
}

// ReadMeter reads site power; W and W_SF are not adjacent in model 203 so
// they are fetched separately.
func (e *Endpoint) ReadMeter(ctx context.Context) (MeterReading, error) {
	var r MeterReading
	w, err := e.readRegs(ctx, AddrMetW, 1)
	if err != nil {
		return r, err
	}
	sf, err := e.readRegs(ctx, AddrMetWSF, 1)
	if err != nil {
		return r, err
	}
	if r.Watts, err = scaledWatts(w[0], sf[0]); err != nil {
		return r, fmt.Errorf("meter W: %w", err)
	}
	return r, nil
}

// SetPowerTargetW writes the atomic 4-register window WSetEna..WSet in one
// FC16 request: enable + watts mode + signed int32 target. A timeout returns
// ErrWriteUncertain and the write is never replayed by the adapter.
func (e *Endpoint) SetPowerTargetW(ctx context.Context, watts int32) error {
	return e.doWrite(ctx, "write power target", func(c *modbus.ModbusClient) error {
		return c.WriteRegisters(AddrCtlWSet, []uint16{
			1, // WSetEna = ENABLED
			1, // WSetMod = WATTS
			uint16(uint32(watts) >> 16),
			uint16(uint32(watts)),
		})
	})
}

// scaledWatts decodes a raw int16 point and its sunssf scale factor,
// rejecting either wire sentinel.
func scaledWatts(raw, sf uint16) (int64, error) {
	if raw == Int16Sentinel {
		return 0, fmt.Errorf("value: %w", ErrInvalidData)
	}
	if sf == SunSSFSentinel {
		return 0, fmt.Errorf("scale factor: %w", ErrInvalidData)
	}
	v := float64(int16(raw)) * math.Pow10(int(int16(sf)))
	if v > math.MaxInt64 || v < math.MinInt64 {
		return 0, fmt.Errorf("scaled value overflow: %w", ErrInvalidData)
	}
	return int64(v), nil
}

// IsTimeout reports whether err is a bounded request timeout.
func IsTimeout(err error) bool { return errors.Is(err, ErrTimeout) }
