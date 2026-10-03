package movie

import (
	"image"
	"time"
)

// Pictures (+0xe8): animated pictures placed in the scene, such as the "?"
// bubble over the storm's captain. Each moves through its keyframes like
// an actor and is drawn from stage start + 1 to start + count
// (FUN_00340404 → FUN_00342508): centred on its point with its bottom on
// it, one frame of a vertical strip at a time. A frame advances every
// interval (FUN_0034249c) and the strip repeats the header's number of
// times; a keyframe's fixed frame (+0xd) overrides it.
type PictureState struct {
	Pic   *Picture
	Key   int // +0x2148
	mover ActorState
	// Frame is the 1-based frame (+0x2160); repeats counts down (+0x2168).
	Frame   int
	repeats int
	frameAt time.Time
}

func newPicture(pic *Picture, now time.Time) *PictureState {
	s := &PictureState{Pic: pic, Frame: 1, repeats: int(pic.Header[pictureRepeats]), frameAt: now}
	s.mover = ActorState{Speed: speeds[defaultClass], at: now}
	if len(pic.Keys) > 0 {
		k := pic.Keys[0]
		s.mover.X, s.mover.Y, s.mover.TX, s.mover.TY = float64(k.X), float64(k.Y), k.X, k.Y
	}
	return s
}

// Point is the picture's position.
func (s *PictureState) Point() image.Point { return s.mover.Point() }

// active reports whether the picture moves in stage st.
func (s *PictureState) active(st int) bool {
	return s.Pic.Start() <= st && st <= s.Pic.Start()+s.Pic.Count()
}

// Drawn reports whether the picture is drawn in stage st (a picture whose
// repeats have run out is not).
func (s *PictureState) Drawn(st int) bool {
	return s.repeats != 0 && s.Pic.Start()+1 <= st && st <= s.Pic.Start()+s.Pic.Count()
}

// next moves the picture to its next keyframe.
func (s *PictureState) next() {
	s.Key++
	if s.Key >= len(s.Pic.Keys) {
		s.mover.TX, s.mover.TY = int(s.mover.X), int(s.mover.Y)
		return
	}
	k := s.Pic.Keys[s.Key]
	s.mover.TX, s.mover.TY = k.X, k.Y
	s.mover.Speed = speedOf(k.Speed, s.mover.Speed)
}

// stepFrame advances the frame (FUN_00342508's frame part).
func (s *PictureState) stepFrame(now time.Time) {
	if s.Key < len(s.Pic.Keys) {
		if f := s.Pic.Keys[s.Key].Frame; f != 0 {
			s.Frame = f
			return
		}
	}
	if s.repeats <= 0 {
		return
	}
	if now.Sub(s.frameAt) > time.Duration(s.Pic.Header[pictureInterval])*time.Millisecond {
		s.frameAt = now
		s.Frame++
	}
	rows, cols := s.Pic.Grid()
	if s.Frame > rows*cols {
		s.Frame = 1
		s.repeats--
	}
}
