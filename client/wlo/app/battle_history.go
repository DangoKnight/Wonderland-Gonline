package app

import clientbattle "wonderland-gonline/client/wlo/battle"

// Grid cells can change between battles and be reused by another pet. Keep
// session history by actor identity, and project it onto the current formation.
type battleActionIdentity struct {
	id, owner uint32
	kind      byte
}

func (c *Client) saveBattleAction(cell clientbattle.Cell, action battlePreviousAction) {
	v := &c.battle
	if v.lastActions == nil {
		v.lastActions = map[clientbattle.Cell]battlePreviousAction{}
	}
	if v.actionHistory == nil {
		v.actionHistory = map[battleActionIdentity]uint16{}
	}
	v.lastActions[cell] = action
	v.actionHistory[battleActionIdentity{action.id, action.owner, action.kind}] = action.skill
}

func (c *Client) forgetBattleAction(cell clientbattle.Cell) {
	v := &c.battle
	delete(v.lastActions, cell)
	delete(v.pendingActions, cell)
	if f := v.state.At(cell); f != nil && v.state.Owned(f) {
		delete(v.actionHistory, battleActionIdentity{f.ID, f.Owner, f.Kind})
	}
}

func (c *Client) restoreBattleActions() {
	v := &c.battle
	if !v.state.Active {
		return
	}
	for _, f := range v.state.Fighters {
		if !v.state.Owned(&f) {
			continue
		}
		key := battleActionIdentity{f.ID, f.Owner, f.Kind}
		if id, ok := v.actionHistory[key]; ok {
			if v.lastActions == nil {
				v.lastActions = map[clientbattle.Cell]battlePreviousAction{}
			}
			v.lastActions[f.Cell] = battlePreviousAction{f.ID, f.Owner, f.Kind, id}
		} else {
			delete(v.lastActions, f.Cell)
		}
	}
}
