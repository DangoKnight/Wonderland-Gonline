package protocol

// CommandPolicy describes inbound routing and interaction permissions. The zero
// value grants no exceptions. It does not validate packets or authorize users.
// Implementations bind policies to their handlers in their command registry.
type CommandPolicy struct {
	World                 bool
	CharacterDeletion     bool
	AllowedDuringBattle   bool
	AllowedDuringMinigame bool
	BlockedDuringTrade    bool
	BeforeWorldGates      bool
	RequiresBattle        bool
}

func (p CommandPolicy) IsWorldCommand() bool    { return p.World }
func (p CommandPolicy) IsDeleteCharacter() bool { return p.CharacterDeletion }
