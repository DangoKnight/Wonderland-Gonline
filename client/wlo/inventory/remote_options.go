package inventory

import "wonderland-gonline/internal/localization"

import (
	"fmt"
	"strconv"
	"wonderland-gonline/internal/game"
)

const remoteSettingLimit = 40000

// RemoteOptions are confirmed client-local settings, copied when Confirm is
// pressed. Resource changes still go through normal server transactions.
type RemoteOptions struct {
	AutoFight, AutoMove, AutoSupply, LeaveNoSupplies                        bool
	LeavePlayerDeaths, LeavePetDeaths, LeaveAfter, AutoUnequip, AutoDiscard bool
	Information                                                             bool
	Thresholds                                                              [4]int // player HP/SP, battle pet HP/SP percentages
	SupplyFirst, SupplyLast                                                 byte
	PlayerDeaths, PetDeaths, LeaveMinutes                                   int
	Discard                                                                 [5]uint16
}

func (d *remoteForm) options() (RemoteOptions, error) {
	o := RemoteOptions{AutoFight: d.checked[0], AutoMove: d.checked[1], AutoSupply: d.checked[2], LeaveNoSupplies: d.checked[3], LeavePlayerDeaths: d.checked[4], LeavePetDeaths: d.checked[5], LeaveAfter: d.checked[6], AutoUnequip: d.checked[7], AutoDiscard: d.checked[8], Information: d.checked[10], Discard: d.Discard}
	values := [5]int{}
	for i, e := range d.Fields {
		n, err := strconv.Atoi(string(e.Text))
		if err != nil || n < 0 || n > remoteSettingLimit {
			return o, fmt.Errorf("%s", localization.UI("ui.inventory.enter_valid_remote_settings_c1f18582"))
		}
		values[i] = n
	}
	if values[0] < 1 || values[1] < values[0] || values[1] > game.BagSize {
		return o, fmt.Errorf("%s", localization.UI("ui.inventory.supply_slots_must_be_between_1_and_50_in_fb0de804"))
	}
	o.SupplyFirst, o.SupplyLast = byte(values[0]), byte(values[1])
	o.PlayerDeaths, o.PetDeaths, o.LeaveMinutes = values[2], values[3], values[4]
	if (o.LeavePlayerDeaths && o.PlayerDeaths == 0) || (o.LeavePetDeaths && o.PetDeaths == 0) || (o.LeaveAfter && o.LeaveMinutes == 0) {
		return o, fmt.Errorf("%s", localization.UI("ui.inventory.enabled_leave_conditions_need_a_positive_4a316ce6"))
	}
	for i, s := range d.Thresholds {
		o.Thresholds[i] = min(remotePercentSteps, max(0, s.Pos))
	}
	return o, nil
}
func (d *remoteForm) start() {
	o, err := d.options()
	if err != nil {
		d.owner.notice(err.Error())
		return
	}
	if d.owner.StartRemote == nil || !d.owner.StartRemote(o) {
		return
	}
	d.Hide()
	if d.Plus != nil {
		d.Plus.Hide()
	}
}
func (d *remoteForm) stop() {
	if d.owner.StopRemote != nil {
		d.owner.StopRemote()
	}
	d.Hide()
}

// Drop on a native discard cell selects an item ID; it never drops/consumes
// that stack. The confirmed Auto Discard option performs later requests.
func (d *remoteForm) acceptDrop(it game.Item, x, y int) bool {
	if !d.Visible {
		return false
	}
	for i, c := range d.DiscardCells {
		if c.HitTest(x, y) {
			if it.Locked || it.ID == itemRemoteControl {
				d.owner.notice(localization.UI("ui.inventory.locked_items_and_remote_control_cannot_b_58eab1a7"))
				return true
			}
			d.Discard[i] = it.ID
			return true
		}
	}
	return false
}
