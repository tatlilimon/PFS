# pfs.fish — Fish wrapper for PFS (Please Find Solution)
# Source this file in your config.fish: source /path/to/PFS/shell/pfs.fish
#
# Uses fish_preexec / fish_postexec events to capture command text, exit code,
# and pipe status. Communicates with the Go binary via env vars — no jq, no temp
# files, no JSON.

# Fish has no early-return for sourced files, so we wrap the entire init in
# a single conditional block to make the guard effective.
if not set -q _PFS_LOADED

set -g _PFS_LOADED 1

# --- State variables ---
set -g __pfs_last_cmd ""
set -g __pfs_last_exit 0
set -g __pfs_last_pipestatus ""
set -g __pfs_skip_capture 0

# --- Hooks ---

# fish_preexec: fires before each command. $argv[1] is the full command string.
function __pfs_preexec --on-event fish_preexec
    set -l cmd "$argv[1]"
    if string match -q "pfs" -- "$cmd"; or string match -q "pfs *" -- "$cmd"
        set -g __pfs_skip_capture 1
        return
    end
    set -g __pfs_skip_capture 0
    set -g __pfs_last_cmd "$cmd"
end

# fish_postexec: fires after each command. $status and $pipestatus are available.
function __pfs_postexec --on-event fish_postexec
    if test "$__pfs_skip_capture" -eq 0
        set -g __pfs_last_exit $status
        set -g __pfs_last_pipestatus (string join " " $pipestatus)
    end
end

# --- The pfs function ---
function pfs
    # If arguments are passed and they're not fix/explain subcommands,
    # pass through to the Go binary directly (e.g., --version, setup, config).
    if test (count $argv) -gt 0
        switch $argv[1]
            case fix explain
                # fall through to captured-env flow
            case '*'
                command pfs $argv
                return $status
        end
    end

    if test "$__pfs_last_exit" -eq 0
        echo "✅ Last command was successful."
        return 0
    end

    set -lx PFS_CMD "$__pfs_last_cmd"
    set -lx PFS_EXIT "$__pfs_last_exit"
    set -lx PFS_PIPESTATUS "$__pfs_last_pipestatus"
    set -lx PFS_OUTPUT ""
    set -lx PFS_CWD "$PWD"
    set -lx PFS_SHELL="fish"

    set -l corrected (env PFS_CMD="$PFS_CMD" PFS_EXIT="$PFS_EXIT" \
        PFS_PIPESTATUS="$PFS_PIPESTATUS" PFS_OUTPUT="" \
        PFS_CWD="$PFS_CWD" PFS_SHELL="fish" \
        command pfs $argv)
    if test -n "$corrected"
        eval $corrected
    end
end

    set -lx PFS_CMD "$__pfs_last_cmd"
    set -lx PFS_EXIT "$__pfs_last_exit"
    set -lx PFS_PIPESTATUS "$__pfs_last_pipestatus"
    set -lx PFS_OUTPUT ""
    set -lx PFS_CWD "$PWD"
    set -lx PFS_SHELL "fish"

    set -l corrected (env PFS_CMD="$PFS_CMD" PFS_EXIT="$PFS_EXIT" \
        PFS_PIPESTATUS="$PFS_PIPESTATUS" PFS_OUTPUT="" \
        PFS_CWD="$PFS_CWD" PFS_SHELL="fish" \
        command pfs $argv)
    if test -n "$corrected"
        eval $corrected
    end
end

end
