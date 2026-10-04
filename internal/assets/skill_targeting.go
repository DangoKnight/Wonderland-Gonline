package assets

import "fmt"

const (
	MaxSkillGrade        = 10
	MaxBattleSideTargets = 8
	MaxFormationOffset   = 3
)

// SkillTargeting is an explicit, inclusive grade range. Offsets are relative to
// the selected fighter on its own side; All includes that entire side.
// Native pattern codes remain provenance until their shapes are verified.
type SkillTargeting struct {
	MinGrade byte              `json:"min_grade"`
	MaxGrade byte              `json:"max_grade"`
	All      bool              `json:"all,omitempty"`
	Offsets  []FormationOffset `json:"offsets,omitempty"`
}
type FormationOffset struct {
	X int8 `json:"x"`
	Y int8 `json:"y"`
}

func (s Skill) TargetingAt(grade int) (SkillTargeting, bool) {
	grade = min(MaxSkillGrade, max(1, grade))
	for _, shape := range s.Targeting {
		if grade >= int(shape.MinGrade) && grade <= int(shape.MaxGrade) {
			return shape, true
		}
	}
	return SkillTargeting{}, false
}
func ValidateSkillTargeting(shapes []SkillTargeting) error {
	var previous byte
	for _, shape := range shapes {
		if shape.MinGrade < 1 || shape.MinGrade <= previous || shape.MaxGrade < shape.MinGrade || shape.MaxGrade > MaxSkillGrade {
			return fmt.Errorf("invalid or overlapping skill targeting grades")
		}
		previous = shape.MaxGrade
		if shape.All {
			if len(shape.Offsets) != 0 {
				return fmt.Errorf("all-target shape cannot have offsets")
			}
			continue
		}
		if len(shape.Offsets) < 1 || len(shape.Offsets) > MaxBattleSideTargets {
			return fmt.Errorf("invalid targeting offset count")
		}
		seen := map[FormationOffset]bool{}
		for _, offset := range shape.Offsets {
			if offset.X < -MaxFormationOffset || offset.X > MaxFormationOffset || offset.Y < -MaxFormationOffset || offset.Y > MaxFormationOffset || seen[offset] {
				return fmt.Errorf("invalid or duplicate formation offset")
			}
			seen[offset] = true
		}
		if !seen[FormationOffset{}] {
			return fmt.Errorf("targeting must include selected fighter")
		}
	}
	return nil
}
