// Package config handles loading and persisting Nuage's local configuration:
// Telegram API credentials and, once set up, the storage channel reference.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config is Nuage's on-disk configuration. ApiID/ApiHash identify the
// Telegram application (from my.telegram.org) and must be supplied by the
// user before auth can run. ChannelID/ChannelAccessHash are populated by
// `nuage init` once the storage channel has been created or selected.
type Config struct {
	ApiID             int    `json:"api_id"`
	ApiHash           string `json:"api_hash"`
	ChannelID         int64  `json:"channel_id,omitempty"`
	ChannelAccessHash int64  `json:"channel_access_hash,omitempty"`
}

// Dir returns the directory Nuage stores its config and session files in:
// $XDG_CONFIG_HOME/nuage, falling back to ~/.config/nuage.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, "nuage"), nil
}

// Path returns the path to config.json inside the Nuage config dir.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// SessionPath returns the path gotd should use for its persisted session file.
func SessionPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "session.json"), nil
}

// Load reads and parses the config file. Callers should check
// os.IsNotExist(err) to distinguish "not set up yet" from a real error.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config at %s: %w", path, err)
	}
	return &cfg, nil
}

// Save writes cfg to the config file, creating the config directory (mode
// 0700) and the file (mode 0600) if needed since it may later hold channel
// identifiers alongside the app credentials.
func Save(cfg *Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir %s: %w", dir, err)
	}
	path, err := Path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config to %s: %w", path, err)
	}
	return nil
}
