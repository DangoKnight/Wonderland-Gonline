package assets

const nativeNPCCaptureRestrictionOffset = 34

// CanCapture uses the native Catchable field, whose zero value permits capture.
// It includes content exceptions; humanoid appearances and support skills are
// not reliable classifiers for boss/monster eligibility.
func (n NPC) CanCapture() bool { return n.CaptureRestriction == 0 }
