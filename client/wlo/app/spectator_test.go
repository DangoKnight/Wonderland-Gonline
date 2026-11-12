package app

import (
	"bytes"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/sessionstream"

	"wonderland-gonline/client/wlo/agentwire"
	"wonderland-gonline/client/wlo/render"
)

func TestSpectatorRegularWorkspaceSettingsAndTransientProfile(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	journal := sessionstream.New("test", time.Now())
	journal.Append(time.Now(), []sessionstream.Event{{Kind: "network", Code: int(login.Packet), Packet: selfPacket(10001, 2, 10017, 1042, 1075, 0, 444444444, 444444444, nil, "Dango")}})
	go func() {
		socket, err := listener.Accept()
		if err != nil {
			return
		}
		defer socket.Close()
		wire := agentwire.New(socket)
		var hello map[string]any
		if wire.ReadJSON(&hello) != nil {
			return
		}
		message := map[string]any{"native": journal.Batch(0), "control": AgentHostControl{Owner: "api", Generation: 1}, "viewer": "test-viewer"}
		if wire.WriteJSON(message) != nil {
			return
		}
		message["native"] = journal.Batch(1)
		requests := make(chan AgentHostRequest)
		stopped := make(chan struct{})
		defer close(stopped)
		go func() {
			defer close(requests)
			for {
				var p AgentHostRequest
				if wire.ReadJSON(&p) != nil {
					return
				}
				select {
				case requests <- p:
				case <-stopped:
					return
				}
			}
		}()
		heartbeat := time.NewTicker(100 * time.Millisecond)
		defer heartbeat.Stop()
		for {
			select {
			case p, ok := <-requests:
				if !ok {
					return
				}
				if p.Operation == "takeover" {
					message["control"] = AgentHostControl{Owner: "viewer", Viewer: "test-viewer", Generation: 2}
				}
			case <-heartbeat.C:
			}
			if wire.WriteJSON(message) != nil {
				return
			}
		}
	}()
	first := testClient(t)
	w := NewWorkspace(first)
	defer w.Close()
	w.options.SettingsPath = filepath.Join(t.TempDir(), "settings.json")
	prefs := defaultDisplaySettings()
	prefs.FPS = 144
	prefs.Scaling = render.ScalingBicubic
	path := applicationSettingsPath(w.options, first.Assets)
	if err := prefs.save(path); err != nil {
		t.Fatal(err)
	}
	if err := w.AddSpectator("tcp://" + listener.Addr().String() + "/#private-capability"); err != nil {
		t.Fatal(err)
	}
	spectator := w.Current()
	if spectator == first || spectator.spectator == nil || spectator.Pics != first.Pics || spectator.resources != first.resources || len(w.Sessions) != 2 {
		t.Fatal("spectator bypassed regular workspace")
	}
	loaded, err := loadDisplaySettings(applicationSettingsPath(w.options, spectator.Assets))
	if err != nil || loaded.FPS != prefs.FPS || loaded.Scaling != prefs.Scaling {
		t.Fatal("spectator lost regular display preferences", loaded, err)
	}
	g := &Game{C: spectator, Workspace: w, display: newDisplayControls(path)}
	width, height := g.renderSize()
	if g.display.settings.FPS != prefs.FPS || width != workspaceWidth || height != ScreenHeight {
		t.Fatal("spectator did not use regular window configuration")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		spectator.Frame()
		if spectator.replica.applied == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if spectator.replica.applied != 1 || spectator.World == nil || spectator.World.Player.ID != 10001 {
		t.Fatal("spectator did not render its hosted scene independently of a game login")
	}
	g.display.open = true
	composed := w.Compose()
	g.drawDisplay(composed)
	if composed.W != workspaceWidth || composed.H != ScreenHeight {
		t.Fatal("spectator omitted regular workspace and local settings surface")
	}
	r := w.spectatorControlRect(1)
	w.Pointer(r.Min.X+2, r.Min.Y+2, true, true, 0)
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if spectator.spectator.State().Owns {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !spectator.spectator.State().Owns {
		t.Fatal("sidebar control did not request intervention")
	}
	profile := w.snapshotProfile()
	raw, _ := json.Marshal(profile)
	if len(profile.Sessions) != 1 || profile.Sessions[0].ID != 1 || profile.ActiveSession != 1 || bytes.Contains(raw, []byte("private-capability")) {
		t.Fatal("spectator capability or transient session persisted", string(raw))
	}
	w.Switch(0)
	if w.Current() != first || !spectator.backgroundSession {
		t.Fatal("spectator did not follow ordinary workspace focus")
	}
	w.Remove(2)
	if len(w.Sessions) != 1 || first.resources == nil {
		t.Fatal("closing spectator damaged normal session")
	}
}
