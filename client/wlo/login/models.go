package login

// nativeModelAppearances is FUN_00484fcc's model-to-body/head table, used by
// AC5:12. These model IDs are not body types or creation-carousel positions.
var nativeModelAppearances = [...]struct{ body, head byte }{
	{},
	{2, 0}, {2, 1}, {1, 0}, {4, 2}, {4, 1}, {3, 0}, {3, 1},
	{4, 4}, {4, 5}, {4, 0}, {4, 6}, {3, 2}, {4, 3},
}

func ModelAppearance(model byte) (body, head byte, ok bool) {
	if model == 0 || int(model) >= len(nativeModelAppearances) {
		return 0, 0, false
	}
	a := nativeModelAppearances[model]
	return a.body, a.head, true
}
