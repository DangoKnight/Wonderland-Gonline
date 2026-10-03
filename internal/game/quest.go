package game

import (
	"errors"
	"time"
)

type QuestState byte

const (
	NotStarted QuestState = iota
	InProgress
	Completed
	Failed
)

type Quest struct {
	ID          uint32     `json:"id"`
	State       QuestState `json:"state"`
	Step        int        `json:"step"`
	Kills       int        `json:"kills"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func (q *Quest) Advance(step int) error {
	if q.State != InProgress || step <= q.Step {
		return errors.New("quest cannot move backwards or advance outside progress")
	}
	q.Step = step
	return nil
}
func (q *Quest) Complete(now time.Time) bool {
	if q.State != InProgress {
		return false
	}
	q.State = Completed
	t := now.UTC()
	q.CompletedAt = &t
	return true
}

// GrantQuestReward commits the completed flag and all item rewards together.
// Persistence must commit the containing character as one transaction.
func (c *Character) GrantQuestReward(id uint32, rewards []Item, maxStack func(uint16) byte, now time.Time) error {
	q, ok := c.Quests[id]
	if !ok || q.State != InProgress {
		return errors.New("quest is not in progress")
	}
	bag := c.Bag
	for _, item := range rewards {
		if e := bag.Add(item, int(item.Count), maxStack(item.ID)); e != nil {
			return e
		}
	}
	q.Complete(now)
	c.Bag = bag
	c.Quests[id] = q
	return nil
}
