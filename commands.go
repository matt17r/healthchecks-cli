package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"
)

// command is one subcommand of hc.
type command struct {
	name    string
	summary string
	write   bool // requires write access to be enabled
	// standalone commands manage their own configuration; the dispatcher won't
	// require HC_API_KEY or build a management client for them.
	standalone bool
	hidden     bool // omitted from the usage listing
	run        func(c *Client, cfg *Config, args []string) error
}

// commands is the registry, in display order.
var commands = []command{
	{"project", "Manage projects (API keys) — list, add, use, remove (alias: projects)", false, true, false, cmdProject},
	{"checks", "List checks (alias: ls)", false, false, false, cmdChecks},
	{"get", "Show a single check by uuid or unique key", false, false, false, cmdGet},
	{"pings", "List recent pings for a check", false, false, false, cmdPings},
	{"flips", "List status changes (flips) for a check", false, false, false, cmdFlips},
	{"channels", "List notification channels (integrations)", false, false, false, cmdChannels},
	{"status", "Check API/database availability", false, false, false, cmdStatus},
	{"open", "Open a check's page in your browser", false, false, false, cmdOpen},
	{"ping", "Ping a check (a check-in; by slug with a ping key, or by uuid)", false, true, false, cmdPing},
	{"create", "Create a new check", true, false, false, cmdCreate},
	{"update", "Update an existing check", true, false, false, cmdUpdate},
	{"pause", "Pause monitoring of a check", true, false, false, cmdPause},
	{"resume", "Resume monitoring of a check", true, false, false, cmdResume},
	{"delete", "Delete a check", true, false, false, cmdDelete},
	{"completion", "Output a shell completion script (bash|zsh|fish)", false, true, false, cmdCompletion},
	{"__complete-ids", "", false, true, true, cmdCompleteIDs},
	{"__complete-projects", "", false, true, true, cmdCompleteProjects},
}

func lookupCommand(name string) *command {
	if name == "ls" {
		name = "checks"
	}
	if name == "projects" {
		name = "project"
	}
	for i := range commands {
		if commands[i].name == name {
			return &commands[i]
		}
	}
	return nil
}

// parsePermuted parses flags that may appear before, after, or interspersed
// with positional arguments. The stdlib flag package stops at the first
// positional, so "hc get <id> --json" would otherwise ignore --json. Returns
// the positional arguments in order.
func parsePermuted(fs *flag.FlagSet, args []string) ([]string, error) {
	var positionals []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, asUsageError(err)
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positionals, nil
		}
		positionals = append(positionals, rest[0])
		args = rest[1:]
	}
}

// parseFlags parses a flag set that takes no positional arguments, tagging a
// bad-flag failure as a usageError (so the dispatcher can exit with code 2).
func parseFlags(fs *flag.FlagSet, args []string) error {
	return asUsageError(fs.Parse(args))
}

// usageError marks an error as the caller's fault (a bad flag or argument)
// rather than a runtime failure, so the dispatcher can map it to exit code 2.
// The flag package has already printed the message and usage by the time one is
// created, so the dispatcher doesn't print it again.
type usageError struct{ err error }

func (u *usageError) Error() string { return u.err.Error() }
func (u *usageError) Unwrap() error { return u.err }

func asUsageError(err error) error {
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return err // ErrHelp is handled separately (exit 0)
	}
	return &usageError{err}
}

// newCmdFlags builds a command's flag set with a --help that prints the given
// help text followed by the command's flags. Help and flag errors go to stderr,
// keeping stdout reserved for data (important for piping and agents).
func newCmdFlags(name, help string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, strings.TrimRight(help, "\n"))
		printFlags(fs)
	}
	return fs
}

// printFlags lists a command's flags with the "--" spelling (the flag package's
// own PrintDefaults shows a single dash). Definitions stay the single source of
// truth, so help can't drift from the actual flags.
func printFlags(fs *flag.FlagSet) {
	var hasAny bool
	fs.VisitAll(func(*flag.Flag) { hasAny = true })
	if !hasAny {
		return
	}
	fmt.Fprintln(os.Stderr, "\nFlags:")
	w := tabwriter.NewWriter(os.Stderr, 0, 2, 2, ' ', 0)
	fs.VisitAll(func(f *flag.Flag) {
		name := "--" + f.Name
		typ, usage := flag.UnquoteUsage(f)
		if typ != "" {
			name += " " + typ
		}
		fmt.Fprintf(w, "  %s\t%s\n", name, usage)
	})
	w.Flush()
}

// Per-command help text. Flags are appended automatically by newCmdFlags.
const (
	checksHelp = `hc checks — list checks (alias: ls)

Usage:
  hc checks [--status <s>] [--tag <t>]... [--slug <s>] [--json] [--show-secrets]

Examples:
  hc checks
  hc checks --status down
  hc checks --tag prod --json`

	getHelp = `hc get — show a single check

Usage:
  hc get <slug|uuid|unique_key> [--json] [--show-secrets]

Examples:
  hc get nightly-backup
  hc get nightly-backup --json`

	pingsHelp = `hc pings — list recent pings for a check

Usage:
  hc pings <slug|uuid|unique_key> [--json]`

	flipsHelp = `hc flips — list status changes (flips) for a check

Usage:
  hc flips <slug|uuid|unique_key> [--json]`

	channelsHelp = `hc channels — list notification channels (integrations)

Usage:
  hc channels [--json]`

	statusHelp = `hc status — show API/database availability

Usage:
  hc status`

	openHelp = `hc open — open a check's page in your browser

Usage:
  hc open <slug|uuid> [--show-secrets]

Needs a read-write key (the dashboard URL contains the check's uuid).
With --show-secrets the URL is written to stdout instead of opening a browser
(handy on a headless/SSH box); that URL contains the uuid, so treat it as
sensitive.`

	pingHelp = `hc ping — check in to a check (a ping)

Usage:
  hc ping <slug|uuid|full-url> [success|start|fail|log|<exit-code>] [--data <body>]

Pinging by slug needs a ping key (HC_PING_KEY, or saved on the project).
A uuid or full ping URL is pinged directly and needs no ping key.

Examples:
  hc ping nightly-backup
  hc ping nightly-backup fail
  hc ping nightly-backup --data "done in 4m"`

	createHelp = `hc create — create a new check (write)

Usage:
  hc create --name <name> [field flags...] [--json] [--show-secrets]

Examples:
  hc create --name "Nightly Backup" --tags "prod db" --timeout 86400 --grace 3600
  hc create --name "Cron job" --schedule "*/5 * * * *" --tz UTC`

	updateHelp = `hc update — update an existing check (write)

Usage:
  hc update <slug|uuid> [field flags...] [--json] [--show-secrets]

Example:
  hc update nightly-backup --grace 7200`

	pauseHelp = `hc pause — pause monitoring of a check (write)

Usage:
  hc pause <slug|uuid> [--json] [--show-secrets]`

	resumeHelp = `hc resume — resume monitoring of a check (write)

Usage:
  hc resume <slug|uuid> [--json] [--show-secrets]`

	deleteHelp = `hc delete — delete a check (write)

Usage:
  hc delete <slug|uuid> [--yes] [--json] [--show-secrets]`
)

// requireID pulls a single positional identifier from the parsed positionals.
func requireID(pos []string, kind string) (string, error) {
	if len(pos) < 1 {
		return "", fmt.Errorf("missing %s (slug, uuid, or unique key)", kind)
	}
	return pos[0], nil
}

func cmdChecks(c *Client, cfg *Config, args []string) error {
	fs := newCmdFlags("checks", checksHelp)
	jsonOut := fs.Bool("json", false, "output raw JSON")
	showSecrets := fs.Bool("show-secrets", false, "reveal uuids and ping URLs (treat output as sensitive)")
	status := fs.String("status", "", "filter by status (up, down, grace, paused, new)")
	tag := multiFlag{}
	fs.Var(&tag, "tag", "filter by tag (repeatable)")
	slug := fs.String("slug", "", "filter by slug")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	q := url.Values{}
	for _, t := range tag {
		q.Add("tag", t)
	}
	if *slug != "" {
		q.Set("slug", *slug)
	}

	checks, raw, err := c.ListChecks(q.Encode())
	if err != nil {
		return err
	}

	// --status is filtered client-side: the Management API only filters by tag
	// and slug. Apply it to both the JSON elements (preserving every field) and
	// the parsed slice used by the table.
	if *jsonOut {
		elems, err := extractArray(raw, "checks")
		if err != nil {
			return err
		}
		return printNDJSONElems(filterChecksByStatus(elems, *status), *showSecrets)
	}

	if *status != "" {
		kept := checks[:0]
		for _, ck := range checks {
			if strings.EqualFold(ck.Status, *status) {
				kept = append(kept, ck)
			}
		}
		checks = kept
	}

	if len(checks) == 0 {
		if *status != "" {
			fmt.Printf("No checks with status %q.\n", *status)
		} else {
			fmt.Println("No checks found.")
		}
		return nil
	}
	// Address checks by their slug; the uuid is a secret revealed only with
	// --show-secrets, where it gets its own column.
	anyHidden := false
	w := newTabwriter()
	if *showSecrets {
		fmt.Fprintln(w, "STATUS\tNAME\tSLUG\tSCHEDULE\tLAST PING\tTAGS\tUUID")
	} else {
		fmt.Fprintln(w, "STATUS\tNAME\tSLUG\tSCHEDULE\tLAST PING\tTAGS")
	}
	for _, ck := range checks {
		if ck.UUID != "" {
			anyHidden = true
		}
		if *showSecrets {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				ck.Status, dash(ck.Name), dash(ck.Slug), ck.scheduleDesc(), humanizeTime(ck.LastPing), dash(ck.Tags), dash(ck.ID()))
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				ck.Status, dash(ck.Name), dash(ck.Slug), ck.scheduleDesc(), humanizeTime(ck.LastPing), dash(ck.Tags))
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if anyHidden && !*showSecrets {
		fmt.Println("\n# uuids and ping URLs hidden — pass --show-secrets to reveal")
	}
	return nil
}

func cmdGet(c *Client, cfg *Config, args []string) error {
	fs := newCmdFlags("get", getHelp)
	jsonOut := fs.Bool("json", false, "output raw JSON")
	showSecrets := fs.Bool("show-secrets", false, "reveal uuid and ping URL (treat output as sensitive)")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	id, err := requireID(pos, "check")
	if err != nil {
		return err
	}

	ck, raw, err := c.GetCheck(id)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(raw, *showSecrets)
	}

	w := newTabwriter()
	row := func(k, v string) { fmt.Fprintf(w, "%s\t%s\n", k, dash(v)) }
	row("Name", ck.Name)
	row("Status", ck.Status)
	row("Slug", ck.Slug)
	row("Tags", ck.Tags)
	row("Schedule", ck.scheduleDesc())
	row("Grace", humanizeDuration(time.Duration(ck.Grace)*time.Second))
	row("Pings", fmt.Sprintf("%d", ck.NPings))
	row("Last ping", humanizeTime(ck.LastPing))
	row("Next ping", humanizeTime(ck.NextPing))
	if ck.Desc != "" {
		row("Description", ck.Desc)
	}
	// The uuid (returned only for read-write keys) is the ping credential, so
	// hide it unless asked. unique_key (read-only keys) is safe to show.
	if ck.UUID != "" {
		if *showSecrets {
			row("ID", ck.UUID)
		} else {
			row("ID", secretHint)
		}
	} else if id := ck.ID(); id != "" {
		row("ID", id)
	}
	if ck.PingURL != "" {
		if *showSecrets {
			row("Ping URL", ck.PingURL)
		} else {
			row("Ping URL", secretHint)
		}
	}
	return w.Flush()
}

func cmdPings(c *Client, cfg *Config, args []string) error {
	fs := newCmdFlags("pings", pingsHelp)
	jsonOut := fs.Bool("json", false, "output raw JSON")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	id, err := requireID(pos, "check")
	if err != nil {
		return err
	}
	id, err = c.resolveID(id)
	if err != nil {
		return err
	}

	pings, raw, err := c.ListPings(id)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printNDJSON(raw, "", true) // pings carry no secrets
	}
	if len(pings) == 0 {
		fmt.Println("No pings found.")
		return nil
	}
	w := newTabwriter()
	fmt.Fprintln(w, "#\tTYPE\tWHEN\tMETHOD\tDURATION")
	for _, p := range pings {
		dur := "-"
		if p.Duration > 0 {
			dur = fmt.Sprintf("%.2fs", p.Duration)
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n",
			p.N, dash(p.Type), humanizeTime(p.Date), dash(p.Method), dur)
	}
	return w.Flush()
}

func cmdFlips(c *Client, cfg *Config, args []string) error {
	fs := newCmdFlags("flips", flipsHelp)
	jsonOut := fs.Bool("json", false, "output raw JSON")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	id, err := requireID(pos, "check")
	if err != nil {
		return err
	}
	id, err = c.resolveID(id)
	if err != nil {
		return err
	}

	flips, raw, err := c.ListFlips(id)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printNDJSON(raw, "", true) // flips carry no secrets
	}
	if len(flips) == 0 {
		fmt.Println("No status changes found.")
		return nil
	}
	w := newTabwriter()
	fmt.Fprintln(w, "WHEN\tSTATUS")
	for _, f := range flips {
		state := "down"
		if f.Up == 1 {
			state = "up"
		}
		fmt.Fprintf(w, "%s\t%s\n", humanizeTime(f.Timestamp), state)
	}
	return w.Flush()
}

func cmdChannels(c *Client, cfg *Config, args []string) error {
	fs := newCmdFlags("channels", channelsHelp)
	jsonOut := fs.Bool("json", false, "output raw JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	channels, raw, err := c.ListChannels()
	if err != nil {
		return err
	}
	if *jsonOut {
		return printNDJSON(raw, "channels", true) // channels carry no secrets
	}
	if len(channels) == 0 {
		fmt.Println("No channels found.")
		return nil
	}
	w := newTabwriter()
	fmt.Fprintln(w, "KIND\tNAME\tID")
	for _, ch := range channels {
		fmt.Fprintf(w, "%s\t%s\t%s\n", dash(ch.Kind), dash(ch.Name), dash(ch.ID))
	}
	return w.Flush()
}

func cmdStatus(c *Client, cfg *Config, args []string) error {
	fs := newCmdFlags("status", statusHelp)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	raw, err := c.Status()
	if err != nil {
		return err
	}
	return printJSON(raw, true) // status carries no secrets
}

// cmdOpen opens a check's dashboard page in the default browser. The page URL
// (…/checks/<uuid>/details/) is built from the check's uuid, so it needs a
// read-write key; with --show-secrets the URL is written to stdout instead.
func cmdOpen(c *Client, cfg *Config, args []string) error {
	fs := newCmdFlags("open", openHelp)
	showSecrets := fs.Bool("show-secrets", false, "print the URL instead of opening a browser (reveals the uuid)")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	id, err := requireID(pos, "check")
	if err != nil {
		return err
	}

	ck, _, err := c.GetCheck(id)
	if err != nil {
		return err
	}
	if ck.UUID == "" {
		return fmt.Errorf("can't open %q: the read-only API doesn't expose the check's uuid (the dashboard needs a read-write key)", id)
	}

	pageURL := strings.TrimRight(cfg.BaseURL, "/") + "/checks/" + ck.UUID + "/details/"
	if *showSecrets {
		fmt.Println(pageURL)
		return nil
	}
	if err := openBrowser(pageURL); err != nil {
		return fmt.Errorf("couldn't launch a browser: %w\n(use --show-secrets to print the URL instead)", err)
	}
	fmt.Printf("Opening %q in your browser…\n", dash(ck.Name))
	return nil
}

// openBrowser launches the OS default handler for url.
func openBrowser(url string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
		args = []string{url}
	case "windows":
		name = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default: // linux, *bsd, …
		name = "xdg-open"
		args = []string{url}
	}
	return exec.Command(name, args...).Start()
}

// cmdPing checks in to a check via the Pinging API. This uses the ping host
// (HC_PING_URL, default https://hc-ping.com), not the Management API, so it
// needs no API key — a slug (with the project ping key), a uuid, or a full ping
// URL identifies the check.
func cmdPing(_ *Client, _ *Config, args []string) error {
	fs := newCmdFlags("ping", pingHelp)
	data := fs.String("data", "", "request body to attach (logged on the check)")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return fmt.Errorf("missing check (slug, uuid, or full ping URL)")
	}
	id := pos[0]
	action := ""
	if len(pos) > 1 {
		action = pos[1]
	}

	base := os.Getenv("HC_PING_URL")
	if base == "" {
		base = "https://hc-ping.com"
	}
	base = strings.TrimRight(base, "/")

	// Resolve the ping target. A full URL is used verbatim; a uuid uses the
	// direct /<uuid> endpoint; anything else is a slug, pinged via the project
	// ping key (/<ping-key>/<slug>). target is a leak-safe label for the success
	// line — it never contains the uuid or the ping key.
	var u, target string
	switch {
	case strings.HasPrefix(id, "http://") || strings.HasPrefix(id, "https://"):
		u = strings.TrimRight(id, "/")
		target = "check"
	case looksLikeUUID(id):
		u = base + "/" + id
		target = "check"
	default:
		key := pingKey()
		if key == "" {
			return fmt.Errorf("pinging %q by slug needs a ping key — set HC_PING_KEY or run 'hc project edit <name>' to save one\n(or pass the check's uuid or full ping URL instead)", id)
		}
		u = base + "/" + key + "/" + id
		target = fmt.Sprintf("%q", id)
	}

	switch action {
	case "", "success":
		// plain ping
	case "start", "fail", "log":
		u += "/" + action
	default:
		if !isNumeric(action) {
			return fmt.Errorf("unknown ping action %q (use: success, start, fail, log, or an exit code)", action)
		}
		u += "/" + action // exit-code endpoint: 0 = success, non-zero = fail
	}

	method := http.MethodGet
	var body io.Reader
	if *data != "" {
		method = http.MethodPost
		body = strings.NewReader(*data)
	}
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ping failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(rb)))
	}
	fmt.Printf("Pinged %s — %s\n", target, strings.TrimSpace(string(rb)))
	return nil
}

// cmdCompleteIDs is a hidden helper used by the shell completion scripts to
// suggest check identifiers. It suggests the slug (the safe, addressable handle)
// rather than the secret uuid, falling back to the id when a check has no slug.
// It never errors out (so completion stays quiet when no key is configured or
// the network is down).
func cmdCompleteIDs(_ *Client, _ *Config, _ []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return nil
	}
	checks, _, err := newClient(cfg).ListChecks("")
	if err != nil {
		return nil
	}
	for _, ck := range checks {
		handle := ck.Slug
		if handle == "" {
			handle = ck.ID()
		}
		if handle != "" {
			fmt.Printf("%s\t%s\n", handle, ck.Name)
		}
	}
	return nil
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ---- Write commands ----

// checkFields registers the flags shared by create and update and returns a
// function that builds the request body from the ones the user actually set.
func checkFields(fs *flag.FlagSet) func() map[string]any {
	name := fs.String("name", "", "check name")
	tags := fs.String("tags", "", "space-separated tags")
	desc := fs.String("desc", "", "description")
	timeout := fs.Int("timeout", 0, "expected period between pings, in seconds")
	grace := fs.Int("grace", 0, "grace period in seconds")
	schedule := fs.String("schedule", "", "cron expression (use with -tz)")
	tz := fs.String("tz", "", "timezone for -schedule")
	unique := fs.String("unique", "", "comma-separated fields for create idempotency (e.g. name)")

	return func() map[string]any {
		body := map[string]any{}
		set := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
		if set["name"] {
			body["name"] = *name
		}
		if set["tags"] {
			body["tags"] = *tags
		}
		if set["desc"] {
			body["desc"] = *desc
		}
		if set["timeout"] {
			body["timeout"] = *timeout
		}
		if set["grace"] {
			body["grace"] = *grace
		}
		if set["schedule"] {
			body["schedule"] = *schedule
		}
		if set["tz"] {
			body["tz"] = *tz
		}
		if set["unique"] {
			body["unique"] = strings.Split(*unique, ",")
		}
		return body
	}
}

func cmdCreate(c *Client, cfg *Config, args []string) error {
	fs := newCmdFlags("create", createHelp)
	jsonOut := fs.Bool("json", false, "output raw JSON")
	showSecrets := fs.Bool("show-secrets", false, "reveal uuid and ping URL (treat output as sensitive)")
	build := checkFields(fs)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	body := build()
	if len(body) == 0 {
		return fmt.Errorf("nothing to create: pass at least -name")
	}

	ck, raw, err := c.CreateCheck(body)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(raw, *showSecrets)
	}
	if *showSecrets {
		fmt.Printf("Created check %q (%s)\n", ck.Name, ck.ID())
	} else {
		fmt.Printf("Created check %q (slug: %s) — pass --show-secrets for the uuid and ping URL\n", ck.Name, dash(ck.Slug))
	}
	return nil
}

func cmdUpdate(c *Client, cfg *Config, args []string) error {
	fs := newCmdFlags("update", updateHelp)
	jsonOut := fs.Bool("json", false, "output raw JSON")
	showSecrets := fs.Bool("show-secrets", false, "reveal uuid and ping URL (treat output as sensitive)")
	build := checkFields(fs)
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	id, err := requireID(pos, "check")
	if err != nil {
		return err
	}
	id, err = c.resolveID(id)
	if err != nil {
		return err
	}
	body := build()
	if len(body) == 0 {
		return fmt.Errorf("nothing to update: pass at least one field flag")
	}

	ck, raw, err := c.UpdateCheck(id, body)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(raw, *showSecrets)
	}
	if *showSecrets {
		fmt.Printf("Updated check %q (%s)\n", ck.Name, ck.ID())
	} else {
		fmt.Printf("Updated check %q (slug: %s)\n", ck.Name, dash(ck.Slug))
	}
	return nil
}

func cmdPause(c *Client, cfg *Config, args []string) error {
	fs := newCmdFlags("pause", pauseHelp)
	jsonOut := fs.Bool("json", false, "output raw JSON")
	showSecrets := fs.Bool("show-secrets", false, "reveal uuid and ping URL (treat output as sensitive)")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	id, err := requireID(pos, "check")
	if err != nil {
		return err
	}
	id, err = c.resolveID(id)
	if err != nil {
		return err
	}
	ck, raw, err := c.PauseCheck(id)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(raw, *showSecrets)
	}
	fmt.Printf("Paused check %q\n", ck.Name)
	return nil
}

func cmdResume(c *Client, cfg *Config, args []string) error {
	fs := newCmdFlags("resume", resumeHelp)
	jsonOut := fs.Bool("json", false, "output raw JSON")
	showSecrets := fs.Bool("show-secrets", false, "reveal uuid and ping URL (treat output as sensitive)")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	id, err := requireID(pos, "check")
	if err != nil {
		return err
	}
	id, err = c.resolveID(id)
	if err != nil {
		return err
	}
	ck, raw, err := c.ResumeCheck(id)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(raw, *showSecrets)
	}
	fmt.Printf("Resumed check %q\n", ck.Name)
	return nil
}

func cmdDelete(c *Client, cfg *Config, args []string) error {
	fs := newCmdFlags("delete", deleteHelp)
	jsonOut := fs.Bool("json", false, "output raw JSON")
	showSecrets := fs.Bool("show-secrets", false, "reveal uuid and ping URL (treat output as sensitive)")
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	id, err := requireID(pos, "check")
	if err != nil {
		return err
	}
	if !*yes && !confirm(fmt.Sprintf("Delete check %s? This cannot be undone.", id)) {
		fmt.Println("Aborted.")
		return nil
	}
	id, err = c.resolveID(id)
	if err != nil {
		return err
	}
	ck, raw, err := c.DeleteCheck(id)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(raw, *showSecrets)
	}
	fmt.Printf("Deleted check %q\n", ck.Name)
	return nil
}

// ---- helpers ----

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// multiFlag collects repeated string flags.
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}
