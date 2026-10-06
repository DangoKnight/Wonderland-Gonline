package login

// ServerSelection identifies an entry in the configured native server list.
type ServerSelection struct {
	Host   string `json:"host"`
	Region int    `json:"region"`
	Index  int    `json:"index"`
}

// RestoreSelection reconnects to a configured server, without authenticating.
// Addresses removed from SERVER.INI are never dialed from a stale profile.
func (s *SelectServer) RestoreSelection(saved ServerSelection) bool {
	s.Show()
	if saved.Region > 0 && saved.Region < len(s.addrs) {
		rows := s.addrs[saved.Region]
		if saved.Index >= 0 && saved.Index < len(rows) && string(rows[saved.Index]) == saved.Host {
			s.SelectRegion(byte(saved.Region - 1))
			s.Connect(saved.Index)
			return true
		}
	}
	for region, rows := range s.addrs {
		for index, host := range rows {
			if region > 0 && string(host) == saved.Host {
				s.SelectRegion(byte(region - 1))
				s.Connect(index)
				return true
			}
		}
	}
	return false
}
