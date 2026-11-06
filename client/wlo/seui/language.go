package seui

// BindLanguage retains an explicit presentation producer on its owning control.
// It is replaced per property, never registered globally, so disposed dialogs
// and their captured data can be collected normally.
func (g *GrBasic) BindLanguage(property string, update func()) {
	update()
	if g.language == nil {
		g.language = map[string]func(){}
	}
	g.language[property] = update
}

func (g *GrBasic) ClearLanguage(property string) { delete(g.language, property) }

func (g *GrBasic) RefreshLanguage() {
	updates := make(map[string]func(), len(g.language))
	for key, fn := range g.language {
		updates[key] = fn
	}
	for key, fn := range updates {
		g.BindLanguage(key, fn)
	}
	for _, child := range g.Children {
		child.Base().RefreshLanguage()
	}
}

func (m *Manager) RefreshLanguage() {
	for _, form := range m.Forms {
		form.Base().RefreshLanguage()
	}
	if m.Env != nil {
		m.Env.HintOwner = nil
	}
}
