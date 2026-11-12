package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"wonderland-gonline/client/wlo/seui"
)

func agentNotificationActions() []agentAction {
	return []agentAction{
		agentActionDef("notice.dismiss", "visible timed notice; does not advance NPC dialogue", nil, func(c *Client, _ map[string]any) error {
			if !c.Notices.shown {
				return fmt.Errorf("no notice is open")
			}
			c.Notices.dismiss(c.Env, c.Now())
			return nil
		}),
		agentActionDef("battle.resultDismiss", "visible battle summary", nil, func(c *Client, _ map[string]any) error {
			f := c.battleResults.form
			if f == nil || !f.Visible {
				return fmt.Errorf("no battle summary is open")
			}
			f.Hide()
			return nil
		}),
	}
}

func (f *battleResultsForm) AutomationInvoke(action string, raw json.RawMessage) error {
	if action != "close" {
		return fmt.Errorf("unsupported battle summary action")
	}
	f.Hide()
	return nil
}

func (f *battleResultsForm) AutomationInfo() seui.AutomationInfo {
	lines := make([]string, len(f.list.Items))
	for i, b := range f.list.Items {
		lines[i] = seui.DecodeText(b)
	}
	return seui.AutomationInfo{Kind: "battleResults", Text: strings.Join(lines, "\n"), Actions: []string{"close"}}
}
