package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Show shell integration instructions",
	Long:  `Detects your current shell and prints the source line to add to your RC file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/bash"
		}

		var rcFile, wrapperFile string
		switch {
		case strings.Contains(shell, "zsh"):
			rcFile = "~/.zshrc"
			wrapperFile = "pfs.zsh"
		case strings.Contains(shell, "fish"):
			rcFile = "~/.config/fish/config.fish"
			wrapperFile = "pfs.fish"
		default:
			rcFile = "~/.bashrc"
			wrapperFile = "pfs.bash"
		}

		// Detect wrapper directory — check common install locations
		exe, _ := os.Executable()
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		wrapperDir := findWrapperDir(exe, []string{"/usr/local/share/pfs/shell", "/usr/share/pfs/shell", "shell"})
		if wrapperDir == "" {
			wrapperDir = "/path/to/PFS/shell"
		}

		fmt.Fprintf(cmd.OutOrStdout(), "PFS Shell Setup\n")
		fmt.Fprintf(cmd.OutOrStdout(), "===============\n\n")
		fmt.Fprintf(cmd.OutOrStdout(), "Detected shell: %s\n\n", shell)
		fmt.Fprintf(cmd.OutOrStdout(), "Add this line to %s:\n\n", rcFile)
		fmt.Fprintf(cmd.OutOrStdout(), "  source %s\n\n", filepath.Join(wrapperDir, wrapperFile))
		fmt.Fprintf(cmd.OutOrStdout(), "Then reload your shell:\n\n")
		fmt.Fprintf(cmd.OutOrStdout(), "  source %s\n\n", rcFile)

		// Config location
		configDir, _ := os.UserConfigDir()
		if configDir == "" {
			configDir = "~/.config"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Config file: %s/pfs/config.yaml\n", configDir)
		fmt.Fprintf(cmd.OutOrStdout(), "\nFirst run tips:\n")
		fmt.Fprintf(cmd.OutOrStdout(), "  • Make sure Ollama is running: ollama serve\n")
		fmt.Fprintf(cmd.OutOrStdout(), "  • Pull a model: ollama pull llama3.2\n")
		fmt.Fprintf(cmd.OutOrStdout(), "  • Just type 'pfs' after a failed command\n")

		return nil
	},
}

// findWrapperDir returns the directory holding the shell wrappers for the
// binary at exe: next to it (repository root), one level up (<repo>/bin or
// <prefix>/bin), under <prefix>/share/pfs/shell (make install), then the
// fallback directories. It returns "" when none contains pfs.bash.
func findWrapperDir(exe string, fallbacks []string) string {
	var candidates []string
	if exe != "" {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "shell"),
			filepath.Join(dir, "..", "shell"),
			filepath.Join(dir, "..", "share", "pfs", "shell"),
		)
	}
	candidates = append(candidates, fallbacks...)
	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, "pfs.bash")); err == nil {
			if abs, err := filepath.Abs(dir); err == nil {
				return filepath.Clean(abs)
			}
			return filepath.Clean(dir)
		}
	}
	return ""
}
