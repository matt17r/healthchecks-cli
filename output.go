package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

// filterChecksByStatus returns only the check objects whose "status" field
// matches (case-insensitively). An empty status returns the input unchanged.
// It works on raw elements so every original field survives the filter.
func filterChecksByStatus(elems []json.RawMessage, status string) []json.RawMessage {
	if status == "" {
		return elems
	}
	out := make([]json.RawMessage, 0, len(elems))
	for _, el := range elems {
		var s struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(el, &s) == nil && strings.EqualFold(s.Status, status) {
			out = append(out, el)
		}
	}
	return out
}

// secretHint is shown in place of a redacted value. It is deliberately
// self-documenting: a reader (human or agent) sees that a secret exists and
// exactly how to reveal it, rather than the field silently vanishing.
const secretHint = "<hidden — pass --show-secrets to reveal>"

// secretFields are the response fields treated as secrets. The bare uuid is the
// ping credential (anyone holding it can ping the check), and the *_url fields
// just wrap it. unique_key and slug are intentionally NOT secret — they're safe
// identifiers you can use to address a check.
var secretFields = []string{"uuid", "ping_url", "update_url", "pause_url", "resume_url"}

// redactObject replaces any secret field values in a single JSON object with
// secretHint. Non-object input is returned unchanged. Field order may change
// (Go marshals map keys sorted), but no non-secret data is lost.
func redactObject(raw json.RawMessage) json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw // not an object (e.g. a scalar or array) — leave as-is
	}
	marker, _ := json.Marshal(secretHint)
	changed := false
	for _, k := range secretFields {
		if _, ok := m[k]; ok {
			m[k] = marker
			changed = true
		}
	}
	if !changed {
		return raw
	}
	out, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return out
}

// printJSON pretty-prints a raw JSON response, redacting secrets unless asked.
func printJSON(raw []byte, showSecrets bool) error {
	if !showSecrets {
		raw = redactObject(raw)
	}
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

// printNDJSON prints a JSON array as newline-delimited JSON: one compact object
// per line. This is the machine-friendly shape for list commands — each record
// parses independently and the output stays greppable and streamable.
//
// raw is the unmodified API response, so every field is preserved (not just the
// ones modelled by our structs). wrapKey names the object key holding the array
// (e.g. "checks"); pass "" when the response is already a top-level array.
// Secrets are redacted per-record unless showSecrets is set.
func printNDJSON(raw []byte, wrapKey string, showSecrets bool) error {
	arr, err := extractArray(raw, wrapKey)
	if err != nil {
		return err
	}
	return printNDJSONElems(arr, showSecrets)
}

// extractArray pulls a JSON array out of raw. wrapKey names the object key
// holding the array (e.g. "checks"); pass "" when raw is already a top-level
// array. A missing wrapKey yields a nil slice (nothing to emit), not an error.
func extractArray(raw []byte, wrapKey string) ([]json.RawMessage, error) {
	var arr []json.RawMessage
	if wrapKey != "" {
		var wrap map[string]json.RawMessage
		if err := json.Unmarshal(raw, &wrap); err != nil {
			return nil, err
		}
		inner, ok := wrap[wrapKey]
		if !ok {
			return nil, nil
		}
		if err := json.Unmarshal(inner, &arr); err != nil {
			return nil, err
		}
		return arr, nil
	}
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, err
	}
	return arr, nil
}

// printNDJSONElems writes each element as a compact line, redacting secrets
// unless showSecrets is set. Splitting this from extraction lets callers filter
// the elements (e.g. by status) while keeping every original field intact.
func printNDJSONElems(arr []json.RawMessage, showSecrets bool) error {
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	var buf bytes.Buffer
	for _, el := range arr {
		if !showSecrets {
			el = redactObject(el)
		}
		buf.Reset()
		if err := json.Compact(&buf, el); err != nil {
			return err
		}
		out.Write(buf.Bytes())
		out.WriteByte('\n')
	}
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
