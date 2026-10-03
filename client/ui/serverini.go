package ui

import (
	"bytes"
	"strconv"
)

// Region is one SERVER.INI region. Text fields keep the file's Big5 bytes.
type Region struct {
	Number  int    // leading two digits; regions are listed in this order
	ID      int    // trailing number, used to key server status
	Name    string // Big5
	Color   uint16 // RGB565 from a <B>/<G>/<R> tag; 0 draws in the list colour
	Servers []Server
}

// Server is one "name*address" line of a region.
type Server struct {
	Name, Address string // Big5 / ASCII
	ID            int    // RegionID*100 + index + 1: the status table key
}

// regionHidden is the region number the client stores separately instead of
// listing (configuration +0x260/+0x264).
const regionHidden = 95

// ParseServerINI parses SERVER.INI as aLogin FUN_003fde2c does. A region line
// has '[' as its third character and starts with its number (1..99), e.g.
// "01[Rhodes Island]1" or "02[Name]<G>2". Up to 100 following lines, until
// the next region line, are servers.
func ParseServerINI(data []byte) (regions []Region, hidden *Region) {
	lines := splitLines(data)
	var start [99]int
	for i := range start {
		start[i] = -1
	}
	for i, l := range lines {
		if len(l) > 2 && l[2] == '[' {
			if n, err := strconv.Atoi(string(l[:2])); err == nil && n > 0 && n < 100 {
				start[n-1] = i
			}
		}
	}
	for n := 1; n < 100; n++ {
		at := start[n-1]
		if at < 0 {
			continue
		}
		r := parseRegionLine(lines[at])
		r.Number = n
		for k, i := 0, at+1; k < 101 && i < len(lines); k, i = k+1, i+1 {
			l := lines[i]
			if len(l) == 0 {
				continue
			}
			if len(l) > 2 && l[2] == '[' {
				break
			}
			star := bytes.IndexByte(l, '*') + 1 // Delphi Pos: 1-based, 0 when absent
			name := "No Name"
			if star >= 2 {
				name = string(l[:star-1])
			}
			if star == 0 || star == len(l) {
				continue // no address: not listed
			}
			r.Servers = append(r.Servers, Server{Name: name, Address: string(l[star:]), ID: r.ID*100 + len(r.Servers) + 1})
		}
		if n == regionHidden {
			h := r
			hidden = &h
			continue
		}
		regions = append(regions, r)
	}
	return regions, hidden
}

func parseRegionLine(l []byte) Region {
	var r Region
	end := bytes.IndexByte(l, ']') + 1
	if end == 0 {
		end = len(l) - 1
	}
	if end < 2 {
		r.Name = "[]"
	} else if end > 4 {
		r.Name = string(l[3 : end-1])
	}
	if l[len(l)-1] == ']' {
		return r
	}
	lt, gt := bytes.IndexByte(l, '<')+1, bytes.IndexByte(l, '>')+1
	if lt > 0 && gt > 0 && gt-lt == 2 {
		switch l[lt] {
		case 'B':
			r.Color = 0x001f
		case 'G':
			r.Color = 0x0540
		case 'R':
			r.Color = 0xf800
		}
		end += 3
	}
	if end < len(l) {
		r.ID, _ = strconv.Atoi(string(bytes.TrimSpace(l[end:])))
	}
	return r
}

// splitLines splits like TStringList.LoadFromFile: CR, LF or CRLF.
func splitLines(data []byte) [][]byte {
	var out [][]byte
	for len(data) > 0 {
		i := bytes.IndexAny(data, "\r\n")
		if i < 0 {
			out = append(out, data)
			break
		}
		out = append(out, data[:i])
		if data[i] == '\r' && i+1 < len(data) && data[i+1] == '\n' {
			i++
		}
		data = data[i+1:]
	}
	return out
}
