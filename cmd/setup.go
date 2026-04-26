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

		var rcFile string
		switch {
		case strings.Contains(shell, "zsh"):
			rcFile = "~/.zshrc"
		case strings.Contains(shell, "bash"):
			rcFile = "~/.bashrc"
		case strings.Contains(shell, "fish"):
			rcFile = "~/.config/fish/config.fish"
		default:
			rcFile = "~/.bashrc"
		}

		home, _ := os.UserHomeDir()
		configPath := filepath.Join(home, ".pfs.env")

		fmt.Printf("PFS Shell Setup\n")
		fmt.Printf("===============\n\n")
		fmt.Printf("Detected shell: %s\n\n", shell)
		fmt.Printf("Add this line to %s:\n\n", rcFile)
		fmt.Printf("  source <(pfs init %s)\n\n", filepath.Base(shell))
		fmt.Printf("Then reload your shell:\n\n")
		fmt.Printf("  source %s\n\n", rcFile)
		fmt.Printf("Config file location: %s\n", configPath)

		return nil
	},
}
