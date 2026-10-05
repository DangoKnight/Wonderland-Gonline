package admin

import (
	"crypto/subtle"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/server"
	"wonderland-go/internal/store"
)

//go:embed web/*
var web embed.FS

type API struct {
	Server    *server.Server
	token     string
	authSlots chan struct{}
}

func New(s *server.Server, token string) (http.Handler, error) {
	if len(token) < 24 {
		return nil, fmt.Errorf("WONDERLAND_ADMIN_TOKEN must contain at least 24 characters")
	}
	a := &API{Server: s, token: token, authSlots: make(chan struct{}, 4)}
	m := http.NewServeMux()
	static, _ := fs.Sub(web, "web")
	m.Handle("GET /", http.FileServer(http.FS(static)))
	m.HandleFunc("GET /register", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, static, "register.html")
	})
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]string{"status": "ok", "parity": "incomplete"})
	})
	m.HandleFunc("POST /register", a.register)
	m.Handle("GET /api/status", a.auth(http.HandlerFunc(a.status)))
	m.Handle("GET /api/sessions", a.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reply(w, 200, s.Sessions()) })))
	m.Handle("POST /api/sessions/{id}/kick", a.auth(http.HandlerFunc(a.kick)))
	m.Handle("GET /api/accounts", a.auth(http.HandlerFunc(a.accounts)))
	m.Handle("POST /api/accounts/{id}/ban", a.auth(http.HandlerFunc(a.ban)))
	m.Handle("DELETE /api/accounts/{id}", a.auth(http.HandlerFunc(a.deleteAccount)))
	m.Handle("POST /api/accounts/{id}/password", a.auth(http.HandlerFunc(a.resetPassword)))
	m.Handle("POST /api/accounts/{id}/mall", a.auth(http.HandlerFunc(a.mallAdjust)))
	m.Handle("POST /api/accounts/{id}/gm", a.auth(http.HandlerFunc(a.gm)))
	m.Handle("GET /api/settings", a.auth(http.HandlerFunc(a.settings)))
	m.Handle("PUT /api/settings", a.auth(http.HandlerFunc(a.settings)))
	m.Handle("GET /api/assets/npcs", a.auth(http.HandlerFunc(a.npcs)))
	m.Handle("GET /api/assets/maps", a.auth(http.HandlerFunc(a.maps)))
	a.bindTools(m)
	m.Handle("GET /api/catalog", a.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reply(w, 200, s.AssetSnapshot().Mall) })))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		m.ServeHTTP(w, r)
	}), nil
}
func (a *API) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(a.token)) != 1 {
			reply(w, 401, map[string]string{"error": "administrator token required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	return decodeSized(w, r, v, 16384)
}
func decodeSized(w http.ResponseWriter, r *http.Request, v any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return e
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("trailing request data")
	}
	return nil
}
func fail(w http.ResponseWriter, status int, msg string) {
	reply(w, status, map[string]string{"error": msg})
}
func (a *API) status(w http.ResponseWriter, r *http.Request) {
	received, unsupported := a.Server.Counters()
	reply(w, 200, map[string]any{
		"name": a.Server.Name(), "uptime_seconds": int(time.Since(a.Server.Started).Seconds()),
		"connections": len(a.Server.Sessions()), "packets_received": received, "unsupported_packets": unsupported,
		"assets": a.Server.AssetSnapshot().Summary(), "parity": "incomplete",
		"services":    map[string]string{"login": a.Server.Config.Login, "world": a.Server.Config.World, "status": a.Server.Config.Status},
		"implemented": []string{"TCP framing, launcher status and handshake", "Account authentication and character roster", "Character creation, starter inventory and deletion codes", "Map synchronization, NPC visibility and portal warps", "Ground item pickup, drop and respawn; bag moves, equipment, native compound synthesis and atomic bag-item repairs", "Map chat, poses and core GM commands", "NPC events, quest marks, journal and starter story", "Native quest minigames and rabbit rewards", "PvE quest and field battles with EXP, levels and loot", "Stat allocation, recovery items, shops, storage and clinics", "Companions, battle pets, capture and Pet Hotel", "Companion mounts and item vehicles with raft wear", "Skill proficiency and evolution", "Teams and team battles", "Atomic player trading", "Item-mall catalogs, balances, atomic purchases and audited point/bonus adjustments", "Configured gacha pack previews and atomic opening", "Persistent AC14 friendships and online/offline presence", "Persisted player preferences and team/trade request controls", "Peer visibility and persisted movement", "SQLite account and character persistence", "Exclusive SQL asset catalog with native offline decoders", "Web accounts, characters, GM studio, operations, asset editors and diagnostics"},
		"pending":     []string{"NPC services, region scripts and story-specific patches", "PvP and trials", "Arcade tickets/prizes and remaining event actions", "Item locks, advanced alchemy and pet rebirth", "Housing and crafting", "Player shops, legacy social handlers, mail and guilds", "Forging and remaining special-item use", "Legacy database migration and MySQL"},
	})
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	select {
	case a.authSlots <- struct{}{}:
		defer func() { <-a.authSlots }()
	default:
		fail(w, 429, "registration is busy")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	if e := decode(w, r, &in); e != nil {
		fail(w, 400, "invalid request")
		return
	}
	if e := store.ValidateCredentials(in.Username, in.Password); e != nil {
		fail(w, 400, e.Error())
		return
	}
	if len(in.Email) > 254 {
		fail(w, 400, "email too long")
		return
	}
	v, e := a.Server.Store.Register(r.Context(), in.Username, in.Password, in.Email)
	if errors.Is(e, store.ErrConflict) {
		fail(w, 409, e.Error())
		return
	}
	if e != nil {
		a.Server.Log.Error("registration failed", "error", e)
		fail(w, 500, "registration failed")
		return
	}
	reply(w, 201, v)
}
func (a *API) accounts(w http.ResponseWriter, r *http.Request) {
	v, e := a.Server.Store.Accounts(r.Context())
	if e != nil {
		fail(w, 500, "could not load accounts")
		return
	}
	reply(w, 200, v)
}
func (a *API) ban(w http.ResponseWriter, r *http.Request) {
	id, e := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if e != nil || id == 0 {
		fail(w, 400, "invalid account ID")
		return
	}
	var in struct {
		Banned bool `json:"banned"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid request")
		return
	}
	if e = a.Server.Store.SetBanned(r.Context(), uint32(id), in.Banned); e != nil {
		fail(w, 400, "could not update account")
		return
	}
	if in.Banned {
		a.Server.KickAccount(uint32(id))
	}
	reply(w, 200, map[string]bool{"ok": true})
}

// gm grants or revokes in-game GM commands; an online session changes immediately.
func (a *API) gm(w http.ResponseWriter, r *http.Request) {
	id, e := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if e != nil || id == 0 {
		fail(w, 400, "invalid account ID")
		return
	}
	var in struct {
		Level byte `json:"gm_level"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid request")
		return
	}
	if e = a.Server.ChangeGMLevel(r.Context(), uint32(id), in.Level); e != nil {
		fail(w, 400, "could not update account")
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}
func (a *API) kick(w http.ResponseWriter, r *http.Request) {
	id, e := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if e != nil || id == 0 {
		fail(w, 400, "invalid session ID")
		return
	}
	if !a.Server.Kick(id) {
		fail(w, 404, "session no longer connected")
		return
	}
	a.Server.Log.Info("administrator terminated session", "session", id)
	reply(w, 200, map[string]bool{"ok": true})
}
func (a *API) settings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		var v map[string]string
		if decode(w, r, &v) != nil {
			fail(w, 400, "invalid request")
			return
		}
		if err := a.Server.UpdateIdentitySettings(r.Context(), v); err != nil {
			fail(w, 400, "invalid settings")
			return
		}
	}
	v, e := a.Server.Store.Settings(r.Context())
	if e != nil {
		fail(w, 500, "could not load settings")
		return
	}
	if v["server_name"] == "" {
		v["server_name"] = a.Server.Name()
	}
	reply(w, 200, v)
}
func (a *API) npcs(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(r.URL.Query().Get("q"))
	out := []assets.NPC{}
	for _, n := range a.Server.AssetSnapshot().NPCs {
		if strings.Contains(strings.ToLower(n.Name), q) || strings.Contains(strconv.Itoa(int(n.ID)), q) {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > 200 {
		out = out[:200]
	}
	reply(w, 200, out)
}
func (a *API) maps(w http.ResponseWriter, r *http.Request) {
	type row struct {
		ID     uint16 `json:"id"`
		Scene  uint16 `json:"scene"`
		NPCs   int    `json:"npcs"`
		Events int    `json:"events"`
		Warps  int    `json:"warps"`
	}
	out := []row{}
	for _, m := range a.Server.AssetSnapshot().Maps {
		out = append(out, row{m.ID, m.Scene, len(m.NPCs), len(m.Events), len(m.Warps)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	reply(w, 200, out)
}

func (a *API) accountChangeResult(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "account not found")
		return
	}
	if err != nil {
		a.Server.Log.Error("account update failed", "error", err)
		fail(w, 500, "could not update account")
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}

func (a *API) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil || id == 0 {
		fail(w, 400, "invalid account ID")
		return
	}
	a.accountChangeResult(w, a.Server.DeleteAccount(r.Context(), uint32(id)))
}

func (a *API) resetPassword(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil || id == 0 {
		fail(w, 400, "invalid account ID")
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if decode(w, r, &in) != nil {
		fail(w, 400, "invalid request")
		return
	}
	if err := store.ValidatePassword(in.Password); err != nil {
		fail(w, 400, err.Error())
		return
	}
	select {
	case a.authSlots <- struct{}{}:
		defer func() { <-a.authSlots }()
	default:
		fail(w, 429, "password reset is busy")
		return
	}
	a.accountChangeResult(w, a.Server.ResetAccountPassword(r.Context(), uint32(id), in.Password))
}
