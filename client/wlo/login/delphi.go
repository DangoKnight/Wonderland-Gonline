package login

import "bytes"

// Delphi string routines with their 1-based positions.

// pos is Pos(sub, s): the 1-based index of the first match, or 0.
func pos(sub string, s []byte) int {
	return bytes.Index(s, []byte(sub)) + 1
}

// copyStr is Copy(s, index, count).
func copyStr(s []byte, index, count int) []byte {
	if index < 1 {
		index = 1
	}
	if index > len(s) || count <= 0 {
		return nil
	}
	return append([]byte(nil), s[index-1:min(index-1+count, len(s))]...)
}

// at is s[i] for a 1-based i, reading the terminator past the end.
func at(s []byte, i int) byte {
	if i < 1 || i > len(s) {
		return 0
	}
	return s[i-1]
}

// strToInt is StrToInt through Val: leading blanks, a sign, decimal or $
// hexadecimal digits, and nothing after them.
func strToInt(s []byte) (int, bool) {
	i := 0
	for i < len(s) && s[i] == ' ' {
		i++
	}
	neg := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}
	base := 10
	if i < len(s) && s[i] == '$' {
		base = 16
		i++
	}
	if i == len(s) {
		return 0, false
	}
	v := 0
	for ; i < len(s); i++ {
		c := s[i]
		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case base == 16 && c|0x20 >= 'a' && c|0x20 <= 'f':
			d = int(c|0x20-'a') + 10
		default:
			return 0, false
		}
		v = v*base + d
		if v > 1<<31 {
			return 0, false
		}
	}
	if neg {
		v = -v
	}
	return v, true
}

// trim is Trim: blanks and control characters at both ends.
func trim(s []byte) []byte {
	return bytes.TrimFunc(s, func(r rune) bool { return r <= ' ' })
}

// upper is UpperCase: ASCII letters only.
func upper(s []byte) []byte {
	out := append([]byte(nil), s...)
	for i, c := range out {
		if c >= 'a' && c <= 'z' {
			out[i] = c - 0x20
		}
	}
	return out
}

// splitLines is TStrings.SetText: lines end at CR, LF or CR LF.
func splitLines(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] != '\r' && b[i] != '\n' {
			continue
		}
		out = append(out, b[start:i])
		if b[i] == '\r' && i+1 < len(b) && b[i+1] == '\n' {
			i++
		}
		start = i + 1
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
}

// joinLines is TStrings.SaveToFile's text: every line ends with CR LF.
func joinLines(lines [][]byte) []byte {
	var b bytes.Buffer
	for _, l := range lines {
		b.Write(l)
		b.WriteString("\r\n")
	}
	return b.Bytes()
}
