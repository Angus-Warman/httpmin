package response

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func echo(conn *Socket) {
	for {
		msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if err := conn.Send(string(msg.Payload)); err != nil {
			return
		}
	}
}

// startEchoServer spins up a real TCP listener running an echo handler,
// returning its address and a cleanup func.
func startEchoServer(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("/", WebSocket(echo))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

// rawClient is a minimal hand-rolled WS client over a raw net.Conn, used so
// the test exercises the actual wire format rather than relying on another
// WS library (which would partially defeat the point of testing our codec).
type rawClient struct {
	conn net.Conn
	br   *bufio.Reader
}

func dialAndHandshake(t *testing.T, addr string) *rawClient {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	keyBytes := make([]byte, 16)
	rand.Read(keyBytes)
	key := base64.StdEncoding.EncodeToString(keyBytes)

	req := "GET / HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("write handshake: %v", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read handshake response: %v", err)
	}
	if resp.StatusCode != 101 {
		t.Fatalf("expected 101, got %d", resp.StatusCode)
	}
	wantAccept := computeAcceptKey(key)
	if got := resp.Header.Get("Sec-WebSocket-Accept"); got != wantAccept {
		t.Fatalf("Sec-WebSocket-Accept = %q, want %q", got, wantAccept)
	}
	return &rawClient{conn: conn, br: br}
}

// writeClientFrame writes a masked frame, as a real client must.
func (c *rawClient) writeClientFrame(t *testing.T, fin bool, opcode Opcode, payload []byte) {
	t.Helper()
	var hdr []byte
	b0 := byte(opcode)
	if fin {
		b0 |= 0x80
	}
	hdr = append(hdr, b0)

	n := len(payload)
	switch {
	case n <= 125:
		hdr = append(hdr, 0x80|byte(n))
	case n <= 65535:
		hdr = append(hdr, 0x80|126)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(n))
		hdr = append(hdr, ext[:]...)
	default:
		hdr = append(hdr, 0x80|127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(n))
		hdr = append(hdr, ext[:]...)
	}
	var key [4]byte
	rand.Read(key[:])
	hdr = append(hdr, key[:]...)

	masked := make([]byte, len(payload))
	copy(masked, payload)
	applyMask(masked, key)

	if _, err := c.conn.Write(hdr); err != nil {
		t.Fatalf("write frame header: %v", err)
	}
	if len(masked) > 0 {
		if _, err := c.conn.Write(masked); err != nil {
			t.Fatalf("write frame payload: %v", err)
		}
	}
}

// readServerFrame reads one (unmasked, since server->client frames are
// never masked) frame from the server.
func (c *rawClient) readServerFrame(t *testing.T) frame {
	t.Helper()
	var hdr [2]byte
	if _, err := c.br.Read(hdr[:1]); err != nil {
		t.Fatalf("read byte0: %v", err)
	}
	if _, err := c.br.Read(hdr[1:2]); err != nil {
		t.Fatalf("read byte1: %v", err)
	}
	fin := hdr[0]&0x80 != 0
	opcode := Opcode(hdr[0] & 0x0F)
	masked := hdr[1]&0x80 != 0
	if masked {
		t.Fatalf("server sent a masked frame, which violates RFC 6455 5.1")
	}
	l7 := hdr[1] & 0x7F
	var payloadLen uint64
	switch {
	case l7 <= 125:
		payloadLen = uint64(l7)
	case l7 == 126:
		var ext [2]byte
		if _, err := readFull(c.br, ext[:]); err != nil {
			t.Fatalf("read ext16: %v", err)
		}
		payloadLen = uint64(binary.BigEndian.Uint16(ext[:]))
	case l7 == 127:
		var ext [8]byte
		if _, err := readFull(c.br, ext[:]); err != nil {
			t.Fatalf("read ext64: %v", err)
		}
		payloadLen = binary.BigEndian.Uint64(ext[:])
	}
	payload := make([]byte, payloadLen)
	if _, err := readFull(c.br, payload); err != nil {
		t.Fatalf("read payload: %v", err)
	}
	return frame{fin: fin, opcode: opcode, payload: payload}
}

func readFull(br *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := br.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func TestHandshakeAndEchoTextMessage(t *testing.T) {
	addr := startEchoServer(t)
	c := dialAndHandshake(t, addr)
	defer c.conn.Close()

	c.writeClientFrame(t, true, OpText, []byte("hello autobahn"))
	got := c.readServerFrame(t)

	if got.opcode != OpText {
		t.Errorf("opcode = %v, want OpText", got.opcode)
	}
	if !bytes.Equal(got.payload, []byte("hello autobahn")) {
		t.Errorf("payload = %q, want %q", got.payload, "hello autobahn")
	}
	if !got.fin {
		t.Errorf("fin = false, want true")
	}
}

func TestFragmentedMessageReassembly(t *testing.T) {
	addr := startEchoServer(t)
	c := dialAndHandshake(t, addr)
	defer c.conn.Close()

	// Send "hello world" split across 3 fragments.
	c.writeClientFrame(t, false, OpText, []byte("hello"))
	c.writeClientFrame(t, false, OpContinuation, []byte(" "))
	c.writeClientFrame(t, true, OpContinuation, []byte("world"))

	got := c.readServerFrame(t)
	if !bytes.Equal(got.payload, []byte("hello world")) {
		t.Errorf("reassembled payload = %q, want %q", got.payload, "hello world")
	}
}

func TestPingInterleavedDuringFragmentation(t *testing.T) {
	addr := startEchoServer(t)
	c := dialAndHandshake(t, addr)
	defer c.conn.Close()

	// Start a fragmented message, send a ping mid-stream, finish the message.
	c.writeClientFrame(t, false, OpText, []byte("part1"))
	c.writeClientFrame(t, true, OpPing, []byte("ping-payload"))
	c.writeClientFrame(t, true, OpContinuation, []byte("part2"))

	// Expect the pong first (server processes frames in order: ping arrives
	// before the final continuation frame).
	pong := c.readServerFrame(t)
	if pong.opcode != OpPong {
		t.Fatalf("first frame back = %v, want OpPong", pong.opcode)
	}
	if !bytes.Equal(pong.payload, []byte("ping-payload")) {
		t.Errorf("pong payload = %q, want echo of ping payload", pong.payload)
	}

	echoed := c.readServerFrame(t)
	if !bytes.Equal(echoed.payload, []byte("part1part2")) {
		t.Errorf("reassembled payload = %q, want %q", echoed.payload, "part1part2")
	}
}

func TestCloseHandshake(t *testing.T) {
	addr := startEchoServer(t)
	c := dialAndHandshake(t, addr)
	defer c.conn.Close()

	payload := make([]byte, 2)
	binary.BigEndian.PutUint16(payload, StatusNormalClosure)
	c.writeClientFrame(t, true, OpClose, payload)

	got := c.readServerFrame(t)
	if got.opcode != OpClose {
		t.Fatalf("opcode = %v, want OpClose", got.opcode)
	}
	if len(got.payload) < 2 {
		t.Fatalf("close payload too short: %v", got.payload)
	}
	code := binary.BigEndian.Uint16(got.payload[:2])
	if code != StatusNormalClosure {
		t.Errorf("echoed close code = %d, want %d", code, StatusNormalClosure)
	}
}

func TestInvalidUTF8TextMessageGetsClosed(t *testing.T) {
	addr := startEchoServer(t)
	c := dialAndHandshake(t, addr)
	defer c.conn.Close()

	// 0xFF is never valid UTF-8.
	c.writeClientFrame(t, true, OpText, []byte{0xFF, 0xFE})

	got := c.readServerFrame(t)
	if got.opcode != OpClose {
		t.Fatalf("opcode = %v, want OpClose (server should fail the connection)", got.opcode)
	}
	code := binary.BigEndian.Uint16(got.payload[:2])
	if code != StatusInvalidPayload {
		t.Errorf("close code = %d, want %d (invalid payload)", code, StatusInvalidPayload)
	}
}

func TestUnmaskedClientFrameIsRejected(t *testing.T) {
	addr := startEchoServer(t)
	c := dialAndHandshake(t, addr)
	defer c.conn.Close()

	// Manually write an UNMASKED frame (mask bit = 0), which a client must
	// never do. Server must fail the connection with a protocol error.
	hdr := []byte{0x81 /* fin+text */, 0x05 /* len=5, mask bit unset */}
	c.conn.Write(hdr)
	c.conn.Write([]byte("hello"))

	got := c.readServerFrame(t)
	if got.opcode != OpClose {
		t.Fatalf("opcode = %v, want OpClose", got.opcode)
	}
	code := binary.BigEndian.Uint16(got.payload[:2])
	if code != StatusProtocolError {
		t.Errorf("close code = %d, want %d", code, StatusProtocolError)
	}
}
