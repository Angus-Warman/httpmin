package response

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

func WebSocket(handler func(*WebSocketConnection)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		socket, err := upgrade(w, r)

		if err != nil {
			return // upgrade handles responding to invalid requests
		}

		handler(socket)
	})
}

// WebSocketConnection is a single upgraded WebSocket connection
type WebSocketConnection struct {
	// MaxMessageSize caps the total size of a reassembled (possibly fragmented)
	// message. Defaults to 16 MB
	MaxMessageSize int64
	subprotocol    string
	rwc            net.Conn
	br             *bufio.Reader
	bw             *bufio.Writer
	readGate       chan struct{} // serializes reads and the close-handshake drain
	writeMu        sync.Mutex
	closeSent      bool
	closeRecv      bool
	closeMu        sync.Mutex
}

// Subprotocol returns the subprotocol negotiated during the handshake.
func (ws *WebSocketConnection) Subprotocol() string {
	return ws.subprotocol
}

// Safe for concurrent use
func (ws *WebSocketConnection) Send(msg string) error {
	opcode := opText

	data := []byte(msg)

	ws.writeMu.Lock()
	defer ws.writeMu.Unlock()
	return ws.writeFrameLocked(true, opcode, data)
}

// Safe for concurrent use
func (ws *WebSocketConnection) SendBytes(data []byte) error {
	opcode := opBinary

	ws.writeMu.Lock()
	defer ws.writeMu.Unlock()
	return ws.writeFrameLocked(true, opcode, data)
}

// Reads are serialized, concurrent calls are queued.
func (ws *WebSocketConnection) Read() (string, error) {
	msg, err := ws.ReadMessage()

	if err != nil {
		return "", err
	}

	if msg.IsBinary {
		return "", fmt.Errorf("read: expected string but received binary message")
	}

	return string(msg.Payload), nil
}

// Reads are serialized, concurrent calls are queued.
func (ws *WebSocketConnection) ReadBytes() ([]byte, error) {
	msg, err := ws.readMessageInternal()

	if err != nil {
		return nil, err
	}

	return msg.payload, nil
}

type WebSocketMessage struct {
	Payload  []byte
	IsBinary bool
}

// Reads are serialized, concurrent calls are queued.
func (ws *WebSocketConnection) ReadMessage() (WebSocketMessage, error) {
	var zero WebSocketMessage

	msg, err := ws.readMessageInternal()

	if err != nil {
		return zero, err
	}

	isBinary := msg.op == opBinary

	return WebSocketMessage{
		Payload:  msg.payload,
		IsBinary: isBinary,
	}, err
}

// Close performs the WebSocket closing handshake and then closes the
// underlying connection. It is safe for concurrent use, additional calls are
// no-ops.
//
// After sending the close frame it briefly waits for the peer's close frame
// so the peer observes a clean closure instead of an abnormal one.
func (ws *WebSocketConnection) Close() error {
	return ws.closeWith(statusNormalClosure, "closing")
}

func (ws *WebSocketConnection) closeWith(code int, reason string) error {
	sent, err := ws.sendCloseFrame(code, reason)
	if err != nil {
		ws.rwc.Close()
		return err
	}
	if !sent {
		// A close frame was already sent (peer-close echo, protocol error or an
		// earlier Close). Just release the socket; the handshake is complete.
		ws.rwc.Close()
		return nil
	}

	// Complete the handshake by waiting for the peer's close frame, unless an
	// application read is in progress: that read will observe the close frame
	// and finish the handshake itself. Only drain when the read side is idle.
	if ws.tryLockReadGate() {
		ws.drainUntilClose()
		ws.unlockReadGate()
	}
	ws.rwc.Close()
	return nil
}

// drainUntilClose reads and discards frames until the peer's close frame
// arrives or the handshake deadline passes. Ping frames are answered so the
// peer keeps functioning during the handshake. The caller must hold the read
// gate. Errors end the drain early; the peer has already received our close
// frame.
func (ws *WebSocketConnection) drainUntilClose() {
	_ = ws.rwc.SetReadDeadline(time.Now().Add(closeHandshakeTimeout))
	defer ws.rwc.SetReadDeadline(time.Time{})

	var buf []byte
	for {
		f, err := ws.readFrameInto(&buf)
		if err != nil {
			return
		}
		switch f.opcode {
		case opClose:
			return
		case opPing:
			_ = ws.WritePong(f.payload)
		}
	}
}

const closeHandshakeTimeout = 5 * time.Second

// failConnection tears down the connection with a close frame carrying status.
// It never blocks behind a concurrent writer: if the write mutex is held, it
// closes the socket directly, which unblocks that writer with an error.
func (ws *WebSocketConnection) failConnection(status int, reason string) {
	payload := encodeClosePayload(status, reason)

	ws.closeMu.Lock()
	first := !ws.closeSent
	ws.closeSent = true
	ws.closeMu.Unlock()

	if first && ws.writeMu.TryLock() {
		_ = ws.writeFrameLocked(true, opClose, payload)
		ws.writeMu.Unlock()
	}
	ws.rwc.Close()
}

// opCode identifies the type of a WebSocket frame, per RFC 6455 section 5.2.
type opCode byte

const (
	opContinuation opCode = 0x0
	opText         opCode = 0x1
	opBinary       opCode = 0x2
	// 0x3-0x7 reserved for future non-control frames
	opClose opCode = 0x8
	opPing  opCode = 0x9
	opPong  opCode = 0xA
	// 0xB-0xF reserved for future control frames
)

func (o opCode) isControl() bool {
	return o >= opClose
}

// Close status codes per RFC 6455 section 7.4.1.
const (
	statusNormalClosure    = 1000
	statusProtocolError    = 1002
	statusNoStatusReceived = 1005 // reserved, MUST NOT be sent over the wire
	statusAbnormalClosure  = 1006 // reserved, MUST NOT be sent over the wire
	statusInvalidPayload   = 1007 // e.g. invalid UTF-8 in a text frame
	statusMessageTooBig    = 1009

	// statusGoingAway         = 1001
	// statusUnsupportedData   = 1003
	// statusPolicyViolation   = 1008
	// statusInternalServerErr = 1011
)

// maxControlFramePayload is the RFC 6455 hard limit (section 5.5): control
// frame payloads MUST NOT exceed 125 bytes.
const maxControlFramePayload = 125

// frame is a single decoded WebSocket frame as it appears on the wire.
// Note this is the *frame* layer; Conn.ReadMessage reassembles fragmented
// frames into a complete *message* for the caller.
type frame struct {
	fin     bool
	opcode  opCode
	payload []byte
}

// protocolError represents a violation of RFC 6455 framing rules that
// should result in the connection being failed with StatusProtocolError
// (or a more specific status, when one is set).
type protocolError struct {
	status int
	msg    string
}

func (e *protocolError) Error() string { return e.msg }

func newProtocolErr(status int, format string, args ...any) *protocolError {
	return &protocolError{status: status, msg: fmt.Sprintf(format, args...)}
}

// After a successful call, the caller owns the connection and must eventually call Close.
func upgrade(w http.ResponseWriter, r *http.Request) (*WebSocketConnection, error) {
	if !r.ProtoAtLeast(1, 1) {
		http.Error(w, "websocket: handshake requires HTTP/1.1 or later", http.StatusUpgradeRequired)
		return nil, errors.New("websocket: handshake requires HTTP/1.1 or later")
	}
	if !strings.EqualFold(r.Method, "GET") {
		http.Error(w, "websocket: method must be GET", http.StatusMethodNotAllowed)
		return nil, errors.New("websocket: method must be GET")
	}
	if !headerContainsToken(r.Header, "Connection", "upgrade") {
		http.Error(w, "websocket: missing Connection: Upgrade", http.StatusUpgradeRequired)
		return nil, errors.New("websocket: missing Connection: Upgrade header")
	}
	if !headerContainsToken(r.Header, "Upgrade", "websocket") {
		http.Error(w, "websocket: missing Upgrade: websocket", http.StatusUpgradeRequired)
		return nil, errors.New("websocket: missing Upgrade: websocket header")
	}
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		w.Header().Set("Sec-WebSocket-Version", "13")
		http.Error(w, "websocket: unsupported version", http.StatusBadRequest)
		return nil, errors.New("websocket: unsupported Sec-WebSocket-Version")
	}
	keys := r.Header.Values("Sec-WebSocket-Key")
	if len(keys) == 0 {
		http.Error(w, "websocket: missing Sec-WebSocket-Key", http.StatusBadRequest)
		return nil, errors.New("websocket: missing Sec-WebSocket-Key")
	}
	if len(keys) > 1 {
		http.Error(w, "websocket: multiple Sec-WebSocket-Key headers", http.StatusBadRequest)
		return nil, errors.New("websocket: multiple Sec-WebSocket-Key headers")
	}
	// The RFC states to remove any leading or trailing whitespace.
	key := strings.TrimSpace(keys[0])
	decodedKey, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(decodedKey) != 16 {
		http.Error(w, "websocket: invalid Sec-WebSocket-Key", http.StatusBadRequest)
		return nil, errors.New("websocket: invalid Sec-WebSocket-Key")
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket: server does not support hijacking", http.StatusInternalServerError)
		return nil, errors.New("websocket: ResponseWriter does not support hijacking")
	}

	accept := computeAcceptKey(key)
	subprotocol := selectSubprotocol(r)

	rwc, brw, err := hj.Hijack()
	if err != nil {
		return nil, fmt.Errorf("websocket: hijack failed: %w", err)
	}

	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n"
	if subprotocol != "" {
		resp += "Sec-WebSocket-Protocol: " + subprotocol + "\r\n"
	}
	resp += "\r\n"

	if _, err := brw.WriteString(resp); err != nil {
		rwc.Close()
		return nil, fmt.Errorf("websocket: writing handshake response: %w", err)
	}
	if err := brw.Flush(); err != nil {
		rwc.Close()
		return nil, fmt.Errorf("websocket: flushing handshake response: %w", err)
	}

	const defaultMaxMessageSize = 16 * 1024 * 1024 // 16 MiB

	// brw.Reader may already have buffered bytes the client sent immediately
	// after the handshake (some clients pipeline). Reuse it rather than
	// wrapping rwc in a fresh bufio.Reader, or we'd drop that buffered data.
	// The bufio.Writer is likewise reused for buffered frame writes.
	c := &WebSocketConnection{
		subprotocol:    subprotocol,
		rwc:            rwc,
		br:             brw.Reader,
		bw:             brw.Writer,
		readGate:       make(chan struct{}, 1),
		MaxMessageSize: defaultMaxMessageSize,
	}
	return c, nil
}

// selectSubprotocol picks the protocol to negotiate. httpmin does not define
// its own subprotocols, so it echoes the first one the client offers; per
// RFC 6455 4.2.2 the server must select one of the offers or fail the
// handshake, and browsers abort the connection if a requested subprotocol
// goes unanswered.
func selectSubprotocol(r *http.Request) string {
	for part := range strings.SplitSeq(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
		token := strings.TrimSpace(part)
		if token != "" {
			return token
		}
	}
	return ""
}

func computeAcceptKey(clientKey string) string {
	// Defined in RFC 6455 section 1.3
	const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

	h := sha1.New()
	h.Write([]byte(clientKey))
	h.Write([]byte(websocketGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func headerContainsToken(h http.Header, name, token string) bool {
	for _, v := range h.Values(name) {
		for part := range strings.SplitSeq(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

// readFrameInto reads a single decoded frame. Data frames (text, binary and
// continuation) append their payload onto *message so fragmented frames
// accumulate without a per-frame allocation, and a single-frame message avoids
// the extra copy of reading into an intermediate buffer. Control frame
// payloads are returned in their own small allocation so they never contaminate
// the message buffer.
func (ws *WebSocketConnection) readFrameInto(message *[]byte) (frame, error) {
	var zero frame

	var hdr [2]byte
	if _, err := io.ReadFull(ws.br, hdr[:]); err != nil {
		return zero, err
	}

	fin := hdr[0]&0x80 != 0
	rsv := hdr[0] & 0x70
	opcode := opCode(hdr[0] & 0x0F)
	masked := hdr[1]&0x80 != 0
	payloadLen7 := hdr[1] & 0x7F

	if rsv != 0 {
		// We don't negotiate any extensions, so any RSV bit set is a
		// protocol violation (RFC 6455 5.2).
		return zero, newProtocolErr(statusProtocolError, "nonzero RSV bits without negotiated extension")
	}

	switch opcode {
	case opContinuation, opText, opBinary, opClose, opPing, opPong:
		// Do nothing
	default:
		return zero, newProtocolErr(statusProtocolError, "unknown opcode %d", opcode)
	}

	if opcode.isControl() && !fin {
		return zero, newProtocolErr(statusProtocolError, "fragmented control frame")
	}

	// Per RFC 6455 5.1: a server MUST close the connection if it receives a
	// frame that is not masked, and a client MUST mask all frames it sends.
	if !masked {
		return zero, newProtocolErr(statusProtocolError, "received unmasked frame from client")
	}

	var payloadLen uint64
	switch {
	case payloadLen7 <= 125:
		payloadLen = uint64(payloadLen7)
	case payloadLen7 == 126:
		var ext [2]byte
		if _, err := io.ReadFull(ws.br, ext[:]); err != nil {
			return zero, err
		}
		payloadLen = uint64(binary.BigEndian.Uint16(ext[:]))
	case payloadLen7 == 127:
		var ext [8]byte
		if _, err := io.ReadFull(ws.br, ext[:]); err != nil {
			return zero, err
		}
		payloadLen = binary.BigEndian.Uint64(ext[:])
		if payloadLen&(1<<63) != 0 {
			return zero, newProtocolErr(statusProtocolError, "most significant bit of 64-bit length set")
		}
	}

	if opcode.isControl() && payloadLen > maxControlFramePayload {
		return zero, newProtocolErr(statusProtocolError, "control frame payload too large: %d", payloadLen)
	}
	if payloadLen > uint64(ws.MaxMessageSize) {
		return zero, newProtocolErr(statusMessageTooBig, "frame payload exceeds max message size: %d", payloadLen)
	}

	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(ws.br, maskKey[:]); err != nil {
			return zero, err
		}
	}

	if opcode.isControl() {
		payload := make([]byte, payloadLen)
		if payloadLen > 0 {
			if _, err := io.ReadFull(ws.br, payload); err != nil {
				return zero, err
			}
			if masked {
				applyMask(payload, maskKey)
			}
		}
		return frame{fin: fin, opcode: opcode, payload: payload}, nil
	}

	start := len(*message)
	*message = append(*message, make([]byte, payloadLen)...)
	if payloadLen > 0 {
		payload := (*message)[start:]
		if _, err := io.ReadFull(ws.br, payload); err != nil {
			return zero, err
		}
		if masked {
			applyMask(payload, maskKey)
		}
	}
	return frame{fin: fin, opcode: opcode, payload: (*message)[start:]}, nil
}

// applyMask XORs payload in place with the rolling 4-byte mask key, per
// RFC 6455 section 5.3. The 4-byte key repeats every 4 bytes, so 8 bytes of
// payload are processed per iteration with a widened key.
func applyMask(payload []byte, key [4]byte) {
	k := binary.LittleEndian.Uint32(key[:])
	k8 := uint64(k)<<32 | uint64(k)

	i := 0
	for ; i+8 <= len(payload); i += 8 {
		v := binary.LittleEndian.Uint64(payload[i:])
		binary.LittleEndian.PutUint64(payload[i:], v^k8)
	}
	for ; i < len(payload); i++ {
		payload[i] ^= byte(k >> (8 * (uint(i) & 3)))
	}
}

// writeFrameLocked encodes and writes a single frame and flushes it to the
// socket. Caller must hold writeMu. Frames are always unmasked because the
// server never masks (RFC 6455 5.1).
func (ws *WebSocketConnection) writeFrameLocked(fin bool, opcode opCode, payload []byte) error {
	b0 := byte(opcode)
	if fin {
		b0 |= 0x80
	}
	if err := ws.bw.WriteByte(b0); err != nil {
		return err
	}

	n := len(payload)
	switch {
	case n <= 125:
		if err := ws.bw.WriteByte(byte(n)); err != nil {
			return err
		}
	case n <= 65535:
		if err := ws.bw.WriteByte(126); err != nil {
			return err
		}
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(n))
		if _, err := ws.bw.Write(ext[:]); err != nil {
			return err
		}
	default:
		if err := ws.bw.WriteByte(127); err != nil {
			return err
		}
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(n))
		if _, err := ws.bw.Write(ext[:]); err != nil {
			return err
		}
	}

	if len(payload) > 0 {
		if _, err := ws.bw.Write(payload); err != nil {
			return err
		}
	}
	return ws.bw.Flush()
}

// writeControl sends a control frame (ping/pong/close). Safe for concurrent
// use; serialized against Send/SendBytes via the same mutex so frames never
// interleave on the wire.
//
// Control writes are bounded by a deadline so a stuck peer can't wedge the
// write mutex forever.
func (ws *WebSocketConnection) writeControl(opcode opCode, payload []byte) error {
	if len(payload) > maxControlFramePayload {
		return fmt.Errorf("websocket: control frame payload exceeds %d bytes", maxControlFramePayload)
	}
	ws.writeMu.Lock()
	defer ws.writeMu.Unlock()

	_ = ws.rwc.SetWriteDeadline(time.Now().Add(closeControlTimeout))
	defer ws.rwc.SetWriteDeadline(time.Time{})

	return ws.writeFrameLocked(true, opcode, payload)
}

const closeControlTimeout = 5 * time.Second

// WritePing sends a ping control frame with an optional payload (<=125 bytes).
func (ws *WebSocketConnection) WritePing(payload []byte) error {
	return ws.writeControl(opPing, payload)
}

// WritePong sends a pong control frame, normally in response to a ping.
func (ws *WebSocketConnection) WritePong(payload []byte) error {
	return ws.writeControl(opPong, payload)
}

// socketMessage represents a complete, reassembled application socketMessage.
type socketMessage struct {
	op      opCode // OpText or OpBinary
	payload []byte
}

// readMessageInternal reads the next complete application message, transparently
// reassembling fragmented frames and handling/auto-responding to control
// frames (ping -> pong) that arrive interleaved between fragments. It
// returns io.EOF (or a wrapped close error) when the peer closes the
// connection.
//
// Concurrent calls are serialized by the read gate.
func (ws *WebSocketConnection) readMessageInternal() (socketMessage, error) {
	ws.lockReadGate()
	defer ws.unlockReadGate()

	var (
		zero      socketMessage
		msgOpcode opCode
		buf       []byte
		started   bool
	)

	for {
		f, err := ws.readFrameInto(&buf)
		if err != nil {
			var pe *protocolError
			if errors.As(err, &pe) {
				ws.failConnection(pe.status, pe.msg)
			}
			return zero, err
		}

		switch f.opcode {
		case opPing:
			if err := ws.WritePong(f.payload); err != nil {
				return zero, err
			}

		case opPong:
			// Unsolicited pongs are valid, nothing to do

		case opClose:
			code, reason, perr := parseClosePayload(f.payload)
			if perr != nil {
				ws.failConnection(statusProtocolError, "invalid close payload")
				return zero, perr
			}
			ws.closeMu.Lock()
			alreadySent := ws.closeSent
			ws.closeRecv = true
			ws.closeMu.Unlock()
			if !alreadySent {
				// Echo the close frame back (RFC 6455 5.5.1 closing handshake).
				ws.failConnection(code, reason)
			} else {
				ws.rwc.Close()
			}
			return zero, &closeError{Code: code, Reason: reason}

		case opText, opBinary:
			if started {
				return zero, newProtocolErr(statusProtocolError, "new message started before previous one finished")
			}
			if int64(len(buf)) > ws.MaxMessageSize {
				ws.failConnection(statusMessageTooBig, "message exceeds max size")
				return zero, newProtocolErr(statusMessageTooBig, "message exceeds max size")
			}
			msgOpcode = f.opcode
			if f.fin {
				return ws.finishMessage(msgOpcode, buf)
			}
			started = true

		case opContinuation:
			if !started {
				return zero, newProtocolErr(statusProtocolError, "continuation frame without preceding start frame")
			}
			if int64(len(buf)) > ws.MaxMessageSize {
				ws.failConnection(statusMessageTooBig, "message exceeds max size")
				return zero, newProtocolErr(statusMessageTooBig, "message exceeds max size")
			}
			if f.fin {
				return ws.finishMessage(msgOpcode, buf)
			}
		}
	}
}

// finishMessage validates a fully-reassembled message (UTF-8 for text
// frames, per RFC 6455 8.1) before handing it to the caller.
func (ws *WebSocketConnection) finishMessage(op opCode, payload []byte) (socketMessage, error) {
	if op == opText && !utf8.Valid(payload) {
		ws.failConnection(statusInvalidPayload, "invalid UTF-8 in text message")
		return socketMessage{}, newProtocolErr(statusInvalidPayload, "invalid UTF-8 in text message")
	}
	return socketMessage{op: op, payload: payload}, nil
}

// closeError is returned from ReadMessage when the peer initiated (or
// completed) the closing handshake.
type closeError struct {
	Code   int
	Reason string
}

func (e *closeError) Error() string {
	return fmt.Sprintf("websocket: closed: code=%d reason=%q", e.Code, e.Reason)
}

func parseClosePayload(payload []byte) (code int, reason string, err error) {
	if len(payload) == 0 {
		return statusNoStatusReceived, "", nil
	}
	if len(payload) == 1 {
		return 0, "", newProtocolErr(statusProtocolError, "close payload has 1 byte")
	}
	code = int(binary.BigEndian.Uint16(payload[:2]))
	reason = string(payload[2:])
	if !utf8.ValidString(reason) {
		return 0, "", newProtocolErr(statusInvalidPayload, "invalid UTF-8 in close reason")
	}
	if !isValidCloseCode(code) {
		return 0, "", newProtocolErr(statusProtocolError, "invalid close code %d", code)
	}
	return code, reason, nil
}

func isValidCloseCode(code int) bool {
	switch {
	case code < 1000:
		return false
	case code == statusNoStatusReceived, code == statusAbnormalClosure:
		return false // reserved, must never appear on the wire
	case code == 1004, code == 1015:
		return false // reserved
	case code >= 1000 && code <= 1003:
		return true
	case code >= 1007 && code <= 1014:
		// 1012-1014 are registered (Service Restart, Try Again Later, Bad Gateway)
		return true
	case code >= 3000 && code <= 4999:
		return true // reserved for libraries/frameworks/private use
	default:
		return false
	}
}

// sendCloseFrame sends a close frame unless one has already been sent for this
// connection (each side sends at most one close frame per handshake). It
// reports whether the frame was actually sent.
func (ws *WebSocketConnection) sendCloseFrame(code int, reason string) (bool, error) {
	ws.closeMu.Lock()
	if ws.closeSent {
		ws.closeMu.Unlock()
		return false, nil
	}
	ws.closeSent = true
	ws.closeMu.Unlock()

	return true, ws.writeControl(opClose, encodeClosePayload(code, reason))
}

// encodeClosePayload builds the wire payload for a close frame. Reserved codes
// (1005/1006) that must never appear on the wire map to a bare close frame
// with no payload (RFC 6455 7.4.1); the reason is truncated so the total
// payload stays within the control frame limit.
func encodeClosePayload(code int, reason string) []byte {
	if code == statusNoStatusReceived || code == statusAbnormalClosure {
		return nil
	}
	payload := make([]byte, 2+len(reason))
	binary.BigEndian.PutUint16(payload[:2], uint16(code))
	copy(payload[2:], reason)
	if len(payload) > maxControlFramePayload {
		payload = payload[:maxControlFramePayload]
	}
	return payload
}

// The read gate serializes access to the socket's read side so application
// reads and the close-handshake drain never consume each other's frames.
func (ws *WebSocketConnection) lockReadGate() {
	ws.readGate <- struct{}{}
}

func (ws *WebSocketConnection) unlockReadGate() {
	<-ws.readGate
}

func (ws *WebSocketConnection) tryLockReadGate() bool {
	select {
	case ws.readGate <- struct{}{}:
		return true
	default:
		return false
	}
}
