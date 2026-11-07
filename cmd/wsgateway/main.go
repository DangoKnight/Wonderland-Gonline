package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	wsGUID     = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	maxPayload = 1024 * 1024 // 1 MiB

	opcodeText   = 1
	opcodeBinary = 2
	opcodeClose  = 8
	opcodePing   = 9
	opcodePong   = 10

	closeNormalClosure = 1000
	closeProtocolError = 1002
	closeMessageTooBig = 1009
)

// Config configures the gateway handler.
type Config struct {
	DefaultTarget  string
	ValidateTarget func(target string) error
	WebDir         string // optional static directory served at /
}

func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// DefaultValidateTarget enforces that the target host is 127.0.0.1 or localhost,
// and the port is within 6000-6999.
func DefaultValidateTarget(target string) error {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return fmt.Errorf("invalid host:port: %w", err)
	}
	if host != "127.0.0.1" && !strings.EqualFold(host, "localhost") {
		return fmt.Errorf("disallowed target host %q: only 127.0.0.1 and localhost allowed", host)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 6000 || port > 6999 {
		return fmt.Errorf("disallowed target port %q: must be between 6000 and 6999", portStr)
	}
	return nil
}

func computeAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func headerContainsToken(headerValue, token string) bool {
	for _, part := range strings.Split(headerValue, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

func encodeFrameHeader(opcode byte, length int) []byte {
	b0 := byte(0x80) | (opcode & 0x0F) // FIN = 1
	if length < 126 {
		return []byte{b0, byte(length)}
	} else if length <= 65535 {
		buf := make([]byte, 4)
		buf[0] = b0
		buf[1] = 126
		binary.BigEndian.PutUint16(buf[2:4], uint16(length))
		return buf
	} else {
		buf := make([]byte, 10)
		buf[0] = b0
		buf[1] = 127
		binary.BigEndian.PutUint64(buf[2:10], uint64(length))
		return buf
	}
}

type wsConn struct {
	conn      net.Conn
	br        *bufio.Reader
	wMu       sync.Mutex
	closeSent bool
}

func (w *wsConn) WriteFrame(opcode byte, payload []byte) error {
	w.wMu.Lock()
	defer w.wMu.Unlock()
	header := encodeFrameHeader(opcode, len(payload))
	packet := make([]byte, len(header)+len(payload))
	copy(packet, header)
	copy(packet[len(header):], payload)
	_, err := w.conn.Write(packet)
	return err
}

func (w *wsConn) SendClose(code uint16) error {
	w.wMu.Lock()
	defer w.wMu.Unlock()
	if w.closeSent {
		return nil
	}
	w.closeSent = true
	payload := make([]byte, 2)
	binary.BigEndian.PutUint16(payload, code)
	header := encodeFrameHeader(opcodeClose, len(payload))
	packet := make([]byte, len(header)+len(payload))
	copy(packet, header)
	copy(packet[len(header):], payload)
	_, err := w.conn.Write(packet)
	return err
}

func (w *wsConn) Close() error {
	return w.conn.Close()
}

func (w *wsConn) ReadClientFrame() ([]byte, byte, error) {
	var b [2]byte
	if _, err := io.ReadFull(w.br, b[:]); err != nil {
		return nil, 0, err
	}
	fin := (b[0] & 0x80) != 0
	rsv := b[0] & 0x70
	opcode := b[0] & 0x0F
	masked := (b[1] & 0x80) != 0
	lenByte := b[1] & 0x7F

	if rsv != 0 {
		_ = w.SendClose(closeProtocolError)
		return nil, opcode, errors.New("RSV bits must be 0")
	}

	if !fin {
		// Reject fragmented messages (FIN=0) with close 1002
		_ = w.SendClose(closeProtocolError)
		return nil, opcode, errors.New("fragmented frame not supported")
	}

	if !masked {
		// Reject unmasked client frames
		_ = w.SendClose(closeProtocolError)
		return nil, opcode, errors.New("client frame must be masked")
	}

	var payloadLen int64
	switch lenByte {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(w.br, ext[:]); err != nil {
			return nil, opcode, err
		}
		payloadLen = int64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(w.br, ext[:]); err != nil {
			return nil, opcode, err
		}
		payloadLen = int64(binary.BigEndian.Uint64(ext[:]))
		if payloadLen < 0 {
			_ = w.SendClose(closeProtocolError)
			return nil, opcode, errors.New("payload length MSB must be 0")
		}
	default:
		payloadLen = int64(lenByte)
	}

	if opcode >= 8 && payloadLen > 125 {
		_ = w.SendClose(closeProtocolError)
		return nil, opcode, errors.New("control frame payload exceeds 125 bytes")
	}

	if payloadLen > maxPayload {
		_ = w.SendClose(closeMessageTooBig)
		return nil, opcode, fmt.Errorf("payload length %d exceeds 1 MiB limit", payloadLen)
	}

	var maskKey [4]byte
	if _, err := io.ReadFull(w.br, maskKey[:]); err != nil {
		return nil, opcode, err
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(w.br, payload); err != nil {
		return nil, opcode, err
	}

	for i := range payload {
		payload[i] ^= maskKey[i%4]
	}

	return payload, opcode, nil
}

func proxyConnection(remoteAddr, target string, ws *wsConn, tcpConn net.Conn) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[%s] recovered panic: %v", remoteAddr, rec)
		}
	}()

	log.Printf("connected: %s -> %s", remoteAddr, target)
	defer log.Printf("disconnected: %s", remoteAddr)

	var once sync.Once
	closeBoth := func() {
		once.Do(func() {
			_ = tcpConn.Close()
			_ = ws.Close()
		})
	}
	defer closeBoth()

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: WS -> TCP
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[%s] recovered panic in ws->tcp: %v", remoteAddr, rec)
			}
			closeBoth()
			wg.Done()
		}()

		for {
			payload, opcode, err := ws.ReadClientFrame()
			if err != nil {
				return
			}
			switch opcode {
			case opcodeText, opcodeBinary:
				if _, err := tcpConn.Write(payload); err != nil {
					return
				}
			case opcodeClose:
				code := uint16(closeNormalClosure)
				if len(payload) >= 2 {
					code = binary.BigEndian.Uint16(payload[:2])
				}
				_ = ws.SendClose(code)
				return
			case opcodePing:
				if err := ws.WriteFrame(opcodePong, payload); err != nil {
					return
				}
			case opcodePong:
				// Discard pong
			default:
				_ = ws.SendClose(closeProtocolError)
				return
			}
		}
	}()

	// Goroutine 2: TCP -> WS
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[%s] recovered panic in tcp->ws: %v", remoteAddr, rec)
			}
			closeBoth()
			wg.Done()
		}()

		buf := make([]byte, 32768)
		for {
			n, err := tcpConn.Read(buf)
			if n > 0 {
				// Server-to-client frames are unmasked binary frames (opcode 2)
				if err := ws.WriteFrame(opcodeBinary, buf[:n]); err != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	wg.Wait()
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func handleWebSocket(w http.ResponseWriter, r *http.Request, cfg Config) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	target := r.URL.Query().Get("target")
	if target == "" {
		target = cfg.DefaultTarget
	}

	validator := cfg.ValidateTarget
	if validator == nil {
		validator = DefaultValidateTarget
	}
	if err := validator(target); err != nil {
		http.Error(w, fmt.Sprintf("disallowed target: %v", err), http.StatusBadRequest)
		return
	}

	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "missing or invalid Upgrade header", http.StatusBadRequest)
		return
	}
	if !headerContainsToken(r.Header.Get("Connection"), "upgrade") {
		http.Error(w, "missing or invalid Connection header", http.StatusBadRequest)
		return
	}
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		http.Error(w, "unsupported Sec-WebSocket-Version", http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key header", http.StatusBadRequest)
		return
	}

	tcpConn, err := net.DialTimeout("tcp", target, 5*time.Second)
	if err != nil {
		log.Printf("[%s] failed to connect to target %s: %v", r.RemoteAddr, target, err)
		http.Error(w, "failed to connect to target", http.StatusBadGateway)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		_ = tcpConn.Close()
		http.Error(w, "webserver does not support hijacking", http.StatusInternalServerError)
		return
	}

	conn, rw, err := hj.Hijack()
	if err != nil {
		_ = tcpConn.Close()
		log.Printf("[%s] hijack failed: %v", r.RemoteAddr, err)
		return
	}

	acceptVal := computeAcceptKey(key)
	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptVal + "\r\n\r\n"

	if _, err := rw.WriteString(response); err != nil {
		_ = tcpConn.Close()
		_ = conn.Close()
		log.Printf("[%s] handshake write failed: %v", r.RemoteAddr, err)
		return
	}
	if err := rw.Flush(); err != nil {
		_ = tcpConn.Close()
		_ = conn.Close()
		log.Printf("[%s] handshake flush failed: %v", r.RemoteAddr, err)
		return
	}

	ws := &wsConn{
		conn: conn,
		br:   rw.Reader,
	}

	proxyConnection(r.RemoteAddr, target, ws, tcpConn)
}

// NewGatewayHandler creates the HTTP handler routing /healthz and /ws.
func NewGatewayHandler(cfg Config) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(w, r, cfg)
	})
	if cfg.WebDir != "" {
		// http.Dir rejects path traversal; the FileServer serves index.html for "/".
		mux.Handle("/", http.FileServer(http.Dir(cfg.WebDir)))
	}
	return mux
}

func main() {
	listenFlag := flag.String("listen", "127.0.0.1:8090", "Listen address")
	targetFlag := flag.String("target", "127.0.0.1:6414", "Default target address")
	allowRemoteFlag := flag.Bool("allow-remote", false, "Allow listening on non-loopback addresses")
	webFlag := flag.String("web", "", "Optional directory of static files (page, wasm, asset bundles) served at /")
	flag.Parse()

	if !*allowRemoteFlag && !isLoopback(*listenFlag) {
		log.Fatalf("refusing to listen on non-loopback address %s without -allow-remote", *listenFlag)
	}

	if err := DefaultValidateTarget(*targetFlag); err != nil {
		log.Fatalf("invalid default target %s: %v", *targetFlag, err)
	}

	cfg := Config{
		DefaultTarget:  *targetFlag,
		ValidateTarget: DefaultValidateTarget,
		WebDir:         *webFlag,
	}

	handler := NewGatewayHandler(cfg)
	server := &http.Server{
		Addr:    *listenFlag,
		Handler: handler,
	}

	log.Printf("Starting wsgateway on %s (default target: %s)", *listenFlag, *targetFlag)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
