# healthchecks-cli

A small, dependency-free CLI for the [healthchecks.io Management API](https://healthchecks.io/docs/api/),
in the style of `gh` / `sqlcmd`: configure it with environment variables and run.

Configure it with environment variables (great for CI) or save named **projects**
with `healthchecks-cli project add` (great for day-to-day use across several healthchecks.io
projects).

Read-only by default. Write commands (create/update/pause/resume/delete) are
disabled unless you opt in — either with `HC_ALLOW_WRITE=1`, or by enabling write
access on the saved project — *and* a read-write API key.

## Install

### Homebrew (macOS / Linux)

```sh
brew install matt17r/tap/healthchecks-cli
```

This installs a prebuilt binary — no compiler needed. Update later with
`brew upgrade healthchecks-cli`. To build from `main` instead, see the source instructions below.

> **Heads up: the binary is unsigned and not notarised by Apple.** It's a Homebrew
> *cask*, so macOS tags the download with the `com.apple.quarantine` flag, and an
> unsigned binary would be blocked by Gatekeeper. To avoid that, the cask's
> post-install step **strips the quarantine flag** (`xattr -dr com.apple.quarantine`).
> Integrity is still verified — Homebrew checks the download's SHA-256 against the
> cask before installing. If you'd rather not have the quarantine flag removed for
> you, **build from source** instead (below); locally built binaries aren't
> quarantined and run without any of this.

### Build from source

Requires Go 1.21+.

```sh
make build           # produces ./healthchecks-cli
make install         # installs to /usr/local/bin (override with PREFIX=...)
```

### Using a shorter name

The binary is called `healthchecks-cli`, but it names itself after however it was
invoked (`argv[0]`), so you can symlink any short name you like and the help,
errors and completions will all use it:

```sh
ln -s "$(command -v healthchecks-cli)" /usr/local/bin/hc      # or any name you prefer
```

Pick a name that doesn't clash with something else on your `PATH`. Generate
completions under the short name too, either by running the completion command via
the symlink or with `--name`:

```sh
hc completion fish > ~/.config/fish/completions/hc.fish
healthchecks-cli completion fish --name hc > ~/.config/fish/completions/hc.fish
```

The config directory (`~/.config/hc/config.json`) and `HC_*` environment variables
are unchanged regardless of the binary's name.

## Configuration

| Variable         | Required | Default                     | Purpose |
|------------------|----------|-----------------------------|---------|
| `HC_API_KEY`     | yes\*    | —                           | Your project's API key (read-only or read-write). |
| `HC_BASE_URL`    | no       | `https://healthchecks.io`   | Management API base URL; point at a self-hosted instance, e.g. `https://hc.example.com`. |
| `HC_ALLOW_WRITE` | no       | `false`                     | Set to `1`/`true` to enable write commands. |
| `HC_PING_URL`    | no       | `https://hc-ping.com`       | Ping host for `healthchecks-cli ping`. Self-hosted is usually `https://hc.example.com/ping`. |
| `HC_PING_KEY`    | no       | —                           | Project ping key, enabling `healthchecks-cli ping <slug>`. Falls back to the active project's saved key. Find it in **Project Settings**. |

\* `HC_API_KEY` is required only when no project is configured. `healthchecks-cli ping`, `healthchecks-cli
completion`, and `healthchecks-cli project` never need it.

API keys are created per-project in **Project Settings → API keys** on healthchecks.io.
A read-only key omits `uuid`/`ping_url`; for those checks `healthchecks-cli` uses the stable
`unique_key` as the ID instead, which still works for `get`/`pings`/`flips`.

> **Addressing checks.** A check's `uuid` doubles as its ping credential —
> anyone who has it can ping the check — so `healthchecks-cli` treats it as a secret and hides
> it by default (see [Secrets](#secrets)). Address checks by their **slug**
> instead; every command that takes an identifier accepts one.

```sh
export HC_API_KEY=your-project-api-key
healthchecks-cli checks
```

### Saved projects

Instead of exporting env vars, you can store one or more projects. `healthchecks-cli project add`
prompts for a name, the API key (hidden as you type), whether to allow writes, and
an optional **ping key** (for slug-based `healthchecks-cli ping`; leave blank to skip and add it
later with `healthchecks-cli project edit`). It then **verifies the key against the API before
saving** so a typo fails fast. If
you enable write access, it also confirms the key is actually read-write (read-only
keys are rejected from `GET /channels/`); if the key turns out to be read-only it
saves the project as read-only rather than recording a write flag the server would
reject:

```sh
healthchecks-cli project add                      # interactive
healthchecks-cli project add work                 # name as an argument; still prompts for the key
healthchecks-cli project add self --base-url https://hc.example.com   # self-hosted instance
healthchecks-cli project add ci --no-verify       # skip the live check (offline setups)

healthchecks-cli project list                     # show projects; * marks the active one
healthchecks-cli project use <name>               # switch the active project
healthchecks-cli project remove <name>            # delete a project (with confirmation)
```

Projects live in `~/.config/hc/config.json` (honouring `XDG_CONFIG_HOME`), written
`0600`. **API keys are stored in plaintext**, the same as `gh` and the AWS CLI — fine
for a personal workstation, but don't commit or sync that file.

Resolution order: `HC_API_KEY` (if set) always wins; otherwise the active project
is used. `HC_BASE_URL` / `HC_ALLOW_WRITE` still override per-invocation on top of a
project. Enabling write access on a project persists that opt-in, so write commands
work without `HC_ALLOW_WRITE` once the project allows it.

## Commands

Read-only:

```sh
healthchecks-cli checks                      # list all checks
healthchecks-cli checks --status down        # filter by status (up, down, grace, paused, new)
healthchecks-cli checks --tag prod --tag db  # filter by tag (repeatable, AND)
healthchecks-cli checks --slug nightly-backup
healthchecks-cli get <slug>                  # show one check (uuid/unique_key also accepted)
healthchecks-cli pings <slug>                # recent pings
healthchecks-cli pings <slug> --body <n>     # logged body of ping #n
healthchecks-cli flips <slug>                # status changes
healthchecks-cli channels                    # notification integrations
healthchecks-cli status                      # API/database availability
healthchecks-cli open <slug>                 # open the check's dashboard page in a browser
```

Write (need `HC_ALLOW_WRITE=1` + read-write key):

```sh
healthchecks-cli create --name "Nightly Backup" --tags "prod db" --timeout 86400 --grace 3600
healthchecks-cli create --name "Cron job" --schedule "*/5 * * * *" --tz UTC
healthchecks-cli update <slug> --grace 7200
healthchecks-cli pause  <slug>
healthchecks-cli resume <slug>
healthchecks-cli delete <slug>               # prompts; --yes to skip
```

Identifiers are resolved in this order: a `uuid` is used as-is; anything else is
tried as a `unique_key`, then as a `slug`. If a slug matches more than one check,
`healthchecks-cli` asks you to disambiguate with the uuid (`--show-secrets` reveals them).

`--unique name` on `create` makes creation idempotent (won't create a duplicate
if a check with the same name already exists).

Flags use `--name` style throughout; the single-dash `-name` form also works
(Go's flag parser accepts both). Run `healthchecks-cli <command> --help` for a command's
usage, flags, and examples.

Pinging (check-ins) — uses the ping host, not the Management API, so no *API* key
is needed. Ping by **slug** (using the project ping key) or by uuid:

```sh
healthchecks-cli ping <slug>              # signal success      (needs a ping key — see below)
healthchecks-cli ping <slug> start        # signal a job has started
healthchecks-cli ping <slug> fail         # signal failure
healthchecks-cli ping <slug> log          # attach a log line without changing status
healthchecks-cli ping <slug> 1            # report by exit code (0 = success, non-zero = fail)
healthchecks-cli ping <slug> --data "backup finished in 4m"   # attach a body (sent as POST)

healthchecks-cli ping <uuid>                                   # ping by uuid (no ping key)
healthchecks-cli ping https://hc-ping.com/<uuid>               # a full ping URL also works
```

`healthchecks-cli ping <slug>` builds `<ping-host>/<ping-key>/<slug>`, so it needs the
project **ping key** — set `HC_PING_KEY`, or save one on the project (`healthchecks-cli project
add`/`edit`). A uuid-shaped argument or full URL is pinged directly and needs no
ping key. The ping key never appears in `healthchecks-cli`'s output.

## Shell completion

`healthchecks-cli completion <bash|zsh|fish|powershell>` prints a completion script (for the name it was invoked as; override with `--name <command>`). Completions
cover subcommands and flags, and suggest your check **slugs** for commands that
take an identifier (via a hidden `healthchecks-cli __complete-ids` helper — it stays silent if
no key is set, and suggests slugs rather than secret uuids).

```sh
# fish
healthchecks-cli completion fish > ~/.config/fish/completions/healthchecks-cli.fish

# zsh (somewhere on your $fpath)
healthchecks-cli completion zsh > "${fpath[1]}/_healthchecks-cli"

# bash
healthchecks-cli completion bash > /usr/local/etc/bash_completion.d/healthchecks-cli
```

```powershell
# PowerShell — add to your $PROFILE
healthchecks-cli completion powershell | Out-String | Invoke-Expression
```

## Output

Commands print a human-readable table/summary by default — convenient at a
terminal, but lossy and whitespace-aligned, so not ideal for scripts or agents.
Add `--json` for machine-readable output.

For **list** commands (`checks`, `pings`, `flips`, `channels`), `--json` emits
[NDJSON](https://ndjson.org): one compact JSON object per line. Each record
parses independently, so the output stays greppable and streamable, and every
field from the API is preserved (not just the ones `healthchecks-cli` renders in tables):

```sh
healthchecks-cli checks --json | jq -r 'select(.status=="down") | .name'
```

For **single-object** commands (`get`, `create`, `status`, …), `--json` prints
the raw API response, pretty-printed.

## Secrets

A check's `uuid` is the credential in its ping URL (`https://hc-ping.com/<uuid>`):
anyone holding it can ping the check. So `healthchecks-cli` treats the `uuid` and the
`ping_url`/`update_url`/`pause_url`/`resume_url` fields as **secrets and hides
them by default** — useful when piping output into logs, scripts, or an LLM
agent's context. `unique_key` and `slug` are *not* secret and stay visible.

Hidden values are shown as a self-documenting placeholder rather than dropped, so
you can tell a secret exists and how to reveal it:

```sh
$ healthchecks-cli get nightly-backup
...
ID        <hidden — pass --show-secrets to reveal>
Ping URL  <hidden — pass --show-secrets to reveal>
```

Pass `--show-secrets` to reveal them (in tables and JSON alike). Treat that
output as sensitive:

```sh
healthchecks-cli get nightly-backup --show-secrets        # reveal uuid + ping URL
healthchecks-cli checks --json --show-secrets             # uuids included in NDJSON
```

Because checks are addressable by slug, you rarely need `--show-secrets` — only
to obtain a raw uuid/ping URL, or to break a tie when a slug is ambiguous.
Read-only API keys never expose these fields at all, so redaction is a no-op for
them.

The project **ping key** (used by `healthchecks-cli ping <slug>`) is likewise a secret: it's
stored `0600`, shown only as `set` in `healthchecks-cli project list`, and never echoed in
`healthchecks-cli`'s output — `healthchecks-cli ping` reports the slug it pinged, not the URL containing the
key.

## Using healthchecks-cli from an LLM / agent

`healthchecks-cli` is built to be driven by agents. The contract:

- **Always pass `--json`.** Lists (`checks`, `pings`, `flips`, `channels`) emit
  NDJSON — one object per line; single-object commands emit one pretty object.
  The default tables are for humans and are whitespace-aligned, so don't parse them.
- **Address checks by `slug`.** It's stable and safe; the `uuid` is hidden by
  default (it's a ping credential). Only pass `--show-secrets` when you actually
  need the uuid or ping URL.
- **Filter at the source** rather than scraping: `--status`, `--tag`, `--slug` on
  `healthchecks-cli checks`. For anything else, pipe NDJSON to `jq` (see below).
- **Check the exit code; parse errors as JSON.** In `--json` mode a failure prints
  `{"error": "...", "status": <http>}` to stdout, so success and failure share one
  parse path. Exit codes:

  | code | meaning |
  |------|---------|
  | 0 | success |
  | 1 | generic runtime error |
  | 2 | usage error (bad flag/argument) |
  | 3 | authentication / permission denied (HTTP 401/403) |
  | 4 | not found (HTTP 404) |

- **Learn a command** with `healthchecks-cli <command> --help` (usage, flags, examples; exits 0).

Searching is left to Unix — the NDJSON output is designed for it:

```sh
healthchecks-cli checks --json | jq -r 'select(.status=="down") | .slug'   # down checks (or: --status down)
healthchecks-cli checks --json | jq -r 'select(.n_pings==0) | .name'        # never-pinged
healthchecks-cli checks --json | grep -i backup                             # quick substring match
healthchecks-cli pings <slug> --json | jq -r 'select(.type=="fail") | .date'
```

## Notes

- The API is rate-limited to 100 requests/minute.
- Times are shown relative to now (e.g. `5m ago`); use `--json` for exact timestamps.
- `healthchecks-cli` covers both the **Management API** (querying/managing checks) and a simple
  `ping` for check-ins. For wrapping a command so it pings on success/failure
  automatically, dedicated runners like `runitor` or `task-mon` are a better fit.
