# pfs.fish — Fish wrapper for PFS (Please Find Solution)
# Source this file in your config.fish: source /path/to/PFS/shell/pfs.fish
#
# Uses the fish_preexec / fish_postexec events to record the last command
# line, its exit status and pipestatus, and passes them to the pfs binary
# through its environment only. pfs prints a command on stdout only after you
# confirm it; the wrapper evaluates exactly that and nothing else.
# No jq, no temp files.

# Fish has no early-return for sourced files, so the whole init is wrapped in
# one conditional block to make the guard effective.
if not set -q __pfs_loaded
    set -g __pfs_loaded 1

    # --- State ---
    set -g __pfs_last_cmd ""
    set -g __pfs_last_exit 0
    set -g __pfs_last_pipestatus ""
    set -g __pfs_pending 0
    set -g __pfs_pending_cmd ""

    # --- Hooks ---

    # fish_preexec: $argv[1] is the full command line.
    function __pfs_preexec --on-event fish_preexec
        set -l cmd $argv[1]
        if test "$cmd" = pfs; or string match -q -- 'pfs *' "$cmd"
            # pfs itself: keep the record of the command it is fixing.
            set -g __pfs_pending 0
            return
        end
        set -g __pfs_pending 1
        set -g __pfs_pending_cmd $cmd
    end

    # fish_postexec: $status and $pipestatus describe the finished command.
    function __pfs_postexec --on-event fish_postexec
        set -l _exit $status
        set -l _pipes (string join ' ' $pipestatus)
        if test "$__pfs_pending" = 1
            set -g __pfs_last_cmd $__pfs_pending_cmd
            set -g __pfs_last_exit $_exit
            set -g __pfs_last_pipestatus $_pipes
        end
        set -g __pfs_pending 0
    end

    # --- The pfs function ---
    function pfs --description 'Fix the last failed command with PFS'
        # Commands that do not look at the last failed command go straight to
        # the binary, and their output is shown, never evaluated.
        if contains -- "$argv[1]" setup config help -h --help --version
            command pfs $argv
            return $status
        end

        if test -z "$__pfs_last_cmd"
            echo "pfs: no command has been recorded in this shell yet." >&2
            return 1
        end
        if test "$__pfs_last_exit" -eq 0
            echo "✅ Last command was successful."
            return 0
        end

        # PFS_EVAL=1 tells the binary that stdout will be evaluated, so it
        # never prints an unconfirmed command there.
        set -l lines (PFS_EVAL=1 PFS_CMD=$__pfs_last_cmd PFS_EXIT=$__pfs_last_exit \
            PFS_PIPESTATUS=$__pfs_last_pipestatus PFS_OUTPUT= PFS_CWD=$PWD \
            PFS_SHELL=fish command pfs $argv)
        set -l st $status
        # Command substitution splits on newlines; join them back (and keep
        # them joined with string collect) so a multi-line command runs as
        # written instead of being flattened into one line by eval.
        set -l out (string join \n -- $lines | string collect)
        if test $st -ne 0; or test -z "$out"
            return $st
        end

        eval $out
        set st $status
        # The fix is now the last command: if it fails too, pfs looks at it next.
        set -g __pfs_last_cmd $out
        set -g __pfs_last_exit $st
        set -g __pfs_last_pipestatus $st
        return $st
    end
end
