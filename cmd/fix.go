package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

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
the LLM for a corrected command.

When PFS_EVAL=1 (set by the shell wrappers, which evaluate stdout), a
command is printed on stdout only after you confirm it in the TUI.`,
	Args: usageArgs(cobra.NoArgs),
	RunE: fixRunE,
}

// isInteractive reports whether the confirmation TUI can be shown. Tests
// replace it so they never start the TUI.
var isInteractive = tui.IsInteractive

// invocation is what the shell wrapper reports about the failed command.
type invocation struct {
	command  string
	exitCode int
	output   string
	cwd      string
	shell    string
	evalMode bool
}

func readInvocation(cmd *cobra.Command) (invocation, error) {
	inv := invocation{
		command:  os.Getenv("PFS_CMD"),
		output:   os.Getenv("PFS_OUTPUT"),
		cwd:      os.Getenv("PFS_CWD"),
		shell:    os.Getenv("PFS_SHELL"),
		evalMode: os.Getenv("PFS_EVAL") == "1",
	}
	if inv.command == "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "PFS requires shell integration. Run 'pfs setup' for instructions.")
		return inv, exitCode(ExitError)
	}
	inv.exitCode, _ = strconv.Atoi(strings.TrimSpace(os.Getenv("PFS_EXIT")))
	return inv, nil
}

// loadConfig loads the config and applies -v/-vv, which raise debug_level
// for this run (diagnostics go to stderr).
func loadConfig(cmd *cobra.Command) (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Config error: %v\n", err)
		return nil, exitCode(ExitError)
	}
	if v := min(verbose, int(config.DebugLevelDetailed)); v > cfg.DebugLevel {
		cfg.DebugLevel = v
	}
	return cfg, nil
}

func environment(inv invocation) llm.EnvironmentContext {
	return llm.EnvironmentContext{
		OS:           runtime.GOOS,
		Shell:        inv.shell,
		Architecture: runtime.GOARCH,
		CWD:          inv.cwd,
		ShellVersion: getShellVersion(inv.shell),
	}
}

func fixRunE(cmd *cobra.Command, args []string) error {
	if err := checkFormat(); err != nil {
		return err
	}
	inv, err := readInvocation(cmd)
	if err != nil {
		return err
	}

	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	envCtx := environment(inv)

	if format == "text" && isInteractive() {
		return runInteractive(cmd, cfg, envCtx, inv)
	}
	return runNonInteractive(cmd, cfg, envCtx, inv)
}

func runInteractive(cmd *cobra.Command, cfg *config.Config, envCtx llm.EnvironmentContext, inv invocation) error {
	provider, err := llm.NewOllamaProvider(cfg)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "LLM provider error: %v\n", err)
		return exitCode(ExitLLMError)
	}

	model := tui.NewModel(cfg, provider, envCtx, inv.command, inv.output, inv.exitCode)
	finalModel, err := tui.RunProgram(model)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "TUI error: %v\n", err)
		return exitCode(ExitError)
	}

	// stdout is captured by the shell wrapper — corrected command ONLY, and
	// only after the user confirmed it.
	if correctedOutput := finalModel.GetCorrectedOutput(); correctedOutput != "" {
		fmt.Fprint(cmd.OutOrStdout(), correctedOutput)
		return nil
	}

	if finalModel.GetState() == tui.StateError {
		var editorErr *tui.EditorError
		if errors.As(finalModel.GetError(), &editorErr) {
			return exitCode(ExitError)
		}
		return exitCode(ExitLLMError)
	}
	if finalModel.Ambiguous() {
		return exitCode(ExitAmbiguous)
	}
	return exitCode(ExitNoFix)
}

func runNonInteractive(cmd *cobra.Command, cfg *config.Config, envCtx llm.EnvironmentContext, inv invocation) error {
	provider, err := llm.NewOllamaProvider(cfg)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "LLM provider error: %v\n", err)
		return exitCode(ExitLLMError)
	}
	provider.SetDebugOutput(cmd.ErrOrStderr())

	correction, err := provider.GetCorrection(context.Background(), inv.command, inv.output, inv.exitCode, envCtx)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "LLM error: %v\n", err)
		return exitCode(ExitLLMError)
	}

	hasCommand := correction.CorrectedCommand != nil && *correction.CorrectedCommand != ""
	result := ExitSuccess
	if !hasCommand {
		result = ExitNoFix
		if len(correction.AlternativeCommands) > 0 {
			result = ExitAmbiguous
		}
	}

	if inv.evalMode {
		// The wrapper evaluates stdout and there is no terminal to confirm
		// on, so show the suggestion and never emit it.
		if format == "json" {
			if err := writeJSON(cmd.ErrOrStderr(), correction); err != nil {
				return err
			}
		} else {
			writeSuggestion(cmd.ErrOrStderr(), correction)
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "pfs: the suggestion was not run (no interactive terminal to confirm it).")
		if result == ExitSuccess {
			result = ExitNoFix
		}
		return exitCode(result)
	}

	if format == "json" {
		if err := writeJSON(cmd.OutOrStdout(), correction); err != nil {
			return err
		}
		if result == ExitSuccess {
			return nil
		}
		return exitCode(result)
	}

	if hasCommand {
		fmt.Fprint(cmd.OutOrStdout(), *correction.CorrectedCommand)
		return nil
	}

	writeSuggestion(cmd.ErrOrStderr(), correction)
	return exitCode(result)
}

func writeJSON(w io.Writer, correction *llm.Correction) error {
	data, err := json.Marshal(correction)
	if err != nil {
		fmt.Fprintf(w, "JSON marshal error: %v\n", err)
		return exitCode(ExitError)
	}
	fmt.Fprintln(w, string(data))
	return nil
}

// writeSuggestion prints a correction as plain text for a person to read.
func writeSuggestion(w io.Writer, c *llm.Correction) {
	if c.Diagnosis != "" {
		fmt.Fprintln(w, llm.SanitizeText(c.Diagnosis))
	}
	if c.CorrectedCommand != nil && *c.CorrectedCommand != "" {
		fmt.Fprintf(w, "Suggested command (not run):\n  %s\n", strings.ReplaceAll(*c.CorrectedCommand, "\n", "\n  "))
	}
	if len(c.AlternativeCommands) > 0 {
		fmt.Fprintln(w, "Possible approaches (not run):")
		for _, alt := range c.AlternativeCommands {
			fmt.Fprintf(w, "  %s\n", llm.SanitizeText(alt))
		}
	}
}

type exitCode int

func (e exitCode) Error() string {
	return fmt.Sprintf("exit code %d", int(e))
}

// knownShells limits which programs getShellVersion may run: PFS_SHELL comes
// from the environment and must not select an arbitrary executable.
var knownShells = map[string]bool{"bash": true, "zsh": true, "fish": true, "sh": true, "dash": true, "ksh": true}

// getShellVersion returns the first line of `<shell> --version`.
func getShellVersion(shell string) string {
	if !knownShells[shell] {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, shell, "--version").Output()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return strings.TrimSpace(line)
}
