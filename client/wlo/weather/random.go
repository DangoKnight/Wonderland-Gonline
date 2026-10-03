package weather

// Rand is Delphi's System.Random (FUN_00012d30): RandSeed = RandSeed ×
// 0x08088405 + 1, and Random(n) is the high half of n × RandSeed.
type Rand struct{ Seed uint32 }

// Intn is Random(n).
func (r *Rand) Intn(n int) int {
	r.Seed = r.Seed*0x08088405 + 1
	return int(uint64(uint32(n)) * uint64(r.Seed) >> 32)
}
