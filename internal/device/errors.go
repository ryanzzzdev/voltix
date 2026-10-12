package device

import (
	"errors"

	"github.com/simonvetter/modbus"
)

// Classified adapter errors. Callers distinguish a known-bounded timeout, an
// I/O/protocol failure, invalid wire data and the uncertain-write case.
var (
	ErrTimeout        = errors.New("modbus: request timed out")
	ErrWriteUncertain = errors.New("modbus: write timed out; device may have applied it")
	ErrInvalidData    = errors.New("sunspec: invalid or not-implemented register value")
	ErrDiscovery      = errors.New("sunspec: discovery failed")
)

// classify maps library errors onto the adapter's error taxonomy.
func classify(err error) error {
	switch {
	case errors.Is(err, modbus.ErrRequestTimedOut):
		return ErrTimeout
	default:
		return err
	}
}

// classifyWrite maps a write failure; a timed-out write may still have been
// applied by the device so it is reported as uncertain rather than retried.
func classifyWrite(err error) error {
	if errors.Is(err, modbus.ErrRequestTimedOut) {
		return ErrWriteUncertain
	}
	return err
}
