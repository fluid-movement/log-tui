package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fluid-movement/log-tui/clog"
)

const currentSchemaVersion = 1

// Host represents a single SSH host in a project.
type Host struct {
	Name          string   `json:"name"`                     // alias from ~/.ssh/config
	Hostname      string   `json:"hostname"`                 // resolved IP/FQDN — used for soft match
	User          string   `json:"user,omitempty"`
	Port          int      `json:"port,omitempty"`
	IdentityFiles []string `json:"identity_files,omitempty"` // from IdentityFile in ~/.ssh/config
}

// Project is a named collection of hosts and a shared log path.
type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Hosts     []Host    `json:"hosts"`
	LogPath   string    `json:"log_path"`
	ConfigVer int       `json:"config_ver,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Config is the top-level config file structure.
type Config struct {
	Version  int       `json:"version"`
	Projects []Project `json:"projects"`
}

func configPath() (string, error) {
	// Honour XDG_CONFIG_HOME explicitly so tests (and Linux) can redirect writes.
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		var err error
		base, err = os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("get user config dir: %w", err)
		}
	}
	return filepath.Join(base, "logviewer", "projects.json"), nil
}

// Load reads the config from disk, running migrations as needed.
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{Version: currentSchemaVersion}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Version < currentSchemaVersion {
		migrate(&cfg)
		if err := save(path, &cfg); err != nil {
			clog.Error("failed to save migrated config", "err", err)
		}
	}

	return &cfg, nil
}

// migrate upgrades a config to the current schema version (no-op for now).
func migrate(cfg *Config) {
	clog.Info("migrating config", "from", cfg.Version, "to", currentSchemaVersion)
	cfg.Version = currentSchemaVersion
}

// Save writes the config to disk.
func Save(cfg *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	return save(path, cfg)
}

func save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// AddProject appends a project and saves.
func AddProject(cfg *Config, p Project) error {
	cfg.Projects = append(cfg.Projects, p)
	return Save(cfg)
}

// DeleteProject removes a project by ID and saves.
func DeleteProject(cfg *Config, id string) error {
	for i, p := range cfg.Projects {
		if p.ID == id {
			cfg.Projects = append(cfg.Projects[:i], cfg.Projects[i+1:]...)
			return Save(cfg)
		}
	}
	return fmt.Errorf("project %q not found", id)
}

// UpdateProject replaces a project by ID and saves.
func UpdateProject(cfg *Config, p Project) error {
	for i, existing := range cfg.Projects {
		if existing.ID == p.ID {
			cfg.Projects[i] = p
			return Save(cfg)
		}
	}
	return fmt.Errorf("project %q not found", p.ID)
}
