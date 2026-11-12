// Command wlo-agent is a headless Chromium orchestrator for the Wonderland
// Gonline web client. It exposes a JSON-RPC/ WebSocket API so an external
// AI/test harness can create agents, query their state, send commands and
// capture screenshots.
package main

import (
	"context"
	"crypto/subtle"
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

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/gorilla/websocket"

	"wonderland-gonline/internal/wsgateway"
)

//go:embed host/*
var hostFS embed.FS

var (
	enableAgent     = flag.Bool("enable", false, "explicitly enable the optional headless automation and web endpoints")
	listenAddr      = flag.String("listen", "127.0.0.1:9000", "agent RPC listen address")
	chromePath      = flag.String("chrome", "", "path to Chrome/Chromium binary (autodetect if empty)")
	assetsDir       = flag.String("assets", "", "directory with wonderland-client.wasm and asset packs")
	gameTarget      = flag.String("game", "127.0.0.1:6414", "game server address for the WebSocket gateway")
	noHeadless      = flag.Bool("no-headless", false, "show Chrome window for debugging")
	diagnosticInput = flag.Bool("diagnostic-input", false, "allow legacy commands and arbitrary JavaScript evaluation for diagnostics")
	browserLog      = flag.Bool("browser-log", false, "log browser console output for diagnostics; may include gameplay text")
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
	viewers     viewerRegistry
	mu          sync.RWMutex
	agents      map[string]*Agent
	chrome      string
	assets      string
	headless    bool
	diagnostics bool
}

func NewManager(chrome, assets string, headless bool) *Manager {
	return &Manager{
		agents:      make(map[string]*Agent),
		chrome:      chrome,
		assets:      assets,
		headless:    headless,
		diagnostics: *diagnosticInput,
	}
}

func (m *Manager) Create(account, password string) (*Agent, error) {
	if account != "" || password != "" {
		return nil, errors.New("create without credentials; use the structured login.submit command after server selection")
	}
	id := fmt.Sprintf("agent-%d", time.Now().UnixNano())

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Headless,
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
	if *browserLog {
		_ = chromedp.Listen(ctx, func(event any) {
			if message, ok := event.(*runtime.EventConsoleAPICalled); ok {
				for _, arg := range message.Args {
					if arg.Type == runtime.RemoteObjectTypeString {
						var line string
						if json.Unmarshal(arg.Value, &line) == nil {
							log.Printf("browser %s: %s", id, line)
						}
					}
				}
			}
		})
	}

	a := &Agent{
		ID:        id,
		CreatedAt: time.Now(),
		cancel:    func() { cancel(); allocCancel() },
		allocCtx:  allocCtx,
		ctx:       ctx,
	}

	pageURL := "http://" + net.JoinHostPort("127.0.0.1", assetsPort) + "/?sessionHost=1"

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

	// Poll state in the background.
	go m.pollState(a)

	return a, nil
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
	m.viewers.mu.Lock()
	for token, ticket := range m.viewers.tickets {
		if ticket.Agent == id {
			delete(m.viewers.tickets, token)
		}
	}
	m.viewers.mu.Unlock()
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
		out, err := chromedp.Run(a.ctx, chromedp.Evaluate[string](script))
		if err != nil {
			return err
		}
		return commandResponse(out)
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
		out, err := chromedp.Run(a.ctx, chromedp.Evaluate[string](script))
		if err != nil {
			return err
		}
		return commandResponse(out)
	}
}

// Older bridges return a string rather than a structured execution ticket.
func commandResponse(out string) error {
	if out == "ok" {
		return nil
	}
	if strings.HasPrefix(out, "err: ") {
		return errors.New(strings.TrimPrefix(out, "err: "))
	}
	return errors.New("client command API unavailable")
}

// StructuredAPI calls only fixed bridge methods; request values are JSON data.
func (m *Manager) StructuredAPI(id, method string, argument json.RawMessage) (json.RawMessage, error) {
	a, ok := m.Get(id)
	if !ok {
		return nil, errors.New("agent not found")
	}
	return m.structuredAPI(a.ctx, id, method, argument)
}
func (m *Manager) structuredAPI(parent context.Context, id, method string, argument json.RawMessage) (json.RawMessage, error) {
	a, ok := m.Get(id)
	if !ok {
		return nil, errors.New("agent not found")
	}
	switch method {
	case "contract":
		argument = nil
	case "execute", "cancel", "commands", "state", "lookup", "navigation", "hostControl", "viewerFrame", "nativeSync":
		if !json.Valid(argument) {
			return nil, errors.New("invalid API argument")
		}
	default:
		return nil, errors.New("unknown API method")
	}
	_ = a
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	argumentJS := ""
	if argument != nil {
		encoded, _ := json.Marshal(string(argument))
		argumentJS = "JSON.parse(" + string(encoded) + ")"
	}
	script := fmt.Sprintf("JSON.stringify(window.wlo && window.wlo.%s ? window.wlo.%s(%s) : {error:'client API unavailable'})", method, method, argumentJS)
	out, err := chromedp.Run(ctx, chromedp.Evaluate[string](script))
	if err != nil {
		return nil, errors.New("client API evaluation failed")
	}
	return structuredResponse([]byte(out))
}

// A successful CDP evaluation can still contain a rejected client operation.
// Promote bridge admission/query errors to RPC errors; executed ticket outcomes
// remain in their owning session's command history.
func structuredResponse(raw []byte) (json.RawMessage, error) {
	if !json.Valid(raw) {
		return nil, errors.New("invalid client API response")
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) == nil {
		if value, ok := object["error"]; ok && len(object) == 1 {
			var message string
			if json.Unmarshal(value, &message) == nil && message != "" {
				return nil, errors.New(message)
			}
		}
	}
	return json.RawMessage(raw), nil
}

var assetsPort string

func assetHandler(assets, gameTarget string) (http.Handler, error) {
	mux := http.NewServeMux()

	// Static host files from embed.
	hostSub, err := fs.Sub(hostFS, "host")
	if err != nil {
		return nil, err
	}
	mux.Handle("/", http.FileServer(http.FS(hostSub)))

	packs, err := assetPackNames(assets)
	if err != nil {
		return nil, err
	}
	mux.HandleFunc("/packs.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(packs)
	})
	mux.HandleFunc("/wonderland-client.wasm", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(assets, "wonderland-client.wasm"))
	})
	allowed := make(map[string]bool, len(packs))
	for _, name := range packs {
		allowed["/assets/"+name] = true
	}
	mux.HandleFunc("/assets/", func(w http.ResponseWriter, r *http.Request) {
		if !allowed[r.URL.Path] {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(assets, filepath.Base(r.URL.Path)))
	})

	// WebSocket gateway for the game server, matching /ws?target=host:port.
	gw := wsgateway.NewGatewayHandler(wsgateway.Config{
		DefaultTarget:  gameTarget,
		ValidateTarget: wsgateway.DefaultValidateTarget,
	})
	mux.Handle("/ws", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target, err := agentGameTarget(r.URL.Query().Get("target"), gameTarget)
		if err != nil {
			http.Error(w, "game endpoint unavailable", http.StatusBadRequest)
			return
		}
		copyURL := *r.URL
		query := copyURL.Query()
		query.Set("target", target)
		copyURL.RawQuery = query.Encode()
		copyRequest := r.Clone(r.Context())
		copyRequest.URL = &copyURL
		gw.ServeHTTP(w, copyRequest)
	}))

	return mux, nil
}

func startAssetsServer(assets, gameTarget string) (string, error) {
	mux, err := assetHandler(assets, gameTarget)
	if err != nil {
		return "", err
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	port := fmt.Sprintf("%d", ln.Addr().(*net.TCPAddr).Port)
	go func() {
		// Reject DNS rebinding and cross-origin access to the local game gateway.
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Host != ln.Addr().String() || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+r.Host) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			mux.ServeHTTP(w, r)
		})
		if err := http.Serve(ln, handler); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("assets server: %v", err)
		}
	}()
	return port, nil
}

func findChrome() string {
	candidates := []string{
		filepath.Join(os.Getenv("PROGRAMFILES"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("PROGRAMFILES(X86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
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

const maxRPCRequestBytes = 1 << 20
const minRPCTokenBytes = 32

// The default upgrader rejects cross-origin browser connections. Non-browser
// callers omit Origin and authenticate with the Authorization header.
var upgrader = websocket.Upgrader{}

func rpcHandler(mgr *Manager, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		supplied := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || len(token) < minRPCTokenBytes || subtle.ConstantTimeCompare([]byte(supplied), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		handleRPC(mgr, w, r)
	})
}

func validateRPCListen(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("agent RPC must bind to a numeric loopback address")
	}
	return nil
}

func handleRPC(mgr *Manager, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade: %v", err)
		return
	}
	defer conn.Close()
	conn.SetReadLimit(maxRPCRequestBytes)

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
	if err := conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
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
	case "agent.viewer.open":
		var p struct {
			ID      string `json:"id"`
			Session int    `json:"session"`
		}
		if json.Unmarshal(req.Params, &p) != nil {
			resp.Error = newError("invalid params", -32602)
			return resp
		}
		out, err := mgr.openViewer(p.ID, p.Session)
		if err != nil {
			resp.Error = newError(err.Error(), 3)
			return resp
		}
		resp.Result = out
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
			ID      string `json:"id"`
			Session int    `json:"session"`
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
		_ = a
		argument, _ := json.Marshal(p.Session)
		out, err := mgr.StructuredAPI(p.ID, "state", argument)
		if err != nil {
			resp.Error = newError(err.Error(), 3)
			return resp
		}
		resp.Result = out
	case "agent.observe":
		var p observationRequest
		if json.Unmarshal(req.Params, &p) != nil {
			resp.Error = newError("invalid params", -32602)
			return resp
		}
		out, err := mgr.Observe(p)
		if err != nil {
			resp.Error = newError(err.Error(), 3)
			return resp
		}
		resp.Result = out
	case "agent.navigation":
		var p struct {
			ID      string          `json:"id"`
			Request json.RawMessage `json:"request"`
		}
		if json.Unmarshal(req.Params, &p) != nil || p.ID == "" || !json.Valid(p.Request) {
			resp.Error = newError("invalid navigation params", -32602)
			return resp
		}
		out, err := mgr.StructuredAPI(p.ID, "navigation", p.Request)
		if err != nil {
			resp.Error = newError(err.Error(), 3)
			return resp
		}
		resp.Result = out
	case "agent.lookup":
		var p struct {
			ID      string `json:"id"`
			Kind    string `json:"kind"`
			Entry   uint32 `json:"entry"`
			Session int    `json:"session"`
		}
		if json.Unmarshal(req.Params, &p) != nil || p.ID == "" || p.Session < 0 {
			resp.Error = newError("invalid lookup params", -32602)
			return resp
		}
		argument, _ := json.Marshal(map[string]any{"kind": p.Kind, "id": p.Entry, "session": p.Session})
		out, err := mgr.StructuredAPI(p.ID, "lookup", argument)
		if err != nil {
			resp.Error = newError(err.Error(), 3)
			return resp
		}
		resp.Result = out
	case "agent.list":
		out := []map[string]any{}
		mgr.mu.RLock()
		for _, a := range mgr.agents {
			out = append(out, map[string]any{"id": a.ID, "createdAt": a.CreatedAt})
		}
		mgr.mu.RUnlock()
		resp.Result = out
	case "agent.contract", "agent.execute", "agent.commands", "agent.cancel":
		var p struct {
			ID      string          `json:"id"`
			Request json.RawMessage `json:"request"`
			Ticket  string          `json:"ticket"`
			Session int             `json:"session"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.ID == "" {
			resp.Error = newError("invalid params", -32602)
			return resp
		}
		method := strings.TrimPrefix(req.Method, "agent.")
		argument := p.Request
		if method == "cancel" {
			argument, _ = json.Marshal(map[string]any{"ticket": p.Ticket, "session": p.Session})
		}
		if method == "commands" {
			argument, _ = json.Marshal(p.Session)
		}
		out, err := mgr.StructuredAPI(p.ID, method, argument)
		if err != nil {
			resp.Error = newError(err.Error(), 3)
			return resp
		}
		resp.Result = out
	case "agent.command":
		if !mgr.diagnostics {
			resp.Error = newError("input diagnostics are disabled; use agent.execute", 3)
			return resp
		}
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
		if !mgr.diagnostics {
			resp.Error = newError("JavaScript diagnostics are disabled", 3)
			return resp
		}
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
	if !*enableAgent {
		log.Fatal("headless agent is disabled; use -enable to opt in")
	}
	token := os.Getenv("WLO_AGENT_TOKEN")
	if len(token) < minRPCTokenBytes {
		log.Fatal("set WLO_AGENT_TOKEN to a random secret of at least 32 bytes")
	}
	if err := validateRPCListen(*listenAddr); err != nil {
		log.Fatal(err)
	}

	if *assetsDir == "" {
		*assetsDir = "bin"
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
	mux.Handle("/rpc", rpcHandler(mgr, token))
	mgr.installViewer(mux)

	log.Printf("wlo-agent listening on ws://%s/rpc", *listenAddr)
	if err := http.ListenAndServe(*listenAddr, mux); err != nil {
		log.Fatal(err)
	}
}
