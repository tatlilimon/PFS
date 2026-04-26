package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var explainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Explain the last command (no fix suggested)",
	Long: `Reads environment variables set by the shell wrapper and asks
the LLM to explain what the last command did, without suggesting a fix.`,
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

		// TODO: Call LLM provider for explanation
		fmt.Fprintln(cmd.ErrOrStderr(), "Explain not yet implemented")

		return nil
	},
}
