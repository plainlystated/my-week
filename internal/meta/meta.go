// Package meta reads and writes the per-profile state file at
// ~/.local/state/my-week/<profile>-meta.yml.
//
// The state file survives weekly cache rollover. Add fields as needed.
package meta

import (
	"errors"
	"io/fs"
	"os"

	"github.com/plainlystated/my-week/internal/paths"
	"gopkg.in/yaml.v3"
)

// Meta is the on-disk state.
type Meta struct {
	// LastDigestSent is the ISO-week (e.g. "2026-W18") of the most recent digest send.
	LastDigestSent string `yaml:"last_digest_sent,omitempty"`
}

// Load reads the meta file for a profile. Returns a zero-value Meta if the file doesn't exist.
func Load(profile string) (*Meta, error) {
	p, err := paths.MetaPath(profile)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return &Meta{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m Meta
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Save writes the meta file. Creates the state dir if needed.
func Save(profile string, m *Meta) error {
	if err := paths.EnsureStateDir(); err != nil {
		return err
	}
	p, err := paths.MetaPath(profile)
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
