package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultBaseURL = "https://healthchecks.io"

// Config is assembled from environment variables or the active profile.
type Config struct {
	APIKey     string
	BaseURL    string
	AllowWrite bool
}

// Profile is one named healthchecks.io project stored in the config file.
type Profile struct {
	Name       string `json:"name"`
	APIKey     string `json:"api_key"`
	AllowWrite bool   `json:"allow_write"`
	BaseURL    string `json:"base_url,omitempty"`
	// PingKey is the project-level key for slug-based check-ins
	// (hc-ping.com/<ping-key>/<slug>). It is a separate credential from APIKey
	// and is not exposed by the Management API, so it can only be pasted in.
	PingKey string `json:"ping_key,omitempty"`
}

// ProfilesFile is the on-disk format of ~/.config/hc/config.json.
type ProfilesFile struct {
	Current  string    `json:"current"`
	Projects []Profile `json:"projects"`
}

func (pf *ProfilesFile) active() *Profile {
	for i := range pf.Projects {
		if pf.Projects[i].Name == pf.Current {
			return &pf.Projects[i]
		}
	}
	return nil
}

func loadConfig() (*Config, error) {
	// HC_API_KEY always wins — useful for CI and one-off overrides.
	if key := cleanAPIKey(os.Getenv("HC_API_KEY")); key != "" {
		baseURL := os.Getenv("HC_BASE_URL")
		if baseURL == "" {
			baseURL = defaultBaseURL
		}
		return &Config{
			APIKey:     key,
			BaseURL:    baseURL,
			AllowWrite: truthy(os.Getenv("HC_ALLOW_WRITE")),
		}, nil
	}

	// Fall back to the active profile.
	pf, err := loadProfilesFile()
	if err != nil {
		return nil, err
	}
	if len(pf.Projects) == 0 || pf.Current == "" {
		return nil, fmt.Errorf("no API key configured — set HC_API_KEY or run 'hc project add'")
	}
	p := pf.active()
	if p == nil {
		return nil, fmt.Errorf("active project %q not found — run 'hc project list'", pf.Current)
	}

	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if v := os.Getenv("HC_BASE_URL"); v != "" {
		baseURL = v
	}
	return &Config{
		APIKey:     p.APIKey,
		BaseURL:    baseURL,
		AllowWrite: p.AllowWrite || truthy(os.Getenv("HC_ALLOW_WRITE")),
	}, nil
}

// pingKey resolves the key for slug-based pinging. HC_PING_KEY wins (handy for
// CI); otherwise the active saved project's stored key is used. Best-effort: a
// missing or unreadable config simply yields "". Needs no API key, so 'hc ping'
// can stay standalone.
func pingKey() string {
	if k := cleanAPIKey(os.Getenv("HC_PING_KEY")); k != "" {
		return k
	}
	pf, err := loadProfilesFile()
	if err != nil {
		return ""
	}
	if p := pf.active(); p != nil {
		return p.PingKey
	}
	return ""
}

// profilesPath returns the path to ~/.config/hc/config.json, respecting XDG.
func profilesPath() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "hc", "config.json"), nil
}

func loadProfilesFile() (*ProfilesFile, error) {
	path, err := profilesPath()
	if err != nil {
		return &ProfilesFile{}, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &ProfilesFile{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var pf ProfilesFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &pf, nil
}

func saveProfilesFile(pf *ProfilesFile) error {
	path, err := profilesPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	// Write to a temp file in the same dir, then rename, so a crash or full
	// disk mid-write can't corrupt an existing credentials file.
	tmp, err := os.CreateTemp(dir, ".config-*.json")
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// cleanAPIKey strips terminal artifacts and control characters that can sneak
// into a key when it's pasted into a prompt — most commonly the bracketed-paste
// escape sequences (ESC[200~ … ESC[201~) a terminal wraps around pasted text —
// then trims surrounding whitespace. healthchecks.io keys are printable ASCII,
// so dropping control bytes is always safe and avoids a cryptic "invalid header
// field value" error from net/http later.
func cleanAPIKey(s string) string {
	// Remove the bracketed-paste markers whole, before dropping the lone ESC,
	// so the "[200~"/"[201~" remnants don't survive as part of the key.
	s = strings.ReplaceAll(s, "\x1b[200~", "")
	s = strings.ReplaceAll(s, "\x1b[201~", "")
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r == 0x7f { // ASCII control characters (incl. ESC, CR, LF, TAB)
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
