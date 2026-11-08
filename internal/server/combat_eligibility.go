package server

// Battle locks are authoritative for ambient aggression and external requests.
// A scripted battle may transition from its own event after closing dialogue;
// it must never interrupt a different event, including the party leader's.
// Caller holds worldMu.
func battleInteractionAvailable(c *Session, ownEvent *eventSession) bool {
	available := func(m *Session, permitted *eventSession) bool {
		return m != nil && m.character != nil && m.ready && !m.storm && m.beach == nil && m.arcade == nil && (m.event == nil || permitted != nil && m.event == permitted)
	}
	if !available(c, ownEvent) {
		return false
	}
	if c.party != nil && len(c.party.members) > 0 {
		leader := c.party.leader()
		if leader != c && !available(leader, ownEvent) {
			return false
		}
	}
	return true
}

// PK is all-or-nothing: never use battleTeam's PvE filtering to silently omit
// an occupied teammate. Preserve party order even when a follower is challenged.
func pkTeam(c *Session) []*Session {
	if c.party == nil || len(c.party.members) == 0 {
		return []*Session{c}
	}
	return c.party.members
}
