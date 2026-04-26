package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/tatlilimon/PFS/internal/config"
	"github.com/tatlilimon/PFS/internal/llm"
)

var explainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Explain the last command (no fix suggested)",
	Long: `Reads environment variables set by the shell wrapper and asks
the LLM to explain what the last command did, without suggesting a fix.`,
	RunE: explainRunE,
}

func explainRunE(cmd *cobra.Command, args []string) error {
	pfsCmd := os.Getenv("PFS_CMD")
	if pfsCmd == "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "PFS requires shell integration. Run 'pfs setup' for instructions.")
		return exitCode(ExitError)
	}

	exitCodeVal, _ := strconv.Atoi(os.Getenv("PFS_EXIT"))
	output := os.Getenv("PFS_OUTPUT")
	cwd := os.Getenv("PFS_CWD")
	shellName := os.Getenv("PFS_SHELL")

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

	envCtx := llm.EnvironmentContext{
		OS:           runtime.GOOS,
		Shell:        shellName,
		Architecture: runtime.GOARCH,
		CWD:          cwd,
		ShellVersion: getShellVersion(shellName),
	}

	ctx := context.Background()
	correction, err := provider.GetCorrection(ctx, pfsCmd, output, exitCodeVal, envCtx)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "LLM error: %v\n", err)
		return exitCode(ExitLLMError)
	}

	fmt.Fprintln(cmd.ErrOrStderr(), correction.Diagnosis)
	if correction.Explanation != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), *correction.Explanation)
	}

	return nil
}
