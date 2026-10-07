package game

// Character stat identifiers carried by AC8:1 (Equip.Send8_1/SendExp).
const (
	StatCurrentHP         byte = 25
	StatCurrentSP         byte = 26
	StatINT               byte = 27
	StatSTR               byte = 28
	StatCON               byte = 29
	StatAGI               byte = 30
	StatWIS               byte = 33
	StatLevel             byte = 35
	StatTotalEXP          byte = 36
	StatUnallocatedPoints byte = 38
	StatAttack            byte = 41
	StatDefense           byte = 42
	StatMagicAttack       byte = 43
	StatMagicDefense      byte = 44
	StatSpeed             byte = 45
	StatSkillGrade        byte = 110
	StatSkillEXP          byte = 111
	StatHPBonus           byte = 207
	StatSPBonus           byte = 208
)

const StatRebirth byte = 39
