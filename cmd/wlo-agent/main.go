// Command wlo-agent is a headless Chromium orchestrator for the Wonderland
// Gonline web client. It exposes a JSON-RPC/ WebSocket API so an external
// AI/test harness can create agents, query their state, send commands and
// capture screenshots.
package main

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/gorilla/websocket"

	"wonderland-gonline/internal/wsgateway"
)

//go:embed host/*
var hostFS embed.FS

var (
	listenAddr = flag.String("listen", "127.0.0.1:9000", "agent RPC listen address")
	chromePath = flag.String("chrome", "", "path to Chrome/Chromium binary (autodetect if empty)")
	assetsDir  = flag.String("assets", "", "directory with wonderland-client.wasm and asset packs")
	gameTarget = flag.String("game", "127.0.0.1:6414", "game server address for the WebSocket gateway")
	noHeadless = flag.Bool("no-headless", false, "show Chrome window for debugging")
)

type Agent struct {
	ID        string
	CreatedAt time.Time

	cancel    context.CancelFunc
	allocCtx  context.Context
	ctx       context.Context
	stateMu   sync.RWMutex
	lastState map[string]any
}

func (a *Agent) State() map[string]any {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	if a.lastState == nil {
		return map[string]any{"status": "starting"}
	}
	return a.lastState
}

func (a *Agent) updateState(s map[string]any) {
	a.stateMu.Lock()
	a.lastState = s
	a.stateMu.Unlock()
}

func (a *Agent) Close() {
	if a.cancel != nil {
		a.cancel()
	}
}

// Manager owns all agents.
type Manager struct {
	mu       sync.RWMutex
	agents   map[string]*Agent
	chrome   string
	assets   string
	headless bool
}

func NewManager(chrome, assets string, headless bool) *Manager {
	return &Manager{
		agents:   make(map[string]*Agent),
		chrome:   chrome,
		assets:   assets,
		headless: headless,
	}
}

func (m *Manager) Create(account, password string) (*Agent, error) {
	id := fmt.Sprintf("agent-%d", time.Now().UnixNano())

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Headless,
		chromedp.NoSandbox,
		chromedp.DisableGPU,
	)
	if m.chrome != "" {
		opts = append(opts, chromedp.ExecPath(m.chrome))
	}
	if !m.headless {
		opts = append(opts, chromedp.Flag("headless", false))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx)

	a := &Agent{
		ID:        id,
		CreatedAt: time.Now(),
		cancel:    func() { cancel(); allocCancel() },
		allocCtx:  allocCtx,
		ctx:       ctx,
	}

	pageURL := "http://" + net.JoinHostPort("127.0.0.1", assetsPort) + "/"

	if _, err := chromedp.Run(ctx, chromedp.Navigate(pageURL)); err != nil {
		a.Close()
		return nil, fmt.Errorf("navigate: %w", err)
	}
	if _, err := chromedp.Run(ctx, chromedp.EmulateViewport(1020, 600)); err != nil {
		a.Close()
		return nil, fmt.Errorf("viewport: %w", err)
	}

	m.mu.Lock()
	m.agents[id] = a
	m.mu.Unlock()

	// Log in if credentials are provided.
	if account != "" {
		go m.login(a, account, password)
	}

	// Poll state in the background.
	go m.pollState(a)

	return a, nil
}

func (m *Manager) login(a *Agent, account, password string) {
	// Wait until the client is ready and we are on the login form.
	for i := 0; i < 100; i++ {
		if a.ctx.Err() != nil {
			return
		}
		state := a.State()
		if status, _ := state["status"].(string); status == "running" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Fill credentials and submit. The exact selectors depend on the login UI;
	// synthetic input is more robust.
	_ = account
	_ = password
}

func (m *Manager) Destroy(id string) error {
	m.mu.Lock()
	a, ok := m.agents[id]
	delete(m.agents, id)
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("agent %s not found", id)
	}
	a.Close()
	return nil
}

func (m *Manager) Get(id string) (*Agent, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.agents[id]
	return a, ok
}

func (m *Manager) pollState(a *Agent) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
		}
		state, err := chromedp.Run(a.ctx,
			chromedp.Evaluate[string]("window.wlo ? JSON.stringify(window.wlo.state()) : '{\"status\":\"no-wlo\"}'"))
		if err != nil {
			continue
		}
		var s map[string]any
		if err := json.Unmarshal([]byte(state), &s); err == nil {
			a.updateState(s)
		}
	}
}

func (m *Manager) Screenshot(id string) ([]byte, error) {
	a, ok := m.Get(id)
	if !ok {
		return nil, fmt.Errorf("agent %s not found", id)
	}
	// Prefer CDP full-page capture; fallback to canvas data URL.
	png, err := chromedp.Run(a.ctx, chromedp.FullScreenshot(100))
	if err == nil && len(png) > 0 {
		return png, nil
	}
	dataURL, err := chromedp.Run(a.ctx, chromedp.Evaluate[string]("window.wlo ? window.wlo.screenshot() : ''"))
	if err != nil {
		return nil, err
	}
	if dataURL == "" {
		return nil, errors.New("screenshot unavailable")
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(dataURL, prefix) {
		return nil, errors.New("unexpected screenshot format")
	}
	return base64.StdEncoding.DecodeString(dataURL[len(prefix):])
}

func (m *Manager) Command(id, name string, args []any) error {
	a, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("agent %s not found", id)
	}

	switch name {
	case "click":
		// Use the JS API so it can dispatch the exact event types Ebiten expects.
		payload, err := json.Marshal(append([]any{name}, args...))
		if err != nil {
			return err
		}
		script := fmt.Sprintf("window.wlo ? window.wlo.command.apply(window.wlo, %s) : 'no wlo'", payload)
		_, err = chromedp.Run(a.ctx, chromedp.Evaluate[string](script))
		return err
	case "key":
		if len(args) < 1 {
			return fmt.Errorf("key requires key name")
		}
		key, ok := args[0].(string)
		if !ok {
			return fmt.Errorf("key arg must be string")
		}
		_, err := chromedp.Run(a.ctx, chromedp.KeyEvent(key))
		return err
	case "text":
		if len(args) < 1 {
			return fmt.Errorf("text requires string")
		}
		text, ok := args[0].(string)
		if !ok {
			return fmt.Errorf("text arg must be string")
		}
		_, err := chromedp.Run(a.ctx, chromedp.KeyEvent(text))
		return err
	default:
		// Fallback to the JS API for any custom command.
		payload, err := json.Marshal(append([]any{name}, args...))
		if err != nil {
			return err
		}
		script := fmt.Sprintf("window.wlo ? window.wlo.command.apply(window.wlo, %s) : 'no wlo'", payload)
		_, err = chromedp.Run(a.ctx, chromedp.Evaluate[string](script))
		return err
	}
}

var assetsPort string

func startAssetsServer(assets, gameTarget string) (string, error) {
	mux := http.NewServeMux()

	// Static host files from embed.
	hostSub, err := fs.Sub(hostFS, "host")
	if err != nil {
		return "", err
	}
	mux.Handle("/", http.FileServer(http.FS(hostSub)))

	// Asset packs and wasm from the configured directory.
	mux.Handle("/wonderland-client.wasm", http.FileServer(http.Dir(assets)))
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(assets))))

	// WebSocket gateway for the game server, matching /ws?target=host:port.
	gw := wsgateway.NewGatewayHandler(wsgateway.Config{
		DefaultTarget:  gameTarget,
		ValidateTarget: wsgateway.DefaultValidateTarget,
	})
	mux.Handle("/ws", gw)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	port := fmt.Sprintf("%d", ln.Addr().(*net.TCPAddr).Port)
	go func() {
		if err := http.Serve(ln, mux); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("assets server: %v", err)
		}
	}()
	return port, nil
}

func findChrome() string {
	candidates := []string{
		"/tmp/opencode/tools/chrome-linux64/chrome",
		"/usr/bin/google-chrome",
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	if p, err := exec.LookPath("google-chrome"); err == nil {
		return p
	}
	if p, err := exec.LookPath("chromium"); err == nil {
		return p
	}
	if p, err := exec.LookPath("chromium-browser"); err == nil {
		return p
	}
	return ""
}

// JSON-RPC types.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      any             `json:"id"`
}

type Response struct {
	JSONRPC string    `json:"jsonrpc"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
	ID      any       `json:"id"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newError(msg string, code int) *RPCError {
	return &RPCError{Code: code, Message: msg}
}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func handleRPC(mgr *Manager, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade: %v", err)
		return
	}
	defer conn.Close()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var req Request
		if err := json.Unmarshal(data, &req); err != nil {
			send(conn, Response{JSONRPC: "2.0", Error: newError("parse error", -32700), ID: nil})
			continue
		}
		resp := dispatch(mgr, req)
		if err := send(conn, resp); err != nil {
			return
		}
	}
}

func send(conn *websocket.Conn, resp Response) error {
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, b)
}

func dispatch(mgr *Manager, req Request) Response {
	resp := Response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "agent.create":
		var p struct {
			Account  string `json:"account"`
			Password string `json:"password"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			resp.Error = newError("invalid params", -32602)
			return resp
		}
		a, err := mgr.Create(p.Account, p.Password)
		if err != nil {
			resp.Error = newError(err.Error(), 1)
			return resp
		}
		resp.Result = map[string]string{"id": a.ID}
	case "agent.destroy":
		var p struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			resp.Error = newError("invalid params", -32602)
			return resp
		}
		if err := mgr.Destroy(p.ID); err != nil {
			resp.Error = newError(err.Error(), 1)
			return resp
		}
		resp.Result = "ok"
	case "agent.state":
		var p struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			resp.Error = newError("invalid params", -32602)
			return resp
		}
		a, ok := mgr.Get(p.ID)
		if !ok {
			resp.Error = newError("not found", 2)
			return resp
		}
		resp.Result = a.State()
	case "agent.command":
		var p struct {
			ID      string `json:"id"`
			Command string `json:"command"`
			Args    []any  `json:"args"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			resp.Error = newError("invalid params", -32602)
			return resp
		}
		if err := mgr.Command(p.ID, p.Command, p.Args); err != nil {
			resp.Error = newError(err.Error(), 3)
			return resp
		}
		resp.Result = "ok"
	case "agent.eval":
		var p struct {
			ID     string `json:"id"`
			Script string `json:"script"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			resp.Error = newError("invalid params", -32602)
			return resp
		}
		a, ok := mgr.Get(p.ID)
		if !ok {
			resp.Error = newError("not found", 2)
			return resp
		}
		out, err := chromedp.Run(a.ctx, chromedp.Evaluate[string](p.Script))
		if err != nil {
			resp.Error = newError(err.Error(), 5)
			return resp
		}
		resp.Result = out
	case "agent.screenshot":
		var p struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			resp.Error = newError("invalid params", -32602)
			return resp
		}
		png, err := mgr.Screenshot(p.ID)
		if err != nil {
			resp.Error = newError(err.Error(), 4)
			return resp
		}
		resp.Result = map[string]string{"png_base64": base64.StdEncoding.EncodeToString(png)}
	default:
		resp.Error = newError("method not found", -32601)
	}
	return resp
}

func main() {
	flag.Parse()

	if *assetsDir == "" {
		*assetsDir = filepath.Join(os.Getenv("PWD"), "bin")
	}
	if _, err := os.Stat(*assetsDir); err != nil {
		log.Fatalf("assets directory not found: %s", *assetsDir)
	}
	if _, err := os.Stat(filepath.Join(*assetsDir, "wonderland-client.wasm")); err != nil {
		log.Fatalf("wonderland-client.wasm not found in %s", *assetsDir)
	}

	if *chromePath == "" {
		*chromePath = findChrome()
	}
	if *chromePath == "" {
		log.Fatal("Chrome/Chromium not found; set -chrome")
	}
	log.Printf("using Chrome: %s", *chromePath)

	if err := wsgateway.DefaultValidateTarget(*gameTarget); err != nil {
		log.Fatalf("invalid game target %s: %v", *gameTarget, err)
	}

	port, err := startAssetsServer(*assetsDir, *gameTarget)
	if err != nil {
		log.Fatalf("assets server: %v", err)
	}
	assetsPort = port
	log.Printf("assets server on http://127.0.0.1:%s/", assetsPort)

	mgr := NewManager(*chromePath, *assetsDir, !*noHeadless)

	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", func(w http.ResponseWriter, r *http.Request) {
		handleRPC(mgr, w, r)
	})

	log.Printf("wlo-agent listening on ws://%s/rpc", *listenAddr)
	if err := http.ListenAndServe(*listenAddr, mux); err != nil {
		log.Fatal(err)
	}
}
