package main

import (
	"bufio"
	"fmt"
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
			"%q is a write command but writes are disabled.\n"+
				"Set HC_ALLOW_WRITE=1 (and make sure HC_API_KEY is a read-write key) to enable it.",
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
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
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
  HC_API_KEY      (required) healthchecks.io project API key
  HC_BASE_URL     Management API base URL (default https://healthchecks.io)
  HC_ALLOW_WRITE  set to 1/true to enable write commands
  HC_PING_URL     ping host for 'hc ping' (default https://hc-ping.com;
                  for self-hosted, e.g. https://hc.example.com/ping)

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
