package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tatlilimon/PFS/internal/config"
)

func setupCmdTestDir(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	return tmpDir
}

func writeTestConfig(t *testing.T, cfg *config.Config) {
	t.Helper()
	dir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "pfs")
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, cfg.Save())
}

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

func TestConfigGet_ReturnsValue(t *testing.T) {
	setupCmdTestDir(t)
	writeTestConfig(t, &config.Config{
		OllamaBaseURL: "http://localhost:11434",
		OllamaModel:   "test-model",
	})

	resetCmd(rootCmd)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "get", "ollama_model"})

	err := rootCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "test-model")
}

func TestConfigSet_SavesValue(t *testing.T) {
	setupCmdTestDir(t)
	cfg, err := config.Load()
	require.NoError(t, err)
	writeTestConfig(t, cfg)

	resetCmd(rootCmd)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "set", "ollama_model", "my-custom-model"})

	err = rootCmd.Execute()
	assert.NoError(t, err)

	loaded, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "my-custom-model", loaded.OllamaModel)
}

func TestConfigSet_Offline(t *testing.T) {
	setupCmdTestDir(t)
	cfg, err := config.Load()
	require.NoError(t, err)
	writeTestConfig(t, cfg)

	resetCmd(rootCmd)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "set", "offline", "true"})

	err = rootCmd.Execute()
	assert.NoError(t, err)

	loaded, err := config.Load()
	require.NoError(t, err)
	assert.True(t, loaded.OfflineMode)
}

func TestConfigSet_InvalidBool(t *testing.T) {
	resetCmd(rootCmd)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "set", "offline", "notabool"})

	err := rootCmd.Execute()
	assert.Error(t, err)
	code, ok := err.(exitCode)
	assert.True(t, ok)
	assert.Equal(t, ExitUsage, int(code))
}

func TestConfigSet_InvalidInt(t *testing.T) {
	resetCmd(rootCmd)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "set", "debug_level", "notanumber"})

	err := rootCmd.Execute()
	assert.Error(t, err)
	code, ok := err.(exitCode)
	assert.True(t, ok)
	assert.Equal(t, ExitUsage, int(code))
}

func TestConfigGet_DefaultValue(t *testing.T) {
	setupCmdTestDir(t)

	resetCmd(rootCmd)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "get", "ollama_base_url"})

	err := rootCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "http://localhost:11434")
}
