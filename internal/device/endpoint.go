package device

import (
	"fmt"
	"sync"
	"time"

	"github.com/simonvetter/modbus"
)

// Endpoint owns one Modbus TCP connection to a device endpoint.
//
// Requests are serialised per endpoint. The library sets a real socket
// read/write deadline of Timeout on every request, so each call returns
// within roughly Timeout once connected. A failed request closes the
// connection; read operations transparently reconnect and retry once,
// write operations never retry (a timed-out write is uncertain).
type Endpoint struct {
	addr    string
	unitID  uint8
	timeout time.Duration

	mu     sync.Mutex
	client *modbus.ModbusClient
}

// NewEndpoint returns an Endpoint for a host:port Modbus TCP address.
// timeout bounds each request's socket I/O; the initial dial is bounded
// separately by the library's fixed 5 s connect timeout.
func NewEndpoint(addr string, timeout time.Duration) *Endpoint {
	return &Endpoint{addr: addr, unitID: UnitID, timeout: timeout}
}

// Close drops the connection if held; the next request reconnects lazily.
func (e *Endpoint) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closeLocked()
}

func (e *Endpoint) closeLocked() {
	if e.client != nil {
		e.client.Close()
		e.client = nil
	}
}

func (e *Endpoint) connectLocked() error {
	c, err := modbus.NewClient(&modbus.ClientConfiguration{
		URL:     "tcp://" + e.addr,
		Timeout: e.timeout,
	})
	if err != nil {
		return fmt.Errorf("modbus client config %s: %w", e.addr, err)
	}
	if err = c.SetUnitId(e.unitID); err != nil {
		return err
	}
	if err = c.SetEncoding(modbus.BIG_ENDIAN, modbus.HIGH_WORD_FIRST); err != nil {
		return err
	}
	if err = c.Open(); err != nil {
		return fmt.Errorf("modbus dial %s: %w", e.addr, err)
	}
	e.client = c
	return nil
}

// doRead serialises a request with at most one reconnect+retry.
func (e *Endpoint) doRead(op string, fn func(c *modbus.ModbusClient) error) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var last error
	for attempt := 0; attempt < 2; attempt++ {
		if e.client == nil {
			if err := e.connectLocked(); err != nil {
				last = err
				continue
			}
		}
		err := fn(e.client)
		if err == nil {
			return nil
		}
		e.closeLocked()
		last = classify(err)
	}
	return fmt.Errorf("%s: %w", op, last)
}

// doWrite serialises a write with no retry: the connection is reset after
// failure so the next call starts clean, but the write itself is not
// replayed because a timed-out write may already have been applied.
func (e *Endpoint) doWrite(op string, fn func(c *modbus.ModbusClient) error) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.client == nil {
		if err := e.connectLocked(); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}
	if err := fn(e.client); err != nil {
		e.closeLocked()
		return fmt.Errorf("%s: %w", op, classifyWrite(err))
	}
	return nil
}
