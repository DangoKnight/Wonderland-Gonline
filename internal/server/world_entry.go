package server

import (
	"math"
	"sort"
	"strings"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

const maxMonsterBookIndex = 5500

// QuestManager.SendStoryConstellations uses active completion marks, rather
// than Completed (which represents a removed mark). Order is native UI order.
var storyStars = [...]struct {
	mark uint32
	star byte
}{
	{13087, 1},  // Xaolan: Cygnus.
	{13173, 6},  // Niss.
	{13203, 8},  // Roca.
	{13151, 20}, // Clive: Bootes.
}

func storyConstellations(c *game.Character) []byte {
	out := protocol.Builder{protocol.CommandStoryConstellation, protocol.StoryConstellationList, 0}
	for _, reward := range storyStars {
		q := c.Quests[reward.mark]
		if q.State == game.InProgress && q.Step > 0 {
			out = out.U8(reward.star)
		}
	}
	out[2] = byte(len(out) - 3)
	return out
}

func (s *Server) bookMonster(id uint16) bool {
	npc, ok := s.Assets.NPCs[id]
	return ok && npc.BookIndex > 0 && npc.BookIndex <= maxMonsterBookIndex
}

func (s *Server) monsterBookPackets(c *game.Character) [][]byte {
	ids := append([]uint16(nil), c.DiscoveredMonsters...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var out [][]byte
	for i, id := range ids {
		if s.bookMonster(id) && (i == 0 || ids[i-1] != id) {
			out = append(out, protocol.Builder{protocol.CommandMonsterBook, protocol.MonsterBookDiscover}.U32(uint32(id)))
		}
	}
	return out
}

// Stage discoveries in the same character transaction as victory rewards.
// Return receipts separately so a failed save never publishes a discovery.
func (s *Server) discoverBattleMonsters(c *game.Character, run *battleRun) [][]byte {
	seen := make(map[uint16]bool, len(c.DiscoveredMonsters))
	for _, id := range c.DiscoveredMonsters {
		seen[id] = true
	}
	var out [][]byte
	for _, monster := range run.b.Defenders {
		if monster.Template == 0 || monster.Template > math.MaxUint16 {
			continue
		}
		id := uint16(monster.Template)
		if seen[id] || !s.bookMonster(id) {
			continue
		}
		seen[id] = true
		c.DiscoveredMonsters = append(c.DiscoveredMonsters, id)
		out = append(out, protocol.Builder{protocol.CommandMonsterBook, protocol.MonsterBookDiscover}.U32(uint32(id)))
	}
	return out
}

// AC89:0 requests the scene status. AC92:1 acknowledges it. Both can arrive
// before AC12:1 and must not publish world presence or mark the map ready.
func (s *Server) sceneReadyCommand(c *Session, p []byte) error {
	if len(p) != 2 {
		return protocol.ErrMalformed
	}
	if (p[0] == protocol.CommandSceneReady && p[1] != protocol.SceneReadyLoaded) ||
		(p[0] == protocol.CommandSceneReadyAck && p[1] != protocol.SceneReadyAcknowledged) {
		return ErrUnsupported
	}
	if p[0] == protocol.CommandSceneReady {
		// Native itemmall capture, packet 2888, as documented in AC89.cs.
		if err := c.send([]byte{protocol.CommandSceneReadyReply, protocol.SceneReadyStatus, 0, 1, 1, 3, 2, 3}); err != nil {
			return err
		}
	}
	if c.motdSent {
		return nil
	}
	c.motdSent = true
	if strings.TrimSpace(s.motd) == "" {
		return nil
	}
	out, err := protocol.Builder{protocol.CommandInventory, protocol.InventoryMessage, 0}.String(s.motd)
	if err != nil {
		return err
	}
	return c.send(out)
}
