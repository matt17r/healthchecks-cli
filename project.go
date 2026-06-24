package main

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"golang.org/x/term"
)

func cmdProject(_ *Client, _ *Config, args []string) error {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
		args = args[1:]
	}
	switch sub {
	case "", "list", "ls":
		return projectList()
	case "use", "switch":
		return projectUse(args)
	case "add":
		return projectAdd(args)
	case "edit":
		return projectEdit(args)
	case "remove", "rm":
		return projectRemove(args)
	default:
		return fmt.Errorf("unknown subcommand %q\n\nUsage: hc project [list|use <name>|add|edit <name>|remove <name>]", sub)
	}
}

func projectList() error {
	pf, err := loadProfilesFile()
	if err != nil {
		return err
	}
	if len(pf.Projects) == 0 {
		fmt.Println("No projects configured.")
		fmt.Println()
		fmt.Println("  hc project add          add a project interactively")
		fmt.Println("  HC_API_KEY=<key> hc …   use a key directly (not saved)")
		return nil
	}

	w := newTabwriter()
	fmt.Fprintln(w, "  NAME\tACCESS\tURL")
	for _, p := range pf.Projects {
		marker := "  "
		if p.Name == pf.Current {
			marker = "* "
		}
		access := "read-only"
		if p.AllowWrite {
			access = "read-write"
		}
		fmt.Fprintf(w, "%s%s\t%s\t%s\n", marker, p.Name, access, projectHost(p.BaseURL))
	}
	w.Flush()
	fmt.Println()
	fmt.Println("  hc project use <name>    switch active project")
	fmt.Println("  hc project add           add another project")
	fmt.Println("  hc project edit <name>   edit an existing project")
	return nil
}

func projectHost(baseURL string) string {
	if baseURL == "" || baseURL == defaultBaseURL {
		return "healthchecks.io"
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return baseURL
	}
	return u.Host
}

func projectUse(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: hc project use <name>")
	}
	name := args[0]

	pf, err := loadProfilesFile()
	if err != nil {
		return err
	}

	var target *Profile
	for i := range pf.Projects {
		if pf.Projects[i].Name == name {
			target = &pf.Projects[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("project %q not found — run 'hc project list'", name)
	}
	if pf.Current == name {
		fmt.Printf("Already on %q.\n", name)
		return nil
	}

	pf.Current = name
	if err := saveProfilesFile(pf); err != nil {
		return err
	}

	access := "read-only"
	if target.AllowWrite {
		access = "read-write"
	}
	fmt.Printf("Switched to %q (%s).\n", name, access)
	return nil
}

func projectAdd(args []string) error {
	fs := flag.NewFlagSet("project add", flag.ContinueOnError)
	baseURL := fs.String("base-url", "", "base URL for self-hosted instances (default: https://healthchecks.io)")
	noVerify := fs.Bool("no-verify", false, "skip the live API check that confirms the key works")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}

	// Validate the base URL up front, before prompting for anything.
	if *baseURL != "" && *baseURL != defaultBaseURL {
		if err := validateBaseURL(*baseURL); err != nil {
			return err
		}
	}

	// Name — positional arg or prompted.
	var name string
	if len(pos) > 0 {
		name = strings.TrimSpace(pos[0])
	}
	if name == "" {
		fmt.Print("Name: ")
		name, err = readLine()
		if err != nil {
			return err
		}
		name = strings.TrimSpace(name)
	}
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}

	// Reject a duplicate name before bothering to ask for a key.
	pf, err := loadProfilesFile()
	if err != nil {
		return err
	}
	for _, p := range pf.Projects {
		if p.Name == name {
			return fmt.Errorf("project %q already exists — use 'hc project use %s' to switch to it", name, name)
		}
	}

	// API key — masked when stdin is a terminal so it doesn't appear on screen.
	fmt.Print("API key: ")
	var apiKey string
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return err
		}
		apiKey = cleanAPIKey(string(b))
	} else {
		apiKey, err = readLine()
		if err != nil {
			return err
		}
		apiKey = cleanAPIKey(apiKey)
	}
	if apiKey == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	// Write access.
	fmt.Print("Allow write access? [y/N]: ")
	ans, err := readLine()
	if err != nil {
		return err
	}
	ans = strings.ToLower(strings.TrimSpace(ans))
	allowWrite := ans == "y" || ans == "yes"

	// Normalise: store empty for the default host.
	base := *baseURL
	if base == defaultBaseURL {
		base = ""
	}

	// Verify the key actually works before saving anything. A single GET
	// /checks/ proves both that the base URL is reachable and the key is valid.
	if !*noVerify {
		effectiveBase := base
		if effectiveBase == "" {
			effectiveBase = defaultBaseURL
		}
		client := newClient(&Config{APIKey: apiKey, BaseURL: effectiveBase})
		fmt.Print("Verifying… ")
		checks, _, verr := client.ListChecks("")
		if verr != nil {
			fmt.Println("failed.")
			return fmt.Errorf("could not verify project: %w\n(pass --no-verify to save without checking)", verr)
		}
		fmt.Printf("ok — %d check(s) visible.\n", len(checks))

		// If the project claims write access, confirm the key actually has it.
		// Read-only keys are forbidden from GET /channels/, so it's a
		// definitive, harmless probe that works even with zero checks. When it
		// proves the key is read-only, save the project that way rather than
		// recording a write flag the server will never honour.
		if allowWrite {
			if _, _, cerr := client.ListChannels(); cerr != nil {
				var apiErr *APIError
				if errors.As(cerr, &apiErr) && (apiErr.Status == 401 || apiErr.Status == 403) {
					fmt.Println("This key is read-only; saving the project as read-only.")
					allowWrite = false
				} else {
					fmt.Printf("Warning: could not confirm write access (%v); saving with write access as requested.\n", cerr)
				}
			}
		}
	}

	pf.Projects = append(pf.Projects, Profile{
		Name:       name,
		APIKey:     apiKey,
		AllowWrite: allowWrite,
		BaseURL:    base,
	})

	isFirst := pf.Current == ""
	if isFirst {
		pf.Current = name
	}
	if err := saveProfilesFile(pf); err != nil {
		return err
	}

	access := "read-only"
	if allowWrite {
		access = "read-write"
	}
	fmt.Printf("Added %q (%s).\n", name, access)

	if isFirst {
		fmt.Printf("Active project set to %q.\n", name)
		return nil
	}

	// Offer to switch; default yes since the user just added it.
	fmt.Fprintf(os.Stderr, "Switch to %q now? [Y/n] ", name)
	ans, err = readLine()
	if err != nil || strings.ToLower(strings.TrimSpace(ans)) != "n" {
		pf.Current = name
		if err := saveProfilesFile(pf); err != nil {
			return err
		}
		fmt.Printf("Switched to %q.\n", name)
	}
	return nil
}

func projectEdit(args []string) error {
	fs := flag.NewFlagSet("project edit", flag.ContinueOnError)
	baseURL := fs.String("base-url", "", "new base URL for self-hosted instances")
	noVerify := fs.Bool("no-verify", false, "skip the live API check that confirms the key works")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return fmt.Errorf("usage: hc project edit <name>")
	}
	name := pos[0]

	if *baseURL != "" && *baseURL != defaultBaseURL {
		if err := validateBaseURL(*baseURL); err != nil {
			return err
		}
	}

	pf, err := loadProfilesFile()
	if err != nil {
		return err
	}

	idx := -1
	for i, p := range pf.Projects {
		if p.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("project %q not found — run 'hc project list'", name)
	}
	existing := pf.Projects[idx]

	// Name.
	fmt.Printf("Name [%s]: ", existing.Name)
	newName, err := readLine()
	if err != nil {
		return err
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		newName = existing.Name
	}
	if newName != existing.Name {
		for _, p := range pf.Projects {
			if p.Name == newName {
				return fmt.Errorf("project %q already exists", newName)
			}
		}
	}

	// API key — masked when stdin is a terminal; blank keeps the existing key.
	fmt.Print("API key (leave blank to keep): ")
	var apiKey string
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return err
		}
		apiKey = cleanAPIKey(string(b))
	} else {
		apiKey, err = readLine()
		if err != nil {
			return err
		}
		apiKey = cleanAPIKey(apiKey)
	}
	if apiKey == "" {
		apiKey = existing.APIKey
	}

	// Write access — default follows current setting.
	var allowWrite bool
	if existing.AllowWrite {
		fmt.Print("Allow write access? [Y/n]: ")
		ans, err := readLine()
		if err != nil {
			return err
		}
		ans = strings.ToLower(strings.TrimSpace(ans))
		allowWrite = ans != "n" && ans != "no"
	} else {
		fmt.Print("Allow write access? [y/N]: ")
		ans, err := readLine()
		if err != nil {
			return err
		}
		ans = strings.ToLower(strings.TrimSpace(ans))
		allowWrite = ans == "y" || ans == "yes"
	}

	// Base URL — flag value wins; otherwise keep existing.
	base := *baseURL
	if base == "" {
		base = existing.BaseURL
	}
	if base == defaultBaseURL {
		base = ""
	}

	// Verify whenever write access is enabled in the final config, or the key
	// changed. The stored allow_write flag may be stale (e.g. set by hand or
	// left over from a bug), so we always re-confirm it against the API.
	// Pass --no-verify to skip this (e.g. for a rename-only edit).
	keyChanged := apiKey != existing.APIKey
	if !*noVerify && (keyChanged || allowWrite) {
		effectiveBase := base
		if effectiveBase == "" {
			effectiveBase = defaultBaseURL
		}
		client := newClient(&Config{APIKey: apiKey, BaseURL: effectiveBase})
		fmt.Print("Verifying… ")
		checks, _, verr := client.ListChecks("")
		if verr != nil {
			fmt.Println("failed.")
			return fmt.Errorf("could not verify project: %w\n(pass --no-verify to save without checking)", verr)
		}
		fmt.Printf("ok — %d check(s) visible.\n", len(checks))

		if allowWrite {
			if _, _, cerr := client.ListChannels(); cerr != nil {
				var apiErr *APIError
				if errors.As(cerr, &apiErr) && (apiErr.Status == 401 || apiErr.Status == 403) {
					fmt.Println("This key is read-only; saving the project as read-only.")
					allowWrite = false
				} else {
					fmt.Printf("Warning: could not confirm write access (%v); saving with write access as requested.\n", cerr)
				}
			}
		}
	}

	pf.Projects[idx] = Profile{
		Name:       newName,
		APIKey:     apiKey,
		AllowWrite: allowWrite,
		BaseURL:    base,
	}
	if pf.Current == name && newName != name {
		pf.Current = newName
	}

	if err := saveProfilesFile(pf); err != nil {
		return err
	}

	access := "read-only"
	if allowWrite {
		access = "read-write"
	}
	fmt.Printf("Updated %q (%s).\n", newName, access)
	return nil
}

// validateBaseURL checks a user-supplied base URL is a well-formed http(s) URL.
func validateBaseURL(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("invalid base URL %q: %w", s, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("base URL must start with http:// or https:// (got %q)", s)
	}
	if u.Host == "" {
		return fmt.Errorf("base URL %q has no host", s)
	}
	return nil
}

// cmdCompleteProjects is a hidden helper for shell completion: it prints the
// configured project names, one per line. It never errors out.
func cmdCompleteProjects(_ *Client, _ *Config, _ []string) error {
	pf, err := loadProfilesFile()
	if err != nil {
		return nil
	}
	for _, p := range pf.Projects {
		fmt.Println(p.Name)
	}
	return nil
}

func projectRemove(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: hc project remove <name>")
	}
	name := args[0]

	pf, err := loadProfilesFile()
	if err != nil {
		return err
	}

	idx := -1
	for i, p := range pf.Projects {
		if p.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("project %q not found — run 'hc project list'", name)
	}

	if !confirm(fmt.Sprintf("Remove project %q?", name)) {
		fmt.Println("Aborted.")
		return nil
	}

	pf.Projects = append(pf.Projects[:idx], pf.Projects[idx+1:]...)
	if pf.Current == name {
		pf.Current = ""
		if len(pf.Projects) > 0 {
			pf.Current = pf.Projects[0].Name
			fmt.Printf("Active project changed to %q.\n", pf.Current)
		}
	}

	if err := saveProfilesFile(pf); err != nil {
		return err
	}
	fmt.Printf("Removed %q.\n", name)
	return nil
}
