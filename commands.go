package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
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
	{"checks", "List checks (alias: ls)", false, false, false, cmdChecks},
	{"get", "Show a single check by uuid or unique key", false, false, false, cmdGet},
	{"pings", "List recent pings for a check", false, false, false, cmdPings},
	{"flips", "List status changes (flips) for a check", false, false, false, cmdFlips},
	{"channels", "List notification channels (integrations)", false, false, false, cmdChannels},
	{"status", "Check API/database availability", false, false, false, cmdStatus},
	{"ping", "Ping a check (a check-in; needs only the uuid)", false, true, false, cmdPing},
	{"create", "Create a new check", true, false, false, cmdCreate},
	{"update", "Update an existing check", true, false, false, cmdUpdate},
	{"pause", "Pause monitoring of a check", true, false, false, cmdPause},
	{"resume", "Resume monitoring of a check", true, false, false, cmdResume},
	{"delete", "Delete a check", true, false, false, cmdDelete},
	{"completion", "Output a shell completion script (bash|zsh|fish)", false, true, false, cmdCompletion},
	{"__complete-ids", "", false, true, true, cmdCompleteIDs},
}

func lookupCommand(name string) *command {
	if name == "ls" {
		name = "checks"
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
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positionals, nil
		}
		positionals = append(positionals, rest[0])
		args = rest[1:]
	}
}

// requireID pulls a single positional identifier from the parsed positionals.
func requireID(pos []string, kind string) (string, error) {
	if len(pos) < 1 {
		return "", fmt.Errorf("missing %s (uuid or unique key)", kind)
	}
	return pos[0], nil
}

func cmdChecks(c *Client, cfg *Config, args []string) error {
	fs := flag.NewFlagSet("checks", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output raw JSON")
	tag := multiFlag{}
	fs.Var(&tag, "tag", "filter by tag (repeatable)")
	slug := fs.String("slug", "", "filter by slug")
	if err := fs.Parse(args); err != nil {
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
	if *jsonOut {
		return printJSON(raw)
	}

	if len(checks) == 0 {
		fmt.Println("No checks found.")
		return nil
	}
	w := newTabwriter()
	fmt.Fprintln(w, "STATUS\tNAME\tSCHEDULE\tLAST PING\tTAGS\tID")
	for _, ck := range checks {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			ck.Status, dash(ck.Name), ck.scheduleDesc(), humanizeTime(ck.LastPing), dash(ck.Tags), dash(ck.ID()))
	}
	return w.Flush()
}

func cmdGet(c *Client, cfg *Config, args []string) error {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output raw JSON")
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
		return printJSON(raw)
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
	if ck.ID() != "" {
		row("ID", ck.ID())
	}
	if ck.PingURL != "" {
		row("Ping URL", ck.PingURL)
	}
	return w.Flush()
}

func cmdPings(c *Client, cfg *Config, args []string) error {
	fs := flag.NewFlagSet("pings", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output raw JSON")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	id, err := requireID(pos, "check")
	if err != nil {
		return err
	}

	pings, raw, err := c.ListPings(id)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(raw)
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
	fs := flag.NewFlagSet("flips", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output raw JSON")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	id, err := requireID(pos, "check")
	if err != nil {
		return err
	}

	flips, raw, err := c.ListFlips(id)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(raw)
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
	fs := flag.NewFlagSet("channels", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output raw JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	channels, raw, err := c.ListChannels()
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(raw)
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
	raw, err := c.Status()
	if err != nil {
		return err
	}
	return printJSON(raw)
}

// cmdPing checks in to a check via the Pinging API. This uses the ping host
// (HC_PING_URL, default https://hc-ping.com), not the Management API, so it
// needs only the check's uuid — no API key.
func cmdPing(_ *Client, _ *Config, args []string) error {
	fs := flag.NewFlagSet("ping", flag.ContinueOnError)
	data := fs.String("data", "", "request body to attach (logged on the check)")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return fmt.Errorf("missing check (uuid or full ping URL)")
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

	var u string
	if strings.HasPrefix(id, "http://") || strings.HasPrefix(id, "https://") {
		u = strings.TrimRight(id, "/")
	} else {
		u = strings.TrimRight(base, "/") + "/" + id
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
	fmt.Printf("Pinged %s — %s\n", u, strings.TrimSpace(string(rb)))
	return nil
}

// cmdCompleteIDs is a hidden helper used by the shell completion scripts to
// suggest check identifiers. It never errors out (so completion stays quiet
// when no key is configured or the network is down).
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
		if id := ck.ID(); id != "" {
			fmt.Printf("%s\t%s\n", id, ck.Name)
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
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output raw JSON")
	build := checkFields(fs)
	if err := fs.Parse(args); err != nil {
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
		return printJSON(raw)
	}
	fmt.Printf("Created check %q (%s)\n", ck.Name, ck.ID())
	return nil
}

func cmdUpdate(c *Client, cfg *Config, args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output raw JSON")
	build := checkFields(fs)
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	id, err := requireID(pos, "check")
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
		return printJSON(raw)
	}
	fmt.Printf("Updated check %q (%s)\n", ck.Name, ck.ID())
	return nil
}

func cmdPause(c *Client, cfg *Config, args []string) error {
	fs := flag.NewFlagSet("pause", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output raw JSON")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	id, err := requireID(pos, "check")
	if err != nil {
		return err
	}
	ck, raw, err := c.PauseCheck(id)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(raw)
	}
	fmt.Printf("Paused check %q\n", ck.Name)
	return nil
}

func cmdResume(c *Client, cfg *Config, args []string) error {
	fs := flag.NewFlagSet("resume", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output raw JSON")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	id, err := requireID(pos, "check")
	if err != nil {
		return err
	}
	ck, raw, err := c.ResumeCheck(id)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(raw)
	}
	fmt.Printf("Resumed check %q\n", ck.Name)
	return nil
}

func cmdDelete(c *Client, cfg *Config, args []string) error {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "output raw JSON")
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
	ck, raw, err := c.DeleteCheck(id)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(raw)
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
