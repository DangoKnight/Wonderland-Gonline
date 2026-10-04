package server

import (
	"context"
	"errors"
	"testing"
	"wonderland-go/internal/protocol"
)

// Raw command numbers preserve existing dispatch/gates, including ported AC10, AC34 and AC45.
func TestCommandRegistryCompatibility(t *testing.T) {
	contains := func(c byte, values ...byte) bool {
		for _, v := range values {
			if c == v {
				return true
			}
		}
		return false
	}
	for code := 0; code <= 255; code++ {
		c := byte(code)
		registration := commandRegistry[c]
		world := contains(c, 2, 4, 5, 6, 7, 8, 10, 11, 12, 14, 15, 16, 18, 19, 20, 21, 22, 23, 25, 27, 30, 31, 32, 33, 34, 36, 39, 40, 44, 45, 50, 56, 57, 59, 62, 65, 66, 67, 68, 69, 71, 75, 77, 87, 89, 90, 91, 92, 104, 183, 186, 226)
		known := world || contains(c, 0, 1, 9, 13, 35, 63)
		p := registration.policy
		if (registration.handler != nil) != known || p.IsWorldCommand() != world ||
			p.IsDeleteCharacter() != (c == 35) ||
			p.AllowedDuringBattle != contains(c, 2, 11, 50) ||
			p.RequiresBattle != contains(c, 50) ||
			p.AllowedDuringMinigame != contains(c, 2, 20, 57, 71) ||
			p.BlockedDuringTrade != contains(c, 5, 8, 15, 19, 20, 21, 23, 27, 30, 31, 36, 40, 45, 56, 59, 62, 65, 67, 68, 69, 71, 75, 77, 90, 104) {
			t.Errorf("command %d changed registration or interaction policy: %+v", c, p)
		}
		for sub := 0; sub <= 255; sub++ {
			packet := []byte{c, byte(sub)}
			early := p.BeforeWorldGates || (registration.beforeWorldGates != nil && registration.beforeWorldGates(packet))
			expected := contains(c, 4, 16, 33, 89, 92, 183, 186, 226) || (c == 21 && contains(byte(sub), 1, 3)) || (c == 32 && sub == 3) || (c == 23 && contains(byte(sub), 25, 54, 77)) || (c == 75 && sub == 2) || (c == 5 && contains(byte(sub), 4, 7))
			if early != expected {
				t.Errorf("command %d:%d changed pre-gate routing", c, sub)
			}
		}
		if registration.beforeWorldGates != nil && (registration.beforeWorldGates(nil) || registration.beforeWorldGates([]byte{c})) {
			t.Errorf("command %d accepts truncated synchronization request", c)
		}
	}
}

func TestCommandRegistryRejectsUnsupportedAndIdleBattle(t *testing.T) {
	for _, command := range []byte{3, 11, 50, 255} {
		if err := (&Server{}).dispatch(context.Background(), &Session{}, []byte{command}); command == 11 || command == 50 {
			if !errors.Is(err, protocol.ErrMalformed) {
				t.Fatalf("world command without character: %v", err)
			}
		} else if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("unsupported command %d: %v", command, err)
		}
	}
	for _, command := range []byte{11, 50} {
		if err := commandRegistry[command].handle(&Server{}, context.Background(), &Session{}, []byte{command}); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("battle command without battle: %v", err)
		}
	}
	if err := (&Server{}).worldCommand(context.Background(), &Session{}, nil); !errors.Is(err, protocol.ErrMalformed) {
		t.Fatal(err)
	}
}
