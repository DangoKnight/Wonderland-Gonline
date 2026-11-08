package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/protocol"
)

func TestMessageBoardComposeFocusAndWindowDragging(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.dispatch([]byte{82, 9})
	b := c.messageBoard
	b.pending = time.Time{}
	for _, child := range b.Children {
		if button, ok := child.(*seui.FixedButton); ok && button.Name == "Btn_AddNew_1" {
			button.Click()
		}
	}
	f := b.compose
	if !f.Visible || f.Image != c.Pics.Find("notice1") || f.Width != 417 || f.Height != 251 || c.Input.Focused != f.title {
		t.Fatal("wrong composer template or initial focus")
	}
	click := func(control seui.Control) {
		at := control.Base().Abs()
		c.Input.X, c.Input.Y = at.X+8, at.Y+8
		c.Input.Hovered = nil
		c.UI.Draw()
		if c.Input.Hovered != control {
			t.Fatalf("input not selectable: %T", c.Input.Hovered)
		}
		c.UI.MouseDown(seui.ButtonLeft, 0, c.Input.X, c.Input.Y)
		c.UI.MouseUp(seui.ButtonLeft, 0, c.Input.X, c.Input.Y)
	}
	click(f.title)
	c.UI.Char('S')
	click(f.body)
	c.UI.Char('B')
	if string(f.title.Text) != "S" || string(f.body.Text) != "B" || f.Dragging {
		t.Fatal("typing did not reach the selected field")
	}
	for _, window := range []seui.Control{b, b.detail, b.compose} {
		c.closeMessageBoard()
		window.Show()
		base := window.Base()
		x, y := base.Left+100, base.Top+30
		c.Input.X, c.Input.Y, c.Input.Hovered = x, y, nil
		c.UI.Draw()
		left, top := base.Left, base.Top
		c.UI.MouseDown(seui.ButtonLeft, 0, x, y)
		if c.Input.Captured != window {
			t.Fatal("window did not capture drag")
		}
		c.Input.Hovered = f.body // Crossing a child must not release the gesture.
		c.UI.MouseMove(0, x+12, y+8, true)
		if base.Left != left+12 || base.Top != top+8 {
			t.Fatal("window did not move")
		}
		c.UI.MouseUp(seui.ButtonLeft, 0, x+12, y+8)
		if c.Input.Captured != nil {
			t.Fatal("window retained drag after release")
		}
	}
}

func TestMessageBoardWindowsAndNativePackets(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	c.dispatch([]byte{82, 9})
	b := c.messageBoard
	if !b.Visible || b.Width != 654 || b.Height != 365 || b.Image < 0 {
		t.Fatal("missing native board window")
	}
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{82, 2, 1, 1}) {
		t.Fatal(got)
	}
	c.dispatch([]byte{82, 4, 1, 0, 0, 0, 1})
	// One server row, independent native lengths and command values.
	entry := protocol.Builder{82, 1}.U32(6).Bytes([]byte("Tester")).U32(7).Bytes([]byte("Welcome")).F64(25569).U32(20002).U32(7)
	c.dispatch(entry)
	if len(b.entries) != 1 || b.entries[0].id != 7 || b.pages() != 1 {
		t.Fatal(b.entries)
	}
	c.Frame()
	if dir := os.Getenv("BOARD_SNAPSHOT_DIR"); dir != "" {
		savePNG(t, filepath.Join(dir, "board-list.png"), c)
	}
	for _, child := range b.Children {
		if row, ok := child.(*boardRow); ok && row.index == 0 {
			row.LeftUp(0, 0, 0)
		}
	}
	if got := sent(); !bytes.Equal(got[len(got)-1], []byte{82, 3, 7, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 64, 248, 216, 64, 1}) {
		t.Fatal(got)
	}
	detail := protocol.Builder{82, 2}.U32(6).Bytes([]byte("Tester")).U32(7).Bytes([]byte("Welcome")).F64(25569).U32(17).U32(20002).Bytes([]byte("Welcome to town."))
	// The exact body is 16 bytes; a bad length must not open a partial notice.
	c.dispatch(detail)
	if b.detail.Visible {
		t.Fatal("malformed detail was displayed")
	}
	detail = protocol.Builder{82, 2}.U32(6).Bytes([]byte("Tester")).U32(7).Bytes([]byte("Welcome")).F64(25569).U32(16).U32(20002).Bytes([]byte("Welcome to town."))
	c.dispatch(detail)
	if !b.Visible || !b.detail.Visible || b.detail.remove.Enabled || !b.detail.mail.Enabled {
		t.Fatal("incorrect notice/author controls")
	}
	if string(b.detail.body.Text) != "Welcome to town." {
		t.Fatal(b.detail.body.Text)
	}
	c.Frame()
	if dir := os.Getenv("BOARD_SNAPSHOT_DIR"); dir != "" {
		savePNG(t, filepath.Join(dir, "board-detail.png"), c)
	}
	b.detail.mail.Click()
	if !c.MailSend.Visible {
		t.Fatal("send mail did not open")
	}
	c.MailSend.Hide()
	b.detail.Hide()
	b.compose.Show()
	b.compose.title.SetText([]byte("Subject"))
	b.compose.body.SetText([]byte("Message"))
	c.Frame()
	if dir := os.Getenv("BOARD_SNAPSHOT_DIR"); dir != "" {
		savePNG(t, filepath.Join(dir, "board-compose.png"), c)
	}
	b.compose.confirm.Click()
	want := []byte{82, 1, 7, 0, 0, 0, 'S', 'u', 'b', 'j', 'e', 'c', 't', 7, 0, 0, 0, 'M', 'e', 's', 's', 'a', 'g', 'e', 1}
	if got := sent(); !bytes.Equal(got[len(got)-1], want) {
		t.Fatal(got)
	}
	c.dispatch([]byte{82, 5})
	if !b.compose.Visible || !b.compose.confirm.Enabled || string(b.compose.body.Text) != "Message" {
		t.Fatal("failed post lost draft")
	}
	c.dispatch([]byte{82, 8, 1})
	if b.compose.Visible || !b.Visible {
		t.Fatal("native post acknowledgement must close only the composer")
	}
	c.closeMessageBoard()
	c.dispatch(detail)
	if b.Visible || b.detail.Visible || b.compose.Visible {
		t.Fatal("late response reopened closed board")
	}
}

func TestMessageBoardLegacyReadCapture(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	c.dispatch([]byte{82, 9})
	c.dispatch([]byte{82, 4, 1, 0, 0, 0, 1})
	// Exact public-board row and selection observed on Rhodes3. Distinct author
	// (12021) and message (1) IDs independently pin their native ordering.
	c.dispatch([]byte{82, 1, 8, 0, 0, 0, 169, 250, 195, 232, 164, 232, 184, 173, 4, 0, 0, 0, 171, 162, 197, 111, 64, 107, 205, 32, 153, 158, 230, 64, 245, 46, 0, 0, 1, 0, 0, 0})
	b := c.messageBoard
	(&boardRow{board: b, index: 0}).LeftUp(0, 0, 0)
	want := []byte{82, 3, 1, 0, 0, 0, 1, 0, 0, 0, 64, 107, 205, 32, 153, 158, 230, 64, 1}
	if got := sent(); !bytes.Equal(got[len(got)-1], want) {
		t.Fatal("read request differs from capture", got)
	}
	c.dispatch([]byte{82, 2, 8, 0, 0, 0, 169, 250, 195, 232, 164, 232, 184, 173, 4, 0, 0, 0, 171, 162, 197, 111, 64, 107, 205, 32, 153, 158, 230, 64, 5, 0, 0, 0, 245, 46, 0, 0, 171, 162, 197, 111, 126})
	if !b.detail.Visible || b.selected.id != 1 || b.selected.author != 12021 || !b.detail.mail.Enabled || b.detail.remove.Enabled {
		t.Fatal("captured notice lost message/author distinction", b.selected)
	}
	b.detail.mail.Click()
	if !c.MailSend.Visible {
		t.Fatal("captured notice cannot open sender mail")
	}
}

func TestMessageBoardMalformedPageKeepsEntries(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.messageBoard.Visible = true
	c.messageBoard.entries = []boardEntry{{id: 12}}
	for _, p := range [][]byte{{82, 1, 255, 255, 255, 255}, {82, 3, 0, 0, 0, 0, 0}, {82, 2, 255, 255, 255, 255}} {
		c.dispatch(p)
	}
	if len(c.messageBoard.entries) != 1 || c.messageBoard.entries[0].id != 12 {
		t.Fatal("malformed page replaced current entries")
	}
}
