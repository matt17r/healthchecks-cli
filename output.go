package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"
)

// printJSON pretty-prints a raw JSON response.
func printJSON(raw []byte) error {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		// Not JSON (e.g. empty body) — print as-is.
		fmt.Println(string(raw))
		return nil
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func newTabwriter() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
}

// humanizeTime turns an RFC3339 timestamp into a relative string like "5m ago".
func humanizeTime(ts string) string {
	if ts == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	d := time.Since(t)
	if d < 0 {
		return "in " + humanizeDuration(-d)
	}
	return humanizeDuration(d) + " ago"
}

func humanizeDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// schedule returns a short human description of how a check is scheduled.
func (c Check) scheduleDesc() string {
	if c.Schedule != "" {
		tz := c.TZ
		if tz == "" {
			tz = "UTC"
		}
		return fmt.Sprintf("%s (%s)", c.Schedule, tz)
	}
	if c.Timeout > 0 {
		return "every " + humanizeDuration(time.Duration(c.Timeout)*time.Second)
	}
	return "-"
}
