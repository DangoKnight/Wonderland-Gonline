// Command quest-catalog-export produces an offline quest-research snapshot.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/assetsql"
	"wonderland-gonline/internal/world"
)

type actionIssue struct {
	Branch byte   `json:"branch"`
	Index  byte   `json:"index"`
	Code   byte   `json:"code"`
	D1     uint16 `json:"d1"`
	D2     uint16 `json:"d2"`
	D3     uint16 `json:"d3"`
	Value  uint32 `json:"value"`
	Reason string `json:"reason"`
}

type eventAudit struct {
	Map    uint16        `json:"map"`
	Event  uint16        `json:"event"`
	Marks  []uint16      `json:"marks"`
	Issues []actionIssue `json:"issues"`
}

func audit(c *assets.Catalog) []eventAudit {
	var result []eventAudit
	w := world.New(c)
	for mid, m := range c.Maps {
		for _, event := range m.Events {
			row := eventAudit{Map: mid, Event: event.ClickID}
			marks := map[uint16]bool{}
			for branchIndex, branch := range event.Branches {
				cond := world.DecodeCond(branch.Condition)
				if cond.Kind == world.ConditionQuest {
					marks[cond.W1] = true
				}
				for operationIndex, raw := range branch.Operations {
					op := world.DecodeOp(raw)
					if op.Code == world.ActionQuestMark {
						marks[op.D1] = true
					}
					reason := ""
					if op.Unsupported() && !w.QuestCompanionAction(mid, &event, branchIndex, operationIndex) {
						reason = "ordinary action unsupported; specialized interception requires separate verification"
					}
					if op.Code == world.ActionLearnSkill {
						if _, ok := c.Skills[op.D1]; !ok {
							reason = "missing native skill definition"
						}
					}
					if op.Code == world.ActionTransform {
						_, permanent := w.Transformation(mid, &event, branchIndex, op)
						_, disguise := w.QuestDisguise(mid, &event, branchIndex, op)
						if !permanent && !disguise {
							reason = "transformation lacks a verified permanent/disguise contract"
						}
					}
					if op.Code == world.ActionActor && (op.D2 == world.ActorActionAnimation || op.D2 == world.ActorActionExpression || op.D2 == world.ActorActionTurn || op.D2 == world.ActorActionConstellation) {
						raw := op.Raw()
						if _, ok := w.StoryFirst(assets.StoryActorCapability, nil, nil, world.StoryContext{Map: mid, Event: event.ClickID, Branch: branch.Index, Raw: &raw}); !ok {
							reason = "actor presentation lacks a reviewed capability"
						}
					}
					if op.Code == world.ActionBattle {
						if len(w.FormationMembers(mid, op.D2)) == 0 && op.D1 < world.MinMonsterTemplateID && op.D2 < world.MinMonsterTemplateID {
							reason = "missing native battle formation"
						}
					}
					if reason != "" {
						row.Issues = append(row.Issues, actionIssue{branch.Index, raw.Index, op.Code, op.D1, op.D2, op.D3, op.Value(), reason})
					}
				}
			}
			if len(marks) == 0 {
				continue
			}
			for mark := range marks {
				row.Marks = append(row.Marks, mark)
			}
			sort.Slice(row.Marks, func(i, j int) bool { return row.Marks[i] < row.Marks[j] })
			result = append(result, row)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Map < result[j].Map || result[i].Map == result[j].Map && result[i].Event < result[j].Event
	})
	return result
}

func main() {
	input := flag.String("assets", "var/assets.db", "current offline assets SQL database")
	output := flag.String("output", "var/quest-research/native-catalog.json", "research JSON snapshot, outside runtime definitions")
	flag.Parse()
	c, err := assetsql.LoadDatabase(*input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	f, err := os.Create(*output)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	err = json.NewEncoder(f).Encode(map[string]any{"maps": c.Maps, "quests": c.QuestDefinitions, "npcs": c.NPCs, "items": c.Items, "skills": c.Skills, "talks": c.Talks, "disabled": c.DisabledEvents, "marks": c.Marks, "event_audit": audit(c)})
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		fmt.Fprintln(os.Stderr, err, closeErr)
		os.Exit(1)
	}
}
