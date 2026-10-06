package admin

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/server"
	"wonderland-gonline/internal/store"
)

const assetEditMaxBytes = 2 << 20

func (a *API) bindTools(m *http.ServeMux) {
	bind := func(pattern string, handler http.HandlerFunc) { m.Handle(pattern, a.auth(handler)) }
	bind("GET /api/operations", a.operations)
	bind("PUT /api/operations", a.operations)
	bind("POST /api/operations/broadcast", a.broadcast)
	bind("POST /api/operations/reload", a.reload)
	bind("POST /api/operations/starter-packs", func(w http.ResponseWriter, r *http.Request) {
		rows, err := a.Server.AdminGiveStarterPacks(r.Context())
		if err != nil {
			a.result(w, err)
			return
		}
		reply(w, http.StatusOK, rows)
	})
	bind("POST /api/operations/save", func(w http.ResponseWriter, r *http.Request) { a.result(w, a.Server.AdminSaveAll(r.Context())) })
	bind("POST /api/operations/shutdown", a.shutdown)
	bind("POST /api/operations/kickall", func(w http.ResponseWriter, r *http.Request) { a.Server.AdminKickAll(); a.result(w, nil) })
	bind("GET /api/logs", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, a.Server.Logs()) })
	bind("GET /api/audit", func(w http.ResponseWriter, r *http.Request) {
		rows, err := a.Server.Store.Audit(r.Context())
		if err != nil {
			a.result(w, err)
			return
		}
		reply(w, 200, rows)
	})
	bind("GET /api/characters", a.characters)
	bind("GET /api/characters/{id}", a.character)
	bind("PUT /api/characters/{id}", a.character)
	bind("DELETE /api/characters/{id}", a.character)
	bind("POST /api/players/{id}/action", a.playerAction)
	bind("GET /api/friends", func(w http.ResponseWriter, r *http.Request) {
		rows, err := a.Server.Store.AdminFriends(r.Context())
		if err != nil {
			a.result(w, err)
			return
		}
		reply(w, 200, rows)
	})
	bind("DELETE /api/friends/{a}/{b}", func(w http.ResponseWriter, r *http.Request) {
		x, e := strconv.ParseUint(r.PathValue("a"), 10, 32)
		y, f := strconv.ParseUint(r.PathValue("b"), 10, 32)
		if e != nil || f != nil {
			fail(w, 400, "invalid character IDs")
			return
		}
		a.result(w, a.Server.AdminRemoveFriend(r.Context(), uint32(x), uint32(y)))
	})
	bind("GET /api/battles", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, a.Server.AdminBattles()) })
	bind("POST /api/battles/{id}/abort", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			a.result(w, err)
			return
		}
		a.result(w, a.Server.AdminAbortBattle(id))
	})
	bind("GET /api/bans", a.bans)
	bind("POST /api/bans", a.bans)
	bind("DELETE /api/bans", a.bans)
	bind("GET /api/guilds", a.guilds)
	bind("PUT /api/guilds", a.guilds)
	bind("DELETE /api/guilds/{id}", a.guilds)
	bind("GET /api/marriages", a.marriages)
	bind("DELETE /api/marriages/{id}", a.marriages)
	bind("POST /api/marriages/{id}/summon", a.marriages)
	bind("GET /api/mail", a.mail)
	bind("POST /api/mail", a.mail)
	bind("DELETE /api/mail/{id}", a.mail)
	bind("GET /api/assets/definitions", func(w http.ResponseWriter, r *http.Request) {
		rows, err := a.Server.AssetDocuments(r.Context())
		if err != nil {
			a.result(w, err)
			return
		}
		reply(w, 200, rows)
	})
	bind("GET /api/assets/definitions/{asset}", a.assetDocument)
	bind("PUT /api/assets/definitions/{asset}", a.assetDocument)
	bind("POST /api/assets/definitions/{asset}/initialize", func(w http.ResponseWriter, r *http.Request) {
		a.result(w, a.Server.EnsureAdminAsset(r.Context(), r.PathValue("asset")))
	})
	bind("GET /api/assets/definitions/{asset}/records", a.assetRecords)
	bind("PUT /api/assets/definitions/{asset}/records/{ordinal}", a.assetRecords)

	// Legacy URL aliases carry typed dataset payloads.
	bind("GET /api/assets/documents", func(w http.ResponseWriter, r *http.Request) {
		rows, err := a.Server.AssetDocuments(r.Context())
		if err != nil {
			a.result(w, err)
			return
		}
		reply(w, 200, rows)
	})
	bind("GET /api/assets/documents/{asset}", a.assetDocument)
	bind("PUT /api/assets/documents/{asset}", a.assetDocument)
	bind("POST /api/assets/documents/{asset}/initialize", func(w http.ResponseWriter, r *http.Request) {
		a.result(w, a.Server.EnsureAdminAsset(r.Context(), r.PathValue("asset")))
	})
	bind("GET /api/assets/documents/{asset}/records", a.assetRecords)
	bind("PUT /api/assets/documents/{asset}/records/{ordinal}", a.assetRecords)
	bind("GET /api/assets/maps/{id}", a.mapDetail)
	bind("GET /api/assets/maps/{id}/actors", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil || id > 65535 {
			fail(w, 400, "invalid map ID")
			return
		}
		rows, err := a.Server.AdminMapActors(uint16(id))
		if err != nil {
			a.result(w, err)
			return
		}
		reply(w, 200, rows)
	})
	bind("GET /api/assets/npcs/{id}/report", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			a.result(w, err)
			return
		}
		reply(w, 200, a.Server.AdminNPCReport(id))
	})
	bind("POST /api/characters/{id}/actors", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			a.result(w, err)
			return
		}
		var edit server.AdminActorEdit
		if decode(w, r, &edit) != nil {
			fail(w, 400, "invalid actor edit")
			return
		}
		a.result(w, a.Server.EditAdminActor(r.Context(), id, edit))
	})
	bind("GET /api/assets/talks", a.talks)
	bind("GET /api/configuration", a.configuration)
	bind("PUT /api/configuration", a.configuration)
}
func pathID(r *http.Request) (uint32, error) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if id == 0 && err == nil {
		err = errors.New("positive ID required")
	}
	return uint32(id), err
}
func (a *API) result(w http.ResponseWriter, err error) {
	if err == nil {
		reply(w, 200, map[string]bool{"ok": true})
		return
	}
	status := 400
	if errors.Is(err, store.ErrAdminConflict) || errors.Is(err, assetdb.ErrEditConflict) {
		status = 409
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		status = 408
	}
	a.Server.Log.Warn("administration operation rejected", "error", err)
	fail(w, status, err.Error())
}
func (a *API) operations(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		var v server.RuntimeSettings
		if decode(w, r, &v) != nil {
			fail(w, 400, "invalid runtime settings")
			return
		}
		if err := a.Server.UpdateRuntimeSettings(r.Context(), v); err != nil {
			a.result(w, err)
			return
		}
	}
	reply(w, 200, a.Server.RuntimeSettings())
}
func (a *API) broadcast(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Text    string `json:"text"`
		Channel string `json:"channel"`
	}
	if decode(w, r, &v) != nil {
		fail(w, 400, "invalid announcement")
		return
	}
	a.result(w, a.Server.AdminAnnouncement(v.Text, v.Channel))
}
func (a *API) reload(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Scope string `json:"scope"`
	}
	if decode(w, r, &v) != nil {
		fail(w, 400, "invalid reload category")
		return
	}
	if v.Scope == "" {
		v.Scope = "all"
	}
	a.result(w, a.Server.AdminReload(r.Context(), v.Scope))
}
func (a *API) shutdown(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Seconds int `json:"seconds"`
	}
	if decode(w, r, &v) != nil {
		fail(w, 400, "invalid shutdown delay")
		return
	}
	a.result(w, a.Server.AdminScheduleShutdown(v.Seconds))
}
func (a *API) characters(w http.ResponseWriter, r *http.Request) {
	after, _ := strconv.ParseUint(r.URL.Query().Get("after"), 10, 32)
	rows, err := a.Server.Store.AdminCharacters(r.Context(), r.URL.Query().Get("q"), uint32(after))
	if err != nil {
		a.result(w, err)
		return
	}
	reply(w, 200, rows)
}
func (a *API) character(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		a.result(w, err)
		return
	}
	if r.Method == http.MethodGet {
		row, err := a.Server.Store.AdminCharacter(r.Context(), id)
		if err != nil {
			a.result(w, err)
			return
		}
		reply(w, 200, row)
		return
	}
	var v struct {
		Version string         `json:"version"`
		State   game.Character `json:"state"`
		Scope   string         `json:"scope"`
	}
	if decodeSized(w, r, &v, assetEditMaxBytes) != nil {
		fail(w, 400, "invalid character edit")
		return
	}
	if r.Method == http.MethodDelete {
		a.result(w, a.Server.DeleteAdminCharacter(r.Context(), id, v.Version))
		return
	}
	if v.Scope != "" {
		a.result(w, a.Server.EditAdminCharacterFields(r.Context(), id, v.Version, v.State, v.Scope))
		return
	}
	a.result(w, a.Server.EditAdminCharacter(r.Context(), id, v.Version, v.State))
}
func (a *API) playerAction(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		a.result(w, err)
		return
	}
	var v struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	if decode(w, r, &v) != nil {
		fail(w, 400, "invalid player action")
		return
	}
	messages, err := a.Server.ExecuteAdminPlayerAction(r.Context(), id, v.Command, v.Args)
	if err != nil {
		a.result(w, err)
		return
	}
	reply(w, 200, map[string]any{"ok": true, "messages": messages})
}
func (a *API) bans(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		rows, err := a.Server.Store.IPBans(r.Context())
		if err != nil {
			a.result(w, err)
			return
		}
		reply(w, 200, rows)
		return
	}
	var v struct {
		IP     string `json:"ip"`
		Reason string `json:"reason"`
	}
	if decode(w, r, &v) != nil {
		fail(w, 400, "invalid IP ban")
		return
	}
	a.result(w, a.Server.AdminSetIPBan(r.Context(), v.IP, v.Reason, r.Method != http.MethodDelete))
}
func (a *API) guilds(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		rows, err := a.Server.Store.AdminGuilds(r.Context())
		if err != nil {
			a.result(w, err)
			return
		}
		reply(w, 200, rows)
		return
	}
	if r.Method == http.MethodDelete {
		id, err := pathID(r)
		if err != nil {
			a.result(w, err)
			return
		}
		a.result(w, a.Server.AdminDeleteGuild(r.Context(), id))
		return
	}
	var v store.AdminGuild
	if decode(w, r, &v) != nil {
		fail(w, 400, "invalid guild edit")
		return
	}
	a.result(w, a.Server.AdminSaveGuild(r.Context(), v))
}
func (a *API) marriages(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		rows, err := a.Server.Store.AdminMarriages(r.Context())
		if err != nil {
			a.result(w, err)
			return
		}
		reply(w, 200, rows)
		return
	}
	id, err := pathID(r)
	if err != nil {
		a.result(w, err)
		return
	}
	if r.Method == http.MethodPost {
		a.result(w, a.Server.AdminSummonMarriage(r.Context(), id))
		return
	}
	a.result(w, a.Server.Store.DeleteAdminMarriage(r.Context(), id))
}
func (a *API) mail(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		rows, err := a.Server.Store.AdminMailHistory(r.Context())
		if err != nil {
			a.result(w, err)
			return
		}
		reply(w, 200, rows)
		return
	}
	if r.Method == http.MethodDelete {
		id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
		if err != nil {
			a.result(w, err)
			return
		}
		a.result(w, a.Server.Store.DeleteAdminMail(r.Context(), id))
		return
	}
	var v struct {
		Receivers []uint32        `json:"receivers"`
		Message   store.AdminMail `json:"message"`
	}
	if decode(w, r, &v) != nil {
		fail(w, 400, "invalid GM mail")
		return
	}
	a.result(w, a.Server.AdminDispatchMail(r.Context(), v.Receivers, v.Message))
}
func (a *API) assetDocument(w http.ResponseWriter, r *http.Request) {
	asset := r.PathValue("asset")
	if r.Method == http.MethodGet {
		v, err := a.Server.ReadAssetDocument(r.Context(), asset)
		if err != nil {
			a.result(w, err)
			return
		}
		reply(w, 200, v)
		return
	}
	var edit server.AssetEdit
	if decodeSized(w, r, &edit, assetEditMaxBytes) != nil {
		fail(w, 400, "invalid asset edit")
		return
	}
	a.result(w, a.Server.EditAsset(r.Context(), asset, edit, "", 0))
}
func (a *API) assetRecords(w http.ResponseWriter, r *http.Request) {
	asset := r.PathValue("asset")
	if r.Method == http.MethodGet {
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil {
			a.result(w, err)
			return
		}
		rows, err := a.Server.ReadAssetRecords(r.Context(), asset, id)
		if err != nil {
			a.result(w, err)
			return
		}
		reply(w, 200, rows)
		return
	}
	ordinal, err := strconv.Atoi(r.PathValue("ordinal"))
	if err != nil || ordinal < 0 || r.URL.Query().Get("collection") == "" {
		fail(w, 400, "invalid asset collection or ordinal")
		return
	}
	var edit server.AssetEdit
	if decodeSized(w, r, &edit, assetEditMaxBytes) != nil {
		fail(w, 400, "invalid asset record")
		return
	}
	a.result(w, a.Server.EditAsset(r.Context(), asset, edit, r.URL.Query().Get("collection"), ordinal))
}
func (a *API) mapDetail(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil || id > 65535 {
		fail(w, 400, "invalid map ID")
		return
	}
	m, ok := a.Server.AssetSnapshot().Maps[uint16(id)]
	if !ok {
		fail(w, 404, "map not found")
		return
	}
	reply(w, 200, m)
}
func (a *API) talks(w http.ResponseWriter, r *http.Request) {
	catalog := a.Server.AssetSnapshot()
	query := r.URL.Query().Get("q")
	type row struct {
		ID     uint32 `json:"id"`
		Text   string `json:"text"`
		Method string `json:"method"`
	}
	out := []row{}
	if id, err := strconv.ParseUint(query, 10, 32); err == nil {
		for method, values := range map[string]map[uint32]string{"id": catalog.Talks.ByID, "offset": catalog.Talks.ByOffset, "index": catalog.Talks.ByIndex} {
			if text, ok := values[uint32(id)]; ok {
				out = append(out, row{uint32(id), strings.ReplaceAll(text, "#n", r.URL.Query().Get("name")), method})
			}
		}
	} else {
		ids := []uint32{}
		for id, text := range catalog.Talks.ByID {
			if query == "" || strings.Contains(strings.ToLower(text), strings.ToLower(query)) {
				ids = append(ids, id)
			}
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids[:min(len(ids), store.AdminPageLimit)] {
			out = append(out, row{id, catalog.Talks.ByID[id], "id"})
		}
	}
	reply(w, 200, out)
}
func (a *API) configuration(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		var in struct {
			Version       string        `json:"version"`
			Configuration config.Config `json:"configuration"`
		}
		if decode(w, r, &in) != nil {
			fail(w, 400, "invalid startup configuration")
			return
		}
		if err := a.Server.SaveStartupConfiguration(in.Version, in.Configuration); err != nil {
			a.result(w, err)
			return
		}
	}
	value, err := a.Server.StartupConfiguration()
	if err != nil {
		a.result(w, err)
		return
	}
	reply(w, 200, value)
}
