package app

import (
	"fmt"
	"image"
	"strings"
	"unicode"
	"unicode/utf8"
)

const sessionTitleMaximumRunes = 40
const titleKeyEnter = 0x0d
const titleKeyEscape = 0x1b
const titleKeyBackspace = 0x08

func (s *Session) DisplayTitle() string {
	if s.Title != "" {
		return s.Title
	}
	return fmt.Sprintf("Session %d", s.ID)
}
func validSessionTitle(title string) bool {
	if !utf8.ValidString(title) || utf8.RuneCountInString(title) > sessionTitleMaximumRunes {
		return false
	}
	for _, r := range title {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func (w *Workspace) titleRect(row int) image.Rectangle {
	r := w.cardRect(row)
	r.Max.Y = r.Min.Y + sessionPreviewTop
	r.Max.X -= sessionCloseWidth
	return r
}
func (w *Workspace) beginTitle(index int) {
	s := w.Sessions[index]
	w.titleEditing, w.titleDraft, w.titleReplace = s.ID, s.DisplayTitle(), true
	w.PendingRemove = 0
}
func (w *Workspace) finishTitle(save bool) {
	if save {
		for _, s := range w.Sessions {
			if s.ID == w.titleEditing {
				s.Title = strings.TrimSpace(w.titleDraft)
				if s.Title == fmt.Sprintf("Session %d", s.ID) {
					s.Title = ""
				}
				break
			}
		}
	}
	w.titleEditing, w.titleDraft, w.titleReplace = 0, "", false
}
func (w *Workspace) titleKey(key uint16) {
	switch key {
	case titleKeyEnter:
		w.finishTitle(true)
	case titleKeyEscape:
		w.finishTitle(false)
	case titleKeyBackspace:
		if w.titleReplace {
			w.titleDraft = ""
		} else if len(w.titleDraft) > 0 {
			_, size := utf8.DecodeLastRuneInString(w.titleDraft)
			w.titleDraft = w.titleDraft[:len(w.titleDraft)-size]
		}
		w.titleReplace = false
	}
}
func (w *Workspace) titleChar(r rune) {
	if w.titleEditing == 0 || !unicode.IsPrint(r) {
		return
	}
	if w.titleReplace {
		w.titleDraft = ""
		w.titleReplace = false
	}
	if utf8.RuneCountInString(w.titleDraft) < sessionTitleMaximumRunes {
		w.titleDraft += string(r)
	}
}
