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
	"unicode/utf8"
)

// websocketGUID is the magic string defined in RFC 6455 section 1.3,
// concatenated with the client's Sec-WebSocket-Key before SHA-1 hashing
// to produce the Sec-WebSocket-Accept header.
const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// Opcode identifies the type of a WebSocket frame, per RFC 6455 section 5.2.
type Opcode byte

const (
	OpContinuation Opcode = 0x0
	OpText         Opcode = 0x1
	OpBinary       Opcode = 0x2
	// 0x3-0x7 reserved for future non-control frames
	OpClose Opcode = 0x8
	OpPing  Opcode = 0x9
	OpPong  Opcode = 0xA
	// 0xB-0xF reserved for future control frames
)

func (o Opcode) isControl() bool {
	return o >= OpClose
}

// Close status codes per RFC 6455 section 7.4.1.
const (
	StatusNormalClosure     = 1000
	StatusGoingAway         = 1001
	StatusProtocolError     = 1002
	StatusUnsupportedData   = 1003
	StatusNoStatusReceived  = 1005 // reserved, MUST NOT be sent over the wire
	StatusAbnormalClosure   = 1006 // reserved, MUST NOT be sent over the wire
	StatusInvalidPayload    = 1007 // e.g. invalid UTF-8 in a text frame
	StatusPolicyViolation   = 1008
	StatusMessageTooBig     = 1009
	StatusInternalServerErr = 1011
)

// maxControlFramePayload is the RFC 6455 hard limit (section 5.5): control
// frame payloads MUST NOT exceed 125 bytes.
const maxControlFramePayload = 125

// MaxMessageSize caps the total size of a reassembled (possibly fragmented)
// message. This guards against memory-exhaustion from a malicious or buggy
// peer. 0 means "use DefaultMaxMessageSize".
const DefaultMaxMessageSize = 16 * 1024 * 1024 // 16 MiB

// frame is a single decoded WebSocket frame as it appears on the wire.
// Note this is the *frame* layer; Conn.ReadMessage reassembles fragmented
// frames into a complete *message* for the caller.
type frame struct {
	fin     bool
	opcode  Opcode
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

// Socket is a single upgraded Socket connection. It is safe to call
// WriteMessage / WriteControl from multiple goroutines concurrently (writes
// are serialized internally); ReadMessage must only be called from one
// goroutine at a time (typically a single read loop), per standard Go
// net.Socket conventions.
type Socket struct {
	rwc        net.Conn
	br         *bufio.Reader
	isServer   bool // servers must NOT mask frames they send; clients MUST mask
	maxMsgSize int64

	writeMu sync.Mutex

	closeSent bool
	closeRecv bool
	closeMu   sync.Mutex
}

// upgrade upgrades an incoming HTTP request to a WebSocket connection. It
// performs the RFC 6455 handshake (validating headers and writing the 101
// response) and then hijacks the underlying TCP connection. After a
// successful call, the caller owns the connection and must eventually call
// Close.
func upgrade(w http.ResponseWriter, r *http.Request) (*Socket, error) {
	if !strings.EqualFold(r.Method, "GET") {
		http.Error(w, "websocket: method must be GET", http.StatusMethodNotAllowed)
		return nil, errors.New("websocket: method must be GET")
	}
	if !headerContainsToken(r.Header, "Connection", "upgrade") {
		http.Error(w, "websocket: missing Connection: Upgrade", http.StatusBadRequest)
		return nil, errors.New("websocket: missing Connection: Upgrade header")
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "websocket: missing Upgrade: websocket", http.StatusBadRequest)
		return nil, errors.New("websocket: missing Upgrade: websocket header")
	}
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		w.Header().Set("Sec-WebSocket-Version", "13")
		http.Error(w, "websocket: unsupported version", http.StatusUpgradeRequired)
		return nil, errors.New("websocket: unsupported Sec-WebSocket-Version")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "websocket: missing Sec-WebSocket-Key", http.StatusBadRequest)
		return nil, errors.New("websocket: missing Sec-WebSocket-Key")
	}
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

	rwc, brw, err := hj.Hijack()
	if err != nil {
		return nil, fmt.Errorf("websocket: hijack failed: %w", err)
	}

	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := brw.WriteString(resp); err != nil {
		rwc.Close()
		return nil, fmt.Errorf("websocket: writing handshake response: %w", err)
	}
	if err := brw.Flush(); err != nil {
		rwc.Close()
		return nil, fmt.Errorf("websocket: flushing handshake response: %w", err)
	}

	// brw.Reader may already have buffered bytes the client sent immediately
	// after the handshake (some clients pipeline). Reuse it rather than
	// wrapping rwc in a fresh bufio.Reader, or we'd drop that buffered data.
	c := &Socket{
		rwc:        rwc,
		br:         brw.Reader,
		isServer:   true,
		maxMsgSize: DefaultMaxMessageSize,
	}
	return c, nil
}

// SetMaxMessageSize overrides DefaultMaxMessageSize for this connection.
func (ws *Socket) SetMaxMessageSize(n int64) { ws.maxMsgSize = n }

func computeAcceptKey(clientKey string) string {
	h := sha1.New()
	h.Write([]byte(clientKey))
	h.Write([]byte(websocketGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// headerContainsToken reports whether the named comma-separated header
// contains the given token, case-insensitively (used for "Connection:
// keep-alive, Upgrade" style headers that Go's r.Header.Get can't match
// directly with EqualFold).
func headerContainsToken(h http.Header, name, token string) bool {
	for _, v := range h.Values(name) {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

// ---- Frame reading ----

// readFrame reads and decodes a single frame from the wire, including
// unmasking if present. It enforces the control-frame size limit and basic
// header sanity (reserved bits, RSV bits, fragmented-control-frame ban)
// but does NOT enforce message-level rules like "no continuation without a
// preceding start frame" — that's ReadMessage's job, since it requires
// state across frames.
func (ws *Socket) readFrame() (frame, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(ws.br, hdr[:]); err != nil {
		return frame{}, err
	}

	fin := hdr[0]&0x80 != 0
	rsv := hdr[0] & 0x70
	opcode := Opcode(hdr[0] & 0x0F)
	masked := hdr[1]&0x80 != 0
	payloadLen7 := hdr[1] & 0x7F

	if rsv != 0 {
		// We don't negotiate any extensions, so any RSV bit set is a
		// protocol violation (RFC 6455 5.2).
		return frame{}, newProtocolErr(StatusProtocolError, "nonzero RSV bits without negotiated extension")
	}

	switch opcode {
	case OpContinuation, OpText, OpBinary, OpClose, OpPing, OpPong:
		// known opcode
	default:
		return frame{}, newProtocolErr(StatusProtocolError, "unknown opcode %d", opcode)
	}

	if opcode.isControl() && !fin {
		return frame{}, newProtocolErr(StatusProtocolError, "fragmented control frame")
	}

	// Per RFC 6455 5.1: a server MUST close the connection if it receives a
	// frame that is not masked, and a client MUST mask all frames it sends.
	if ws.isServer && !masked {
		return frame{}, newProtocolErr(StatusProtocolError, "received unmasked frame from client")
	}
	if !ws.isServer && masked {
		return frame{}, newProtocolErr(StatusProtocolError, "received masked frame from server")
	}

	var payloadLen uint64
	switch {
	case payloadLen7 <= 125:
		payloadLen = uint64(payloadLen7)
	case payloadLen7 == 126:
		var ext [2]byte
		if _, err := io.ReadFull(ws.br, ext[:]); err != nil {
			return frame{}, err
		}
		payloadLen = uint64(binary.BigEndian.Uint16(ext[:]))
	case payloadLen7 == 127:
		var ext [8]byte
		if _, err := io.ReadFull(ws.br, ext[:]); err != nil {
			return frame{}, err
		}
		payloadLen = binary.BigEndian.Uint64(ext[:])
		if payloadLen&(1<<63) != 0 {
			return frame{}, newProtocolErr(StatusProtocolError, "most significant bit of 64-bit length set")
		}
	}

	if opcode.isControl() && payloadLen > maxControlFramePayload {
		return frame{}, newProtocolErr(StatusProtocolError, "control frame payload too large: %d", payloadLen)
	}
	if payloadLen > uint64(ws.maxMsgSize) {
		return frame{}, newProtocolErr(StatusMessageTooBig, "frame payload exceeds max message size: %d", payloadLen)
	}

	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(ws.br, maskKey[:]); err != nil {
			return frame{}, err
		}
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(ws.br, payload); err != nil {
		return frame{}, err
	}
	if masked {
		applyMask(payload, maskKey)
	}

	return frame{fin: fin, opcode: opcode, payload: payload}, nil
}

// applyMask XORs payload in place with the rolling 4-byte mask key, per
// RFC 6455 section 5.3.
func applyMask(payload []byte, key [4]byte) {
	for i := range payload {
		payload[i] ^= key[i%4]
	}
}

// ---- Frame writing ----

// writeFrame encodes and writes a single frame. Caller must hold writeMu.
func (ws *Socket) writeFrameLocked(fin bool, opcode Opcode, payload []byte) error {
	var hdr []byte
	b0 := byte(opcode)
	if fin {
		b0 |= 0x80
	}
	hdr = append(hdr, b0)

	maskBit := byte(0)
	if !ws.isServer {
		maskBit = 0x80 // clients MUST mask
	}

	n := len(payload)
	switch {
	case n <= 125:
		hdr = append(hdr, maskBit|byte(n))
	case n <= 65535:
		hdr = append(hdr, maskBit|126)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(n))
		hdr = append(hdr, ext[:]...)
	default:
		hdr = append(hdr, maskBit|127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(n))
		hdr = append(hdr, ext[:]...)
	}

	if _, err := ws.rwc.Write(hdr); err != nil {
		return err
	}

	if maskBit != 0 {
		var key [4]byte
		// A real client should use crypto/rand here. This implementation
		// targets server-side Autobahn testing (fuzzing client mode is a
		// possible extension), so a fixed-but-present mask keeps the wire
		// format correct without pulling in extra complexity.
		copy(key[:], []byte{0x12, 0x34, 0x56, 0x78})
		masked := make([]byte, len(payload))
		copy(masked, payload)
		applyMask(masked, key)
		if _, err := ws.rwc.Write(key[:]); err != nil {
			return err
		}
		if len(masked) > 0 {
			if _, err := ws.rwc.Write(masked); err != nil {
				return err
			}
		}
		return nil
	}

	if len(payload) > 0 {
		if _, err := ws.rwc.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

func (ws *Socket) Send(msg string) error {
	opcode := OpText

	data := []byte(msg)

	ws.writeMu.Lock()
	defer ws.writeMu.Unlock()
	return ws.writeFrameLocked(true, opcode, data)
}

// Safe for concurrent use
func (ws *Socket) SendBytes(data []byte) error {
	opcode := OpBinary

	ws.writeMu.Lock()
	defer ws.writeMu.Unlock()
	return ws.writeFrameLocked(true, opcode, data)
}

// writeControl sends a control frame (ping/pong/close). Safe for concurrent
// use; serialized against WriteMessage via the same mutex so frames never
// interleave on the wire.
func (ws *Socket) writeControl(opcode Opcode, payload []byte) error {
	if len(payload) > maxControlFramePayload {
		return fmt.Errorf("websocket: control frame payload exceeds %d bytes", maxControlFramePayload)
	}
	ws.writeMu.Lock()
	defer ws.writeMu.Unlock()
	return ws.writeFrameLocked(true, opcode, payload)
}

// WritePing sends a ping control frame with an optional payload (<=125 bytes).
func (ws *Socket) WritePing(payload []byte) error { return ws.writeControl(OpPing, payload) }

// WritePong sends a pong control frame, normally in response to a ping.
func (ws *Socket) WritePong(payload []byte) error { return ws.writeControl(OpPong, payload) }

// ---- Message-level reads (handles fragmentation + control frames) ----

// Message represents a complete, reassembled application message.
type Message struct {
	Opcode  Opcode // OpText or OpBinary
	Payload []byte
}

// ReadMessage reads the next complete application message, transparently
// reassembling fragmented frames and handling/auto-responding to control
// frames (ping -> pong) that arrive interleaved between fragments. It
// returns io.EOF (or a wrapped close error) when the peer closes the
// connection.
//
// Must be called from a single goroutine at a time.
func (ws *Socket) ReadMessage() (Message, error) {
	var (
		msgOpcode Opcode
		buf       []byte
		started   bool
	)

	for {
		f, err := ws.readFrame()
		if err != nil {
			var pe *protocolError
			if errors.As(err, &pe) {
				_ = ws.sendCloseLocked(pe.status, pe.msg)
				ws.rwc.Close()
			}
			return Message{}, err
		}

		switch f.opcode {
		case OpPing:
			if err := ws.WritePong(f.payload); err != nil {
				return Message{}, err
			}
			continue

		case OpPong:
			// Unsolicited pongs are valid (e.g. heartbeats); nothing to do.
			continue

		case OpClose:
			code, reason, perr := parseClosePayload(f.payload)
			if perr != nil {
				_ = ws.sendCloseLocked(StatusProtocolError, "invalid close payload")
				ws.rwc.Close()
				return Message{}, perr
			}
			ws.closeMu.Lock()
			alreadySent := ws.closeSent
			ws.closeRecv = true
			ws.closeMu.Unlock()
			if !alreadySent {
				// Echo the close frame back (RFC 6455 5.5.1 closing handshake).
				_ = ws.sendCloseLocked(code, reason)
			}
			ws.rwc.Close()
			return Message{}, &CloseError{Code: code, Reason: reason}

		case OpText, OpBinary:
			if started {
				return Message{}, newProtocolErr(StatusProtocolError, "new message started before previous one finished")
			}
			msgOpcode = f.opcode
			buf = append(buf, f.payload...)
			if int64(len(buf)) > ws.maxMsgSize {
				_ = ws.sendCloseLocked(StatusMessageTooBig, "message too big")
				ws.rwc.Close()
				return Message{}, newProtocolErr(StatusMessageTooBig, "message exceeds max size")
			}
			if f.fin {
				return ws.finishMessage(msgOpcode, buf)
			}
			started = true
			continue

		case OpContinuation:
			if !started {
				return Message{}, newProtocolErr(StatusProtocolError, "continuation frame without preceding start frame")
			}
			buf = append(buf, f.payload...)
			if int64(len(buf)) > ws.maxMsgSize {
				_ = ws.sendCloseLocked(StatusMessageTooBig, "message too big")
				ws.rwc.Close()
				return Message{}, newProtocolErr(StatusMessageTooBig, "message exceeds max size")
			}
			if f.fin {
				return ws.finishMessage(msgOpcode, buf)
			}
			continue
		}
	}
}

// finishMessage validates a fully-reassembled message (UTF-8 for text
// frames, per RFC 6455 8.1) before handing it to the caller.
func (ws *Socket) finishMessage(opcode Opcode, payload []byte) (Message, error) {
	if opcode == OpText && !utf8.Valid(payload) {
		_ = ws.sendCloseLocked(StatusInvalidPayload, "invalid UTF-8 in text message")
		ws.rwc.Close()
		return Message{}, newProtocolErr(StatusInvalidPayload, "invalid UTF-8 in text message")
	}
	return Message{Opcode: opcode, Payload: payload}, nil
}

// CloseError is returned from ReadMessage when the peer initiated (or
// completed) the closing handshake.
type CloseError struct {
	Code   int
	Reason string
}

func (e *CloseError) Error() string {
	return fmt.Sprintf("websocket: closed: code=%d reason=%q", e.Code, e.Reason)
}

func parseClosePayload(payload []byte) (code int, reason string, err error) {
	if len(payload) == 0 {
		return StatusNoStatusReceived, "", nil
	}
	if len(payload) == 1 {
		return 0, "", newProtocolErr(StatusProtocolError, "close payload has 1 byte")
	}
	code = int(binary.BigEndian.Uint16(payload[:2]))
	reason = string(payload[2:])
	if !utf8.ValidString(reason) {
		return 0, "", newProtocolErr(StatusInvalidPayload, "invalid UTF-8 in close reason")
	}
	if !isValidCloseCode(code) {
		return 0, "", newProtocolErr(StatusProtocolError, "invalid close code %d", code)
	}
	return code, reason, nil
}

func isValidCloseCode(code int) bool {
	switch {
	case code < 1000:
		return false
	case code == StatusNoStatusReceived, code == StatusAbnormalClosure:
		return false // reserved, must never appear on the wire
	case code == 1004, code == 1015:
		return false // reserved
	case code >= 1000 && code <= 1003:
		return true
	case code >= 1007 && code <= 1011:
		return true
	case code >= 3000 && code <= 4999:
		return true // reserved for libraries/frameworks/private use
	default:
		return false
	}
}

// Close performs the closing handshake: sends a close frame (if one hasn't
// already been sent) and closes the underlying connection. It does not wait
// for the peer's close frame in response; callers using ReadMessage in a
// loop will observe that as a CloseError return.
//
// code must be a valid RFC 6455 close code (e.g. StatusNormalClosure); pass
// StatusNormalClosure for a plain, no-error close.
func (ws *Socket) Close(code int, reason string) error {
	err := ws.sendCloseLocked(code, reason)
	ws.rwc.Close()
	return err
}

// sendCloseLocked sends a close frame, unless one has already been sent for
// this connection (the close handshake only ever sends one close frame per
// side). The name is historical ("Locked" refers to closeMu guarding
// closeSent below, not writeMu — writeControl acquires that itself).
func (ws *Socket) sendCloseLocked(code int, reason string) error {
	ws.closeMu.Lock()
	if ws.closeSent {
		ws.closeMu.Unlock()
		return nil
	}
	ws.closeSent = true
	ws.closeMu.Unlock()

	if code == StatusNoStatusReceived || code == StatusAbnormalClosure {
		// 1005/1006 are reserved for local use only and must never be sent
		// on the wire (RFC 6455 7.4.1). This happens when the peer's close
		// frame had no status (represented internally as 1005) and we're
		// echoing it back — send a bare close frame with no payload instead
		// of leaking 1005/1006 onto the wire.
		var empty []byte
		return ws.writeControl(OpClose, empty)
	}
	payload := make([]byte, 2+len(reason))
	binary.BigEndian.PutUint16(payload[:2], uint16(code))
	copy(payload[2:], reason)
	if len(payload) > maxControlFramePayload {
		payload = payload[:maxControlFramePayload]
	}
	return ws.writeControl(OpClose, payload)
}
