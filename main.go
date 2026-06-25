package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

// version is overridable at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	switch args[0] {
	case "-h", "--help", "help":
		usage()
		return
	case "-v", "--version", "version":
		fmt.Printf("hc %s\n", version)
		return
	}

	cmd := lookupCommand(args[0])
	if cmd == nil {
		fmt.Fprintf(os.Stderr, "hc: unknown command %q\n\n", args[0])
		usage()
		os.Exit(2)
	}

	jsonMode := wantsJSON(args[1:])

	// Standalone commands (ping, completion, ...) manage their own config and
	// don't need a management API key or client.
	if cmd.standalone {
		if err := cmd.run(nil, nil, args[1:]); err != nil {
			handleExit(err, jsonMode)
		}
		return
	}

	cfg, err := loadConfig()
	if err != nil {
		handleExit(err, jsonMode)
	}

	if cmd.write && !cfg.AllowWrite {
		handleExit(fmt.Errorf(
			"%q is a write command but write access is disabled.\n"+
				"Run 'hc project add' with a read-write key, or set HC_ALLOW_WRITE=1.",
			cmd.name), jsonMode)
	}

	client := newClient(cfg)
	if err := cmd.run(client, cfg, args[1:]); err != nil {
		handleExit(err, jsonMode)
	}
}

// handleExit reports a terminal error and exits with a code that lets callers
// (scripts and agents) branch on the failure class:
//
//	0  success
//	1  generic runtime error
//	2  usage error (bad flag or argument)
//	3  authentication / permission denied (HTTP 401/403)
//	4  not found (HTTP 404)
//
// In --json mode the error is emitted as a JSON object on stdout
// ({"error":…,"status":…}) so success and failure share one parse path;
// otherwise it's a plain "hc: …" line on stderr.
func handleExit(err error, jsonMode bool) {
	if errors.Is(err, flag.ErrHelp) {
		os.Exit(0) // --help: usage already printed to stderr
	}
	var ue *usageError
	if errors.As(err, &ue) {
		os.Exit(2) // the flag package already printed the message and usage
	}

	code := 1
	status := 0
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		status = apiErr.Status
		switch apiErr.Status {
		case 401, 403:
			code = 3
		case 404:
			code = 4
		}
	}

	if jsonMode {
		obj := map[string]any{"error": err.Error()}
		if status != 0 {
			obj["status"] = status
		}
		b, _ := json.Marshal(obj)
		fmt.Fprintln(os.Stdout, string(b))
	} else {
		fmt.Fprintf(os.Stderr, "hc: %v\n", err)
	}
	os.Exit(code)
}

// wantsJSON reports whether --json (or -json) appears among a command's args,
// so the dispatcher can format terminal errors to match.
func wantsJSON(args []string) bool {
	for _, a := range args {
		if a == "--" {
			break // end of flags
		}
		if a == "-json" || a == "--json" {
			return true
		}
	}
	return false
}

// confirm prompts on stderr and reads a yes/no answer from stdin.
func confirm(prompt string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
	line, err := readLine()
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}

// readLine reads a single line from stdin one byte at a time, so it never
// buffers past the newline. That lets it be safely interleaved with
// term.ReadPassword, which reads directly from the file descriptor.
func readLine() (string, error) {
	var b []byte
	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				break
			}
			b = append(b, buf[0])
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
	}
	return strings.TrimRight(string(b), "\r"), nil
}

func usage() {
	fmt.Fprintf(os.Stderr, `hc — a CLI for the healthchecks.io Management API

Usage:
  hc <command> [flags] [arguments]

Commands:
`)
	w := tabwriter.NewWriter(os.Stderr, 0, 2, 2, ' ', 0)
	for _, c := range commands {
		if c.hidden {
			continue
		}
		marker := ""
		if c.write {
			marker = " *"
		}
		fmt.Fprintf(w, "  %s\t%s%s\n", c.name, c.summary, marker)
	}
	w.Flush()
	fmt.Fprintf(os.Stderr, `
  help        Show this help
  version     Show version

  (* = write command; requires HC_ALLOW_WRITE=1 and a read-write API key)

Environment:
  HC_API_KEY      API key override — bypasses saved projects, useful for CI
  HC_BASE_URL     Management API base URL override
  HC_ALLOW_WRITE  set to 1/true to enable write commands for the current key
  HC_PING_URL     ping host for 'hc ping' (default https://hc-ping.com;
                  for self-hosted, e.g. https://hc.example.com/ping)
  HC_PING_KEY     project ping key, enabling 'hc ping <slug>' (otherwise read
                  from the active project; find it in Project Settings)

Projects (persistent API key storage):
  hc project add           add a project interactively
  hc project edit [name]   edit a project (defaults to the active one)
  hc project use <name>    switch active project
  hc project list          list configured projects

Most commands accept --json to print the raw API response (NDJSON for lists).
Run 'hc <command> --help' for a command's usage, flags, and examples.

Secrets:
  A check's uuid is its ping credential, so uuids and ping URLs are hidden by
  default. Address checks by their slug instead; pass --show-secrets to reveal
  the uuid/ping URL (treat that output as sensitive).

Exit codes:
  0 success   1 error   2 usage   3 auth/forbidden   4 not found

Examples:
  hc checks
  hc checks --status down            # filter by status (also --tag, --slug)
  hc get <slug> --json
  hc pings <slug>
  hc pause <slug>                    # write commands accept a slug too
  hc get <slug> --show-secrets       # reveal the uuid + ping URL
  hc open <slug>                     # open the check's dashboard page
  hc ping <slug>                     # signal success (needs a ping key)
  hc ping <slug> fail                # signal failure (also: start, log, <exit-code>)
  hc completion fish > ~/.config/fish/completions/hc.fish
`)
}
