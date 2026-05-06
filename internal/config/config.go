package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const appName = "redmine-cli"
const DefaultProfileName = "default"

// Profile stores authentication defaults for one Redmine instance.
type Profile struct {
	Host     string `yaml:"host,omitempty"`
	APIKey   string `yaml:"api_key,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// Config stores authentication profiles for Redmine.
type Config struct {
	CurrentProfile string             `yaml:"current_profile,omitempty"`
	Profiles       map[string]Profile `yaml:"profiles,omitempty"`
}

// DefaultPath returns the CLI configuration path.
func DefaultPath() (string, error) {
	if path := os.Getenv("REDMINE_CONFIG"); path != "" {
		return path, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appName, "config.yml"), nil
}

// Load reads configuration. A missing file returns an empty config.
func Load(path string) (Config, error) {
	var cfg Config
	if path == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return cfg, err
		}
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Save writes configuration with owner-only permissions.
func Save(path string, cfg Config) error {
	if path == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// SetProfile stores a profile and makes sure the profiles map exists.
func (c *Config) SetProfile(name string, profile Profile) {
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	c.Profiles[name] = profile
}

// DeleteProfile removes a saved profile.
func (c *Config) DeleteProfile(name string) {
	delete(c.Profiles, name)
	if len(c.Profiles) == 0 {
		c.Profiles = nil
	}
	if c.CurrentProfile == name {
		c.CurrentProfile = ""
	}
}

// Remove deletes configuration. A missing file is not an error.
func Remove(path string) error {
	if path == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return err
		}
	}
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
