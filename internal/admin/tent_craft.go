package admin

import (
	"net/http"
	"sort"

	"wonderland-gonline/internal/assets"
)

type tentCraftRow struct {
	FormulaID uint16 `json:"formula_id"`
	ItemID    uint16 `json:"item_id"`
	Name      string `json:"name"`
	Duration  uint32 `json:"duration_seconds"`
	Tent      bool   `json:"tent_output"`
}

// tentCraft lists the manufacturing catalog and rewrites recipe durations.
func (a *API) tentCraft(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		c := a.Server.AssetSnapshot()
		rows := make([]tentCraftRow, 0, len(c.Manufacturing))
		for id, formula := range c.Manufacturing {
			rows = append(rows, tentCraftRow{
				FormulaID: id,
				ItemID:    formula.Output.ItemID,
				Name:      c.Items[formula.Output.ItemID].Name,
				Duration:  formula.DurationSeconds,
				Tent:      formula.TentOutput,
			})
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].Tent != rows[j].Tent {
				return rows[i].Tent
			}
			return rows[i].FormulaID < rows[j].FormulaID
		})
		reply(w, 200, map[string]any{"formulas": rows, "max_duration_seconds": assets.ManufacturingMaxDurationSeconds})
	case http.MethodPost:
		var v struct {
			All      bool     `json:"all"`
			Reset    bool     `json:"reset"`
			IDs      []uint16 `json:"ids"`
			Duration uint32   `json:"duration_seconds"`
		}
		if decode(w, r, &v) != nil {
			fail(w, 400, "invalid tent craft update")
			return
		}
		if v.Reset {
			updated, err := a.Server.ResetManufactureDurations(r.Context())
			if err != nil {
				a.result(w, err)
				return
			}
			reply(w, 200, map[string]any{"updated": updated})
			return
		}
		if !v.All && len(v.IDs) == 0 {
			fail(w, 400, "select formulas or request all")
			return
		}
		updated, err := a.Server.SetManufactureDurations(r.Context(), v.IDs, v.All, v.Duration)
		if err != nil {
			a.result(w, err)
			return
		}
		reply(w, 200, map[string]any{"updated": updated})
	default:
		fail(w, 405, "method not allowed")
	}
}
