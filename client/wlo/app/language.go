package app

import (
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/localization"
)

// Prepare without changing active presentation or gameplay state. The settings
// file can be committed before this non-failing update runs on the game thread.
func (w *Workspace) prepareLanguage(language string) (func(), error) {
	code, err := localization.Normalize(language)
	if err != nil {
		return nil, err
	}
	r := w.resources
	if code == r.language {
		return func() {}, nil
	}
	catalog, err := localization.Load(r.root, code)
	if err != nil {
		return nil, err
	}
	a := w.Current().Assets
	skins := r.languageSkins[code]
	if skins == nil {
		skins = picdb.New()
		for _, root := range a.UISkinRoots(code) {
			for _, name := range []string{"default", "white"} {
				if err := skins.LoadDir(login.Path(root, name), false, 0); err != nil {
					return nil, err
				}
			}
		}
		if r.languageSkins == nil {
			r.languageSkins = map[string]*picdb.DB{}
		}
		r.languageSkins[code] = skins
	}
	return func() {
		localization.Install(catalog)
		w.options.Language = ""
		r.pics.ReplaceSkins(skins)
		r.language = code
		set := func(table string, id uint32, field string, target *string) {
			if text, ok := catalog.Find(localization.Key(table, id, field)); ok {
				*target = text
			}
		}
		for id, item := range r.items {
			set("item", uint32(id), "name", &item.Definition.Name)
			set("item", uint32(id), "description", &item.Description)
			r.items[id] = item
		}
		if r.skillCatalog != nil {
			for id, skill := range r.skillCatalog.Definitions {
				set("skill", uint32(id), "name", &skill.Name)
				set("skill", uint32(id), "description", &skill.Description)
				r.skillCatalog.Definitions[id] = skill
			}
		}
		for id, npc := range r.npcTemplates {
			set("npc", id, "name", &npc.Name)
			r.npcTemplates[id] = npc
		}
		for id, name := range r.sceneNames {
			set("scene", uint32(id), "name", &name)
			r.sceneNames[id] = name
		}
		for reference, id := range r.talkIDs {
			value := r.talks[reference]
			set("talk", id, "text", &value)
			r.talks[reference] = value
		}
		for i := range r.instanceDefinitions {
			d := &r.instanceDefinitions[i]
			set("scene", uint32(d.ID), "name", &d.Name)
			if d.MarkID != 0 {
				set("mark", uint32(d.MarkID), "name", &d.Name)
				set("mark", uint32(d.MarkID), "description", &d.Description)
			}
		}
		for _, session := range w.Sessions {
			c := session.Client
			c.options.Language = "" // Future sessions inherit the current workspace choice.
			if c.World != nil {
				c.World.SceneName = r.sceneNames[c.mapScenes[c.World.Player.Map]]
				for _, npc := range c.World.NPCs {
					if template, ok := r.npcTemplates[npc.Template]; ok {
						npc.Info.Name = template.Name
					}
				}
				for _, pet := range c.World.Companions {
					if template, ok := r.npcTemplates[uint32(pet.ID)]; ok {
						pet.Info.Name = template.Name
					}
				}
			}
			for _, body := range c.battle.bodies {
				if body.nameKey != "" {
					if name, ok := catalog.Find(body.nameKey); ok {
						body.name = clientassets.Big5Text(name)
					}
				}
			}
			if c.movie != nil {
				for _, npc := range c.movie.npcs {
					if template, ok := r.npcTemplates[npc.Template]; ok {
						npc.Info.Name = template.Name
					}
				}
			}
			c.UI.RefreshLanguage()
			if c.Notebook != nil && c.notebookState.Marks != nil {
				c.loadNotebook()
			}
			if c.Skills != nil {
				c.Skills.Refresh()
			}
			if c.BattleSkills != nil {
				c.BattleSkills.Refresh()
			}
			if c.HotKeys != nil {
				c.HotKeys.Refresh()
			}
			if c.Team != nil {
				c.Team.Refresh()
				c.Team.Instances.Refresh()
			}
			if c.Talk != nil {
				c.Talk.RefreshLanguage()
			}
			if c.World != nil && c.areaTitle.name != "" {
				c.areaTitle.name = c.World.SceneName
				c.areaTitle.makeArtwork(c.Env.Text)
			}
		}
	}, nil
}
