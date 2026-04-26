# pfs.bash — Bash wrapper for PFS (Please Find Solution)
# Source this file in your .bashrc: source /path/to/PFS/shell/pfs.bash
#
# Captures last command text, exit code, and PIPESTATUS via DEBUG trap +
# PROMPT_COMMAND. Communicates with the Go binary via env vars — no jq, no temp
# files, no JSON.

# Guard against double-sourcing
if [ -n "${__PFS_LOADED:-}" ]; then
    return 0 2>/dev/null || true
fi
__PFS_LOADED=1

# --- State variables ---
__PFS_LAST_CMD=""
__PFS_LAST_EXIT=0
__PFS_LAST_PIPESTATUS=""
__PFS_INSIDE_PFS=0    # Set to 1 while pfs function is executing; prevents
                       # the DEBUG trap from overwriting captured state.

# --- Hooks ---

# DEBUG trap: fires before every command. Captures the command string.
# When "pfs" is detected, sets __PFS_INSIDE_PFS=1 which causes all subsequent
# trap invocations (sub-commands inside the pfs function) to be skipped until
# PROMPT_COMMAND resets the flag.
__pfs_debug_trap() {
    if [ "${__PFS_INSIDE_PFS:-0}" -eq 1 ]; then
        return
    fi
    local cmd="${BASH_COMMAND:-}"
    if [[ "$cmd" == "pfs" || "$cmd" == "pfs "* ]]; then
        __PFS_INSIDE_PFS=1
        return
    fi
    __PFS_LAST_CMD="$cmd"
}

# PROMPT_COMMAND callback: fires just before the prompt is redrawn, after the
# previous command pipeline has completed. This is where we can read $? and
# ${PIPESTATUS[@]} reliably.
__pfs_prompt_command() {
    __PFS_INSIDE_PFS=0
    __PFS_LAST_EXIT=$?
    __PFS_LAST_PIPESTATUS="${PIPESTATUS[*]}"
}

# Install DEBUG trap — chain with any existing trap.
__pfs_previous_debug_trap="$(trap -p DEBUG 2>/dev/null | sed "s/^trap -- '//;s/' *DEBUG.*$//")"
if [ -n "$__pfs_previous_debug_trap" ]; then
    trap '__pfs_previous_debug_trap; __pfs_debug_trap' DEBUG
else
    trap '__pfs_debug_trap' DEBUG
fi
unset __pfs_previous_debug_trap

# Append to PROMPT_COMMAND (coexist with other tools like starship, direnv, etc.)
if [ -z "${PROMPT_COMMAND:-}" ]; then
    PROMPT_COMMAND='__pfs_prompt_command'
elif [[ "$PROMPT_COMMAND" != *"__pfs_prompt_command"* ]]; then
    PROMPT_COMMAND='__pfs_prompt_command; '"$PROMPT_COMMAND"
fi

# --- The pfs function ---
pfs() {
    if [ "$__PFS_LAST_EXIT" -eq 0 ]; then
        echo "✅ Last command was successful."
        return 0
    fi

    # Export env vars for the Go binary.
    export PFS_CMD="$__PFS_LAST_CMD"
    export PFS_EXIT="$__PFS_LAST_EXIT"
    export PFS_PIPESTATUS="$__PFS_LAST_PIPESTATUS"
    export PFS_OUTPUT=""
    export PFS_CWD="$PWD"
    export PFS_SHELL="bash"

    # Direct eval pattern: Go binary prints corrected command to stdout.
    local corrected
    corrected="$(command pfs "$@")" && eval "$corrected"
}
