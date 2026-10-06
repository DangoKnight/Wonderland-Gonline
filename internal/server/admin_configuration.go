package server

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/config"
)

type StartupConfiguration struct {
	Engine          string        `json:"engine"`
	Path            string        `json:"path"`
	Version         string        `json:"version"`
	Configuration   config.Config `json:"configuration"`
	Running         config.Config `json:"running"`
	RestartRequired bool          `json:"restart_required"`
}

func (s *Server) SetConfigurationPath(path string) { s.configPath = path }
func (s *Server) startupConfigurationLocked() (StartupConfiguration, error) {
	path := s.configPath
	if path == "" {
		path = "config.admin.json"
	}
	raw, err := os.ReadFile(path)
	version := "missing"
	next := s.Config
	if err == nil {
		version = assetdb.DocumentVersion(raw)
		next, err = config.Load(path)
		if err != nil {
			return StartupConfiguration{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return StartupConfiguration{}, err
	}
	return StartupConfiguration{Engine: "SQLite", Path: path, Version: version, Configuration: next, Running: s.Config, RestartRequired: true}, nil
}
func (s *Server) StartupConfiguration() (StartupConfiguration, error) {
	s.adminEditMu.Lock()
	defer s.adminEditMu.Unlock()
	return s.startupConfigurationLocked()
}
func (s *Server) SaveStartupConfiguration(version string, next config.Config) error {
	s.adminEditMu.Lock()
	defer s.adminEditMu.Unlock()
	if err := config.Validate(next); err != nil {
		return err
	}
	current, err := s.startupConfigurationLocked()
	if err != nil {
		return err
	}
	if version != current.Version {
		return errors.New("configuration changed; refresh before saving")
	}
	raw, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	file, err := os.CreateTemp(filepath.Dir(current.Path), ".wonderland-config-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(raw); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, current.Path); err != nil {
		return err
	}
	s.configPath = current.Path
	s.Log.Info("startup configuration saved", "path", current.Path, "restart_required", true)
	return nil
}
