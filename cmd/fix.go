package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tatlilimon/PFS/internal/config"
	"github.com/tatlilimon/PFS/internal/llm"
	"github.com/tatlilimon/PFS/internal/tui"
)

var fixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Diagnose and fix the last failed command",
	Long: `Reads environment variables set by the shell wrapper (PFS_CMD,
PFS_EXIT, PFS_OUTPUT, PFS_PIPESTATUS, PFS_CWD, PFS_SHELL) and asks
the LLM for a corrected command.`,
	RunE: fixRunE,
}

func fixRunE(cmd *cobra.Command, args []string) error {
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

	envCtx := llm.EnvironmentContext{
		OS:           runtime.GOOS,
		Shell:        shellName,
		Architecture: runtime.GOARCH,
		CWD:          cwd,
		ShellVersion: getShellVersion(shellName),
	}

	if format == "json" || !tui.IsTerminal() {
		return runNonInteractive(cmd, cfg, envCtx, pfsCmd, output, exitCodeVal)
	}

	return runInteractive(cmd, cfg, envCtx, pfsCmd, output, exitCodeVal)
}

func runInteractive(cmd *cobra.Command, cfg *config.Config, envCtx llm.EnvironmentContext, command, output string, exitCodeVal int) error {
	provider, err := llm.NewOllamaProvider(cfg)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "LLM provider error: %v\n", err)
		return exitCode(ExitLLMError)
	}

	model := tui.NewModel(cfg, provider, envCtx, command, output, exitCodeVal)
	finalModel, err := tui.RunProgram(model)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "TUI error: %v\n", err)
		return exitCode(ExitError)
	}

	correctedOutput := finalModel.GetCorrectedOutput()

	// stdout is captured by the shell wrapper — corrected command ONLY
	if correctedOutput != "" {
		fmt.Fprint(cmd.OutOrStdout(), correctedOutput)
		return nil
	}

	return exitCode(ExitNoFix)
}

func runNonInteractive(cmd *cobra.Command, cfg *config.Config, envCtx llm.EnvironmentContext, command, output string, exitCodeVal int) error {
	provider, err := llm.NewOllamaProvider(cfg)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "LLM provider error: %v\n", err)
		return exitCode(ExitLLMError)
	}

	ctx := context.Background()
	correction, err := provider.GetCorrection(ctx, command, output, exitCodeVal, envCtx)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "LLM error: %v\n", err)
		return exitCode(ExitLLMError)
	}

	if format == "json" {
		data, marshalErr := json.Marshal(correction)
		if marshalErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "JSON marshal error: %v\n", marshalErr)
			return exitCode(ExitError)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	if correction.CorrectedCommand != nil && *correction.CorrectedCommand != "" {
		fmt.Fprint(cmd.OutOrStdout(), *correction.CorrectedCommand)
		return nil
	}

	fmt.Fprintln(cmd.ErrOrStderr(), correction.Diagnosis)
	return exitCode(ExitNoFix)
}

type exitCode int

func (e exitCode) Error() string {
	return fmt.Sprintf("exit code %d", int(e))
}

// getShellVersion tries to get the shell version string.
func getShellVersion(shell string) string {
	if shell == "" {
		return ""
	}
	out, err := exec.Command(shell, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
