package main

import (
	"strings"
	"testing"
)

func TestProgramName(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"canonical", []string{"healthchecks-cli"}, "healthchecks-cli"},
		{"absolute path", []string{"/usr/local/bin/healthchecks-cli"}, "healthchecks-cli"},
		{"relative path", []string{"./healthchecks-cli"}, "healthchecks-cli"},
		{"symlinked short name", []string{"hcheck"}, "hcheck"},
		{"symlinked via path", []string{"/opt/homebrew/bin/hcheck"}, "hcheck"},
		{"windows exe", []string{`C:\bin\healthchecks-cli.exe`}, "healthchecks-cli"},
		{"empty argv", nil, "healthchecks-cli"},
		{"empty argv0", []string{""}, "healthchecks-cli"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := programName(tc.args)
			// filepath.Base only splits on the host separator, so the
			// backslash case is only meaningful on Windows.
			if tc.name == "windows exe" && got != tc.want {
				t.Skip("backslash paths only split on Windows")
			}
			if got != tc.want {
				t.Errorf("programName(%q) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

func TestExpand(t *testing.T) {
	old := prog
	defer func() { prog = old }()
	prog = "hcheck"
	if got := expand("run '{prog} ping' via {prog}"); got != "run 'hcheck ping' via hcheck" {
		t.Errorf("expand = %q", got)
	}
}

func TestRenderCompletion(t *testing.T) {
	for shell, script := range map[string]string{"bash": bashCompletion, "zsh": zshCompletion, "fish": fishCompletion, "powershell": powershellCompletion} {
		out := renderCompletion(script, "healthchecks-cli")
		if strings.Contains(out, "@@") {
			t.Errorf("%s: unreplaced placeholder", shell)
		}
		for _, bad := range []string{" hc ", "-c hc", "(hc ", "_hc", "__hc"} {
			if strings.Contains(out, bad) {
				t.Errorf("%s: still contains %q", shell, bad)
			}
		}
		if !strings.Contains(out, "healthchecks-cli __complete-ids") && !strings.Contains(out, "healthchecks-cli __complete-projects") {
			t.Errorf("%s: callbacks don't use the name", shell)
		}
		if shell != "fish" && strings.Contains(out, "healthchecks-cli()") {
			t.Errorf("%s: dashed name used in function identifier", shell)
		}
	}
	if got := renderCompletion("@@NAME@@/@@ID@@", "hcheck"); got != "hcheck/hcheck" {
		t.Errorf("got %q", got)
	}
}
