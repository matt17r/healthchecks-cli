package main

import "fmt"

func cmdCompletion(_ *Client, _ *Config, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: hc completion <bash|zsh|fish>")
	}
	switch args[0] {
	case "bash":
		fmt.Print(bashCompletion)
	case "zsh":
		fmt.Print(zshCompletion)
	case "fish":
		fmt.Print(fishCompletion)
	default:
		return fmt.Errorf("unsupported shell %q (use bash, zsh, or fish)", args[0])
	}
	return nil
}

// Commands that accept a check identifier as their first positional argument;
// the completion scripts offer real check IDs (via `hc __complete-ids`) here.
//
// bash ------------------------------------------------------------------------

const bashCompletion = `# bash completion for hc
# install: hc completion bash > /etc/bash_completion.d/hc   (or source it from ~/.bashrc)
_hc() {
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
        COMPREPLY=( $(compgen -W "bash zsh fish" -- "$cur") )
        return
    fi

    if [[ "$cmd" == "project" || "$cmd" == "projects" ]]; then
        if [[ $COMP_CWORD -eq 2 ]]; then
            COMPREPLY=( $(compgen -W "list add use edit remove" -- "$cur") )
        elif [[ "${COMP_WORDS[2]}" == "use" || "${COMP_WORDS[2]}" == "edit" || "${COMP_WORDS[2]}" == "remove" ]]; then
            COMPREPLY=( $(compgen -W "$(hc __complete-projects 2>/dev/null)" -- "$cur") )
        fi
        return
    fi

    case "$cmd" in
        get|pings|flips|open|update|pause|resume|delete|ping)
            if [[ "$cur" != -* ]]; then
                local ids
                ids=$(hc __complete-ids 2>/dev/null | cut -f1)
                COMPREPLY=( $(compgen -W "$ids" -- "$cur") )
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
complete -F _hc hc
`

// zsh -------------------------------------------------------------------------

const zshCompletion = `#compdef hc
# install: hc completion zsh > "${fpath[1]}/_hc"  (then restart your shell)
_hc() {
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
            _values 'shell' bash zsh fish
            ;;
        project|projects)
            if (( CURRENT == 3 )); then
                _values 'subcommand' list add use edit remove
            elif [[ ${words[3]} == "use" || ${words[3]} == "edit" || ${words[3]} == "remove" ]]; then
                local -a names
                names=(${(f)"$(hc __complete-projects 2>/dev/null)"})
                _describe 'project' names
            fi
            ;;
        get|pings|flips|open|update|pause|resume|delete|ping)
            local -a ids
            ids=(${(f)"$(hc __complete-ids 2>/dev/null | sed 's/\t/:/')"})
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
_hc "$@"
`

// fish ------------------------------------------------------------------------

const fishCompletion = `# fish completion for hc
# install: hc completion fish > ~/.config/fish/completions/hc.fish
function __hc_ids
    hc __complete-ids 2>/dev/null
end

function __hc_projects
    hc __complete-projects 2>/dev/null
end

set -l cmds project projects checks ls get pings flips channels status open ping create update pause resume delete completion help version
set -l id_cmds get pings flips open update pause resume delete ping

# Disable file completion for hc by default.
complete -c hc -f

# Subcommands (only as the first argument).
complete -c hc -n "not __fish_seen_subcommand_from $cmds" -a project    -d "Manage projects (API keys)"
complete -c hc -n "not __fish_seen_subcommand_from $cmds" -a checks     -d "List checks"
complete -c hc -n "not __fish_seen_subcommand_from $cmds" -a get        -d "Show a single check"
complete -c hc -n "not __fish_seen_subcommand_from $cmds" -a pings      -d "List recent pings"
complete -c hc -n "not __fish_seen_subcommand_from $cmds" -a flips      -d "List status changes"
complete -c hc -n "not __fish_seen_subcommand_from $cmds" -a channels   -d "List notification channels"
complete -c hc -n "not __fish_seen_subcommand_from $cmds" -a status     -d "API/database availability"
complete -c hc -n "not __fish_seen_subcommand_from $cmds" -a open       -d "Open a check in your browser"
complete -c hc -n "not __fish_seen_subcommand_from $cmds" -a ping       -d "Ping a check"
complete -c hc -n "not __fish_seen_subcommand_from $cmds" -a create     -d "Create a check"
complete -c hc -n "not __fish_seen_subcommand_from $cmds" -a update     -d "Update a check"
complete -c hc -n "not __fish_seen_subcommand_from $cmds" -a pause      -d "Pause a check"
complete -c hc -n "not __fish_seen_subcommand_from $cmds" -a resume     -d "Resume a check"
complete -c hc -n "not __fish_seen_subcommand_from $cmds" -a delete     -d "Delete a check"
complete -c hc -n "not __fish_seen_subcommand_from $cmds" -a completion -d "Output a completion script"

# Check IDs for commands that take one.
complete -c hc -n "__fish_seen_subcommand_from $id_cmds" -a "(__hc_ids)"

# Shell names for 'completion'.
complete -c hc -n "__fish_seen_subcommand_from completion" -a "bash zsh fish"

# 'project' subcommands, and project names for those that take one.
complete -c hc -n "__fish_seen_subcommand_from project projects; and not __fish_seen_subcommand_from list ls add use switch edit remove rm" -a "list add use edit remove" -d "project subcommand"
complete -c hc -n "__fish_seen_subcommand_from use switch edit remove rm" -a "(__hc_projects)" -d project

# Flags.
complete -c hc -l json -d "Output raw JSON"
complete -c hc -n "__fish_seen_subcommand_from checks ls get create update pause resume delete" -l show-secrets -d "Reveal uuids and ping URLs"
complete -c hc -n "__fish_seen_subcommand_from checks ls" -l status -d "Filter by status (up, down, grace, paused)"
complete -c hc -n "__fish_seen_subcommand_from checks ls" -l tag  -d "Filter by tag"
complete -c hc -n "__fish_seen_subcommand_from checks ls" -l slug -d "Filter by slug"
complete -c hc -n "__fish_seen_subcommand_from open" -l show-secrets -d "Print the URL instead of opening a browser"
complete -c hc -n "__fish_seen_subcommand_from create update" -l name -l tags -l desc -l timeout -l grace -l schedule -l tz
complete -c hc -n "__fish_seen_subcommand_from create" -l unique -d "Fields for create idempotency"
complete -c hc -n "__fish_seen_subcommand_from delete" -l yes  -d "Skip confirmation"
complete -c hc -n "__fish_seen_subcommand_from ping"   -l data -d "Body to attach to the ping"
`
