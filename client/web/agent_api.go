//go:build js && wasm

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"syscall/js"

	"wonderland-gonline/client/wlo/app"
)

// clientRef holds the running client so the agent API can introspect it.
// It is set by main after app.New succeeds and before app.Run starts.
var clientRef *app.Client

// installAgentAPI exposes window.wlo for external automation.
//
//	state()          -> JSON object with current client state
//	command(name, args...) -> enqueue a synthetic input command
//	screenshot()     -> data URL of the Ebiten canvas
//	onStateChange(cb) -> register a callback called when state updates
func installAgentAPI() {
	wlo := js.Global().Get("Object").New()

	wlo.Set("state", js.FuncOf(func(this js.Value, args []js.Value) any {
		return jsValueOf(agentState())
	}))

	wlo.Set("command", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 {
			return "err: missing command name"
		}
		name := args[0].String()
		cmdArgs := make([]any, 0, len(args)-1)
		for i := 1; i < len(args); i++ {
			cmdArgs = append(cmdArgs, jsValueToAny(args[i]))
		}
		if err := agentCommand(name, cmdArgs); err != nil {
			return fmt.Sprintf("err: %v", err)
		}
		return "ok"
	}))

	wlo.Set("screenshot", js.FuncOf(func(this js.Value, args []js.Value) any {
		canvas := js.Global().Get("document").Call("querySelector", "canvas")
		if canvas.IsUndefined() || canvas.IsNull() {
			return "err: canvas not found"
		}
		return canvas.Call("toDataURL", "image/png").String()
	}))

	wlo.Set("onStateChange", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 || args[0].Type() != js.TypeFunction {
			return "err: callback required"
		}
		stateCallback = args[0]
		return "ok"
	}))

	// grid(radius) returns a JSON walkability grid centred on the player.
	wlo.Set("grid", js.FuncOf(func(this js.Value, args []js.Value) any {
		if clientRef == nil {
			return "err: client not ready"
		}
		radius := 15
		if len(args) > 0 {
			radius = args[0].Int()
		}
		if radius < 1 || radius > 100 {
			return "err: radius must be 1..100"
		}
		ox, oy, cols, rows, cells := clientRef.AgentWalkGrid(radius)
		b, _ := json.Marshal(map[string]any{
			"originX": ox, "originY": oy,
			"cols": cols, "rows": rows,
			"cellSize": 20,
			"cells":    encodeBase64(cells),
		})
		return string(b)
	}))

	// mapGrid(step) returns a coarse walkability overview of the whole map.
	wlo.Set("mapGrid", js.FuncOf(func(this js.Value, args []js.Value) any {
		if clientRef == nil {
			return "err: client not ready"
		}
		step := 4
		if len(args) > 0 {
			step = args[0].Int()
		}
		if step < 1 || step > 64 {
			return "err: step must be 1..64"
		}
		cols, rows, cells := clientRef.AgentMapGrid(step)
		b, _ := json.Marshal(map[string]any{
			"cols": cols, "rows": rows,
			"step": step, "cellSize": 20,
			"cells": encodeBase64(cells),
		})
		return string(b)
	}))

	// plan(x, y) returns the client's short-range path as a JSON array.
	wlo.Set("plan", js.FuncOf(func(this js.Value, args []js.Value) any {
		if clientRef == nil {
			return "err: client not ready"
		}
		if len(args) < 2 {
			return "err: plan requires x, y"
		}
		path := clientRef.AgentPlan(args[0].Int(), args[1].Int())
		out := make([]map[string]int, 0, len(path))
		for _, p := range path {
			out = append(out, map[string]int{"x": p.X, "y": p.Y})
		}
		b, _ := json.Marshal(out)
		return string(b)
	}))

	js.Global().Set("wlo", wlo)
}

var stateCallback js.Value

func notifyStateChange() {
	if stateCallback.IsUndefined() || stateCallback.IsNull() {
		return
	}
	stateCallback.Invoke(jsValueOf(agentState()))
}

// agentState returns a snapshot of the running client state.
func agentState() map[string]any {
	c := clientRef
	if c == nil {
		return map[string]any{"status": "initializing"}
	}

	state := map[string]any{
		"status":    "running",
		"screen":    map[string]int{"width": app.ScreenWidth, "height": app.ScreenHeight},
		"inWorld":   c.World != nil,
		"openForms": openForms(c),
	}
	if c.Servers != nil {
		state["serverSelect"] = map[string]any{
			"visible":        c.Servers.Visible,
			"serverCount":    c.Servers.Servers.Count(),
			"serverSelected": c.Servers.Servers.Selected,
			"serverHover":    c.Servers.Servers.HoverRow,
			"regionSelected": c.Servers.Regions.Selected,
		}
	}
	cx, cy := c.AgentCamera()
	state["camera"] = map[string]int{"x": cx, "y": cy}
	items := c.AgentFloorItems()
	floor := make([]map[string]any, 0, len(items))
	for _, it := range items {
		floor = append(floor, map[string]any{
			"slot": it.Slot,
			"id":   it.ID,
			"name": c.AgentItemName(it.ID),
			"x":    it.X,
			"y":    it.Y,
			"sx":   it.X - cx,
			"sy":   it.Y - cy,
		})
	}
	state["floorItems"] = floor
	shown, choosing, options, lines := c.AgentTalk()
	talk := map[string]any{"shown": shown, "choosing": choosing}
	if answers := c.AgentEventAnswers(); len(answers) > 0 {
		talk["answers"] = answers
	}
	if len(options) > 0 {
		talk["options"] = byteStrings(options)
		pts := c.AgentTalkOptionPoints()
		out := make([]map[string]int, 0, len(pts))
		for _, p := range pts {
			out = append(out, map[string]int{"x": p.X, "y": p.Y})
		}
		talk["optionPoints"] = out
	}
	if len(lines) > 0 {
		talk["lines"] = byteStrings(lines)
	}
	state["talk"] = talk
	state["eventActive"] = c.AgentEventActive()
	state["eventDone"] = c.AgentEventDone()
	state["movieActive"] = c.AgentMovieActive()
	state["walking"] = c.AgentWalking()
	if c.Stats != nil {
		state["stats"] = map[string]any{
			"hp": c.Stats.HP, "maxHP": c.Stats.MaxHP,
			"sp": c.Stats.SP, "maxSP": c.Stats.MaxSP,
			"level": c.Stats.Level, "exp": c.Stats.EXP, "gold": c.Stats.Gold,
			"str": c.Stats.STR, "con": c.Stats.CON, "int": c.Stats.INT,
			"wis": c.Stats.WIS, "agi": c.Stats.AGI, "points": c.Stats.Points,
		}
	}
	state["waterTravel"] = c.AgentWaterTravel()
	state["vehicle"] = c.AgentVehicle()
	battle := c.AgentBattleState()
	battleOut := map[string]any{
		"active": battle.Active, "ready": battle.Ready, "finished": battle.Finished,
		"turn": battle.Turn, "self": battle.Self, "side": battle.Side,
		"victorySide": battle.VictorySide, "result": battle.Result,
		"expGained": battle.ExpGained, "levelGained": battle.LevelGained,
		"rewardLines": battle.RewardLines,
	}
	fighters := make([]map[string]any, 0, len(battle.Fighters))
	for _, f := range battle.Fighters {
		fighters = append(fighters, map[string]any{
			"x": f.X, "y": f.Y, "side": f.Side, "kind": f.Kind,
			"id": f.ID, "owner": f.Owner, "hp": f.HP, "maxHP": f.MaxHP,
			"sp": f.SP, "maxSP": f.MaxSP, "level": f.Level,
			"removed": f.Removed, "waiting": f.Waiting, "canChoose": f.CanChoose,
		})
	}
	battleOut["fighters"] = fighters
	state["battle"] = battleOut

	pets := c.AgentPets()
	petOut := make([]map[string]any, 0, len(pets))
	for _, p := range pets {
		petOut = append(petOut, map[string]any{
			"slot": p.Slot, "id": p.ID, "name": p.Name, "level": p.Level,
			"hp": p.HP, "maxHP": p.MaxHP, "sp": p.SP, "maxSP": p.MaxSP,
			"amity": p.Amity, "potential": p.Potential,
		})
	}
	state["pets"] = petOut
	visible, mode, stock, gold := c.AgentShop()
	shop := map[string]any{"visible": visible, "mode": mode, "gold": gold}
	entries := make([]map[string]any, 0, len(stock))
	for _, e := range stock {
		entries = append(entries, map[string]any{
			"slot": e.Slot, "id": e.ID, "name": e.Name, "price": e.Price,
		})
	}
	shop["stock"] = entries
	state["shop"] = shop
	hotel := c.AgentHotel()
	hotelOut := make([]map[string]any, 0, len(hotel))
	for _, p := range hotel {
		hotelOut = append(hotelOut, map[string]any{
			"slot": p.Slot, "id": p.ID, "name": p.Name, "level": p.Level,
			"hp": p.HP, "maxHP": p.MaxHP, "sp": p.SP, "maxSP": p.MaxSP,
			"amity": p.Amity, "potential": p.Potential,
		})
	}
	state["hotel"] = hotelOut
	companions := c.AgentCompanions()
	compOut := make([]map[string]any, 0, len(companions))
	for _, m := range companions {
		compOut = append(compOut, map[string]any{
			"owner": m.Owner, "id": m.ID, "name": m.Name, "x": m.X, "y": m.Y,
		})
	}
	state["companions"] = compOut

	bag := c.AgentBag()
	bagOut := make([]map[string]any, 0, len(bag))
	for _, it := range bag {
		bagOut = append(bagOut, map[string]any{
			"slot": it.Slot, "id": it.ID, "count": it.Count, "name": it.Name,
		})
	}
	state["bag"] = bagOut
	equipment := c.AgentEquipment()
	eqOut := make([]map[string]any, 0, len(equipment))
	for _, it := range equipment {
		eqOut = append(eqOut, map[string]any{
			"slot": it.Slot, "id": it.ID, "count": it.Count, "name": it.Name,
		})
	}
	state["equipment"] = eqOut
	quests := c.AgentQuests()
	questOut := make([]map[string]any, 0, len(quests))
	for _, q := range quests {
		questOut = append(questOut, map[string]any{
			"id": q.ID, "name": q.Name, "description": q.Description,
			"completed": q.Completed,
		})
	}
	state["quests"] = questOut
	if c.World != nil {
		mw, mh := c.AgentMapSize()
		state["mapSize"] = map[string]int{"width": mw, "height": mh}
		areas := c.AgentAreas()
		out := make([]map[string]any, 0, len(areas))
		for _, a := range areas {
			out = append(out, map[string]any{
				"id": a.ID, "x1": a.X1, "y1": a.Y1, "x2": a.X2, "y2": a.Y2,
				"events": a.Events, "sign": a.Sign,
			})
		}
		state["areas"] = out
	}
	if c.Create != nil {
		abs := c.Create.NameField.Abs()
		state["createCharacter"] = map[string]any{
			"visible": c.Create.Visible,
			"step":    c.Create.Step,
			"name":    string(c.Create.NameField.Text),
			"nameRect": map[string]int{
				"x": abs.X, "y": abs.Y,
				"w": c.Create.NameField.Width, "h": c.Create.NameField.Height,
			},
		}
	}

	if w := c.World; w != nil {
		state["map"] = map[string]any{
			"id":   w.Player.Map,
			"name": w.SceneName,
		}
		state["player"] = map[string]any{
			"id":   w.Player.ID,
			"name": string(w.Player.Name),
			"x":    w.Player.X,
			"y":    w.Player.Y,
			"map":  w.Player.Map,
		}
		peers := make([]map[string]any, 0, len(w.Peers))
		for id, p := range w.Peers {
			peers = append(peers, map[string]any{
				"id":   id,
				"x":    p.X,
				"y":    p.Y,
				"name": string(p.Name),
			})
		}
		state["peers"] = peers
		npcs := make([]map[string]any, 0, len(w.NPCs))
		for id, n := range w.NPCs {
			npcs = append(npcs, map[string]any{
				"id":   id,
				"x":    n.X,
				"y":    n.Y,
				"name": n.Info.Name,
			})
		}
		state["npcs"] = npcs
	}

	return state
}

func encodeBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func byteStrings(in [][]byte) []string {
	out := make([]string, 0, len(in))
	for _, b := range in {
		out = append(out, string(b))
	}
	return out
}

// openForms returns a list of visible top-level form names.
func openForms(c *app.Client) []string {
	// This is a placeholder: detecting open forms requires access to the
	// UI manager. We return an empty list until that is wired in.
	_ = c
	return []string{}
}

// agentCommand dispatches a synthetic input event to the Ebiten canvas.
func agentCommand(name string, args []any) error {
	c := clientRef
	if c == nil {
		return fmt.Errorf("client not ready")
	}
	doc := js.Global().Get("document")
	canvas := doc.Call("querySelector", "canvas")
	if canvas.IsUndefined() || canvas.IsNull() {
		return fmt.Errorf("canvas not found")
	}

	switch name {
	case "click":
		if len(args) < 2 {
			return fmt.Errorf("click requires x, y")
		}
		x, ok1 := toInt(args[0])
		y, ok2 := toInt(args[1])
		if !ok1 || !ok2 {
			return fmt.Errorf("click args must be numbers")
		}
		// seui updates Input.Hovered only in the Draw pass, so the press and
		// release must happen in later frames than the move.
		dispatchMouse(canvas, "mousemove", x, y)
		setTimeout(func() {
			dispatchMouse(canvas, "mousedown", x, y)
			setTimeout(func() {
				dispatchMouse(canvas, "mouseup", x, y)
			}, 300)
		}, 300)
	case "rightClick":
		if len(args) < 2 {
			return fmt.Errorf("rightClick requires x, y")
		}
		x, ok1 := toInt(args[0])
		y, ok2 := toInt(args[1])
		if !ok1 || !ok2 {
			return fmt.Errorf("rightClick args must be numbers")
		}
		dispatchMouse(canvas, "mousemove", x, y)
		dispatchMouseButton(canvas, "mousedown", x, y, 2)
		dispatchMouseButton(canvas, "mouseup", x, y, 2)
	case "move":
		if len(args) < 2 {
			return fmt.Errorf("move requires x, y")
		}
		x, ok1 := toInt(args[0])
		y, ok2 := toInt(args[1])
		if !ok1 || !ok2 {
			return fmt.Errorf("move args must be numbers")
		}
		dispatchMouse(canvas, "mousemove", x, y)
	case "key":
		if len(args) < 1 {
			return fmt.Errorf("key requires key name")
		}
		key, ok := args[0].(string)
		if !ok {
			return fmt.Errorf("key arg must be string")
		}
		dispatchKey(canvas, "keydown", key)
		dispatchKey(canvas, "keyup", key)
	case "text":
		if len(args) < 1 {
			return fmt.Errorf("text requires string")
		}
		text, ok := args[0].(string)
		if !ok {
			return fmt.Errorf("text arg must be string")
		}
		for _, r := range text {
			k := string(r)
			dispatchKey(canvas, "keydown", k)
			dispatchKey(canvas, "keypress", k)
			dispatchKey(canvas, "keyup", k)
		}
	case "walkWorld":
		if len(args) < 2 {
			return fmt.Errorf("walkWorld requires x, y")
		}
		x, ok1 := toInt(args[0])
		y, ok2 := toInt(args[1])
		if !ok1 || !ok2 {
			return fmt.Errorf("walkWorld args must be numbers")
		}
		if !c.AgentWalkWorld(x, y) {
			return fmt.Errorf("walkWorld rejected")
		}
	case "interactNPC":
		if len(args) < 1 {
			return fmt.Errorf("interactNPC requires id")
		}
		id, ok := toInt(args[0])
		if !ok {
			return fmt.Errorf("interactNPC id must be a number")
		}
		if !c.AgentInteractNPC(uint16(id)) {
			return fmt.Errorf("interactNPC rejected")
		}
	case "clickID":
		if len(args) < 1 {
			return fmt.Errorf("clickID requires id")
		}
		id, ok := toInt(args[0])
		if !ok {
			return fmt.Errorf("clickID id must be a number")
		}
		if !c.AgentClickID(uint16(id)) {
			return fmt.Errorf("clickID rejected")
		}
	case "pickupFloor":
		if len(args) < 1 {
			return fmt.Errorf("pickupFloor requires slot")
		}
		slot, ok := toInt(args[0])
		if !ok {
			return fmt.Errorf("pickupFloor slot must be a number")
		}
		if !c.AgentPickupFloorSlot(uint16(slot)) {
			return fmt.Errorf("pickupFloor rejected")
		}
	case "boardRaft":
		if len(args) < 2 {
			return fmt.Errorf("boardRaft requires x, y")
		}
		x, ok1 := toInt(args[0])
		y, ok2 := toInt(args[1])
		if !ok1 || !ok2 {
			return fmt.Errorf("boardRaft args must be numbers")
		}
		if !c.AgentBoardRaft(x, y) {
			return fmt.Errorf("boardRaft rejected")
		}
	case "battleAttack":
		if len(args) < 2 {
			return fmt.Errorf("battleAttack requires x, y")
		}
		tx, ok1 := toInt(args[0])
		ty, ok2 := toInt(args[1])
		if !ok1 || !ok2 {
			return fmt.Errorf("battleAttack args must be numbers")
		}
		if !c.AgentBattleAttack(tx, ty) {
			return fmt.Errorf("battleAttack rejected")
		}
	case "battleDefend":
		if !c.AgentBattleDefend() {
			return fmt.Errorf("battleDefend rejected")
		}
	case "battleSkill":
		if len(args) < 3 {
			return fmt.Errorf("battleSkill requires id, x, y")
		}
		id, ok1 := toInt(args[0])
		tx, ok2 := toInt(args[1])
		ty, ok3 := toInt(args[2])
		if !ok1 || !ok2 || !ok3 {
			return fmt.Errorf("battleSkill args must be numbers")
		}
		if !c.AgentBattleSkill(uint16(id), tx, ty) {
			return fmt.Errorf("battleSkill rejected")
		}
	case "battleItem":
		if len(args) < 3 {
			return fmt.Errorf("battleItem requires slot, x, y")
		}
		slot, ok1 := toInt(args[0])
		tx, ok2 := toInt(args[1])
		ty, ok3 := toInt(args[2])
		if !ok1 || !ok2 || !ok3 {
			return fmt.Errorf("battleItem args must be numbers")
		}
		if !c.AgentBattleItem(slot, tx, ty) {
			return fmt.Errorf("battleItem rejected")
		}
	case "battleAuto":
		on := true
		if len(args) > 0 {
			if v, ok := args[0].(bool); ok {
				on = v
			}
		}
		c.AgentBattleAuto(on)
	case "battleFlee":
		if !c.AgentBattleFlee() {
			return fmt.Errorf("battleFlee rejected")
		}
	case "heal":
		if len(args) < 1 {
			return fmt.Errorf("heal requires slot")
		}
		slot, ok := toInt(args[0])
		if !ok {
			return fmt.Errorf("heal slot must be a number")
		}
		if !c.AgentHeal(slot) {
			return fmt.Errorf("heal rejected")
		}
	case "allocate":
		if len(args) < 5 {
			return fmt.Errorf("allocate requires str, con, int, wis, agi")
		}
		vals := make([]int, 5)
		for i := 0; i < 5; i++ {
			v, ok := toInt(args[i])
			if !ok {
				return fmt.Errorf("allocate args must be numbers")
			}
			vals[i] = v
		}
		if !c.AgentAllocate(uint16(vals[0]), uint16(vals[1]), uint16(vals[2]), uint16(vals[3]), uint16(vals[4])) {
			return fmt.Errorf("allocate rejected")
		}
	case "buy":
		if len(args) < 2 {
			return fmt.Errorf("buy requires slot, quantity")
		}
		slot, ok1 := toInt(args[0])
		qty, ok2 := toInt(args[1])
		if !ok1 || !ok2 || slot < 1 || qty < 1 || slot > 255 || qty > 255 {
			return fmt.Errorf("buy args must be numbers in range")
		}
		if !c.AgentBuy(byte(slot), byte(qty)) {
			return fmt.Errorf("buy rejected")
		}
	case "equip":
		if len(args) < 1 {
			return fmt.Errorf("equip requires slot")
		}
		slot, ok := toInt(args[0])
		if !ok || slot < 1 {
			return fmt.Errorf("equip slot must be a number")
		}
		if !c.AgentEquip(slot) {
			return fmt.Errorf("equip rejected")
		}
	case "unequip":
		if len(args) < 1 {
			return fmt.Errorf("unequip requires slot")
		}
		slot, ok := toInt(args[0])
		if !ok || slot < 1 {
			return fmt.Errorf("unequip slot must be a number")
		}
		if !c.AgentUnequip(slot) {
			return fmt.Errorf("unequip rejected")
		}
	case "petDeposit":
		if len(args) < 1 {
			return fmt.Errorf("petDeposit requires slot")
		}
		slot, ok := toInt(args[0])
		if !ok || slot < 1 || slot > 4 {
			return fmt.Errorf("petDeposit slot must be 1..4")
		}
		if !c.AgentPetDeposit(byte(slot)) {
			return fmt.Errorf("petDeposit rejected")
		}
	case "petWithdraw":
		if len(args) < 1 {
			return fmt.Errorf("petWithdraw requires slot")
		}
		slot, ok := toInt(args[0])
		if !ok || slot < 1 {
			return fmt.Errorf("petWithdraw slot must be a number")
		}
		if !c.AgentPetWithdraw(byte(slot)) {
			return fmt.Errorf("petWithdraw rejected")
		}
	case "answer":
		if len(args) < 1 {
			return fmt.Errorf("answer requires index")
		}
		idx, ok := toInt(args[0])
		if !ok {
			return fmt.Errorf("answer index must be a number")
		}
		if !c.AgentAnswer(idx) {
			return fmt.Errorf("answer rejected")
		}
	case "pose":
		if len(args) < 1 {
			return fmt.Errorf("pose requires cell")
		}
		cell, ok := toInt(args[0])
		if !ok {
			return fmt.Errorf("pose cell must be a number")
		}
		if !c.AgentPose(cell) {
			return fmt.Errorf("pose rejected")
		}
	case "petSelect":
		if len(args) < 1 {
			return fmt.Errorf("petSelect requires slot")
		}
		slot, ok := toInt(args[0])
		if !ok {
			return fmt.Errorf("petSelect slot must be a number")
		}
		if !c.AgentPetSelect(slot) {
			return fmt.Errorf("petSelect rejected")
		}
	case "petRest":
		c.AgentPetRest()
	default:
		return fmt.Errorf("unknown command %q", name)
	}
	notifyStateChange()
	return nil
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

// setTimeout runs fn once after roughly ms milliseconds. The callback keeps
// its js.Func alive until it fires.
func setTimeout(fn func(), ms int) {
	var f js.Func
	f = js.FuncOf(func(js.Value, []js.Value) any {
		fn()
		f.Release()
		return nil
	})
	js.Global().Call("setTimeout", f, ms)
}

func dispatchMouse(canvas js.Value, kind string, x, y int) {
	ev := map[string]any{
		"clientX": x,
		"clientY": y,
		"offsetX": x,
		"offsetY": y,
		"screenX": x,
		"screenY": y,
		"bubbles": true,
	}
	canvas.Call("dispatchEvent", js.Global().Get("MouseEvent").New(kind, ev))
}

func dispatchMouseButton(canvas js.Value, kind string, x, y, button int) {
	ev := map[string]any{
		"clientX": x,
		"clientY": y,
		"offsetX": x,
		"offsetY": y,
		"screenX": x,
		"screenY": y,
		"button":  button,
		"buttons": 1,
		"bubbles": true,
	}
	canvas.Call("dispatchEvent", js.Global().Get("MouseEvent").New(kind, ev))
}

func dispatchKey(target js.Value, kind, key string) {
	win := js.Global()
	ev := map[string]any{
		"key":        key,
		"code":       key,
		"bubbles":    true,
		"cancelable": true,
	}
	target.Call("dispatchEvent", win.Get("KeyboardEvent").New(kind, ev))
	win.Call("dispatchEvent", win.Get("KeyboardEvent").New(kind, ev))
}

// jsValueOf converts a Go value to a js.Value using JSON as an intermediate.
func jsValueOf(v any) js.Value {
	b, err := json.Marshal(v)
	if err != nil {
		return js.Global().Get("Object").New()
	}
	return js.Global().Get("JSON").Call("parse", string(b))
}

// jsValueToAny converts a js.Value to a Go any.
func jsValueToAny(v js.Value) any {
	switch v.Type() {
	case js.TypeNumber:
		return v.Float()
	case js.TypeString:
		return v.String()
	case js.TypeBoolean:
		return v.Bool()
	case js.TypeNull, js.TypeUndefined:
		return nil
	default:
		return v.String()
	}
}
