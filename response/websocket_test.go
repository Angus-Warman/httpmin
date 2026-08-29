package response

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func echo(conn *WebSocketConnection) {
	for {
		msg, err := conn.Read()
		if err != nil {
			return
		}

		if err := conn.Send(msg); err != nil {
			return
		}
	}
}

func startServer(t *testing.T, fn func(*WebSocketConnection)) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("/", WebSocket(fn))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

func startEchoServer(t *testing.T) string {
	return startServer(t, echo)
}

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

// writeClientFrame writes a masked frame
func (c *rawClient) writeClientFrame(t *testing.T, fin bool, opcode opCode, payload []byte) {
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
	opcode := opCode(hdr[0] & 0x0F)
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

	c.writeClientFrame(t, true, opText, []byte("hello autobahn"))
	got := c.readServerFrame(t)

	if got.opcode != opText {
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
	c.writeClientFrame(t, false, opText, []byte("hello"))
	c.writeClientFrame(t, false, opContinuation, []byte(" "))
	c.writeClientFrame(t, true, opContinuation, []byte("world"))

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
	c.writeClientFrame(t, false, opText, []byte("part1"))
	c.writeClientFrame(t, true, opPing, []byte("ping-payload"))
	c.writeClientFrame(t, true, opContinuation, []byte("part2"))

	// Expect the pong first (server processes frames in order: ping arrives
	// before the final continuation frame).
	pong := c.readServerFrame(t)
	if pong.opcode != opPong {
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
	binary.BigEndian.PutUint16(payload, statusNormalClosure)
	c.writeClientFrame(t, true, opClose, payload)

	got := c.readServerFrame(t)
	if got.opcode != opClose {
		t.Fatalf("opcode = %v, want OpClose", got.opcode)
	}
	if len(got.payload) < 2 {
		t.Fatalf("close payload too short: %v", got.payload)
	}
	code := binary.BigEndian.Uint16(got.payload[:2])
	if code != statusNormalClosure {
		t.Errorf("echoed close code = %d, want %d", code, statusNormalClosure)
	}
}

func TestInvalidUTF8TextMessageGetsClosed(t *testing.T) {
	addr := startEchoServer(t)
	c := dialAndHandshake(t, addr)
	defer c.conn.Close()

	// 0xFF is never valid UTF-8.
	c.writeClientFrame(t, true, opText, []byte{0xFF, 0xFE})

	got := c.readServerFrame(t)
	if got.opcode != opClose {
		t.Fatalf("opcode = %v, want OpClose (server should fail the connection)", got.opcode)
	}
	code := binary.BigEndian.Uint16(got.payload[:2])
	if code != statusInvalidPayload {
		t.Errorf("close code = %d, want %d (invalid payload)", code, statusInvalidPayload)
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
	if got.opcode != opClose {
		t.Fatalf("opcode = %v, want OpClose", got.opcode)
	}
	code := binary.BigEndian.Uint16(got.payload[:2])
	if code != statusProtocolError {
		t.Errorf("close code = %d, want %d", code, statusProtocolError)
	}
}

// dialRaw performs a raw HTTP handshake and returns the client and the parsed
// response so tests can inspect status codes and headers.
func dialRaw(t *testing.T, addr string, req string) (*rawClient, *http.Response) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("write handshake: %v", err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read handshake response: %v", err)
	}
	return &rawClient{conn: conn, br: br}, resp
}

func handshakeRequest(addr string, extraHeaders string) string {
	keyBytes := make([]byte, 16)
	rand.Read(keyBytes)
	key := base64.StdEncoding.EncodeToString(keyBytes)
	return "GET / HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		extraHeaders + "\r\n"
}

func TestSubprotocolIsEchoed(t *testing.T) {
	addr := startEchoServer(t)
	c, resp := dialRaw(t, addr, handshakeRequest(addr, "Sec-WebSocket-Protocol: chat, superchat\r\n"))
	defer c.conn.Close()

	if resp.StatusCode != 101 {
		t.Fatalf("status = %d, want 101", resp.StatusCode)
	}
	if got := resp.Header.Get("Sec-WebSocket-Protocol"); got != "chat" {
		t.Errorf("Sec-WebSocket-Protocol = %q, want first offered protocol %q", got, "chat")
	}
}

func TestHandshakeRejectsHTTP10(t *testing.T) {
	addr := startEchoServer(t)
	req := "GET / HTTP/1.0\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: AAAAAAAAAAAAAAAAAAAAAA==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	_, resp := dialRaw(t, addr, req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUpgradeRequired {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUpgradeRequired)
	}
}

func TestDuplicateSecWebSocketKeyRejected(t *testing.T) {
	addr := startEchoServer(t)
	req := "GET / HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: AAAAAAAAAAAAAAAAAAAAAA==\r\n" +
		"Sec-WebSocket-Key: BBBBBBBBBBBBBBBBBBBBBB==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	_, resp := dialRaw(t, addr, req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestRegisteredCloseCodesAreAccepted(t *testing.T) {
	for _, code := range []int{1012, 1013, 1014} {
		t.Run(fmt.Sprintf("code-%d", code), func(t *testing.T) {
			addr := startEchoServer(t)
			c := dialAndHandshake(t, addr)
			defer c.conn.Close()

			payload := make([]byte, 2)
			binary.BigEndian.PutUint16(payload, uint16(code))
			c.writeClientFrame(t, true, opClose, payload)

			got := c.readServerFrame(t)
			if got.opcode != opClose {
				t.Fatalf("opcode = %v, want OpClose", got.opcode)
			}
			gotCode := binary.BigEndian.Uint16(got.payload[:2])
			if gotCode != uint16(code) {
				t.Errorf("echoed close code = %d, want %d (must not fail with 1002)", gotCode, code)
			}
		})
	}
}

func TestApplyMask(t *testing.T) {
	key := [4]byte{0xde, 0xad, 0xbe, 0xef}
	for n := 0; n <= 40; n++ {
		payload := make([]byte, n)
		rand.Read(payload)
		orig := append([]byte(nil), payload...)

		applyMask(payload, key)

		// Unmask with a straightforward byte-by-byte reference and compare.
		for i := range payload {
			payload[i] ^= key[i%4]
		}
		if !bytes.Equal(payload, orig) {
			t.Fatalf("mask/unmask mismatch for length %d", n)
		}
	}
}

func closeAfterFirstRead(conn *WebSocketConnection) {
	if _, err := conn.Read(); err != nil {
		return
	}
	conn.Close()
}

func TestAppInitiatedCloseAwaitsCloseEcho(t *testing.T) {
	addr := startServer(t, closeAfterFirstRead)
	c := dialAndHandshake(t, addr)
	defer c.conn.Close()

	c.writeClientFrame(t, true, opText, []byte("hi"))

	// The server reads our message and closes; its close frame arrives first.
	got := c.readServerFrame(t)
	if got.opcode != opClose {
		t.Fatalf("opcode = %v, want OpClose", got.opcode)
	}
	code := binary.BigEndian.Uint16(got.payload[:2])
	if code != statusNormalClosure {
		t.Errorf("close code = %d, want %d", code, statusNormalClosure)
	}

	// Because the server is completing the closing handshake it must keep the
	// socket open, waiting for our close echo, rather than closing immediately.
	c.conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	var one [1]byte
	_, err := c.conn.Read(one[:])
	if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		t.Fatalf("server closed the socket before receiving our close echo: %v", err)
	}
	c.conn.SetReadDeadline(time.Time{})

	// Complete the handshake; the server should now release the socket.
	payload := make([]byte, 2)
	binary.BigEndian.PutUint16(payload, statusNormalClosure)
	c.writeClientFrame(t, true, opClose, payload)

	c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	defer c.conn.SetReadDeadline(time.Time{})
	for {
		if _, err := c.conn.Read(one[:]); err != nil {
			return // server finished the handshake and closed
		}
	}
}
