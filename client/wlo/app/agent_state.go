package app

import "wonderland-gonline/client/wlo/seui"

// AgentState must be queried on the owning game loop. Transports marshal it
// before returning it to another goroutine.
func (c *Client) AgentState() map[string]any {
	if c == nil {
		return map[string]any{"status": "initializing"}
	}
	if err := c.AgentAccessError(); err != nil {
		return map[string]any{"status": "agent_access_blocked", "error": err.Error(), "apiVersion": AgentAPIVersion, "agentAPIAllowed": false, "sessions": c.AgentSessions()}
	}

	state := map[string]any{
		"status":     "running",
		"screen":     map[string]int{"width": ScreenWidth, "height": ScreenHeight},
		"inWorld":    c.World != nil,
		"openForms":  agentOpenForms(c),
		"apiVersion": AgentAPIVersion,
		"details":    c.AgentDetails(),
		"sessions":   c.AgentSessions(),
		"forms":      c.AgentForms(),
		"commands":   c.AgentCommandResults(),
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
		talk["options"] = agentByteStrings(options)
		pts := c.AgentTalkOptionPoints()
		out := make([]map[string]int, 0, len(pts))
		for _, p := range pts {
			out = append(out, map[string]int{"x": p.X, "y": p.Y})
		}
		talk["optionPoints"] = out
	}
	if len(lines) > 0 {
		talk["lines"] = agentByteStrings(lines)
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
			"name": seui.DecodeText(w.Player.Name),
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
				"name": seui.DecodeText(p.Name),
			})
		}
		state["peers"] = peers
		npcs := make([]map[string]any, 0, len(w.NPCs))
		interactionReady := c.agentNPCInteractionReady()
		for id, n := range w.NPCs {
			inRange := agentNPCInInteractionRange(w.Player, n)
			npcs = append(npcs, map[string]any{
				"id":                 id,
				"clickID":            id,
				"x":                  n.X,
				"y":                  n.Y,
				"name":               n.Info.Name,
				"visible":            n.Visible(),
				"template":           n.Template,
				"inInteractionRange": inRange,
				"interactable":       interactionReady && n.Visible() && inRange,
			})
		}
		state["npcs"] = npcs
	}

	return state
}

func agentByteStrings(in [][]byte) []string {
	out := make([]string, 0, len(in))
	for _, b := range in {
		out = append(out, seui.DecodeText(b))
	}
	return out
}

// openForms returns a list of visible top-level form names.
func agentOpenForms(c *Client) []string {
	out := []string{}
	for _, form := range c.AgentForms() {
		out = append(out, form.Name)
	}
	return out
}
