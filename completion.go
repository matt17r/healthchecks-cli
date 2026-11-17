package main

import (
	"errors"
	"fmt"
	"strings"
)

const completionHelp = `{prog} completion — output a shell completion script

Usage:
  {prog} completion <bash|zsh|fish|powershell> [--name <command>]

The script completes the name this program was invoked as ({prog}); pass
--name to generate it for a different name, e.g. a symlink you'll create.

Examples:
  {prog} completion fish > ~/.config/fish/completions/{prog}.fish
  {prog} completion zsh > "${fpath[1]}/_{prog}"
  {prog} completion bash > /usr/local/etc/bash_completion.d/{prog}
  {prog} completion powershell | Out-String | Invoke-Expression   # add to $PROFILE
  {prog} completion fish --name hc > ~/.config/fish/completions/hc.fish`

func cmdCompletion(_ *Client, _ *Config, args []string) error {
	if len(args) > 0 && args[0] == "help" {
		args = []string{"--help"}
	}
	fs := newCmdFlags("completion", completionHelp)
	name := fs.String("name", prog, "command name to generate completions for")
	pos, err := parsePermuted(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New(expand("usage: {prog} completion <bash|zsh|fish|powershell> [--name <command>]"))
	}
	if strings.TrimSpace(*name) == "" || strings.ContainsAny(*name, " \t\n/\\'\"$`") {
		return fmt.Errorf("invalid --name %q", *name)
	}
	var script string
	switch pos[0] {
	case "bash":
		script = bashCompletion
	case "zsh":
		script = zshCompletion
	case "fish":
		script = fishCompletion
	case "powershell":
		script = powershellCompletion
	default:
		return fmt.Errorf("unsupported shell %q (use bash, zsh, fish, or powershell)", pos[0])
	}
	fmt.Print(renderCompletion(script, *name))
	return nil
}

// completionIdent turns a command name into something safe to embed in shell
// function and variable names (e.g. "healthchecks-cli" -> "healthchecks_cli").
func completionIdent(name string) string {
	return strings.Map(func(r rune) rune {
		if r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return r
		}
		return '_'
	}, name)
}

// renderCompletion fills the @@NAME@@ (command name, used for registration and
// the __complete-* callbacks) and @@ID@@ (identifier-safe form, used for
// function/variable names) placeholders in a completion script.
func renderCompletion(script, name string) string {
	return strings.NewReplacer("@@NAME@@", name, "@@ID@@", completionIdent(name)).Replace(script)
}

// Commands that accept a check identifier as their first positional argument;
// the completion scripts offer real check IDs (via `<name> __complete-ids`) here.
//
// bash ------------------------------------------------------------------------

const bashCompletion = `# bash completion for @@NAME@@
# install: @@NAME@@ completion bash > /etc/bash_completion.d/@@NAME@@   (or source it from ~/.bashrc)
_@@ID@@() {
    local cur prev cmd
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    local cmds="project checks ls get pings flips channels status open ping create update pause resume delete completion help version"

    if [[ $COMP_CWORD -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "$cmds" -- "$cur") )
        return
    fi

    cmd="${COMP_WORDS[1]}"

    if [[ "$cmd" == "completion" ]]; then
        COMPREPLY=( $(compgen -W "bash zsh fish powershell" -- "$cur") )
        return
    fi

    if [[ "$cmd" == "project" || "$cmd" == "projects" ]]; then
        if [[ $COMP_CWORD -eq 2 ]]; then
            COMPREPLY=( $(compgen -W "list add use edit remove" -- "$cur") )
        elif [[ "${COMP_WORDS[2]}" == "use" || "${COMP_WORDS[2]}" == "edit" || "${COMP_WORDS[2]}" == "remove" ]]; then
            # compgen -W splits its wordlist on $IFS, which would shred a
            # project name containing spaces into several fake candidates —
            # so match prefixes by hand against whole lines instead.
            COMPREPLY=()
            local name
            while IFS= read -r name; do
                [[ -n "$name" && "$name" == "$cur"* ]] && COMPREPLY+=("$name")
            done < <(@@NAME@@ __complete-projects 2>/dev/null)
        fi
        return
    fi

    case "$cmd" in
        get|pings|flips|open|update|pause|resume|delete|ping)
            if [[ "$cur" != -* ]]; then
                COMPREPLY=()
                local id rest
                while IFS=$'\t' read -r id rest; do
                    [[ -n "$id" && "$id" == "$cur"* ]] && COMPREPLY+=("$id")
                done < <(@@NAME@@ __complete-ids 2>/dev/null)
                return
            fi
            ;;
    esac

    local flags="--json"
    case "$cmd" in
        checks|ls)          flags="--json --show-secrets --status --tag --slug" ;;
        get|pause|resume)   flags="--json --show-secrets" ;;
        open)               flags="--show-secrets" ;;
        create)             flags="--json --show-secrets --name --tags --desc --timeout --grace --schedule --tz --unique" ;;
        update)             flags="--json --show-secrets --name --tags --desc --timeout --grace --schedule --tz" ;;
        delete)             flags="--json --show-secrets --yes" ;;
        ping)               flags="--data" ;;
    esac
    COMPREPLY=( $(compgen -W "$flags" -- "$cur") )
}
complete -F _@@ID@@ @@NAME@@
`

// zsh -------------------------------------------------------------------------

const zshCompletion = `#compdef @@NAME@@
# install: @@NAME@@ completion zsh > "${fpath[1]}/_@@ID@@"  (then restart your shell)
_@@ID@@() {
    local -a cmds
    cmds=(
        'project:Manage projects (API keys)'
        'checks:List checks'
        'get:Show a single check'
        'pings:List recent pings'
        'flips:List status changes'
        'channels:List notification channels'
        'status:API/database availability'
        'open:Open a check in your browser'
        'ping:Ping a check'
        'create:Create a check'
        'update:Update a check'
        'pause:Pause a check'
        'resume:Resume a check'
        'delete:Delete a check'
        'completion:Output a completion script'
    )

    if (( CURRENT == 2 )); then
        _describe 'command' cmds
        return
    fi

    local cmd=${words[2]}
    case $cmd in
        completion)
            _values 'shell' bash zsh fish powershell
            ;;
        project|projects)
            if (( CURRENT == 3 )); then
                _values 'subcommand' list add use edit remove
            elif [[ ${words[3]} == "use" || ${words[3]} == "edit" || ${words[3]} == "remove" ]]; then
                local -a names
                names=(${(f)"$(@@NAME@@ __complete-projects 2>/dev/null)"})
                _describe 'project' names
            fi
            ;;
        get|pings|flips|open|update|pause|resume|delete|ping)
            local -a ids
            ids=(${(f)"$(@@NAME@@ __complete-ids 2>/dev/null | sed 's/\t/:/')"})
            _describe 'check' ids
            ;;
        checks|ls)
            _arguments '--json[output raw JSON]' '--show-secrets[reveal uuids and ping URLs]' '--status=[filter by status]' '--tag=[filter by tag]' '--slug=[filter by slug]'
            ;;
        create|update)
            _arguments '--json' '--show-secrets[reveal uuid and ping URL]' '--name=' '--tags=' '--desc=' '--timeout=' '--grace=' '--schedule=' '--tz='
            ;;
        *)
            _arguments '--json[output raw JSON]'
            ;;
    esac
}
_@@ID@@ "$@"
`

// fish ------------------------------------------------------------------------

const fishCompletion = `# fish completion for @@NAME@@
# install: @@NAME@@ completion fish > ~/.config/fish/completions/@@NAME@@.fish
function __@@ID@@_ids
    @@NAME@@ __complete-ids 2>/dev/null
end

function __@@ID@@_projects
    @@NAME@@ __complete-projects 2>/dev/null
end

set -l cmds project projects checks ls get pings flips channels status open ping create update pause resume delete completion help version
set -l id_cmds get pings flips open update pause resume delete ping

# Disable file completion for @@NAME@@ by default.
complete -c @@NAME@@ -f

# Subcommands (only as the first argument).
complete -c @@NAME@@ -n "not __fish_seen_subcommand_from $cmds" -a project    -d "Manage projects (API keys)"
complete -c @@NAME@@ -n "not __fish_seen_subcommand_from $cmds" -a checks     -d "List checks"
complete -c @@NAME@@ -n "not __fish_seen_subcommand_from $cmds" -a get        -d "Show a single check"
complete -c @@NAME@@ -n "not __fish_seen_subcommand_from $cmds" -a pings      -d "List recent pings"
complete -c @@NAME@@ -n "not __fish_seen_subcommand_from $cmds" -a flips      -d "List status changes"
complete -c @@NAME@@ -n "not __fish_seen_subcommand_from $cmds" -a channels   -d "List notification channels"
complete -c @@NAME@@ -n "not __fish_seen_subcommand_from $cmds" -a status     -d "API/database availability"
complete -c @@NAME@@ -n "not __fish_seen_subcommand_from $cmds" -a open       -d "Open a check in your browser"
complete -c @@NAME@@ -n "not __fish_seen_subcommand_from $cmds" -a ping       -d "Ping a check"
complete -c @@NAME@@ -n "not __fish_seen_subcommand_from $cmds" -a create     -d "Create a check"
complete -c @@NAME@@ -n "not __fish_seen_subcommand_from $cmds" -a update     -d "Update a check"
complete -c @@NAME@@ -n "not __fish_seen_subcommand_from $cmds" -a pause      -d "Pause a check"
complete -c @@NAME@@ -n "not __fish_seen_subcommand_from $cmds" -a resume     -d "Resume a check"
complete -c @@NAME@@ -n "not __fish_seen_subcommand_from $cmds" -a delete     -d "Delete a check"
complete -c @@NAME@@ -n "not __fish_seen_subcommand_from $cmds" -a completion -d "Output a completion script"

# Check IDs for commands that take one.
complete -c @@NAME@@ -n "__fish_seen_subcommand_from $id_cmds" -a "(__@@ID@@_ids)"

# Shell names for 'completion'.
complete -c @@NAME@@ -n "__fish_seen_subcommand_from completion" -a "bash zsh fish powershell"

# 'project' subcommands, and project names for those that take one.
complete -c @@NAME@@ -n "__fish_seen_subcommand_from project projects; and not __fish_seen_subcommand_from list ls add use switch edit remove rm" -a "list add use edit remove" -d "project subcommand"
complete -c @@NAME@@ -n "__fish_seen_subcommand_from use switch edit remove rm" -a "(__@@ID@@_projects)" -d project

# Flags.
complete -c @@NAME@@ -l json -d "Output raw JSON"
complete -c @@NAME@@ -n "__fish_seen_subcommand_from checks ls get create update pause resume delete" -l show-secrets -d "Reveal uuids and ping URLs"
complete -c @@NAME@@ -n "__fish_seen_subcommand_from checks ls" -l status -d "Filter by status (up, down, grace, paused)"
complete -c @@NAME@@ -n "__fish_seen_subcommand_from checks ls" -l tag  -d "Filter by tag"
complete -c @@NAME@@ -n "__fish_seen_subcommand_from checks ls" -l slug -d "Filter by slug"
complete -c @@NAME@@ -n "__fish_seen_subcommand_from open" -l show-secrets -d "Print the URL instead of opening a browser"
complete -c @@NAME@@ -n "__fish_seen_subcommand_from create update" -l name -l tags -l desc -l timeout -l grace -l schedule -l tz
complete -c @@NAME@@ -n "__fish_seen_subcommand_from create" -l unique -d "Fields for create idempotency"
complete -c @@NAME@@ -n "__fish_seen_subcommand_from delete" -l yes  -d "Skip confirmation"
complete -c @@NAME@@ -n "__fish_seen_subcommand_from ping"   -l data -d "Body to attach to the ping"
`

// PowerShell ------------------------------------------------------------------

const powershellCompletion = `# PowerShell completion for @@NAME@@
# install (current session): @@NAME@@ completion powershell | Out-String | Invoke-Expression
# install (permanent): add the line above to your $PROFILE

$__@@ID@@Commands = 'project', 'checks', 'ls', 'get', 'pings', 'flips', 'channels', 'status', 'open', 'ping', 'create', 'update', 'pause', 'resume', 'delete', 'completion', 'help', 'version'
$__@@ID@@IdCommands = 'get', 'pings', 'flips', 'open', 'update', 'pause', 'resume', 'delete', 'ping'
$__@@ID@@ProjectSubcommands = 'list', 'add', 'use', 'edit', 'remove'

function __@@ID@@_complete_ids { @@NAME@@ __complete-ids 2>$null }
function __@@ID@@_complete_projects { @@NAME@@ __complete-projects 2>$null }

Register-ArgumentCompleter -Native -CommandName @@NAME@@ -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    # CompletionText is inserted onto the command line as-is, with no
    # quoting done for us (unlike the built-in completers) — so a project
    # name containing a space must be quoted here, or accepting it splits
    # into multiple arguments once the line is actually run.
    function __@@ID@@_quote($v) {
        if ($v -match '[\s''"]') {
            return "'" + ($v -replace "'", "''") + "'"
        }
        return $v
    }

    $result = [System.Collections.Generic.List[System.Management.Automation.CompletionResult]]::new()
    function __@@ID@@_add($values) {
        foreach ($v in $values) {
            if ($v -like "$wordToComplete*") {
                $result.Add([System.Management.Automation.CompletionResult]::new((__@@ID@@_quote $v), $v, 'ParameterValue', $v))
            }
        }
    }

    # CommandElements only contains *completed* words: with a trailing space
    # (wordToComplete -eq '') the word being typed has no element at all, but
    # with a partial word typed it's the last element. Normalize to $words,
    # the list of words typed before the one currently being completed.
    $tokens = $commandAst.CommandElements | ForEach-Object { $_.Extent.Text }
    if ($wordToComplete -eq '') {
        $words = $tokens
    } else {
        $words = $tokens[0..($tokens.Count - 2)]
    }

    if ($words.Count -le 1) {
        __@@ID@@_add $__@@ID@@Commands
        return $result
    }

    $cmd = $words[1]

    if ($cmd -eq 'completion') {
        __@@ID@@_add @('bash', 'zsh', 'fish', 'powershell')
        return $result
    }

    if ($cmd -eq 'project' -or $cmd -eq 'projects') {
        if ($words.Count -eq 2) {
            __@@ID@@_add $__@@ID@@ProjectSubcommands
        } elseif ($words.Count -ge 3 -and ($words[2] -eq 'use' -or $words[2] -eq 'edit' -or $words[2] -eq 'remove')) {
            __@@ID@@_add (__@@ID@@_complete_projects)
        }
        return $result
    }

    if (($__@@ID@@IdCommands -contains $cmd) -and ($wordToComplete -notlike '-*')) {
        foreach ($line in (__@@ID@@_complete_ids)) {
            $parts = $line -split "` + "`t" + `", 2
            $id = $parts[0]
            $desc = if ($parts.Count -gt 1) { $parts[1] } else { $id }
            if ($id -like "$wordToComplete*") {
                $result.Add([System.Management.Automation.CompletionResult]::new((__@@ID@@_quote $id), $id, 'ParameterValue', $desc))
            }
        }
        return $result
    }

    $flags = @('--json')
    switch ($cmd) {
        { $_ -in 'checks', 'ls' } { $flags = @('--json', '--show-secrets', '--status', '--tag', '--slug') }
        { $_ -in 'get', 'pause', 'resume' } { $flags = @('--json', '--show-secrets') }
        'open' { $flags = @('--show-secrets') }
        'create' { $flags = @('--json', '--show-secrets', '--name', '--tags', '--desc', '--timeout', '--grace', '--schedule', '--tz', '--unique') }
        'update' { $flags = @('--json', '--show-secrets', '--name', '--tags', '--desc', '--timeout', '--grace', '--schedule', '--tz') }
        'delete' { $flags = @('--json', '--show-secrets', '--yes') }
        'ping' { $flags = @('--data') }
    }
    __@@ID@@_add $flags
    return $result
}
`
