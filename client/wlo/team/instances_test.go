package team

import (
	"testing"
	"wonderland-gonline/client/wlo/seui"
)

func TestInstanceRepliesValidateBeforeApplying(t *testing.T) {
	f := &Instances{Owner: &Form{State: &State{Self: 1}, Notice: func(string) {}}, Detail: &seui.Editor{}, Info: &seui.Editor{}, Leave: &seui.FixedButton{}, Start: &seui.FixedButton{}, New: &creationForm{}}
	for i := range f.Rows {
		f.Rows[i] = &seui.FixedButton{}
		f.RoomRows[i] = &seui.Editor{}
	}
	for i := range f.CreateRows {
		f.CreateRows[i] = &seui.FixedButton{}
		f.Labels[i] = &seui.Editor{}
	}
	// Use actual form fixture in app integration tests; parser validation must
	// reject before invoking layout or touching existing rows.
	f.Listings = []InstanceListing{{ID: 61501, Name: "Original"}}
	for _, p := range [][]byte{{85, 1, 1, 1, 6}, {85, 1, 1, 1, 1, 61, 240, 5, 'x'}, {85, 203, 0, 0, 0, 0, 1}, {85, 14, 2, 49, 117, 0}} {
		if f.Apply(p) {
			t.Fatal("accepted malformed", p)
		}
		if len(f.Listings) != 1 || f.Listings[0].Name != "Original" || f.Room != 0 {
			t.Fatal("partial state applied")
		}
	}
}
