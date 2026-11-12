package inventory

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/game"
)

func TestAutomationShopUsesNativeCartAndPendingGate(t *testing.T) {
	f, sent := serviceFixture()
	env := &seui.Env{Pics: picdb.New()}
	seui.NewManager(env, &seui.Input{})
	f.InitFixedForm(f, env)
	env.UI.Add(f)
	f.Inventory.Env = env
	f.Inventory.Now = time.Now
	f.Visible = true
	f.Mode = ServiceBuy
	f.Gold = func() uint32 { return 1000 }
	f.Stock = []ShopEntry{{Slot: 5, ID: 42008, Price: 10}}
	invoke := func(action, raw string) error { return seui.InvokeAutomation(f, action, json.RawMessage(raw)) }
	if err := invoke("select", `{"Side":0,"Index":0}`); err != nil {
		t.Fatal(err)
	}
	if err := invoke("confirm", `null`); err != nil {
		t.Fatal(err)
	}
	if len(*sent) != 1 || !bytes.Equal((*sent)[0], []byte{27, 1, 1, 5, 1}) {
		t.Fatal("non-native shop packet", *sent)
	}
	if err := invoke("confirm", `null`); err == nil || len(*sent) != 1 {
		t.Fatal("duplicated resource request")
	}
	if f.Inventory.State.Bag != (game.Inventory{}) {
		t.Fatal("API granted unconfirmed items")
	}
}

func TestAutomationTradeZeroBasedWithdrawal(t *testing.T) {
	f := &TradeForm{}
	f.offer[1] = 2
	f.mine[1] = 1
	if err := f.AutomationInvoke("withdraw", json.RawMessage(`0`)); err != nil || f.offer[1] != 0 || f.mine[1] != 0 {
		t.Fatal("first offer cell not withdrawn", err)
	}
	f.offer[tradeCells] = 3
	f.mine[tradeCells] = tradeCells
	if err := f.AutomationInvoke("withdraw", json.RawMessage(`19`)); err != nil || f.offer[tradeCells] != 0 {
		t.Fatal("last offer cell not withdrawn", err)
	}
}

func TestAutomationRemoteRejectsTruncatedFixedArrays(t *testing.T) {
	// Validate the published contract before a decoded fixed array can silently
	// zero-fill omitted pet assignments or thresholds.
	info := seui.AutomationDescription(seui.AutomationInfo{Kind: "remote", Actions: []string{"configure"}})
	raw, _ := json.Marshal(RemoteOptions{SupplyFirst: 1, SupplyLast: game.BagSize})
	if err := seui.ValidateAutomationAction(info, "configure", raw); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	json.Unmarshal(raw, &fields)
	fields["Advanced"] = json.RawMessage(`[]`)
	bad, _ := json.Marshal(fields)
	if seui.ValidateAutomationAction(info, "configure", bad) == nil {
		t.Fatal("truncated actor configuration accepted")
	}
}
