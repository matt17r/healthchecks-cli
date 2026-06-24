package main

import (
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

	// Standalone commands (ping, completion, ...) manage their own config and
	// don't need a management API key or client.
	if cmd.standalone {
		if err := cmd.run(nil, nil, args[1:]); err != nil {
			fatal(err)
		}
		return
	}

	cfg, err := loadConfig()
	if err != nil {
		fatal(err)
	}

	if cmd.write && !cfg.AllowWrite {
		fatal(fmt.Errorf(
			"%q is a write command but write access is disabled.\n"+
				"Run 'hc project add' with a read-write key, or set HC_ALLOW_WRITE=1.",
			cmd.name))
	}

	client := newClient(cfg)
	if err := cmd.run(client, cfg, args[1:]); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "hc: %v\n", err)
	os.Exit(1)
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

Projects (persistent API key storage):
  hc project add           add a project interactively
  hc project edit <name>   edit an existing project (key, name, access)
  hc project use <name>    switch active project
  hc project list          list configured projects

Most commands accept --json to print the raw API response.

Examples:
  hc checks
  hc checks --tag prod --tag db
  hc get <uuid> --json
  hc pings <uuid>
  hc ping <uuid>            # signal success
  hc ping <uuid> fail       # signal failure (also: start, log, <exit-code>)
  HC_ALLOW_WRITE=1 hc pause <uuid>
  hc completion fish > ~/.config/fish/completions/hc.fish
`)
}
