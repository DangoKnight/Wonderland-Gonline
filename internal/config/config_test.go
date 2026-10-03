package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestListenAddressUniqueness(t *testing.T) {
	for _, tc := range []struct {
		port  string
		valid bool
	}{{"0", true}, {"6414", false}} {
		t.Run(tc.port, func(t *testing.T) {
			c := Default()
			c.Login = "127.0.0.1:" + tc.port
			c.World = c.Login
			b, err := json.Marshal(c)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, b, 0600); err != nil {
				t.Fatal(err)
			}
			_, err = Load(path)
			if (err == nil) != tc.valid {
				t.Fatal("listen address validation", err)
			}
		})
	}
}

func TestAssetsDatabaseConfiguration(t *testing.T) {
	if Default().AssetsDatabase != "var/assets.db" {
		t.Fatal("wrong default assets database")
	}
	for _, value := range []string{"", "var/custom-assets.db"} {
		c := Default()
		c.AssetsDatabase = value
		data, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "config.json")
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path)
		if value == "" {
			if err == nil {
				t.Fatal("empty assets database accepted")
			}
		} else if err != nil || got.AssetsDatabase != value {
			t.Fatal("assets database path lost", err)
		}
	}
}

func TestStatusServerIDs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ids   []uint16
		valid bool
	}{
		{"local region", []uint16{1, 101, 301}, true},
		{"empty", nil, false},
		{"zero", []uint16{0}, false},
		{"outside native table", []uint16{10000}, false},
		{"duplicate", []uint16{301, 301}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Default()
			c.StatusServerIDs = tc.ids
			data, err := json.Marshal(c)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "config.json")
			if err = os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			got, err := Load(path)
			if (err == nil) != tc.valid {
				t.Fatalf("status IDs validation: %v", err)
			}
			if tc.valid && len(got.StatusServerIDs) != len(tc.ids) {
				t.Fatal("status IDs lost")
			}
		})
	}
}

func TestDatabaseRolesMustRemainSeparate(t *testing.T) {
	root := t.TempDir()
	gameplay := filepath.Join(root, "gameplay.db")
	if err := os.WriteFile(gameplay, nil, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias.db")
	if err := os.Symlink(gameplay, alias); err != nil {
		t.Fatal(err)
	}
	for _, assetPath := range []string{gameplay, filepath.Join(root, "sub", "..", "gameplay.db"), alias} {
		c := Default()
		c.Database = gameplay
		c.AssetsDatabase = assetPath
		raw, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "config.json")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatal("same database accepted", assetPath)
		}
	}
}
