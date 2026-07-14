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

// activeProfile is a best-effort lookup of the current saved project. A missing
// or unreadable config simply yields nil, so callers can stay quiet rather than
// error out (used by 'hc ping' and the override warnings).
func activeProfile() *Profile {
	pf, err := loadProfilesFile()
	if err != nil {
		return nil
	}
	return pf.active()
}

// apiKeyOverrideDesc describes how HC_API_KEY shadows the selection selName
// (whose stored key is selKey). It returns "" when there's no override — the env
// var is unset, or its value is exactly selKey (so the selection is already what
// commands use). Otherwise the returned sentence names which saved project the
// env key actually belongs to, when one matches, so the user knows precisely
// which project's data they're seeing. Note selKey may be "" (a selection with
// no stored key); a set env var still differs from it, which is the point.
func apiKeyOverrideDesc(pf *ProfilesFile, selName, selKey string) string {
	env := cleanAPIKey(os.Getenv("HC_API_KEY"))
	if env == "" || env == selKey {
		return ""
	}
	for i := range pf.Projects {
		if pf.Projects[i].APIKey == env {
			return fmt.Sprintf("HC_API_KEY is set and matches project %q — 'hc' commands show that project, not %q, until you unset HC_API_KEY.",
				pf.Projects[i].Name, selName)
		}
	}
	return fmt.Sprintf("HC_API_KEY is set — 'hc' commands use that key, not project %q, until you unset HC_API_KEY.", selName)
}

// warnAPIKeyOverride prints a one-line note to stderr when HC_API_KEY shadows the
// active project with a *different* project — the case where 'hc project use'
// appears to have no effect because the environment key silently wins (see
// loadConfig). It's the terse per-command safety net; 'hc project use/add/list'
// give the fuller version. HC_NO_KEY_WARNING silences this repetitive warning
// (the project-command notes stay, since those are deliberate one-offs).
func warnAPIKeyOverride() {
	if truthy(os.Getenv("HC_NO_KEY_WARNING")) {
		return
	}
	pf, err := loadProfilesFile()
	if err != nil {
		return
	}
	active := pf.active()
	if active == nil {
		return // just an env key in play; no selection to shadow
	}
	if desc := apiKeyOverrideDesc(pf, active.Name, active.APIKey); desc != "" {
		fmt.Fprintf(os.Stderr, "hc: note: %s\n", desc)
	}
}

// warnPingKeyOverride is the ping-key analogue: HC_PING_KEY shadows the active
// project's stored ping key for slug check-ins. Unlike the API key, a project
// with no stored ping key legitimately relies on HC_PING_KEY, so that case is
// not treated as an override.
func warnPingKeyOverride() {
	if truthy(os.Getenv("HC_NO_KEY_WARNING")) {
		return
	}
	env := cleanAPIKey(os.Getenv("HC_PING_KEY"))
	if env == "" {
		return
	}
	if p := activeProfile(); p != nil && p.PingKey != "" && p.PingKey != env {
		fmt.Fprintf(os.Stderr,
			"hc: note: HC_PING_KEY is set — pinging via that key, not your selected project %q's ping key. Unset HC_PING_KEY to use the project.\n",
			p.Name)
	}
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
	if p := activeProfile(); p != nil {
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
