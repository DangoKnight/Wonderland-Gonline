package store

import "wonderland-go/internal/game"

// Overlay live reservations on freshly loaded SQL state before planning a
// transaction. The resulting flags are never written by the typed SQL projection.
func preserveItemReservations(next *game.Character, snapshots []game.Character) error {
	for _, before := range snapshots {
		if before.ID == next.ID {
			return game.PreserveItemLocks(before, next)
		}
	}
	return nil
}
