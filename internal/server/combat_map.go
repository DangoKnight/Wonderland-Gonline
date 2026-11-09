package server

import "wonderland-gonline/internal/assets"

// Use the encounter's original map, including when a participant is removed or
// sent to their spawn point while an already-calculated round is playing.
func (s *Server) battleMapDefinition(run *battleRun) assets.Map {
	id := run.backdropLocation.Map
	if id == 0 && len(run.members) > 0 {
		id = run.members[0].c.character.Map
	}
	return s.Assets.Maps[id]
}
