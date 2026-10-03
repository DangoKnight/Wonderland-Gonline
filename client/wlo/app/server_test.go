package app

import (
	"net"
	"os"
	"testing"
)

// TestLoginAgainstServer runs the client against a live server: status
// signals, the 1/9 description, a rejected and an accepted login. Set
// WLO_SERVER_HOST to the server's host (ports 6414 and 6416) and
// WLO_ACCOUNT/WLO_PASSWORD to a registered account.
func TestLoginAgainstServer(t *testing.T) {
	loginLive(t)
}

// liveAccount reads the live-server settings or skips.
func liveAccount(t *testing.T) (host, account, password string) {
	host, account, password = os.Getenv("WLO_SERVER_HOST"), os.Getenv("WLO_ACCOUNT"), os.Getenv("WLO_PASSWORD")
	if host == "" || account == "" || password == "" {
		t.Skip("WLO_SERVER_HOST, WLO_ACCOUNT and WLO_PASSWORD are not set")
	}
	return
}

// loginLive logs in against the live server and returns at the roster.
func loginLive(t *testing.T) *Client {
	host, account, password := liveAccount(t)
	c := testClient(t)
	c.Net.Dial = func(network, address string) (net.Conn, error) {
		_, port, _ := net.SplitHostPort(address)
		return net.Dial(network, net.JoinHostPort(host, port))
	}

	c.Frame()
	c.Servers.Regions.Selected = 0
	c.Servers.Regions.OnSelect(0)
	until(t, c, "a signal from the status service", func() bool { return c.Servers.Signals[0] != 0 })

	c.Servers.Connect(0)
	until(t, c, "the account form", func() bool { return c.Login.Visible })
	type_ := func(s string) {
		for _, b := range []byte(s) {
			c.UI.Char(b)
		}
		c.UI.KeyDown(0x0d, 0)
	}
	type_(account)
	type_(password + "x")
	until(t, c, "the wrong-password reply", func() bool { return c.Login.Visible })
	if len(c.Login.Account.Text) != 0 {
		t.Fatal("1/6 should clear the account field")
	}
	c.UI.Focus(c.Login.Account)
	type_(account)
	type_(password)
	until(t, c, "the character list", func() bool { return !c.Login.Visible && c.Login.LoginTime.IsZero() })
	return c
}

// TestCreateAgainstServer creates a character in slot 1 of an account
// without characters: 63/2, 1/3, the name check (9/2, 9/3), the wizard
// and 9/1, after which the server sends the world-entry packets.
// WLO_CREATE_NAME is the new character's name.
func TestCreateAgainstServer(t *testing.T) {
	_, _, password := liveAccount(t)
	name := os.Getenv("WLO_CREATE_NAME")
	if name == "" {
		t.Skip("WLO_CREATE_NAME is not set")
	}
	c := loginLive(t)
	var world [][]byte
	c.Unhandled = func(p []byte) { world = append(world, append([]byte(nil), p...)) }
	if c.Chars.Slots[1].Level != 0 {
		t.Fatal("slot 1 is not empty")
	}
	c.Chars.Manage[0].OnClickTag(c.Chars.Manage[0].Tag)
	cr := c.Create
	until(t, c, "the creation wizard (1/3)", func() bool { return cr.Visible && cr.Step == 1 })
	c.UI.Focus(cr.NameField)
	for _, b := range []byte(name) {
		c.UI.Char(b)
	}
	cr.RolePics[4].OnClickTag(4)
	cr.Buttons[2].Click()
	until(t, c, "the accepted name (9/3)", func() bool { return cr.Step == 2 })
	for range 5 {
		cr.StatArrows[6].Click()
	}
	cr.Buttons[2].Click() // to the element
	cr.Elements[3].OnClickTag(3)
	cr.Buttons[2].Click() // to the colours
	cr.Parts[1].OnClickTag(1)
	cr.ColorArrows[1].Click()
	cr.Buttons[2].Click() // to the password dialog
	if cr.Step != 5 || !c.Password.Visible {
		t.Fatalf("step %d, password dialog %v", cr.Step, c.Password.Visible)
	}
	// The server checks the first pair against the account password.
	for i, v := range []string{password, password, "secret1", "secret1"} {
		c.Password.Fields[i+1].SetText([]byte(v))
	}
	c.Password.OK.OnClick()
	until(t, c, "the world-entry packets", func() bool { return len(world) > 10 || c.disconnectText != nil })
	if c.disconnectText != nil {
		t.Fatalf("server refused the character: %s", c.disconnectText)
	}
	if cr.Visible || c.Password.Visible {
		t.Fatal("the creation forms should close after 9/1")
	}
	t.Logf("world entry: %d packets, first % x", len(world), world[0])
}

// TestDeleteAgainstServer deletes the character in slot 1 through the
// password dialog (35/2). WLO_DELETE_CODE is the character's secret code.
func TestDeleteAgainstServer(t *testing.T) {
	_, _, password := liveAccount(t)
	code := os.Getenv("WLO_DELETE_CODE")
	if code == "" {
		t.Skip("WLO_DELETE_CODE is not set")
	}
	c := loginLive(t)
	if c.Chars.Slots[1].Level == 0 {
		t.Fatal("slot 1 is empty")
	}
	c.Chars.Manage[0].OnClickTag(c.Chars.Manage[0].Tag)
	if !c.Password.Visible || c.Chars.Visible {
		t.Fatal("Delete should open the dialog over a hidden selection")
	}
	for i, v := range []string{password, password, code, code} {
		c.Password.Fields[i+1].SetText([]byte(v))
	}
	c.Password.OK.OnClick()
	until(t, c, "the deletion result (35/2)", func() bool { return c.Chars.Slots[1].Level == 0 })
	if c.Password.Visible || !c.Chars.Visible {
		t.Fatal("the selection should return after the dialog")
	}
}
