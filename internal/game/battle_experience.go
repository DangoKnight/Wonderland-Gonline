package game

// BattleEXPFormula selects the server's startup-only battle EXP policy.
type BattleEXPFormula string

const (
	BattleEXPSimple        BattleEXPFormula = "simple"
	BattleEXPParticipation BattleEXPFormula = "participation"
)

func (f BattleEXPFormula) Valid() bool { return f == BattleEXPSimple || f == BattleEXPParticipation }
