package app

import (
	"image"

	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/protocol"
)

// AgentFloorItem is a ground-item snapshot for the headless agent API.
type AgentFloorItem struct {
	Slot uint16
	ID   uint16
	X, Y int
}

// AgentFloorItems returns the current map's ground items (chests, drops).
func (c *Client) AgentFloorItems() []AgentFloorItem {
	out := make([]AgentFloorItem, 0, len(c.floorItems))
	for slot, item := range c.floorItems {
		out = append(out, AgentFloorItem{Slot: slot, ID: item.id, X: item.x, Y: item.y})
	}
	return out
}

// AgentCamera returns the world camera's top-left corner.
func (c *Client) AgentCamera() (int, int) {
	if c.World == nil {
		return 0, 0
	}
	return c.World.Camera()
}

// AgentTalk returns the state of the event talk window.
func (c *Client) AgentTalk() (shown, choosing bool, options [][]byte, lines [][]byte) {
	if c.Talk == nil {
		return false, false, nil, nil
	}
	for _, line := range c.Talk.Lines() {
		lines = append(lines, append([]byte(nil), line...))
	}
	for _, option := range c.Talk.Options() {
		options = append(options, append([]byte(nil), option...))
	}
	return c.Talk.Shown(), c.Talk.Choosing(), options, lines
}

// AgentWalkWorld plans a walk to world coordinates using the client's normal
// walk logic, without going through screen-space hit testing.
func (c *Client) AgentWalkWorld(x, y int) bool {
	if c.World == nil || c.held || c.event.active || c.sceneFrozen() {
		return false
	}
	c.groundHeld = false
	c.waterTravel.goal = nil
	c.World.Step(c.Now())
	c.nextWalk = c.Now().Add(walkRepeat)
	c.walkWaterAware(x, y, c.Now())
	c.World.MarkWalk()
	return true
}

// AgentInteractNPC triggers the same interaction path as clicking an NPC.
func (c *Client) AgentInteractNPC(id uint16) bool {
	if c.World == nil {
		return false
	}
	n := c.World.NPCs[id]
	if n == nil {
		return false
	}
	c.clickNPC(n)
	return true
}

// AgentPickupFloorSlot picks up a ground item by its slot id.
func (c *Client) AgentPickupFloorSlot(slot uint16) bool {
	if c.World == nil {
		return false
	}
	item, ok := c.floorItems[slot]
	if !ok {
		return false
	}
	at := c.floorItemPoint(item)
	return c.pickupFloorItem(at.X, at.Y)
}

// AgentArea is a map area snapshot for automation (warps, event zones).
type AgentArea struct {
	ID     uint16
	X1, Y1 int
	X2, Y2 int
	Events int
	Sign   byte
}

// AgentAreas returns the current map's areas.
func (c *Client) AgentAreas() []AgentArea {
	if c.World == nil {
		return nil
	}
	out := make([]AgentArea, 0, len(c.World.Areas))
	for _, a := range c.World.Areas {
		out = append(out, AgentArea{
			ID: a.ID, X1: a.Rect.Min.X, Y1: a.Rect.Min.Y,
			X2: a.Rect.Max.X, Y2: a.Rect.Max.Y, Events: a.Events, Sign: a.Sign,
		})
	}
	return out
}

// AgentMapSize returns the current scene's pixel size.
func (c *Client) AgentMapSize() (int, int) {
	if c.World == nil || c.World.Scene == nil {
		return 0, 0
	}
	return c.World.Scene.Width, c.World.Scene.Height
}

// AgentWalkGrid returns a walkability grid centred on the player. radius is
// in collision cells (20 world pixels each).
func (c *Client) AgentWalkGrid(radius int) (originX, originY, cols, rows int, cells []byte) {
	if c.World == nil || c.World.Scene == nil {
		return 0, 0, 0, 0, nil
	}
	return c.World.Scene.AgentGrid(c.World.Player.X, c.World.Player.Y, radius)
}

// AgentMapGrid returns a coarse walkability overview of the whole map.
func (c *Client) AgentMapGrid(step int) (cols, rows int, cells []byte) {
	if c.World == nil || c.World.Scene == nil {
		return 0, 0, nil
	}
	return c.World.Scene.AgentMapGrid(step)
}

// AgentPlan returns the client's own path to a world point.
func (c *Client) AgentPlan(x, y int) []image.Point {
	if c.World == nil || c.World.Scene == nil {
		return nil
	}
	return c.World.Scene.AgentPlan(c.World.Player.X, c.World.Player.Y, x, y)
}

// AgentWalking reports whether the player is currently moving.
func (c *Client) AgentWalking() bool {
	return c.World != nil && c.World.Walking()
}

// AgentItem is one inventory item snapshot.
type AgentItem struct {
	Slot  int
	ID    uint16
	Count byte
	Name  string
}

// AgentItemName resolves an item id to its display name.
func (c *Client) AgentItemName(id uint16) string {
	return c.items[id].Definition.Name
}

// AgentBag returns the non-empty bag slots.
func (c *Client) AgentBag() []AgentItem {
	if c.Inventory == nil || c.Inventory.State == nil {
		return nil
	}
	out := make([]AgentItem, 0, len(c.Inventory.State.Bag))
	for i, it := range c.Inventory.State.Bag {
		if it.Empty() {
			continue
		}
		out = append(out, AgentItem{
			Slot: i + 1, ID: it.ID, Count: it.Count,
			Name: c.items[it.ID].Definition.Name,
		})
	}
	return out
}

// AgentEquipment returns the worn equipment.
func (c *Client) AgentEquipment() []AgentItem {
	if c.Inventory == nil || c.Inventory.State == nil {
		return nil
	}
	out := make([]AgentItem, 0, len(c.Inventory.State.Equipment))
	for i, it := range c.Inventory.State.Equipment {
		if it.Empty() {
			continue
		}
		out = append(out, AgentItem{
			Slot: i + 1, ID: it.ID, Count: it.Count,
			Name: c.items[it.ID].Definition.Name,
		})
	}
	return out
}

// AgentQuest is one quest/book entry.
type AgentQuest struct {
	ID          uint16
	Name        string
	Description string
	Completed   bool
}

// AgentQuests returns the active and completed quest entries.
func (c *Client) AgentQuests() []AgentQuest {
	entries := c.notebookState.Entries()
	out := make([]AgentQuest, 0, len(entries))
	for _, e := range entries {
		out = append(out, AgentQuest{
			ID: e.ID, Name: e.Name, Description: e.Description, Completed: e.Completed,
		})
	}
	return out
}

// AgentPet is one party/hotel pet roster entry.
type AgentPet struct {
	Slot      int
	ID        uint16
	Name      string
	Level     byte
	HP        uint32
	MaxHP     uint32
	SP        uint16
	MaxSP     uint16
	Amity     byte
	Potential byte
}

// AgentPets returns the authoritative pet roster.
func (c *Client) AgentPets() []AgentPet {
	if c.Inventory == nil || c.Inventory.State == nil {
		return nil
	}
	out := make([]AgentPet, 0, len(c.Inventory.State.Pets))
	for i, p := range c.Inventory.State.Pets {
		if p.ID == 0 {
			continue
		}
		out = append(out, AgentPet{
			Slot: i + 1, ID: p.ID, Name: string(p.Name),
			Level: p.Stats.Level, HP: p.Stats.HP, MaxHP: p.Stats.MaxHP,
			SP: p.Stats.SP, MaxSP: p.Stats.MaxSP,
			Amity: p.Amity, Potential: p.Potential,
		})
	}
	return out
}

// AgentCompanions returns the following companions/pets.
func (c *Client) AgentCompanions() []world.AgentCompanion {
	if c.World == nil {
		return nil
	}
	return c.World.AgentCompanions()
}

// AgentWater reports whether a world point is water.
func (c *Client) AgentWater(x, y int) bool {
	return c.World != nil && c.World.Scene != nil && c.World.Scene.AgentWater(x, y)
}

// AgentLand reports whether a world point is clear land.
func (c *Client) AgentLand(x, y int) bool {
	return c.World != nil && c.World.Scene != nil && c.World.Scene.AgentLand(x, y)
}

// AgentWaterEdge reports whether the player stands next to clear water.
func (c *Client) AgentWaterEdge() bool {
	if c.World == nil || c.World.Scene == nil {
		return false
	}
	return c.World.Scene.AgentWaterEdge(c.World.Player.X, c.World.Player.Y)
}

// AgentBoardRaft targets a water point through the native boarding path.
func (c *Client) AgentBoardRaft(x, y int) bool {
	if c.World == nil || c.World.Scene == nil || c.World.Scene.WaterTravel {
		return false
	}
	if !c.World.Scene.AgentWater(x, y) {
		return false
	}
	if _, clear := c.World.Scene.AgentDirectWaterEntry(c.World.Player.X, c.World.Player.Y, x, y); !clear {
		return false
	}
	if !c.AgentWaterEdge() {
		return false
	}
	c.walkWaterAware(x, y, c.Now())
	return true
}

// AgentWaterTravel reports whether the player is currently on water.
func (c *Client) AgentWaterTravel() bool {
	return c.World != nil && c.World.Scene != nil && c.World.Scene.WaterTravel
}

// AgentVehicle returns the mounted water vehicle id, if any.
func (c *Client) AgentVehicle() uint16 {
	if c.World == nil {
		return 0
	}
	return c.World.Player.VehicleID
}

// AgentTalkOptionPoints returns the screen centre of each dialogue option.
func (c *Client) AgentTalkOptionPoints() []image.Point {
	if c.Talk == nil {
		return nil
	}
	n := len(c.Talk.Options())
	out := make([]image.Point, n)
	for i := 0; i < n; i++ {
		out[i] = c.Talk.OptionPoint(i)
	}
	return out
}

// AgentAllocate submits stat points in STR, CON, INT, WIS, AGI order.
func (c *Client) AgentAllocate(str, con, int_, wis, agi uint16) bool {
	vals := [5]uint16{str, con, int_, wis, agi}
	ids := [5]byte{world.StatSTR, world.StatCON, world.StatINT, world.StatWIS, world.StatAGI}
	p := protocol.Builder{protocol.CommandStats, protocol.StatsStatUpdate, 0, 0}
	n := byte(0)
	for i, v := range vals {
		if v == 0 {
			continue
		}
		p = p.U8(ids[i]).U32(uint32(v))
		n++
	}
	if n == 0 {
		return false
	}
	p[3] = n
	return c.Net.Send(p) == nil
}

// AgentClickID sends an event click for a map click id (NPC, area or warp).
func (c *Client) AgentClickID(id uint16) bool {
	if c.World == nil {
		return false
	}
	c.sendNPCClick(id)
	return true
}

// AgentShopEntry is one shop catalog row.
type AgentShopEntry struct {
	Slot  byte
	ID    uint16
	Name  string
	Price uint32
}

// AgentShop returns the open shop's catalog and the player's gold.
func (c *Client) AgentShop() (visible bool, mode int, stock []AgentShopEntry, gold uint32) {
	if c.Services == nil || !c.Services.Visible {
		return false, 0, nil, c.Stats.Gold
	}
	for _, e := range c.Services.Stock {
		stock = append(stock, AgentShopEntry{
			Slot: e.Slot, ID: e.ID, Name: c.AgentItemName(e.ID), Price: e.Price,
		})
	}
	return true, int(c.Services.Mode), stock, c.Stats.Gold
}

// AgentBuy buys quantity of a stock slot from the open shop.
func (c *Client) AgentBuy(slot, quantity byte) bool {
	if c.Services == nil || !c.Services.Visible || slot == 0 || quantity == 0 {
		return false
	}
	return c.Net.Send([]byte{protocol.CommandShop, protocol.ShopBuy, 1, slot, quantity}) == nil
}

// AgentEquip equips a bag item by slot.
func (c *Client) AgentEquip(slot int) bool {
	if c.Inventory == nil || c.Inventory.State == nil || slot < 1 || slot > len(c.Inventory.State.Bag) {
		return false
	}
	if c.Inventory.State.Bag[slot-1].Empty() {
		return false
	}
	return c.Net.Send([]byte{protocol.CommandInventory, protocol.InventoryEquip, byte(slot)}) == nil
}

// AgentUnequip removes a worn item into the first free bag slot.
func (c *Client) AgentUnequip(slot int) bool {
	if c.Inventory == nil || c.Inventory.State == nil {
		return false
	}
	if slot < 1 || slot > len(c.Inventory.State.Equipment) {
		return false
	}
	if c.Inventory.State.Equipment[slot-1].Empty() {
		return false
	}
	free := c.Inventory.State.FirstFree()
	if free == 0 {
		return false
	}
	return c.Net.Send([]byte{protocol.CommandInventory, protocol.InventoryUnequip, byte(slot), free}) == nil
}

// AgentPetDeposit deposits a party pet slot into the pet hotel.
func (c *Client) AgentPetDeposit(slot byte) bool {
	if slot < 1 || slot > 4 {
		return false
	}
	return c.Net.Send([]byte{protocol.CommandNPCService, protocol.PetHotelDeposit, slot}) == nil
}

// AgentPetWithdraw withdraws a hotel pet slot.
func (c *Client) AgentPetWithdraw(slot byte) bool {
	if slot < 1 {
		return false
	}
	return c.Net.Send([]byte{protocol.CommandNPCService, protocol.PetHotelWithdraw, slot}) == nil
}

// AgentHotel returns the hotel roster when the service form is open.
func (c *Client) AgentHotel() []AgentPet {
	if c.Services == nil {
		return nil
	}
	out := make([]AgentPet, 0, len(c.Services.Hotel))
	for i, p := range c.Services.Hotel {
		if p.ID == 0 {
			continue
		}
		out = append(out, AgentPet{
			Slot: i + 1, ID: p.ID, Name: string(p.Name),
			Level: p.Stats.Level, HP: p.Stats.HP, MaxHP: p.Stats.MaxHP,
			SP: p.Stats.SP, MaxSP: p.Stats.MaxSP,
			Amity: p.Amity, Potential: p.Potential,
		})
	}
	return out
}

// AgentEventAnswers returns the answer values for the current question.
func (c *Client) AgentEventAnswers() []byte {
	return append([]byte(nil), c.event.answers...)
}

// AgentAnswer submits a question option by index without a UI click.
func (c *Client) AgentAnswer(index int) bool {
	if index < 0 || index >= len(c.event.answers) {
		return false
	}
	c.answer(c.event.answers[index])
	return true
}

// AgentPose plays a pose/action cell.
func (c *Client) AgentPose(cell int) bool {
	return c.playPose(cell)
}

// AgentPetSelect makes a party pet the active/following pet by slot (1..4).
func (c *Client) AgentPetSelect(slot int) bool {
	if slot < 1 || slot > 4 {
		return false
	}
	c.Net.Send(protocol.Builder{protocol.CommandBattlePet, protocol.BattlePetSelect}.U32(uint32(slot)))
	return true
}

// AgentPetRest rests the active pet.
func (c *Client) AgentPetRest() bool {
	c.Net.Send([]byte{protocol.CommandBattlePet, protocol.BattlePetRest})
	return true
}

// AgentBattleFighter is one battle roster entry.
type AgentBattleFighter struct {
	X, Y       int
	Side, Kind byte
	ID, Owner  uint32
	HP, MaxHP  uint32
	SP, MaxSP  uint16
	Level      byte
	Removed    bool
	Waiting    bool
	CanChoose  bool
}

// AgentBattle is a battle state snapshot.
type AgentBattle struct {
	Active      bool
	Ready       bool
	Finished    bool
	Turn        uint64
	Self        uint32
	Side        byte
	VictorySide byte
	Result      string // "win", "lose" or ""
	ExpGained   int64
	LevelGained int
	RewardLines []string
	Fighters    []AgentBattleFighter
}

// AgentBattleState returns the current battle roster, turn state and the last
// result/rewards when the server has named a winning side.
func (c *Client) AgentBattleState() AgentBattle {
	b := AgentBattle{
		Active: c.battle.state.Active, Ready: c.battle.state.Ready,
		Finished: c.battle.state.Finished, Turn: c.battle.state.Turn,
		Self: c.battle.state.Self, Side: c.battle.state.Side,
		VictorySide: c.battle.victorySide,
	}
	for _, f := range c.battle.state.Fighters {
		b.Fighters = append(b.Fighters, AgentBattleFighter{
			X: int(f.X), Y: int(f.Y), Side: f.Side, Kind: f.Kind,
			ID: f.ID, Owner: f.Owner, HP: f.HP, MaxHP: f.MaxHP,
			SP: f.SP, MaxSP: f.MaxSP, Level: f.Level,
			Removed: f.Removed, Waiting: f.Waiting,
			CanChoose: c.battle.state.CanChoose(f.Cell),
		})
	}
	if b.VictorySide != 0 {
		side := b.Side
		if c.World != nil {
			for _, f := range c.battle.state.Fighters {
				if f.ID == c.World.Player.ID {
					side = f.Side
					break
				}
			}
		}
		if side != 0 {
			if b.VictorySide == side {
				b.Result = "win"
			} else {
				b.Result = "lose"
			}
		}
	} else if !b.Active && b.ExpGained > 0 {
		// Some battles omit AC11:12's winning side; a positive EXP change is
		// still an unambiguous win.
		b.Result = "win"
	} else if !b.Active && c.Stats != nil && c.Stats.HP <= 1 && b.ExpGained == 0 {
		b.Result = "lose"
	}
	r := &c.battleResults
	b.RewardLines = append([]string(nil), r.lines...)
	b.ExpGained = int64(r.after.player.exp) - int64(r.before.player.exp)
	b.LevelGained = int(r.after.player.level) - int(r.before.player.level)
	return b
}

func (c *Client) agentFighterAt(cx, cy int) (remoteFighter, bool) {
	for _, f := range c.remote.battle.fighters {
		if int(f.x) == cx && int(f.y) == cy {
			return f, true
		}
	}
	return remoteFighter{}, false
}

// AgentBattleAttack submits the actor's basic attack at a target cell.
func (c *Client) AgentBattleAttack(tx, ty int) bool {
	if !c.battle.state.Active || c.battleAnimating() {
		return false
	}
	actor, ok := c.battleActor()
	if !ok {
		return false
	}
	target, found := c.agentFighterAt(tx, ty)
	if !found || target.side == actor.side {
		return false
	}
	c.submitHotbar(c.battleBinding(skills.BasicAttack, actor), target)
	return true
}

// AgentBattleDefend submits the actor's defend action.
func (c *Client) AgentBattleDefend() bool {
	if !c.battle.state.Active || c.battleAnimating() {
		return false
	}
	actor, ok := c.battleActor()
	if !ok {
		return false
	}
	c.submitHotbar(c.battleBinding(skills.Defense, actor), actor)
	return true
}

// AgentBattleSkill submits a learned skill at a target cell.
func (c *Client) AgentBattleSkill(id uint16, tx, ty int) bool {
	if !c.battle.state.Active || c.battleAnimating() {
		return false
	}
	actor, ok := c.battleActor()
	if !ok {
		return false
	}
	target, found := c.agentFighterAt(tx, ty)
	if !found {
		return false
	}
	c.submitHotbar(c.battleBinding(id, actor), target)
	return true
}

// AgentBattleItem submits a bag item (healing, revive) at a target cell.
func (c *Client) AgentBattleItem(slot int, tx, ty int) bool {
	if !c.battle.state.Active || c.battleAnimating() {
		return false
	}
	if c.Inventory == nil || c.Inventory.State == nil || slot < 1 || slot > len(c.Inventory.State.Bag) {
		return false
	}
	item := c.Inventory.State.Bag[slot-1]
	if item.Empty() {
		return false
	}
	actor, ok := c.battleActor()
	if !ok {
		return false
	}
	target, found := c.agentFighterAt(tx, ty)
	if !found {
		return false
	}
	actorState := c.battle.state.At(clientbattle.Cell{X: actor.x, Y: actor.y})
	if actorState == nil {
		return false
	}
	c.battle.item = &battleItemSelection{
		actor:      clientbattle.Cell{X: actor.x, Y: actor.y},
		generation: actorState.Generation,
		slot:       byte(slot),
		item:       item,
	}
	c.submitBattleItem(target)
	return true
}

// AgentBattleAuto toggles the client's AUTO combat mode.
func (c *Client) AgentBattleAuto(on bool) bool {
	c.battle.auto = on
	if on {
		c.battleAutoTick()
	}
	return on
}

// AgentBattleFlee submits the native flee request.
func (c *Client) AgentBattleFlee() bool {
	if !c.battle.state.Active || c.battleAnimating() {
		return false
	}
	actor, ok := c.battleActor()
	if !ok {
		return false
	}
	c.submitBattleSpecial(actor, actor, battleFleeSkill, protocol.BattleActionFleeRequest)
	return true
}

// AgentHeal uses a bag item outside battle (field healing).
func (c *Client) AgentHeal(slot int) bool {
	if c.Inventory == nil || c.Inventory.State == nil || slot < 1 || slot > len(c.Inventory.State.Bag) {
		return false
	}
	if c.Inventory.State.Bag[slot-1].Empty() {
		return false
	}
	c.Inventory.Show()
	c.Inventory.Use(slot, false)
	c.Inventory.Hide()
	return true
}

// AgentEventActive reports whether an event script is running.
func (c *Client) AgentEventActive() bool {
	return c.event.active
}

// AgentEventDone reports whether a finished event step awaits acknowledgment.
func (c *Client) AgentEventDone() bool {
	return c.event.done
}

// AgentMovieActive reports whether a cutscene is playing.
func (c *Client) AgentMovieActive() bool {
	return c.movie != nil
}
