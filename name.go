package main

import (
	"os"
	"path/filepath"
	"strings"
)

// prog is the name this binary was invoked as, used in all usage, help and
// error text. It comes from argv[0] rather than os.Executable() so a symlink
// (e.g. `hc -> healthchecks-cli`) is reported under the name the user typed.
var prog = programName(os.Args)

// programName derives the invocation name from argv[0], falling back to the
// canonical name when it's missing or unusable. A Windows ".exe" suffix is
// dropped so help reads naturally.
func programName(args []string) string {
	const fallback = "healthchecks-cli"
	if len(args) == 0 || args[0] == "" {
		return fallback
	}
	name := filepath.Base(args[0])
	if strings.EqualFold(filepath.Ext(name), ".exe") {
		name = strings.TrimSuffix(name, filepath.Ext(name))
	}
	if name == "" || name == "." || name == string(filepath.Separator) {
		return fallback
	}
	return name
}

// expand substitutes the {prog} placeholder in help text and messages with the
// current program name.
func expand(s string) string {
	return strings.ReplaceAll(s, "{prog}", prog)
}
