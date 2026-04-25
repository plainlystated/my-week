// Package paths centralizes the conventional file locations used by my-week.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// ConfigPath returns ~/.config/my-week/<profile>.yml, honoring XDG_CONFIG_HOME.
func ConfigPath(profile string) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, profile+".yml"), nil
}

// StateDir returns ~/.local/state/my-week, honoring XDG_STATE_HOME.
func StateDir() (string, error) {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "my-week"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "my-week"), nil
}

// CachePath returns the weekly cache file path for a given profile + ISO week.
// isoWeek is formatted as "2026-W18".
func CachePath(profile, isoWeek string) (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%s.md", profile, isoWeek)), nil
}

// MetaPath returns the meta state file path for a profile.
func MetaPath(profile string) (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, profile+"-meta.yml"), nil
}

// EnsureStateDir creates the state directory if it doesn't exist.
func EnsureStateDir() error {
	dir, err := StateDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}

func configDir() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "my-week"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "my-week"), nil
}
