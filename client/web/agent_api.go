//go:build js && wasm

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"syscall/js"

	"wonderland-gonline/client/wlo/app"
	"wonderland-gonline/client/wlo/seui"
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
	wlo.Set("nativeSync", js.FuncOf(func(this js.Value, args []js.Value) any {
		var p struct {
			Session int    `json:"session"`
			After   uint64 `json:"after"`
			Epoch   string `json:"epoch"`
		}
		if clientRef == nil || len(args) != 1 || seui.DecodeAutomation([]byte(js.Global().Get("JSON").Call("stringify", args[0]).String()), &p) != nil {
			return jsValueOf(map[string]any{"error": "invalid native sync request"})
		}
		c := clientRef.AgentSession(p.Session)
		if c == nil {
			return jsValueOf(map[string]any{"error": "session unavailable"})
		}
		batch, err := c.AgentNativeStream(p.After)
		if err != nil {
			return jsValueOf(map[string]any{"error": err.Error()})
		}
		if p.Epoch != "" && p.Epoch != batch.Epoch {
			batch, err = c.AgentNativeStream(0)
		}
		if err != nil {
			return jsValueOf(map[string]any{"error": err.Error()})
		}
		return jsValueOf(map[string]any{"native": batch, "control": c.AgentHostState()})
	}))
	wlo.Set("hostControl", js.FuncOf(func(this js.Value, args []js.Value) any {
		var p app.AgentHostRequest
		if clientRef == nil || len(args) != 1 || seui.DecodeAutomation([]byte(js.Global().Get("JSON").Call("stringify", args[0]).String()), &p) != nil {
			return jsValueOf(map[string]any{"error": "invalid host control request"})
		}
		c := clientRef.AgentSession(p.Session)
		if c == nil {
			return jsValueOf(map[string]any{"error": "session unavailable"})
		}
		result, err := c.AgentHostSubmit(p)
		if err != nil {
			return jsValueOf(map[string]any{"error": err.Error()})
		}
		return jsValueOf(result)
	}))
	wlo.Set("viewerFrame", js.FuncOf(func(this js.Value, args []js.Value) any {
		if clientRef == nil || len(args) != 1 || args[0].Type() != js.TypeNumber {
			return jsValueOf(map[string]any{"error": "invalid session"})
		}
		c := clientRef.AgentSession(args[0].Int())
		if c == nil {
			return jsValueOf(map[string]any{"error": "session unavailable"})
		}
		return jsValueOf(c.AgentViewerFrame())
	}))
	wlo.Set("contract", js.FuncOf(func(this js.Value, args []js.Value) any { return jsValueOf(app.AgentContract()) }))
	wlo.Set("navigation", js.FuncOf(func(this js.Value, args []js.Value) any {
		if clientRef == nil || len(args) != 1 || args[0].Type() != js.TypeObject || args[0].IsNull() {
			return jsValueOf(map[string]any{"error": "invalid navigation request"})
		}
		var p app.AgentNavigationRequest
		if seui.DecodeAutomation([]byte(js.Global().Get("JSON").Call("stringify", args[0]).String()), &p) != nil {
			return jsValueOf(map[string]any{"error": "invalid navigation request"})
		}
		c := clientRef.AgentSession(p.Session)
		if c == nil {
			return jsValueOf(map[string]any{"error": "session unavailable"})
		}
		result, err := c.AgentNavigation(p)
		if err != nil {
			return jsValueOf(map[string]any{"error": err.Error()})
		}
		return jsValueOf(result)
	}))
	wlo.Set("lookup", js.FuncOf(func(this js.Value, args []js.Value) any {
		if clientRef == nil || len(args) != 1 || args[0].Type() != js.TypeObject || args[0].IsNull() {
			return jsValueOf(map[string]any{"error": "invalid lookup request"})
		}
		var p struct {
			Kind    string `json:"kind"`
			ID      uint32 `json:"id"`
			Session int    `json:"session"`
		}
		if json.Unmarshal([]byte(js.Global().Get("JSON").Call("stringify", args[0]).String()), &p) != nil {
			return jsValueOf(map[string]any{"error": "invalid lookup request"})
		}
		c := clientRef.AgentSession(p.Session)
		if c == nil {
			return jsValueOf(map[string]any{"error": "session unavailable"})
		}
		result, err := c.AgentLookup(p.Kind, p.ID)
		if err != nil {
			return jsValueOf(map[string]any{"error": err.Error()})
		}
		return jsValueOf(result)
	}))
	wlo.Set("execute", js.FuncOf(func(this js.Value, args []js.Value) any {
		if clientRef == nil || len(args) != 1 || args[0].Type() != js.TypeObject || args[0].IsNull() {
			return jsValueOf(map[string]any{"error": "client unavailable or invalid request"})
		}
		data := js.Global().Get("JSON").Call("stringify", args[0])
		var route struct {
			Session int `json:"session"`
		}
		if json.Unmarshal([]byte(data.String()), &route) != nil {
			return jsValueOf(map[string]any{"error": "invalid request"})
		}
		c := clientRef.AgentSession(route.Session)
		if c == nil {
			return jsValueOf(map[string]any{"error": "session unavailable"})
		}
		result, err := c.AgentSubmit([]byte(data.String()))
		if err != nil {
			return jsValueOf(map[string]any{"error": err.Error()})
		}
		return jsValueOf(result)
	}))
	wlo.Set("commands", js.FuncOf(func(this js.Value, args []js.Value) any {
		if clientRef == nil {
			return jsValueOf([]any{})
		}
		id := 0
		if len(args) > 0 && args[0].Type() == js.TypeNumber {
			id = args[0].Int()
		}
		c := clientRef.AgentSession(id)
		if c == nil {
			return jsValueOf(map[string]any{"error": "session unavailable"})
		}
		return jsValueOf(c.AgentCommandResults())
	}))
	wlo.Set("cancel", js.FuncOf(func(this js.Value, args []js.Value) any {
		if clientRef == nil || len(args) != 1 {
			return false
		}
		if args[0].Type() == js.TypeString {
			return clientRef.AgentCancel(args[0].String())
		}
		if args[0].Type() != js.TypeObject || args[0].IsNull() {
			return false
		}
		id, ticket := args[0].Get("session"), args[0].Get("ticket")
		if id.Type() != js.TypeNumber || ticket.Type() != js.TypeString {
			return false
		}
		c := clientRef.AgentSession(id.Int())
		return c != nil && c.AgentCancel(ticket.String())
	}))

	wlo.Set("state", js.FuncOf(func(this js.Value, args []js.Value) any {
		id := 0
		if len(args) > 0 && args[0].Type() == js.TypeNumber {
			id = args[0].Int()
		}
		if clientRef == nil {
			return jsValueOf(agentState())
		}
		c := clientRef.AgentSession(id)
		if c == nil {
			return jsValueOf(map[string]any{"error": "session unavailable"})
		}
		return jsValueOf(agentStateFor(c))
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

	// Guard legacy diagnostic/query helpers as well as the semantic API. State
	// and command status remain readable so refusal can be explained to callers.
	keys := js.Global().Get("Object").Call("keys", wlo)
	for i := 0; i < keys.Length(); i++ {
		name := keys.Index(i).String()
		switch name {
		case "state", "execute", "commands", "cancel", "contract", "hostControl":
			continue
		}
		original := wlo.Get(name)
		wlo.Set(name, js.FuncOf(func(this js.Value, args []js.Value) any {
			id := 0
			if len(args) > 0 && args[0].Type() == js.TypeObject && !args[0].IsNull() {
				if value := args[0].Get("session"); value.Type() == js.TypeNumber {
					id = value.Int()
				}
			}
			if name == "viewerFrame" && len(args) > 0 && args[0].Type() == js.TypeNumber {
				id = args[0].Int()
			}
			if clientRef != nil {
				if c := clientRef.AgentSession(id); c != nil {
					if err := c.AgentAccessError(); err != nil {
						return jsValueOf(map[string]any{"error": err.Error()})
					}
				}
			}
			forward := make([]any, len(args))
			for i, v := range args {
				forward[i] = v
			}
			return original.Invoke(forward...)
		}))
	}
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
	if clientRef == nil {
		return agentStateFor(nil)
	}
	return agentStateFor(clientRef.AgentSession(0))
}
func agentStateFor(c *app.Client) map[string]any { return c.AgentState() }

func encodeBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

// agentCommand dispatches a synthetic input event to the Ebiten canvas.
func agentCommand(name string, args []any) error {
	c := clientRef
	if c == nil {
		return fmt.Errorf("client not ready")
	}
	if c.AgentHostState().Owner != "" {
		return fmt.Errorf("legacy input diagnostics are disabled on hosted sessions; use semantic commands or attach a viewer")
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
