package picdb

import "testing"

func TestReplaceSkinsKeepsIndicesAndNonSkinResources(t *testing.T) {
	db := New()
	db.Entries = []Entry{{Name: "Button", Path: "data/media/menu/Skins/default", W: 10}, {Name: "Item", Path: "data/pictures/Items", W: 20}, {Name: "OnlyChinese", Path: "data/media/locales/zh-TW/menu/Skins/default", W: 30}}
	for i, e := range db.Entries {
		db.index[key(e.Name)] = i
	}
	replacement := New()
	replacement.Entries = []Entry{{Name: "Button", Path: "data/media/locales/zh-TW/menu/Skins/default", W: 40}}
	replacement.index[key("Button")] = 0
	db.ReplaceSkins(replacement)
	if db.Find("Button") != 0 || db.Find("Item") != 1 || db.Entries[0].W != 40 || db.Entries[1].W != 20 || db.Entries[2].W != 0 {
		t.Fatal("skin replacement changed indices, unrelated resources or retained a missing locale", db.Entries)
	}
	replacement.Entries = append(replacement.Entries, Entry{Name: "OnlyChinese", Path: "data/media/locales/zh-TW/menu/Skins/default", W: 30})
	replacement.index[key("OnlyChinese")] = 1
	db.ReplaceSkins(replacement)
	if db.Find("OnlyChinese") != 2 || db.Entries[2].W != 30 {
		t.Fatal("locale-only skin could not be restored")
	}
}
