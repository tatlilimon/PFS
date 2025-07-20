#!/bin/sh

# This script should be sourced in your .zshrc or .bashrc file.
# It sets up shell hooks to capture the last command's exit code and provides the `pfs` function.

# --- State Variables ---
# These variables will store the details of the last executed command.
__PFS_LAST_COMMAND=""
__PFS_LAST_EXIT_CODE=""
__PFS_LAST_COMMAND_SAVED_FOR_DEBUG_TRAP="" # Bash specific

# --- Shell-Specific Hooks ---

# Check if pfs executen on zsh
if [ -n "$ZSH_VERSION" ]; then
    # Zsh uses preexec and precmd hooks.
    autoload -U add-zsh-hook
    
    # Executed before a command is run. Store the command string.
    pfs_preexec() {
        if [[ "$1" != "pfs"* ]]; then
            __PFS_LAST_COMMAND="$1"
        fi
    }
    
    # Executed before the prompt is displayed. Store the exit code.
    pfs_precmd() {
        __PFS_LAST_EXIT_CODE=$?
    }
    
    # Register the hooks
    add-zsh-hook preexec pfs_preexec
    add-zsh-hook precmd pfs_precmd

# Check if pfs executen on zsh Bash
elif [ -n "$BASH_VERSION" ]; then
    # Bash uses a DEBUG trap and PROMPT_COMMAND.
    
    # The DEBUG trap runs *before* every command. We save the command here.
    # It's tricky because it also runs for PROMPT_COMMAND, so need a guard.
    pfs_debug_trap() {
        # Save the command, prevent recursion with PROMPT_COMMAND, and ignore `pfs` itself.
        if [ "$BASH_COMMAND" != "$__PFS_LAST_COMMAND_SAVED_FOR_DEBUG_TRAP" ] && [[ "$BASH_COMMAND" != "pfs"* ]]; then
            # To avoid capturing the `pfs` command itself, add a condition.
            # This ensures __PFS_LAST_COMMAND holds the command that failed.
            __PFS_LAST_COMMAND="$BASH_COMMAND"
        fi
    }
    
    # PROMPT_COMMAND runs just before the prompt. We save the exit code here.
    # We also update the guard variable for the DEBUG trap.
    pfs_prompt_command() {
        __PFS_LAST_EXIT_CODE=$?
        __PFS_LAST_COMMAND_SAVED_FOR_DEBUG_TRAP="$__PFS_LAST_COMMAND"
    }

    # Register the hooks
    # We need to handle if PROMPT_COMMAND is already set.
    if [ -n "$PROMPT_COMMAND" ] && ! [[ "$PROMPT_COMMAND" =~ "pfs_prompt_command" ]]; then
        PROMPT_COMMAND="pfs_prompt_command; $PROMPT_COMMAND"
    else
        PROMPT_COMMAND="pfs_prompt_command"
    fi
    
    trap 'pfs_debug_trap' DEBUG
fi

# --- The `pfs` Function ---
# This function analyzes the last failed shell command.
function pfs() {
    # Capture the exit code of the immediately preceding command. This is crucial for command chains like `lsa; pfs`.
    local exit_code_at_start=$?

    local exit_code_to_use
    # If `pfs` was called in a chain (e.g., `lsa; pfs`), then $? will be non-zero.
    # If `pfs` was called interactively after a failure, $? will be 0 (from the last successful hook),
    # but __PFS_LAST_EXIT_CODE will be non-zero.
    if [ "$exit_code_at_start" -ne 0 ]; then
        exit_code_to_use=$exit_code_at_start
    else
        exit_code_to_use=$__PFS_LAST_EXIT_CODE
    fi

    # 1. Check if the last command was successful.
    if [ "$exit_code_to_use" -eq 0 ]; then
        echo "✅ Last command was successful. Nothing to fix."
        return 0
    fi

    local command_to_use="$__PFS_LAST_COMMAND"
    # In a command chain, the whole chain is captured. Need to extract the failed command.
    # We'll assume the failed command is the first one in a semicolon-separated list.
    if [[ "$command_to_use" == *";"* ]]; then
        command_to_use=$(echo "$command_to_use" | cut -d';' -f1)
    fi

    # 2. Check for `jq`.
    if ! command -v jq &> /dev/null; then
        echo "Error: jq is not installed. Please install jq to use pfs."
        return 1
    fi
    
    # 3. Pipe the command's info to the Go application.
    jq -n --arg cmd "$command_to_use" --arg exit_code "$exit_code_to_use" \
        '{command: $cmd, exit_code: $exit_code | tonumber, output: ""}' | /usr/local/bin/pfs "$@"

    # 4. Handle the corrected command from the Go app.
    local corrected_cmd_file="/tmp/pfs_cmd"
    if [ -f "$corrected_cmd_file" ]; then
        local corrected_command
        corrected_command=$(cat "$corrected_cmd_file")
        rm -f "$corrected_cmd_file"
        echo "Executing: $corrected_command"
        eval "$corrected_command"
    fi
}