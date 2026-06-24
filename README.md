# hc

A small, dependency-free CLI for the [healthchecks.io Management API](https://healthchecks.io/docs/api/),
in the style of `gh` / `sqlcmd`: configure it with environment variables and run.

Read-only by default. Write commands (create/update/pause/resume/delete) are
disabled unless you opt in with `HC_ALLOW_WRITE=1` *and* supply a read-write API key.

## Install

Requires Go 1.21+ to build.

```sh
make build           # produces ./hc
make install         # installs to /usr/local/bin (override with PREFIX=...)
```

## Configuration

| Variable         | Required | Default                     | Purpose |
|------------------|----------|-----------------------------|---------|
| `HC_API_KEY`     | yes\*    | —                           | Your project's API key (read-only or read-write). |
| `HC_BASE_URL`    | no       | `https://healthchecks.io`   | Management API base URL; point at a self-hosted instance, e.g. `https://hc.example.com`. |
| `HC_ALLOW_WRITE` | no       | `false`                     | Set to `1`/`true` to enable write commands. |
| `HC_PING_URL`    | no       | `https://hc-ping.com`       | Ping host for `hc ping`. Self-hosted is usually `https://hc.example.com/ping`. |

\* `hc ping` and `hc completion` don't need `HC_API_KEY` — pinging only needs the check's uuid.

API keys are created per-project in **Project Settings → API keys** on healthchecks.io.
A read-only key omits `uuid`/`ping_url`; for those checks `hc` uses the stable
`unique_key` as the ID instead, which still works for `get`/`pings`/`flips`.

```sh
export HC_API_KEY=your-project-api-key
hc checks
```

## Commands

Read-only:

```sh
hc checks                      # list all checks
hc checks --tag prod --tag db  # filter by tag (repeatable, AND)
hc checks --slug nightly-backup
hc get <uuid|unique_key>       # show one check
hc pings <uuid>                # recent pings
hc flips <uuid>                # status changes
hc channels                    # notification integrations
hc status                      # API/database availability
```

Write (need `HC_ALLOW_WRITE=1` + read-write key):

```sh
hc create -name "Nightly Backup" -tags "prod db" -timeout 86400 -grace 3600
hc create -name "Cron job" -schedule "*/5 * * * *" -tz UTC
hc update <uuid> -grace 7200
hc pause  <uuid>
hc resume <uuid>
hc delete <uuid>               # prompts; -yes to skip
```

`-unique name` on `create` makes creation idempotent (won't create a duplicate
if a check with the same name already exists).

Pinging (check-ins) — uses the ping host, not the Management API, so no key needed:

```sh
hc ping <uuid>              # signal success
hc ping <uuid> start        # signal a job has started
hc ping <uuid> fail         # signal failure
hc ping <uuid> log          # attach a log line without changing status
hc ping <uuid> 1            # report by exit code (0 = success, non-zero = fail)
hc ping <uuid> --data "backup finished in 4m"   # attach a body (sent as POST)
hc ping https://hc-ping.com/<uuid>              # a full ping URL also works
```

## Shell completion

`hc completion <bash|zsh|fish>` prints a completion script. Completions cover
subcommands and flags, and suggest your real check IDs for commands that take one
(via a hidden `hc __complete-ids` helper — it stays silent if no key is set).

```sh
# fish
hc completion fish > ~/.config/fish/completions/hc.fish

# zsh (somewhere on your $fpath)
hc completion zsh > "${fpath[1]}/_hc"

# bash
hc completion bash > /usr/local/etc/bash_completion.d/hc
```

## Output

Commands print a human-readable table/summary by default. Add `--json` to any
command to get the raw API response, pretty-printed — handy for piping to `jq`:

```sh
hc checks --json | jq -r '.checks[] | select(.status=="down") | .name'
```

## Notes

- The API is rate-limited to 100 requests/minute.
- Times are shown relative to now (e.g. `5m ago`); use `--json` for exact timestamps.
- `hc` covers both the **Management API** (querying/managing checks) and a simple
  `ping` for check-ins. For wrapping a command so it pings on success/failure
  automatically, dedicated runners like `runitor` or `task-mon` are a better fit.
