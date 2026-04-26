package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		if !validConfigKeys[key] {
			fmt.Fprintf(cmd.ErrOrStderr(), "unknown config key: %s\n", key)
			return exitCode(ExitUsage)
		}

		// TODO: Wire to config.Load() once config system is rewritten (T1)
		fmt.Fprintf(cmd.ErrOrStderr(), "config get not yet implemented (key=%s)\n", key)
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
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

		// TODO: Wire to config.Load()/Save() once config system is rewritten (T1)
		fmt.Fprintf(cmd.ErrOrStderr(), "config set not yet implemented (key=%s, value=%s)\n", key, value)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
}
