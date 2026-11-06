package app

import (
	"encoding/binary"
	"time"

	"wonderland-gonline/internal/localization"
	"wonderland-gonline/internal/protocol"
)

const teamLeadershipPromptDuration = 5 * time.Second

func (c *Client) applyTeamFormation(p []byte) bool {
	wasMember := len(c.World.TeamMembers(c.World.Player.ID)) > 1
	if !c.World.ApplyTeam(p) {
		return false
	}
	members := c.World.TeamMembers(c.World.Player.ID)
	if len(members) > 1 {
		roster := protocol.Builder{protocol.CommandTeam, protocol.TeamRoster}.U32(members[0]).U8(byte(len(members) - 1))
		for _, id := range members[1:] {
			roster = roster.U32(id)
		}
		c.TeamState.ApplyRoster(roster)
		c.setJoinTeamTarget(false)
	} else if wasMember || p[1] == protocol.TeamLeave && len(p) == protocol.TeamIDPacketBytes && binary.LittleEndian.Uint32(p[2:]) == c.World.Player.ID || p[1] == protocol.TeamRoster && len(p) == protocol.TeamRosterHeaderBytes && binary.LittleEndian.Uint32(p[2:]) == c.World.Player.ID {
		c.TeamState.ClearParty()
	}
	if c.teamTransferLeader != 0 && c.TeamState.Leader != c.teamTransferLeader {
		c.closeTeamTransfer()
	}
	c.Team.Refresh()
	return true
}

func (c *Client) closeTeamTransfer() {
	closePrompt := c.teamTransferClose
	c.teamTransferClose, c.teamTransferLeader, c.teamTransferDeadline = nil, 0, time.Time{}
	if closePrompt != nil {
		closePrompt()
	}
}

func (c *Client) teamLeadershipPacket(p []byte) bool {
	if len(p) < protocol.TeamLeaderTimeoutBytes {
		return true
	}
	switch p[2] {
	case protocol.TeamLeadershipOffered:
		if len(p) != protocol.TeamLeaderOfferNoticeBytes {
			return true
		}
		leader := binary.LittleEndian.Uint32(p[3:])
		if !c.inTeam() || c.TeamState.Leader != leader || leader == c.World.Player.ID || c.UI.Modal != nil || c.battle.state.Active || !c.Inventory.CanAct() {
			c.Net.Send([]byte{protocol.CommandTeam, protocol.TeamTransferLeadership, protocol.TeamTransferResponse, protocol.TeamTransferDeclined})
			return true
		}
		c.teamTransferLeader, c.teamTransferDeadline = leader, c.Now().Add(teamLeadershipPromptDuration)
		reply := func(result byte) {
			if c.teamTransferLeader != leader {
				return
			}
			valid := c.Now().Before(c.teamTransferDeadline) && c.TeamState.Leader == leader && c.inTeam() && !c.battle.state.Active && c.Inventory.CanAct()
			c.closeTeamTransfer()
			if !valid {
				c.Net.Send([]byte{protocol.CommandTeam, protocol.TeamTransferLeadership, protocol.TeamTransferTimeout})
				return
			}
			c.Net.Send([]byte{protocol.CommandTeam, protocol.TeamTransferLeadership, protocol.TeamTransferResponse, result})
		}
		c.teamTransferClose = c.Team.Confirm(localization.UI("ui.team.leadership_offer"), 0, func() { reply(protocol.TeamTransferAccepted) }, func() { reply(protocol.TeamTransferDeclined) })
	case protocol.TeamLeadershipChanged:
		if len(p) == protocol.TeamLeaderChangeNoticeBytes {
			c.applyTeamFormation(p)
		}
	case protocol.TeamLeadershipStatus:
		if len(p) < protocol.TeamLeaderStatusNoticeBytes {
			return true
		}
		if p[3] == protocol.TeamLeadershipRefused && len(p) != protocol.TeamLeaderRefusalNoticeBytes || p[3] != protocol.TeamLeadershipRefused && len(p) != protocol.TeamLeaderStatusNoticeBytes {
			return true
		}
		c.closeTeamTransfer()
		key := ""
		switch p[3] {
		case protocol.TeamLeadershipRefused:
			key = "ui.team.leadership_refused"
		case protocol.TeamLeadershipTimedOut:
			key = "ui.team.leadership_timeout"
		case protocol.TeamLeadershipBusy:
			key = "ui.team.leadership_busy"
		}
		if key != "" {
			c.Notices.Show([]byte(localization.UI(key)), 2*time.Second, c.Now())
		}
	}
	return true
}

func (c *Client) teamTransferTick() {
	if c.teamTransferLeader == 0 {
		return
	}
	if c.World == nil || c.TeamState == nil || !c.inTeam() || c.TeamState.Leader != c.teamTransferLeader {
		c.closeTeamTransfer()
		return
	}
	if !c.Now().Before(c.teamTransferDeadline) {
		c.closeTeamTransfer()
		c.Net.Send([]byte{protocol.CommandTeam, protocol.TeamTransferLeadership, protocol.TeamTransferTimeout})
	}
}
