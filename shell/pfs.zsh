# pfs.zsh — Zsh wrapper for PFS (Please Find Solution)
# Source this file in your .zshrc: source /path/to/PFS/shell/pfs.zsh
#
# Uses add-zsh-hook preexec/precmd to capture command text, exit code, and
# pipestatus. Communicates with the Go binary via env vars — no jq, no temp
# files, no JSON.

if [ -n "${_PFS_LOADED:-}" ]; then
    return 0 2>/dev/null || true
fi
_PFS_LOADED=1

# --- State variables ---
_PFS_LAST_CMD=""
_PFS_LAST_EXIT=0
_PFS_LAST_PIPESTATUS=""
_PFS_SKIP_CAPTURE=0

# --- Hooks ---

# preexec: fires before each command. $1 is the full command string.
pfs_preexec() {
    local cmd="${1:-}"
    if [[ "$cmd" == "pfs" || "$cmd" == "pfs "* ]]; then
        _PFS_SKIP_CAPTURE=1
        return
    fi
    _PFS_SKIP_CAPTURE=0
    _PFS_LAST_CMD="$cmd"
}

# precmd: fires before each prompt. $? and $pipestatus reflect the last pipeline.
pfs_precmd() {
    local _exit=$?
    local _pipes="${pipestatus[*]}"
    if [ "$_PFS_SKIP_CAPTURE" -eq 0 ]; then
        _PFS_LAST_EXIT=$_exit
        _PFS_LAST_PIPESTATUS="$_pipes"
    fi
}

autoload -Uz add-zsh-hook
add-zsh-hook preexec pfs_preexec
add-zsh-hook precmd  pfs_precmd

# --- The pfs function ---
pfs() {
    # If arguments are passed and they're not fix/explain subcommands,
    # pass through to the Go binary directly (e.g., --version, setup, config).
    if [ $# -gt 0 ]; then
        case "$1" in
            fix|explain) ;; # fall through to captured-env flow
            *) command pfs "$@"; return $? ;;
        esac
    fi

    if [ "$_PFS_LAST_EXIT" -eq 0 ]; then
        echo "✅ Last command was successful."
        return 0
    fi

    export PFS_CMD="$_PFS_LAST_CMD"
    export PFS_EXIT="$_PFS_LAST_EXIT"
    export PFS_PIPESTATUS="$_PFS_LAST_PIPESTATUS"
    export PFS_OUTPUT=""
    export PFS_CWD="$PWD"
    export PFS_SHELL="zsh"

    local corrected
    corrected="$(command pfs "$@")" && eval "$corrected"
}
