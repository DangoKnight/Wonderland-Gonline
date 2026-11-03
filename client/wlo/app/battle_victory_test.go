package app

import (
	"bytes"
	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"
	"io"
	"os"
	"strings"
	"testing"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/protocol"
)

func TestBattleVictoryVoiceRouletteAndRecording(t *testing.T) {
	for _, tc := range []struct {
		roll   int
		suffix string
	}{{1, "_bta3"}, {10, "_bta3"}, {11, "_bta2"}, {30, "_bta2"}, {31, "_bta1"}, {60, "_bta1"}, {61, ""}, {100, ""}} {
		if got := battleVictoryVoiceSuffix(tc.roll); got != tc.suffix {
			t.Fatal(tc.roll, got)
		}
	}
	c, _, _ := enterBattle(t)
	if bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE"); bundle != "" {
		useCompiledPetAssets(t, c, bundle)
	}
	for _, suffix := range []string{"_bta1", "_bta2", "_bta3"} {
		path := c.recordedBattleVoice("10", 2, suffix)
		raw, err := clientfs.ReadFile(login.Path(c.Assets.Media, strings.Split(path, `\`)...))
		if path == "" || err != nil {
			t.Fatal("missing victory recording", path, err)
		}
		decoded, err := vorbis.DecodeWithSampleRate(sampleRate, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(path, err)
		}
		pcm, err := io.ReadAll(decoded)
		if err != nil || len(pcm) == 0 {
			t.Fatal("missing/undecodable victory recording", path)
		}
	}
}

func victoryCapture(c *Client) *[]string {
	c.World.Player.Body, c.World.Player.Head = 2, 0
	c.battle.voiceRandom = func(int) int { return 0 }
	played := []string{}
	c.Env.Sound = func(path string) {
		if strings.Contains(path, "_bta") {
			played = append(played, path)
		}
	}
	return &played
}

func TestBattleVictoryVoiceWaitsForLastActionAndPlaysOnce(t *testing.T) {
	c, now := overkillClient(t)
	played := victoryCapture(c)
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 100, 0, 0, 0, 1})
	c.dispatch([]byte{11, 12, 1})
	c.dispatch([]byte{11, 12, 1})
	c.dispatch([]byte{11, 1, 4, 2, 0})
	c.dispatch([]byte{11, 0, 17, 39, 0, 0, 0, 0})
	c.battleFrame(false)
	if len(*played) != 0 {
		t.Fatal("victory interrupted attack")
	}
	*now = now.Add(150 * time.Millisecond)
	c.battleFrame(false)
	if len(*played) != 0 {
		t.Fatal("victory interrupted contact")
	}
	*now = now.Add(time.Second)
	c.battleFrame(false)
	c.battleFrame(false)
	if len(*played) != 1 || (*played)[0] != `..\audio\odd\1002_bta3.ogg` {
		t.Fatal("victory line", *played)
	}
	*now = now.Add(time.Minute)
	c.battleFrame(false)
	if len(*played) != 1 {
		t.Fatal("victory replayed")
	}
}

func TestBattleVictoryVoiceEarlyCloseAndLosingSide(t *testing.T) {
	for _, winner := range []byte{1, 2} {
		c, _, _ := enterBattle(t)
		played := victoryCapture(c)
		c.dispatch([]byte{11, 12, winner})
		c.dispatch([]byte{11, 0, 17, 39, 0, 0, 0, 0})
		want := 0
		if winner == 1 {
			want = 1
		}
		if len(*played) != want {
			t.Fatal("early close or losing side voice", winner, *played)
		}
	}
}

func TestBattleVictoryVoiceCanChooseHumanPetButNotForFallenOwner(t *testing.T) {
	for _, hp := range []byte{0, 100} {
		c, _, _ := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
		played := victoryCapture(c)
		c.npcTemplates[14162] = world.NPCTemplate{Voice: 201}
		c.battle.voiceRandom = func(n int) int {
			if n == 2 {
				return 1
			}
			return 0
		}
		c.dispatch([]byte{51, 1, 4, 2, 25, hp, 0, 0, 0})
		c.dispatch([]byte{11, 12, 1})
		c.battleFrame(false)
		if hp == 0 && len(*played) != 0 {
			t.Fatal("fallen owner cheered")
		}
		if hp > 0 && (len(*played) != 1 || !strings.Contains((*played)[0], "3003_bta3")) {
			t.Fatal("pet voice selection", *played)
		}
	}
}

func TestGonlineCombatVictoryVoiceWaitsForChain(t *testing.T) {
	c, now := overkillClient(t)
	played := victoryCapture(c)
	c.combat.enabled = true
	attack := []byte{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 100, 0, 0, 0, 1}
	deliverCombatRound(t, c, 2, []protocol.CombatStep{{Packets: [][]byte{attack}, Delay: time.Second}})
	c.dispatch([]byte{11, 12, 1})
	c.dispatch([]byte{11, 0, 17, 39, 0, 0, 0, 0})
	c.battleFrame(false)
	if len(*played) != 0 {
		t.Fatal("queued ending spoke before round")
	}
	for i := 0; i < 40 && c.battle.state.Active; i++ {
		*now = now.Add(100 * time.Millisecond)
		c.battleFrame(false)
	}
	if len(*played) != 1 || c.battle.state.Active || c.battle.state.At(clientbattle.Cell{X: 2, Y: 2}) != nil {
		t.Fatal("queued ending voice", *played)
	}
}
