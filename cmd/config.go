package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/tatlilimon/PFS/internal/config"
)

var validConfigKeys = map[string]bool{
	"ollama_base_url": true,
	"ollama_model":    true,
	"offline":         true,
	"debug_level":     true,
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage PFS configuration",
	Long:  `Get or set configuration values for PFS.`,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Print a config value",
	Args:  cobra.ExactArgs(1),
	RunE:  configGetRunE,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value",
	Args:  cobra.ExactArgs(2),
	RunE:  configSetRunE,
}

func configGetRunE(cmd *cobra.Command, args []string) error {
	key := args[0]
	if !validConfigKeys[key] {
		fmt.Fprintf(cmd.ErrOrStderr(), "unknown config key: %s\n", key)
		return exitCode(ExitUsage)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Config error: %v\n", err)
		return exitCode(ExitError)
	}

	var value string
	switch key {
	case "ollama_base_url":
		value = cfg.OllamaBaseURL
	case "ollama_model":
		value = cfg.OllamaModel
	case "offline":
		value = strconv.FormatBool(cfg.OfflineMode)
	case "debug_level":
		value = strconv.Itoa(cfg.DebugLevel)
	}

	fmt.Fprintln(cmd.OutOrStdout(), value)
	return nil
}

func configSetRunE(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]
	if !validConfigKeys[key] {
		fmt.Fprintf(cmd.ErrOrStderr(), "unknown config key: %s\n", key)
		return exitCode(ExitUsage)
	}

	if key == "offline" {
		if _, err := strconv.ParseBool(value); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "invalid boolean value for %s: %s\n", key, value)
			return exitCode(ExitUsage)
		}
	}
	if key == "debug_level" {
		if _, err := strconv.Atoi(value); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "invalid integer value for %s: %s\n", key, value)
			return exitCode(ExitUsage)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Config error: %v\n", err)
		return exitCode(ExitError)
	}

	switch key {
	case "ollama_base_url":
		cfg.OllamaBaseURL = value
	case "ollama_model":
		cfg.OllamaModel = value
	case "offline":
		cfg.OfflineMode, _ = strconv.ParseBool(value)
	case "debug_level":
		cfg.DebugLevel, _ = strconv.Atoi(value)
	}

	if err := cfg.Save(); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Failed to save config: %v\n", err)
		return exitCode(ExitError)
	}

	return nil
}

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
}
