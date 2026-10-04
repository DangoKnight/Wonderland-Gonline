package server

import (
	"errors"
	"strconv"
	"wonderland-go/internal/game"
)

func parseAdminGold(value string) (uint64, error) { return strconv.ParseUint(value, 10, 32) }
func parseAdminUint16(value string) (uint16, error) {
	n, err := strconv.ParseUint(value, 10, 16)
	return uint16(n), err
}
func parseAdminSlot(value string) (byte, error) {
	n, err := strconv.ParseUint(value, 10, 8)
	if err != nil || n < 1 || n > game.BagSize {
		return 0, errors.New("invalid bag slot")
	}
	return byte(n), nil
}
