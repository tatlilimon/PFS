package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tatlilimon/PFS/internal/config"
	"github.com/tatlilimon/PFS/internal/llm"
)

var explainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Explain the last command (no fix suggested)",
	Long: `Reads environment variables set by the shell wrapper and asks
the LLM to explain what the last command did, without suggesting a fix.
Nothing is printed on stdout and nothing is run.`,
	Args: usageArgs(cobra.NoArgs),
	RunE: explainRunE,
}

func explainRunE(cmd *cobra.Command, args []string) error {
	if err := checkFormat(); err != nil {
		return err
	}
	inv, err := readInvocation(cmd)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Config error: %v\n", err)
		return exitCode(ExitError)
	}

	provider, err := llm.NewOllamaProvider(cfg)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "LLM provider error: %v\n", err)
		return exitCode(ExitLLMError)
	}
	provider.SetDebugOutput(cmd.ErrOrStderr())

	correction, err := provider.GetCorrection(context.Background(), inv.command, inv.output, inv.exitCode, environment(inv))
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "LLM error: %v\n", err)
		return exitCode(ExitLLMError)
	}

	fmt.Fprintln(cmd.ErrOrStderr(), llm.SanitizeText(correction.Diagnosis))
	if correction.Explanation != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), llm.SanitizeText(*correction.Explanation))
	}

	return nil
}
