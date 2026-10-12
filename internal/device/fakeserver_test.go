package device

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// fakeServer is a minimal Modbus TCP server for bounded-failure tests: it
// understands FC3 reads and FC16 writes over an in-memory register map, with
// test-injected delay, garbage and connection-drop behaviours.
type fakeServer struct {
	ln net.Listener

	mu      sync.Mutex
	regs    map[uint16]uint16
	log     [][2]int // [fc, addr] of each request; [-1, addr] marks unmapped reads
	delay   time.Duration
	garbage bool
	hangup  bool // close the connection mid-request
}

func newFakeServer(t *testing.T, regs map[uint16]uint16) *fakeServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeServer{ln: ln, regs: regs}
	go s.serve()
	t.Cleanup(func() { s.ln.Close() })
	return s
}

func (s *fakeServer) addr() string { return s.ln.Addr().String() }

func (s *fakeServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *fakeServer) handle(conn net.Conn) {
	defer conn.Close()
	mbap := make([]byte, 7)
	for {
		if _, err := io.ReadFull(conn, mbap); err != nil {
			return
		}
		s.mu.Lock()
		hangup, garbage, delay := s.hangup, s.garbage, s.delay
		s.mu.Unlock()
		if hangup {
			return
		}
		pdu := make([]byte, int(binary.BigEndian.Uint16(mbap[4:6]))-1)
		if _, err := io.ReadFull(conn, pdu); err != nil {
			return
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		if garbage {
			conn.Write([]byte{0xDE, 0xAD, 0xBE, 0xEF})
			continue
		}
		resp := s.exec(pdu)
		out := make([]byte, 7+len(resp))
		copy(out, mbap[:4])
		binary.BigEndian.PutUint16(out[4:6], uint16(1+len(resp)))
		out[6] = mbap[6]
		copy(out[7:], resp)
		if _, err := conn.Write(out); err != nil {
			return
		}
	}
}

func (s *fakeServer) exec(pdu []byte) []byte {
	fc := pdu[0]
	if len(pdu) < 5 {
		return []byte{fc | 0x80, 0x03}
	}
	addr := binary.BigEndian.Uint16(pdu[1:3])
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log = append(s.log, [2]int{int(fc), int(addr)})
	switch fc {
	case 0x03:
		count := int(binary.BigEndian.Uint16(pdu[3:5]))
		resp := []byte{fc, byte(2 * count)}
		for i := 0; i < count; i++ {
			v, ok := s.regs[addr+uint16(i)]
			if !ok {
				s.log = append(s.log, [2]int{-1, int(addr + uint16(i))})
				return []byte{fc | 0x80, 0x02}
			}
			resp = append(resp, byte(v>>8), byte(v))
		}
		return resp
	case 0x10:
		count := int(binary.BigEndian.Uint16(pdu[3:5]))
		data := pdu[6:]
		if len(data) < 2*count {
			return []byte{fc | 0x80, 0x03}
		}
		for i := 0; i < count; i++ {
			s.regs[addr+uint16(i)] = binary.BigEndian.Uint16(data[2*i:])
		}
		return []byte{fc, pdu[1], pdu[2], pdu[3], pdu[4]}
	default:
		return []byte{fc | 0x80, 0x01}
	}
}

func (s *fakeServer) set(fn func(*fakeServer)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(s)
}

// inverterRegs seeds the minimum register set ReadInverter needs.
func inverterRegs(w, wsf, soc, socsf, st, evtHi, evtLo uint16) map[uint16]uint16 {
	return map[uint16]uint16{
		AddrBase: 0x5375, AddrBase + 1: 0x6E53,
		40002: 1, 40003: 66,
		40070: 103, 40071: 50,
		40122: 124, 40123: 24,
		40148: 704, 40149: 65,
		40215: EndMarkerID, 40216: 0, // end marker is ID+L=0, two registers
		AddrInvW: w, AddrInvWSF: wsf,
		AddrInvChaState: soc, AddrInvChaStateSF: socsf,
		AddrInvSt:   st,
		AddrInvEvt1: evtHi, AddrInvEvt1 + 1: evtLo,
	}
}
