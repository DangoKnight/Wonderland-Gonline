package minigame

import (
	"fmt"
	"image"
	"sort"
	"time"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/surface"
)

// Memory is the numbered sequence game, not matching pairs. FUN_0014a558,
// FUN_0014a98c, FUN_0014ab68/FUN_0014a348 and FUN_0014ac84 show 3..9 unique
// digits briefly, hide them and require clicks in ascending order. Twelve
// completed rounds wins; a wrong click or the ten-second deadline costs a life.
type Memory struct {
	Round
	Score, Lives int
	phase        memoryPhase
	cards        []memoryCard
	next         int
	deadline     time.Time
}
type memoryCard struct {
	Value, X, Y int
	Taken       bool
}
type memoryPhase byte

const (
	memoryCountdown memoryPhase = iota
	memoryPreview
	memoryPlaying
	memoryCorrect
	memoryWrong
	memoryWinRounds   = 12
	memoryRoundLimit  = 10 * time.Second
	memoryPreviewTime = time.Second
	memoryCorrectWait = 2500 * time.Millisecond
	memoryWrongWait   = 3 * time.Second
	memoryCardWidth   = 38
	memoryCardHeight  = 37
	memoryLeft        = 201
	memoryTop         = 101
	memoryAreaWidth   = 362
	memoryAreaHeight  = 263
	memoryMaxCards    = 9
)

func NewMemory(pics *picdb.DB) *Memory {
	return &Memory{Round: newRound(pics), Lives: arcadeLives}
}
func (m *Memory) Pictures() []string {
	return []string{"CJ_BG1", "little_circle", "Icon_X_1", "Icon_O_1", "Num_White5_1", "icon_heart"}
}
func (m *Memory) Begin(now time.Time) {
	if m.Started || m.Done() {
		return
	}
	m.Round.Begin(now)
	m.newCards()
	m.phase, m.deadline = memoryCountdown, now.Add(arcadeCountdown)
}
func (m *Memory) newCards() {
	n := min(3+m.Score/4, memoryMaxCards)
	values := make([]int, memoryMaxCards)
	for i := range values {
		values[i] = i + 1
	}
	for i := len(values) - 1; i > 0; i-- {
		j := m.Rand(i + 1)
		values[i], values[j] = values[j], values[i]
	}
	values = values[:n]
	sort.Ints(values)
	m.cards = nil
	for i, v := range values {
		card := memoryCard{Value: v}
		placed := false
		for range 500 {
			card.X, card.Y = memoryLeft+m.Rand(memoryAreaWidth), memoryTop+m.Rand(memoryAreaHeight)
			placed = true
			for _, other := range m.cards {
				if abs(card.X-other.X) < memoryCardWidth && abs(card.Y-other.Y) < memoryCardHeight {
					placed = false
					break
				}
			}
			if placed {
				break
			}
		}
		// Bounded placement also works with a deterministic or pathological RNG.
		if !placed {
			card.X, card.Y = memoryLeft+i*memoryCardWidth, memoryTop
		}
		m.cards = append(m.cards, card)
	}
	m.next = 0
}
func (m *Memory) Update(now time.Time) {
	if !m.ready(now) {
		return
	}
	if now.Before(m.deadline) {
		return
	}
	switch m.phase {
	case memoryCountdown:
		m.phase, m.deadline = memoryPreview, now.Add(memoryPreviewTime)
	case memoryPreview:
		m.phase, m.deadline = memoryPlaying, now.Add(memoryRoundLimit-memoryPreviewTime)
	case memoryPlaying:
		m.mistake(now)
	case memoryCorrect:
		if m.Score >= memoryWinRounds {
			m.finish(true, now)
			return
		}
		m.newCards()
		m.phase, m.deadline = memoryCountdown, now.Add(arcadeCountdown)
	case memoryWrong:
		if m.Lives == 0 {
			m.finish(false, now)
			return
		}
		m.newCards()
		m.phase, m.deadline = memoryCountdown, now.Add(arcadeCountdown)
	}
}
func (m *Memory) mistake(now time.Time) {
	m.Lives--
	m.phase, m.deadline = memoryWrong, now.Add(memoryWrongWait)
	m.sound("sound\\wav1607.wav")
}
func (m *Memory) Click(x, y int, now time.Time) {
	m.Update(now)
	if !m.ready(now) || m.phase != memoryPlaying {
		return
	}
	for i := range m.cards {
		c := &m.cards[i]
		if c.Taken || !image.Pt(x, y).In(image.Rect(c.X, c.Y, c.X+memoryCardWidth, c.Y+memoryCardHeight)) {
			continue
		}
		c.Taken = true
		if i != m.next {
			m.mistake(now)
			return
		}
		m.next++
		if m.next == len(m.cards) {
			m.Score++
			m.phase, m.deadline = memoryCorrect, now.Add(memoryCorrectWait)
		}
		return
	}
}
func (m *Memory) Draw(dst *surface.Surface, _, _ int, now time.Time) {
	m.background(dst, "CJ_BG1")
	arcadeText(dst, 30, 15, fmt.Sprintf("Memory  Lives: %d  Score: %d/%d", m.Lives, m.Score, memoryWinRounds))
	arcadeText(dst, 200, 450, "Remember the numbers, then click from lowest to highest.")
	if !m.Started {
		return
	}
	for _, c := range m.cards {
		if c.Taken && m.phase != memoryWrong {
			continue
		}
		m.picture(dst, "little_circle", c.X, c.Y)
		dst.Frame(image.Rect(c.X, c.Y, c.X+memoryCardWidth, c.Y+memoryCardHeight), arcadeWhite)
		if m.phase != memoryPlaying || c.Taken {
			arcadeText(dst, c.X+14, c.Y+12, fmt.Sprint(c.Value))
		}
	}
	if m.phase == memoryPlaying {
		arcadeText(dst, 620, 15, fmt.Sprintf("Time: %d", max(0, int(m.deadline.Sub(now).Seconds()))))
	}
	m.outcome(dst)
}
