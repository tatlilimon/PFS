package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigGetMissingKey(t *testing.T) {
	resetCmd(rootCmd)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "get", "nonexistent_key"})

	err := rootCmd.Execute()
	assert.Error(t, err)

	code, ok := err.(exitCode)
	assert.True(t, ok)
	assert.Equal(t, ExitUsage, int(code))
	assert.Contains(t, buf.String(), "unknown config key")
}

func TestConfigSetMissingArgs(t *testing.T) {
	resetCmd(rootCmd)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "set", "ollama_model"})

	err := rootCmd.Execute()
	assert.Error(t, err)
}
