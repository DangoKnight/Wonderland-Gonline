package picdb

import "strings"

func skinPath(path string) bool {
	return strings.Contains(strings.ToLower(strings.ReplaceAll(path, "\\", "/")), "/menu/skins/")
}

// SkinRegistry caches only immutable file metadata. Pixel surfaces remain owned
// by the live DB and may be released between switches without poisoning caches.
func (db *DB) SkinRegistry() *DB {
	out := New()
	for _, entry := range db.Entries {
		if !skinPath(entry.Path) {
			continue
		}
		entry.Image, entry.Loaded, entry.failed = nil, false, false
		out.index[key(entry.Name)] = len(out.Entries)
		out.Entries = append(out.Entries, entry)
	}
	return out
}

// ReplaceSkins preserves every numeric picture index held by existing controls.
// Non-skin pictures (items, map objects and sprite frames) are left intact.
// The replacement registry is completely prepared before this commit step.
func (db *DB) ReplaceSkins(replacement *DB) {
	for i, old := range db.Entries {
		if !skinPath(old.Path) {
			continue
		}
		if at := replacement.Find(old.Name); at >= 0 {
			next := replacement.Entries[at]
			if old.file != "" && old.file == next.file && old.Registered && (old.Image != nil || !old.failed) && old.Tint == next.Tint && old.W == next.W && old.H == next.H {
				continue
			}
			if old.Image != nil {
				old.Image.Close()
			}
			db.Entries[i] = replacement.Entries[at]
		} else {
			if old.Image != nil {
				old.Image.Close()
			}
			// A locale-only resource has no common fallback. Keep its slot for
			// controls and later switches, but never display the previous locale.
			old.Image, old.W, old.H, old.Registered, old.Loaded, old.failed, old.file = nil, 0, 0, false, false, true, ""
			db.Entries[i] = old
		}
	}
	for _, entry := range replacement.Entries {
		if db.Find(entry.Name) < 0 {
			db.index[key(entry.Name)] = len(db.Entries)
			db.Entries = append(db.Entries, entry)
		}
	}
}
