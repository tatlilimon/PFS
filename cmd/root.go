package main

import (
	"github.com/spf13/cobra"
)

// Exit codes as specified by G8 guardrail.
const (
	ExitSuccess   = 0
	ExitError     = 1
	ExitLLMError  = 2
	ExitNoFix     = 3
	ExitAmbiguous = 4
	ExitUsage     = 5
)

// Version is set at build time via -ldflags.
var Version = "dev"

var (
	format  string
	verbose int
)

var rootCmd = &cobra.Command{
	Use:   "pfs",
	Short: "Please Find Solution — diagnose and fix failed shell commands",
	Long: `PFS analyzes your last failed shell command using an LLM
and suggests a fix. Requires shell integration (run 'pfs setup').`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return fixCmd.RunE(cmd, args)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&format, "format", "text", "output format: text or json")
	rootCmd.PersistentFlags().CountVarP(&verbose, "verbose", "v", "verbosity level (stackable: -v, -vv)")

	rootCmd.SetVersionTemplate("pfs version {{.Version}}\n")
	rootCmd.Version = Version

	rootCmd.AddCommand(fixCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(explainCmd)
	rootCmd.AddCommand(setupCmd)
}

func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		if code, ok := err.(exitCode); ok {
			return int(code)
		}
		return ExitError
	}
	return ExitSuccess
}
