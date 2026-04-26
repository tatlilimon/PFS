package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

// resetCmd clears flag state so tests don't leak into each other.
func resetCmd(cmd *cobra.Command) {
	resetFlags := func(flags *pflag.FlagSet) {
		flags.VisitAll(func(f *pflag.Flag) {
			f.Changed = false
			f.Value.Set(f.DefValue)
		})
	}
	resetFlags(cmd.Flags())
	resetFlags(cmd.PersistentFlags())
}

func TestHelpOutput(t *testing.T) {
	resetCmd(rootCmd)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"--help"})

	err := rootCmd.Execute()
	assert.NoError(t, err)

	output := buf.String()
	for _, sub := range []string{"fix", "config", "explain", "setup"} {
		assert.True(t, strings.Contains(output, sub), "help should mention %q", sub)
	}
}

func TestVersionFlag(t *testing.T) {
	resetCmd(rootCmd)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"--version"})

	err := rootCmd.Execute()
	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "pfs version")
}

func TestExitCodes(t *testing.T) {
	assert.Equal(t, 0, ExitSuccess)
	assert.Equal(t, 1, ExitError)
	assert.Equal(t, 2, ExitLLMError)
	assert.Equal(t, 3, ExitNoFix)
	assert.Equal(t, 4, ExitAmbiguous)
	assert.Equal(t, 5, ExitUsage)
}

func TestFixNoWrapper(t *testing.T) {
	resetCmd(rootCmd)
	os.Unsetenv("PFS_CMD")

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"fix"})

	err := rootCmd.Execute()
	assert.Error(t, err)

	code, ok := err.(exitCode)
	assert.True(t, ok, "error should be exitCode type")
	assert.Equal(t, ExitError, int(code))
	assert.Contains(t, buf.String(), "shell integration")
}
