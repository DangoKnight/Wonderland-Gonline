package session

import (
	"os"
	"testing"
	"time"
)

func TestParseStatus(t *testing.T) {
	got, err := ParseStatus([]byte{0xc9, 0, 1, 1, 0, 2, 101, 0, 3})
	if err != nil || len(got) != 2 || got[1] != 2 || got[101] != 3 {
		t.Fatal(got, err)
	}
	if _, err := ParseStatus([]byte{1, 2, 3}); err == nil {
		t.Fatal("accepted a foreign reply")
	}
}

func TestClassify(t *testing.T) {
	r, reject := Classify([]byte{63, 2}, false)
	if r != LoginPending || !reject {
		t.Fatal(r, reject)
	}
	if r, _ := Classify([]byte{1, 6}, true); r != LoginRejected {
		t.Fatal(r)
	}
	if r, _ := Classify([]byte{0, 19}, true); r != LoginDuplicate {
		t.Fatal(r)
	}
	if r, _ := Classify([]byte{63, 2, 1, 0, 0, 0}, false); r != LoginOK {
		t.Fatal(r)
	}
}

// TestLiveLogin runs against a server named by WONDERLAND_LIVE_HOST with an
// account WONDERLAND_LIVE_ACCOUNT / WONDERLAND_LIVE_PASSWORD.
func TestLiveLogin(t *testing.T) {
	host := os.Getenv("WONDERLAND_LIVE_HOST")
	if host == "" {
		t.Skip("WONDERLAND_LIVE_HOST not set")
	}
	signals, err := Status(host, 3*time.Second)
	if err != nil || len(signals) == 0 {
		t.Fatal(signals, err)
	}
	for _, tc := range []struct {
		password string
		want     LoginResult
	}{{"wrong-password", LoginRejected}, {os.Getenv("WONDERLAND_LIVE_PASSWORD"), LoginOK}} {
		c, err := Dial(host, 3*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Login([]byte(os.Getenv("WONDERLAND_LIVE_ACCOUNT")), []byte(tc.password)); err != nil {
			t.Fatal(err)
		}
		reject, result := false, LoginPending
		timeout := time.After(5 * time.Second)
		for result == LoginPending {
			select {
			case p, ok := <-c.Packets():
				if !ok {
					t.Fatal("connection closed:", c.Err())
				}
				result, reject = Classify(p, reject)
			case <-timeout:
				t.Fatal("no login reply")
			}
		}
		c.Close()
		if result != tc.want {
			t.Errorf("password %q: got %v, want %v", tc.password, result, tc.want)
		}
	}
}
