package login

import "bytes"

// Player IDs whose FUN_0010e9cc class is 0x69..0x6e (IDs 100..400 after
// FUN_004852f8) may use the allowed entries.
const (
	exemptClassLow  = 0x69
	exemptClassHigh = 0x6e
	playerIDOffset  = 4500000
	playerIDLimit   = 0x895441
)

// playerClass is FUN_0010e9cc of a player ID (FUN_004852f8 first).
func playerClass(id int32) byte {
	if id > playerIDOffset && id < playerIDLimit {
		id -= playerIDOffset
	}
	switch {
	case id-1 >= 0 && id-1 < 0x50:
		return 2
	case id >= 0x5a && id < 0x5e:
		return 8
	case id >= 100 && id <= 0x96:
		return 0x69
	case id >= 0x97 && id <= 400:
		return 0x6b
	case id >= 0x3e9 && id <= 0x421:
		return 9
	case id > 0x4b0 && id < 0x7d1:
		return 0xe
	}
	return 0
}

func exempt(playerID int32) bool {
	c := playerClass(playerID)
	return c >= exemptClassLow && c <= exemptClassHigh
}

// NameAllowed is FUN_004a6d00: false when the name contains a banned
// substring or word. playerID is DAT_008265f4 +4.
func NameAllowed(name []byte, playerID int32) bool {
	if len(name) == 0 {
		return true
	}
	for _, w := range bannedSubstrings {
		if bytes.Contains(name, []byte(w)) {
			return false
		}
	}
	check := func(words []string, allowed []string) bool {
		for _, w := range words {
			if !containsWord(name, []byte(w)) {
				continue
			}
			ok := false
			for _, a := range allowed {
				if w == a {
					ok = exempt(playerID)
				}
			}
			if !ok {
				return false
			}
		}
		return true
	}
	return check(bannedWords[:], allowedWords[:]) && check(bannedNames[:], allowedNames[:])
}

// big5Lead is a byte that starts a double-byte character (> 0x80).
func big5Lead(c byte) bool { return c > 0x80 }

// containsWord is FUN_004a8e64 negated: whether s contains w, comparing
// whole characters. A one-byte word matches single-byte characters only;
// a word starting with a single byte is found anywhere (Pos); a word
// starting with a double byte matches at character starts.
func containsWord(s, w []byte) bool {
	if len(s) == 0 || len(w) == 0 {
		return false
	}
	if len(w) == 1 {
		for i := 0; i < len(s); {
			if big5Lead(s[i]) {
				i += 2
				continue
			}
			if s[i] == w[0] {
				return true
			}
			i++
		}
		return false
	}
	if !big5Lead(w[0]) {
		return bytes.Contains(s, w)
	}
	for i := 0; i < len(s); {
		if !big5Lead(s[i]) {
			i++
			continue
		}
		end := min(i+len(w), len(s))
		if bytes.Equal(s[i:end], w) {
			return true
		}
		i += 2
	}
	return false
}
