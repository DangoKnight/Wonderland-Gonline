package assets

type SkillEvolution struct {
	FromSkill     uint16 `json:"from_skill"`
	ToSkill       uint16 `json:"to_skill"`
	RequiredGrade byte   `json:"required_grade"`
}
