package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"wonderland-go/internal/store"
)

func (a *API) mallAdjust(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil || id == 0 {
		fail(w, http.StatusBadRequest, "invalid account ID")
		return
	}
	var in struct {
		Currency string `json:"currency"`
		Delta    *int64 `json:"delta"`
	}
	if decode(w, r, &in) != nil || in.Delta == nil || (in.Currency != "points" && in.Currency != "bonus") {
		fail(w, http.StatusBadRequest, "provide a currency (points or bonus) and integer delta")
		return
	}
	balances, err := a.Server.AdjustMallBalance(r.Context(), uint32(id), in.Currency == "bonus", *in.Delta)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, http.StatusNotFound, "account not found")
		return
	}
	if errors.Is(err, store.ErrMallAdjustment) {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		a.Server.Log.Error("mall adjustment failed", "error", err)
		fail(w, http.StatusInternalServerError, "could not adjust mall balance")
		return
	}
	reply(w, http.StatusOK, map[string]any{"ok": true, "balances": balances})
}
