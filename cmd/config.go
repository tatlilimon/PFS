package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tatlilimon/PFS/internal/config"
)

// configKeys maps the accepted key names to their canonical name. The YAML
// file and the README call the offline flag "offline_mode"; "offline" is the
// original CLI spelling and stays accepted.
var configKeys = map[string]string{
	"ollama_base_url": "ollama_base_url",
	"ollama_model":    "ollama_model",
	"offline":         "offline",
	"offline_mode":    "offline",
	"debug_level":     "debug_level",
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage PFS configuration",
	Long: `Get or set configuration values for PFS.

Keys: ollama_base_url, ollama_model, offline_mode (alias: offline), debug_level (0, 1 or 2).`,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Print a config value",
	Args:  usageArgs(cobra.ExactArgs(1)),
	RunE:  configGetRunE,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value",
	Args:  usageArgs(cobra.ExactArgs(2)),
	RunE:  configSetRunE,
}

func configGetRunE(cmd *cobra.Command, args []string) error {
	key, ok := configKeys[args[0]]
	if !ok {
		fmt.Fprintf(cmd.ErrOrStderr(), "unknown config key: %s\n", args[0])
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
	key, ok := configKeys[args[0]]
	value := args[1]
	if !ok {
		fmt.Fprintf(cmd.ErrOrStderr(), "unknown config key: %s\n", args[0])
		return exitCode(ExitUsage)
	}

	if err := validateConfigValue(key, value); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "invalid value for %s: %v\n", args[0], err)
		return exitCode(ExitUsage)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Config error: %v\n", err)
		return exitCode(ExitError)
	}

	switch key {
	case "ollama_base_url":
		cfg.OllamaBaseURL = strings.TrimRight(value, "/")
	case "ollama_model":
		cfg.OllamaModel = strings.TrimSpace(value)
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

func validateConfigValue(key, value string) error {
	switch key {
	case "ollama_base_url":
		u, err := url.Parse(value)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("%q is not an http(s) URL; example: http://localhost:11434", value)
		}
	case "ollama_model":
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("model name cannot be empty")
		}
	case "offline":
		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("%q is not a boolean (use true or false)", value)
		}
	case "debug_level":
		n, err := strconv.Atoi(value)
		if err != nil || n < int(config.DebugLevelNone) || n > int(config.DebugLevelDetailed) {
			return fmt.Errorf("%q is not 0, 1 or 2", value)
		}
	}
	return nil
}

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
}
