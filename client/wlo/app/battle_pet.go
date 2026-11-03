package app

import clientbattle "wonderland-gonline/client/wlo/battle"

func (c *Client) canChangeBattlePet() bool {
	v := &c.battle
	if !v.state.Active || !v.state.Ready || v.timedOut || c.battleAnimating() {
		return false
	}
	for i := range v.state.Fighters {
		f := &v.state.Fighters[i]
		if f.Kind == clientbattle.Player && v.state.Owned(f) && f.HP > 0 && !f.Removed && !f.Submitted && !c.hotbar.submitted[[2]byte{f.X, f.Y}] {
			for _, choice := range c.combat.choices {
				if choice[2] == f.X && choice[3] == f.Y {
					return false
				}
			}
			return true
		}
	}
	return false
}
