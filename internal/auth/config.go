// Package auth stores profiles and credentials and resolves
// which of them a run uses.
//
// Non-secret settings live in config.json under the
// configuration directory (~/.config/transpareo by default).
// Secrets live in the operating system's keyring, or in
// credentials.json beside the config when no keyring is
// available. A project directory may carry
// .transpareo/config.json naming the profile to use there.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Config is the content of config.json.
type Config struct {
	DefaultProfile string                   `json:"defaultProfile,omitempty"`
	Profiles       map[string]ProfileConfig `json:"profiles"`
}

// ProfileConfig is what config.json records about a profile;
// the secret lives in the credential store named by Store.
type ProfileConfig struct {
	Host     string   `json:"host"`
	ClientID string   `json:"clientId"`
	Scope    []string `json:"scope,omitempty"`
	Store    string   `json:"store"`
}

// ProjectConfig is the content of .transpareo/config.json in a
// project directory. It names a profile and nothing else.
type ProjectConfig struct {
	Profile string `json:"profile"`
}

const (
	configFile      = "config.json"
	credentialsFile = "credentials.json"
	projectDir      = ".transpareo"
)

var profileName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ValidateProfileName refuses names that could not be a file
// name or a keyring account.
func ValidateProfileName(name string) error {
	if !profileName.MatchString(name) {
		return fmt.Errorf("%q is not a valid profile name "+
			"(letters, digits, dots, dashes and underscores)", name)
	}
	return nil
}

// ConfigDir returns the directory of config.json and the file
// fallback: TRANSPAREO_CONFIG_DIR, else XDG_CONFIG_HOME/transpareo,
// else ~/.config/transpareo.
func ConfigDir(getenv func(string) string) (string, error) {
	if dir := getenv("TRANSPAREO_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	if base := getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(base, "transpareo"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding the home directory: %w", err)
	}
	return filepath.Join(home, ".config", "transpareo"), nil
}

// LoadConfig reads config.json from dir. A missing file is an
// empty configuration.
func LoadConfig(dir string) (*Config, error) {
	cfg := &Config{Profiles: map[string]ProfileConfig{}}
	data, err := os.ReadFile(filepath.Join(dir, configFile))
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(dir, configFile), err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]ProfileConfig{}
	}
	return cfg, nil
}

// Save writes config.json to dir, readable by its owner only.
func (c *Config) Save(dir string) error {
	return writePrivate(filepath.Join(dir, configFile), c)
}

// writePrivate writes v as JSON with mode 0600 into a directory
// with mode 0700, replacing the file atomically.
func writePrivate(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// FindProjectProfile walks from dir upwards and returns the
// profile named by the first .transpareo/config.json it finds,
// or an empty string.
func FindProjectProfile(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		path := filepath.Join(dir, projectDir, configFile)
		data, err := os.ReadFile(path)
		if err == nil {
			var pc ProjectConfig
			if err := json.Unmarshal(data, &pc); err != nil {
				return "", fmt.Errorf("%s: %w", path, err)
			}
			return strings.TrimSpace(pc.Profile), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}
