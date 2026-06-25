package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactObject(t *testing.T) {
	in := `{"name":"backup","slug":"backup","unique_key":"abc123","uuid":"u-1","ping_url":"https://hc-ping.com/u-1","update_url":"x","pause_url":"y","resume_url":"z","timeout":86400}`
	out := redactObject(json.RawMessage(in))

	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("redacted output is not valid JSON: %v", err)
	}

	// Secrets are replaced with the hint, not dropped.
	for _, k := range secretFields {
		v, ok := m[k]
		if !ok {
			t.Errorf("secret field %q was dropped; want it present with the hint", k)
			continue
		}
		if v != secretHint {
			t.Errorf("secret field %q = %v; want %q", k, v, secretHint)
		}
	}

	// Safe fields are untouched.
	if m["unique_key"] != "abc123" {
		t.Errorf("unique_key = %v; want it preserved", m["unique_key"])
	}
	if m["slug"] != "backup" {
		t.Errorf("slug = %v; want it preserved", m["slug"])
	}
	if m["name"] != "backup" {
		t.Errorf("name = %v; want it preserved", m["name"])
	}
	if m["timeout"].(float64) != 86400 {
		t.Errorf("timeout = %v; want 86400", m["timeout"])
	}
}

func TestRedactObjectNoSecrets(t *testing.T) {
	// An object with no secret fields should be returned unchanged.
	in := `{"status":"healthy","total":3}`
	out := redactObject(json.RawMessage(in))
	if string(out) != in {
		t.Errorf("redactObject changed a secret-free object: got %q, want %q", out, in)
	}
}

func TestLooksLikeUUID(t *testing.T) {
	cases := map[string]bool{
		"3f8e5d2a-1b4c-4e6f-8a90-1c2d3e4f5a6b": true,  // canonical uuid
		"3F8E5D2A-1B4C-4E6F-8A90-1C2D3E4F5A6B": true,  // uppercase hex
		"nightly-backup":                       false, // a slug
		"abc123":                               false, // short
		"3f8e5d2a1b4c4e6f8a901c2d3e4f5a6b":     false, // no hyphens
		"3f8e5d2a-1b4c-4e6f-8a90-1c2d3e4f5a6z": false, // non-hex char
		"":                                     false,
	}
	for in, want := range cases {
		if got := looksLikeUUID(in); got != want {
			t.Errorf("looksLikeUUID(%q) = %v; want %v", in, got, want)
		}
	}
}

func TestSecretHintMentionsFlag(t *testing.T) {
	// The placeholder must tell the reader how to reveal the value.
	if !strings.Contains(secretHint, "--show-secrets") {
		t.Errorf("secretHint = %q; want it to mention --show-secrets", secretHint)
	}
}
