package role

// lookupGroups is FUN_0045ac44's table: for each archive family it may be
// asked about, the patches it checks in order (the last whose first ID is
// at most the ID wins), whether the comparison uses the ID less one, and
// whether a match clears the ID shift. Families whose base starts with a
// shift of -1 (the weapons) store IDs one lower until a clearing patch
// matches. Generated from the decompile; an authored data table.
type lookupMember struct {
	Name     string
	MinusOne bool
	Reset    bool
}

type lookupGroup struct {
	Shift   int
	Members []lookupMember
}

var lookupGroups = map[string]lookupGroup{
	"002a": {0, []lookupMember{{"002a_c01", false, false}}},
	"002c": {0, []lookupMember{{"002c_1", false, false}, {"002c_01", false, false}, {"002c_b01", false, false}, {"002c_c01", false, false}, {"002c1", false, false}, {"002c2", false, false}}},
	"002e": {0, []lookupMember{{"002e_1", false, false}, {"002e_a01", false, false}, {"002e_b01", false, false}, {"002e_c01", false, false}, {"002e1", false, false}, {"002e1_c01", false, false}, {"002e2", false, false}, {"002e3", false, false}}},
	"002s": {0, []lookupMember{{"002s_b01", false, false}, {"002s_c01", false, false}, {"002s2", false, false}}},
	"002w": {-1, []lookupMember{{"002w1", false, true}, {"002w_01", true, false}, {"002w_a01", true, false}, {"002w1_b01", false, true}, {"002w1_c01", false, true}, {"002w2", false, true}, {"002w3", false, true}}},
	"003a": {0, []lookupMember{{"003a_c01", false, false}}},
	"003c": {0, []lookupMember{{"003c_1", false, false}, {"003c_01", false, false}, {"003c_b01", false, false}, {"003c_c01", false, false}, {"003c1", false, false}, {"003c2", false, false}}},
	"003e": {0, []lookupMember{{"003e_1", false, false}, {"003e_a01", false, false}, {"003e_b01", false, false}, {"003e_c01", false, false}, {"003e1", false, false}, {"003e1_c01", false, false}, {"003e2", false, false}, {"003e3", false, false}}},
	"003s": {0, []lookupMember{{"003s_b01", false, false}, {"003s_c01", false, false}, {"003s2", false, false}}},
	"003w": {-1, []lookupMember{{"003w1", false, true}, {"003w_01", true, false}, {"003w_a01", true, false}, {"003w1_b01", false, true}, {"003w1_c01", false, true}, {"003w2", false, true}, {"003w3", false, true}}},
	"004a": {0, []lookupMember{{"004a_c01", false, false}}},
	"004c": {0, []lookupMember{{"004c_1", false, false}, {"004c_01", false, false}, {"004c_b01", false, false}, {"004c_c01", false, false}, {"004c1", false, false}, {"004c2", false, false}}},
	"004e": {0, []lookupMember{{"004e_1", false, false}, {"004e_a01", false, false}, {"004e_b01", false, false}, {"004e_c01", false, false}, {"004e1", false, false}, {"004e1_c01", false, false}, {"004e2", false, false}, {"004e3", false, false}}},
	"004s": {0, []lookupMember{{"004s_b01", false, false}, {"004s_c01", false, false}, {"004s2", false, false}}},
	"004w": {-1, []lookupMember{{"004w1", false, true}, {"004w_01", true, false}, {"004w_a01", true, false}, {"004w1_b01", false, true}, {"004w1_c01", false, true}, {"004w2", false, true}, {"004w3", false, true}}},
	"005a": {0, []lookupMember{{"005a_c01", false, false}}},
	"005c": {0, []lookupMember{{"005c_1", false, false}, {"005c_01", false, false}, {"005c_b01", false, false}, {"005c_c01", false, false}, {"005c1", false, false}, {"005c2", false, false}}},
	"005e": {0, []lookupMember{{"005e_1", false, false}, {"005e_a01", false, false}, {"005e_b01", false, false}, {"005e_c01", false, false}, {"005e1", false, false}, {"005e1_c01", false, false}, {"005e2", false, false}, {"005e3", false, false}}},
	"005s": {0, []lookupMember{{"005s_b01", false, false}, {"005s_c01", false, false}, {"005s2", false, false}}},
	"005w": {-1, []lookupMember{{"005w1", false, true}, {"005w_01", true, false}, {"005w_a01", true, false}, {"005w1_b01", false, true}, {"005w1_c01", false, true}, {"005w2", false, true}, {"005w3", false, true}}},
	"001":  {0, []lookupMember{{"001_1", false, false}, {"001_01", false, false}, {"001_a01", false, false}, {"001_b01", false, false}, {"001_C01", false, false}, {"001_C02", false, false}, {"001_C03", false, false}, {"001_C04", false, false}}},
	"001w": {-1, []lookupMember{}},
	"008":  {0, []lookupMember{{"008_1", false, false}, {"008_01", false, false}, {"008_a01", false, false}, {"008_b01", false, false}, {"008_c01", false, false}}},
	"007":  {0, []lookupMember{{"007_1", false, false}, {"007_b01", false, false}, {"007_c01", false, false}}},
	"006":  {0, []lookupMember{{"006_a01", false, false}, {"006_c01", false, false}, {"006_c02", false, false}}},
}
