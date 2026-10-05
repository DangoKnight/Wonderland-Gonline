package inventory

import (
	"math"
	"time"

	"wonderland-go/client/wlo/world"
	"wonderland-go/internal/protocol"
)

const (
	allocationAttributes = 5
	allocationWait       = 5 * time.Second
	allocationArrowLeft  = 154
	allocationArrowTop   = 339
	allocationArrowStep  = 16
)

var allocationStats = [allocationAttributes]byte{world.StatSTR, world.StatCON, world.StatINT, world.StatWIS, world.StatAGI}

func (f *Form) initAllocation() {
	f.Now = time.Now
	for i := range f.Increase {
		b := f.button("Btn_ArrowUp_2", allocationArrowLeft, allocationArrowTop+i*allocationArrowStep, 9, 11, nil)
		b.Tag = i
		b.OnClickTag = f.AddPoint
		b.SetHint([]byte("Allocate one attribute point"))
		f.Increase[i] = b
	}
	f.Allocate = f.button("Btn_OK_1", 0, 423, 56, 20, f.SubmitAllocation)
	f.CancelAllocation = f.button("Btn_Cancel_1", 0, 423, 56, 20, f.CancelPoints)
	f.syncAllocationControls()
}

func (f *Form) draftTotal() uint32 {
	var total uint32
	for _, n := range f.PendingPoints {
		total += uint32(n)
	}
	return total
}

func (f *Form) Attributes() [allocationAttributes]uint16 {
	return [allocationAttributes]uint16{f.Stats.STR, f.Stats.CON, f.Stats.INT, f.Stats.WIS, f.Stats.AGI}
}

// AddPoint follows FUN_00353ed4, with a separate draft instead of changing
// authoritative stats. Cancellation (FUN_00354074) simply discards the draft.
func (f *Form) allocationAllowed() bool {
	return f.Stats != nil && f.allowed() && (f.Env == nil || !f.Blocked())
}

func (f *Form) AddPoint(attribute int) {
	if attribute < 0 || attribute >= allocationAttributes || !f.allocationAllowed() || f.AllocationWaiting ||
		f.draftTotal() >= uint32(f.Stats.Points) {
		return
	}
	values := f.Attributes()
	if uint32(values[attribute])+uint32(f.PendingPoints[attribute]) >= math.MaxUint16 {
		return
	}
	f.PendingPoints[attribute]++
	f.syncAllocationControls()
}
func (f *Form) CancelPoints() {
	f.PendingPoints = [allocationAttributes]uint16{}
	f.syncAllocationControls()
}

// SubmitAllocation ports FUN_003540c4: character target 0, counted stat/word
// entries in STR, CON, INT, WIS, AGI order. AC8:1 replies supply final values.
func (f *Form) SubmitAllocation() {
	if !f.allocationAllowed() || f.AllocationWaiting || f.draftTotal() == 0 || f.draftTotal() > uint32(f.Stats.Points) || f.Send == nil {
		return
	}
	for i, n := range f.PendingPoints {
		if uint32(f.Attributes()[i])+uint32(n) > math.MaxUint16 {
			f.CancelPoints()
			return
		}
	}
	p := protocol.Builder{protocol.CommandStats, protocol.StatsStatUpdate, 0, 0}
	for i, n := range f.PendingPoints {
		if n != 0 {
			p[3]++
			p = p.U8(allocationStats[i]).U16(n)
		}
	}
	f.PendingPoints = [allocationAttributes]uint16{}
	f.AllocationWaiting = true
	f.allocationSent = f.Now()
	f.syncAllocationControls()
	f.Send(p)
}

func (f *Form) AllocationReply() {
	f.AllocationWaiting = false
	f.CancelPoints()
}

func (f *Form) syncAllocationControls() {
	if f.Stats == nil {
		return
	}
	if f.AllocationWaiting && f.Now != nil && !f.Now().Before(f.allocationSent.Add(allocationWait)) {
		f.AllocationWaiting = false
		if f.Notice != nil {
			f.Notice("Attribute allocation has not been confirmed. Check your points before trying again.")
		}
	}
	if f.draftTotal() > uint32(f.Stats.Points) {
		f.PendingPoints = [allocationAttributes]uint16{}
	}
	editing := f.draftTotal() != 0
	for i, b := range f.Increase {
		if b == nil {
			continue
		}
		b.SetVisible(f.Mode != 2 && f.Stats.Points > 0)
		b.Enabled = !f.AllocationWaiting && f.draftTotal() < uint32(f.Stats.Points)
		if uint32(f.Attributes()[i])+uint32(f.PendingPoints[i]) >= math.MaxUint16 {
			b.Enabled = false
		}
	}
	if f.Allocate != nil {
		f.Allocate.Left, f.CancelAllocation.Left = f.Width/2-20-56, f.Width/2+20
		f.Allocate.SetVisible(editing && f.Mode != 2)
		f.CancelAllocation.SetVisible(editing && f.Mode != 2)
		f.Close.SetVisible(!editing)
	}
}
