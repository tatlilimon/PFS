package main

import (
	"errors"
	"fmt"

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
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return usageError{fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())}
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return fixCmd.RunE(cmd, args)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&format, "format", "text", "output format: text or json")
	rootCmd.PersistentFlags().CountVarP(&verbose, "verbose", "v", "verbosity level (stackable: -v, -vv)")

	rootCmd.SetVersionTemplate("pfs version {{.Version}}\n")
	rootCmd.Version = Version

	// No shell completions (guardrail G12).
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return usageError{err}
	})

	rootCmd.AddCommand(fixCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(explainCmd)
	rootCmd.AddCommand(setupCmd)
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	cmd, err := rootCmd.ExecuteC()
	if err == nil {
		return ExitSuccess
	}
	var code exitCode
	if errors.As(err, &code) {
		return int(code)
	}
	var uerr usageError
	if errors.As(err, &uerr) {
		path := "pfs"
		if cmd != nil {
			path = cmd.CommandPath()
		}
		fmt.Fprintf(rootCmd.ErrOrStderr(), "pfs: %v\nRun '%s --help' for usage.\n", uerr.err, path)
		return ExitUsage
	}
	fmt.Fprintf(rootCmd.ErrOrStderr(), "pfs: %v\n", err)
	return ExitError
}

// usageError marks invalid flags, arguments or values (exit code 5).
type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

// usageArgs wraps a cobra argument validator so its errors map to ExitUsage.
func usageArgs(validate cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := validate(cmd, args); err != nil {
			return usageError{err}
		}
		return nil
	}
}

func checkFormat() error {
	if format != "text" && format != "json" {
		return usageError{fmt.Errorf("invalid --format %q (use text or json)", format)}
	}
	return nil
}
