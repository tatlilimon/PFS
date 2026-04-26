package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var fixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Diagnose and fix the last failed command",
	Long: `Reads environment variables set by the shell wrapper (PFS_CMD,
PFS_EXIT, PFS_OUTPUT, PFS_PIPESTATUS, PFS_CWD, PFS_SHELL) and asks
the LLM for a corrected command.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pfsCmd := os.Getenv("PFS_CMD")
		if pfsCmd == "" {
			fmt.Fprintln(cmd.ErrOrStderr(), "PFS requires shell integration. Run 'pfs setup' for instructions.")
			return exitCode(ExitError)
		}

		_ = os.Getenv("PFS_EXIT")
		_ = os.Getenv("PFS_OUTPUT")
		_ = os.Getenv("PFS_PIPESTATUS")
		_ = os.Getenv("PFS_CWD")
		_ = os.Getenv("PFS_SHELL")

		// TODO: Load config once available
		// cfg, err := config.Load()

		// TODO: Call LLM provider with the env var data
		fmt.Fprintln(cmd.ErrOrStderr(), "Fix not yet implemented")

		return nil
	},
}

// exitCode is a sentinel error that carries an exit code.
type exitCode int

func (e exitCode) Error() string {
	return fmt.Sprintf("exit code %d", int(e))
}
