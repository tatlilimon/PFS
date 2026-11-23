# pfs.zsh — Zsh wrapper for PFS (Please Find Solution)
# Source this file in your .zshrc: source /path/to/PFS/shell/pfs.zsh
#
# Uses add-zsh-hook preexec/precmd to record the last command line, its exit
# status and pipestatus, and passes them to the pfs binary through its
# environment only (nothing is exported to your shell). pfs prints a command on
# stdout only after you confirm it; the wrapper evaluates exactly that and
# nothing else. No jq, no temp files.

if [[ -n ${_PFS_LOADED:-} ]]; then
    return 0
fi
typeset -g _PFS_LOADED=1

# --- State ---
typeset -g _PFS_LAST_CMD=""
typeset -g _PFS_LAST_EXIT=0
typeset -g _PFS_LAST_PIPESTATUS=""
typeset -g _PFS_PENDING=0      # a command line started and has not finished
typeset -g _PFS_PENDING_CMD=""

# --- Hooks ---

# preexec: $1 is the command line as typed ($3 is the full text when history
# is off).
pfs_preexec() {
    local cmd="${1:-$3}"
    if [[ $cmd == pfs || $cmd == "pfs "* ]]; then
        # pfs itself: keep the record of the command it is fixing.
        _PFS_PENDING=0
        return
    fi
    _PFS_PENDING=1
    _PFS_PENDING_CMD=$cmd
}

# precmd: $? and $pipestatus describe the command line that just finished.
# Both are read in one statement: any command in between would reset them.
pfs_precmd() {
    local _exit=$? _pipes="${pipestatus[*]}"
    if (( _PFS_PENDING )); then
        _PFS_LAST_CMD=$_PFS_PENDING_CMD
        _PFS_LAST_EXIT=$_exit
        _PFS_LAST_PIPESTATUS=$_pipes
    fi
    _PFS_PENDING=0
    return $_exit
}

autoload -Uz add-zsh-hook
add-zsh-hook preexec pfs_preexec
add-zsh-hook precmd pfs_precmd

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

    if [[ -z $_PFS_LAST_CMD ]]; then
        print -u2 "pfs: no command has been recorded in this shell yet."
        return 1
    fi
    if (( _PFS_LAST_EXIT == 0 )); then
        echo "✅ Last command was successful."
        return 0
    fi

    # PFS_EVAL=1 tells the binary that stdout will be evaluated, so it never
    # prints an unconfirmed command there.
    local __pfs_out __pfs_status
    __pfs_out=$(PFS_EVAL=1 PFS_CMD=$_PFS_LAST_CMD PFS_EXIT=$_PFS_LAST_EXIT \
        PFS_PIPESTATUS=$_PFS_LAST_PIPESTATUS PFS_OUTPUT= PFS_CWD=$PWD \
        PFS_SHELL=zsh command pfs "$@")
    __pfs_status=$?
    if (( __pfs_status != 0 )) || [[ -z $__pfs_out ]]; then
        return $__pfs_status
    fi

    eval "$__pfs_out"
    __pfs_status=$?
    # The fix is now the last command: if it fails too, pfs looks at it next.
    _PFS_LAST_CMD=$__pfs_out
    _PFS_LAST_EXIT=$__pfs_status
    _PFS_LAST_PIPESTATUS=$__pfs_status
    return $__pfs_status
}
