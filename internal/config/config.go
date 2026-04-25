// Package config loads the per-profile YAML config from ~/.config/my-week/<profile>.yml.
package config

import (
	"fmt"
	"os"

	"github.com/plainlystated/my-week/internal/paths"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Profile    string  `yaml:"profile"`
	CupProfile string  `yaml:"cup_profile"`
	ClickUp    ClickUp `yaml:"clickup"`
	Email      Email   `yaml:"email"`
	Digest     Digest  `yaml:"digest"`

	LookaheadDays             int `yaml:"lookahead_days"`
	LookaheadMultiplier       int `yaml:"lookahead_multiplier"`
	StalenessThresholdMinutes int `yaml:"staleness_threshold_minutes"`
}

type ClickUp struct {
	SpaceID string `yaml:"space_id"`
	Lists   Lists  `yaml:"lists"`
}

type Lists struct {
	Admin     string `yaml:"admin"`
	Birthdays string `yaml:"birthdays"`
	Inbox     string `yaml:"inbox"` // optional
}

type Email struct {
	To            string `yaml:"to"`
	From          string `yaml:"from"`
	SubjectPrefix string `yaml:"subject_prefix"`
}

type Digest struct {
	SendOn    string `yaml:"send_on"`    // day-of-week, lowercase (e.g. "monday")
	SendAfter string `yaml:"send_after"` // HH:MM 24h
}

// Load reads, parses, and validates the config for the given profile.
func Load(profile string) (*Config, error) {
	path, err := paths.ConfigPath(profile)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	c.applyDefaults()
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.LookaheadDays == 0 {
		c.LookaheadDays = 7
	}
	if c.LookaheadMultiplier == 0 {
		c.LookaheadMultiplier = 3
	}
	if c.StalenessThresholdMinutes == 0 {
		c.StalenessThresholdMinutes = 90
	}
	if c.Digest.SendOn == "" {
		c.Digest.SendOn = "monday"
	}
	if c.Digest.SendAfter == "" {
		c.Digest.SendAfter = "08:00"
	}
}

func (c *Config) validate() error {
	required := []struct {
		name string
		val  string
	}{
		{"profile", c.Profile},
		{"cup_profile", c.CupProfile},
		{"clickup.space_id", c.ClickUp.SpaceID},
		{"clickup.lists.admin", c.ClickUp.Lists.Admin},
		{"clickup.lists.birthdays", c.ClickUp.Lists.Birthdays},
		{"email.to", c.Email.To},
		{"email.from", c.Email.From},
	}
	for _, f := range required {
		if f.val == "" {
			return fmt.Errorf("required field %q is empty", f.name)
		}
	}
	return nil
}
