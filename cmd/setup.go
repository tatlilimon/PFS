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
		wrapperDir := detectWrapperDir()

		fmt.Fprintf(cmd.OutOrStdout(), "PFS Shell Setup\n")
		fmt.Fprintf(cmd.OutOrStdout(), "===============\n\n")
		fmt.Fprintf(cmd.OutOrStdout(), "Detected shell: %s\n\n", shell)
		fmt.Fprintf(cmd.OutOrStdout(), "Add this line to %s:\n\n", rcFile)
		fmt.Fprintf(cmd.OutOrStdout(), "  source %s/shell/%s\n\n", wrapperDir, wrapperFile)
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

func detectWrapperDir() string {
	// Try to find the wrapper directory relative to the binary
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		shellDir := filepath.Join(dir, "..", "shell")
		if _, err := os.Stat(filepath.Join(shellDir, "pfs.bash")); err == nil {
			abs, _ := filepath.Abs(shellDir)
			return abs
		}
	}

	// Check common locations
	candidates := []string{
		"/usr/local/share/pfs",
		"/usr/share/pfs",
		".",
	}
	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, "shell", "pfs.bash")); err == nil {
			abs, _ := filepath.Abs(dir)
			return abs
		}
	}

	// Fallback: tell user to use the repo path
	return "/path/to/PFS"
}
