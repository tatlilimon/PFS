# pfs.bash — Bash wrapper for PFS (Please Find Solution)
# Source this file in your .bashrc, after other prompt tools (starship,
# bash-preexec, direnv):
#
#   source /path/to/PFS/shell/pfs.bash
#
# Records the last command line, its exit status and PIPESTATUS, and passes
# them to the pfs binary through its environment only (nothing is exported to
# your shell). pfs prints a command on stdout only after you confirm it in its
# terminal UI; the wrapper evaluates exactly that and nothing else. No jq, no
# temp files.

# Guard against double-sourcing
if [ -n "${__PFS_LOADED:-}" ]; then
    return 0 2>/dev/null || true
fi
__PFS_LOADED=1

# --- State ---
__PFS_LAST_CMD=""
__PFS_LAST_EXIT=0
__PFS_LAST_PIPESTATUS=""
__PFS_PENDING_CMD=""    # command line that started, not yet finished
__PFS_AT_PROMPT=0       # 1 from the end of PROMPT_COMMAND until the next command starts
__PFS_HIST_AT_PROMPT="" # newest history number when the prompt was drawn
__PFS_RAN_PFS=0         # the command line that is running is pfs itself

# "  123  command" -> 123
__pfs_history_number() {
    local entry
    entry=$(HISTTIMEFORMAT='' builtin history 1 2>/dev/null) || return 0
    if [[ $entry =~ ^[[:space:]]*([0-9]+) ]]; then
        printf '%s' "${BASH_REMATCH[1]}"
    fi
}

# DEBUG trap: runs before every simple command. Only the first one after a
# prompt starts a new command line; take its full text from history, so
# pipelines, quotes and multi-line commands are kept as typed.
__pfs_debug_trap() {
    [ "${__PFS_AT_PROMPT:-0}" = 1 ] || return 0
    __PFS_AT_PROMPT=0

    local this="" entry
    if [[ -o history ]]; then
        entry=$(HISTTIMEFORMAT='' builtin history 1 2>/dev/null)
        if [[ $entry =~ ^[[:space:]]*([0-9]+)[*[:space:]][[:space:]](.*)$ ]] &&
            [ "${BASH_REMATCH[1]}" != "$__PFS_HIST_AT_PROMPT" ]; then
            this=${BASH_REMATCH[2]}
        fi
    fi
    # Not saved in history (leading space with HISTCONTROL=ignorespace, a
    # duplicate with ignoredups, HISTIGNORE, history off): use the simple
    # command bash is about to run.
    [ -n "$this" ] || this=$BASH_COMMAND

    if [[ $this == pfs || $this == "pfs "* ]]; then
        __PFS_RAN_PFS=1
        __PFS_PENDING_CMD=""
    else
        __PFS_RAN_PFS=0
        __PFS_PENDING_CMD=$this
    fi
    return 0
}

# First entry of PROMPT_COMMAND: $? and PIPESTATUS still describe the command
# line that just finished, provided nothing ran before this function.
__pfs_prompt_command() {
    local exit_status=$? pipes="${PIPESTATUS[*]}"
    if [ "$__PFS_RAN_PFS" = 1 ]; then
        # pfs itself ran; it updates the record after evaluating a fix.
        __PFS_RAN_PFS=0
    elif [ -n "$__PFS_PENDING_CMD" ]; then
        __PFS_LAST_CMD=$__PFS_PENDING_CMD
        __PFS_LAST_EXIT=$exit_status
        __PFS_LAST_PIPESTATUS=$pipes
    fi
    __PFS_PENDING_CMD=""
    __pfs_keep_prompt_order
    return "$exit_status"
}

# Last entry of PROMPT_COMMAND: the next DEBUG trap starts a new command line.
__pfs_prompt_ready() {
    __PFS_HIST_AT_PROMPT=$(__pfs_history_number)
    __PFS_AT_PROMPT=1
}

# Keep __pfs_prompt_command first and __pfs_prompt_ready last, even when
# another tool edits PROMPT_COMMAND after this file was sourced. Entries are
# separated by newlines, so a trailing comment in your PROMPT_COMMAND cannot
# swallow them.
__pfs_keep_prompt_order() {
    if [[ "$(declare -p PROMPT_COMMAND 2>/dev/null)" == "declare -a"* ]]; then
        local n=${#PROMPT_COMMAND[@]}
        if [ "$n" -ge 2 ] && [ "${PROMPT_COMMAND[0]}" = __pfs_prompt_command ] &&
            [ "${PROMPT_COMMAND[n - 1]}" = __pfs_prompt_ready ]; then
            return 0
        fi
        local -a rest=()
        local entry
        for entry in "${PROMPT_COMMAND[@]}"; do
            [[ $entry == __pfs_prompt_command || $entry == __pfs_prompt_ready ]] || rest+=("$entry")
        done
        PROMPT_COMMAND=(__pfs_prompt_command "${rest[@]}" __pfs_prompt_ready)
        return 0
    fi

    local pc=${PROMPT_COMMAND:-}
    if [[ $pc == __pfs_prompt_command$'\n'* && $pc == *$'\n'__pfs_prompt_ready ]]; then
        return 0
    fi
    pc=${pc//__pfs_prompt_command$'\n'/}
    pc=${pc//$'\n'__pfs_prompt_ready/}
    PROMPT_COMMAND=__pfs_prompt_command$'\n'${pc:+$pc$'\n'}__pfs_prompt_ready
}

# Install the DEBUG trap, chained after an existing one.
__pfs_previous_trap=$(trap -p DEBUG)
if [ -n "$__pfs_previous_trap" ]; then
    # `trap -p` prints: trap -- '<command>' DEBUG, quoted for reuse.
    __pfs_previous_trap=${__pfs_previous_trap#trap -- }
    __pfs_previous_trap=${__pfs_previous_trap% DEBUG}
    eval "__pfs_previous_trap=$__pfs_previous_trap"
    trap -- "$__pfs_previous_trap"$'\n''__pfs_debug_trap' DEBUG
else
    trap '__pfs_debug_trap' DEBUG
fi
unset __pfs_previous_trap

__pfs_keep_prompt_order
__PFS_AT_PROMPT=0

# --- The pfs function ---
pfs() {
    # Commands that do not look at the last failed command go straight to the
    # binary, and their output is shown, never evaluated.
    case "${1-}" in
        setup | config | help | -h | --help | --version)
            command pfs "$@"
            return
            ;;
    esac

    if [ -z "$__PFS_LAST_CMD" ]; then
        echo "pfs: no command has been recorded in this shell yet." >&2
        return 1
    fi
    if [ "$__PFS_LAST_EXIT" -eq 0 ]; then
        echo "✅ Last command was successful."
        return 0
    fi

    # Direct eval pattern: the binary prints the confirmed command on stdout.
    # PFS_EVAL=1 tells it stdout will be evaluated, so it never prints an
    # unconfirmed command there.
    local __pfs_out __pfs_status
    __pfs_out=$(PFS_EVAL=1 PFS_CMD="$__PFS_LAST_CMD" PFS_EXIT="$__PFS_LAST_EXIT" \
        PFS_PIPESTATUS="$__PFS_LAST_PIPESTATUS" PFS_OUTPUT="" PFS_CWD="$PWD" \
        PFS_SHELL=bash command pfs "$@")
    __pfs_status=$?
    if [ "$__pfs_status" -ne 0 ] || [ -z "$__pfs_out" ]; then
        return "$__pfs_status"
    fi

    eval "$__pfs_out"
    __pfs_status=$?
    # The fix is now the last command: if it fails too, pfs looks at it next.
    __PFS_LAST_CMD=$__pfs_out
    __PFS_LAST_EXIT=$__pfs_status
    __PFS_LAST_PIPESTATUS=$__pfs_status
    return "$__pfs_status"
}
