package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
	"wonderland-go/internal/protocol"
)

type Config struct {
	StatusServerIDs []uint16 `json:"status_server_ids"`
	Name            string   `json:"name"`
	Login           string   `json:"login_address"`
	World           string   `json:"world_address"`
	Status          string   `json:"status_address"`
	HTTP            string   `json:"http_address"`
	Database        string   `json:"database"`
	AssetsDatabase  string   `json:"assets_database"`
	MaxConnections  int      `json:"max_connections"`
	IdleSeconds     int      `json:"idle_seconds"`
}

func Default() Config {
	return Config{StatusServerIDs: []uint16{protocol.StatusLegacyServerID, protocol.StatusDefaultServerID}, Name: "Wonderland Go", Login: "127.0.0.1:6414", World: "127.0.0.1:6415", Status: "127.0.0.1:6416", HTTP: "127.0.0.1:8080", Database: "var/wonderland.db", AssetsDatabase: "var/assets.db", MaxConnections: 512, IdleSeconds: 120}
}
func Load(path string) (Config, error) {
	c := Default()
	if path != "" {
		f, e := os.Open(path)
		if e != nil {
			return c, e
		}
		defer f.Close()
		d := json.NewDecoder(f)
		d.DisallowUnknownFields()
		if e = d.Decode(&c); e != nil {
			return c, e
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			return c, fmt.Errorf("trailing config data")
		}
	}
	if c.Name == "" || len(c.Name) > 200 || c.Database == "" || c.AssetsDatabase == "" {
		return c, fmt.Errorf("name, database and assets database are required")
	}
	gameplayPath, err := filepath.Abs(c.Database)
	if err != nil {
		return c, err
	}
	assetsPath, err := filepath.Abs(c.AssetsDatabase)
	if err != nil {
		return c, err
	}
	gameplayInfo, gameplayErr := os.Stat(gameplayPath)
	assetsInfo, assetsErr := os.Stat(assetsPath)
	if gameplayPath == assetsPath || (gameplayErr == nil && assetsErr == nil && os.SameFile(gameplayInfo, assetsInfo)) {
		return c, fmt.Errorf("database and assets_database must be separate files")
	}
	if c.MaxConnections < 1 || c.MaxConnections > 100000 || c.IdleSeconds < 1 || c.IdleSeconds > 86400 {
		return c, fmt.Errorf("invalid connection limits")
	}
	if len(c.StatusServerIDs) == 0 || len(c.StatusServerIDs) > protocol.StatusMaxServerRecords {
		return c, fmt.Errorf("status_server_ids must contain 1 to %d IDs", protocol.StatusMaxServerRecords)
	}
	statusIDs := map[uint16]bool{}
	for _, id := range c.StatusServerIDs {
		if id == 0 || id > protocol.StatusMaxServerID || statusIDs[id] {
			return c, fmt.Errorf("status_server_ids must be unique IDs from 1 to %d", protocol.StatusMaxServerID)
		}
		statusIDs[id] = true
	}
	seen := map[string]bool{}
	for _, a := range []string{c.Login, c.World, c.Status, c.HTTP} {
		_, port, e := net.SplitHostPort(a)
		if e != nil {
			return c, e
		}
		// Port zero asks the OS for a distinct available port per listener.
		if port == "0" {
			continue
		}
		if seen[a] {
			return c, fmt.Errorf("duplicate listen address %s", a)
		}
		seen[a] = true
	}
	return c, nil
}
func (c Config) IdleTimeout() time.Duration { return time.Duration(c.IdleSeconds) * time.Second }
