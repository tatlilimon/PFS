package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	origConfigDir := configDir
	tmpDir := t.TempDir()
	configDir = tmpDir
	t.Cleanup(func() {
		configDir = origConfigDir
	})
	return tmpDir
}

func TestLoadDefaults(t *testing.T) {
	_ = setupTestDir(t)

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "http://localhost:11434", cfg.OllamaBaseURL)
	assert.Equal(t, "llama3.2", cfg.OllamaModel)
	assert.True(t, cfg.OfflineMode)
	assert.Equal(t, 0, cfg.DebugLevel)
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := setupTestDir(t)

	cfg := &Config{
		OllamaBaseURL: "http://custom:1234",
		OllamaModel:   "custom-model",
		OfflineMode:   false,
		DebugLevel:    2,
	}

	err := cfg.Save()
	require.NoError(t, err)

	configFile := filepath.Join(tmpDir, "pfs", "config.yaml")
	assert.FileExists(t, configFile)

	loaded, err := Load()
	require.NoError(t, err)

	assert.Equal(t, cfg.OllamaBaseURL, loaded.OllamaBaseURL)
	assert.Equal(t, cfg.OllamaModel, loaded.OllamaModel)
	assert.Equal(t, cfg.OfflineMode, loaded.OfflineMode)
	assert.Equal(t, cfg.DebugLevel, loaded.DebugLevel)
}

func TestMigration(t *testing.T) {
	tmpDir := setupTestDir(t)

	home := t.TempDir()
	t.Setenv("HOME", home)

	envContent := "OLLAMA_BASE_URL=http://migrated:9999\nOLLAMA_MODEL=migrated-model\nOFFLINE_MODE=false\nDEBUG_LEVEL=2\n"
	envPath := filepath.Join(home, ".pfs.env")
	err := os.WriteFile(envPath, []byte(envContent), 0644)
	require.NoError(t, err)

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "http://migrated:9999", cfg.OllamaBaseURL)
	assert.Equal(t, "migrated-model", cfg.OllamaModel)
	assert.False(t, cfg.OfflineMode)
	assert.Equal(t, 2, cfg.DebugLevel)

	assert.NoFileExists(t, envPath)
	assert.FileExists(t, envPath+".bak")

	configFile := filepath.Join(tmpDir, "pfs", "config.yaml")
	assert.FileExists(t, configFile)
}

func TestLoadCorrupt(t *testing.T) {
	tmpDir := setupTestDir(t)

	configDir := filepath.Join(tmpDir, "pfs")
	err := os.MkdirAll(configDir, 0755)
	require.NoError(t, err)

	corruptData := []byte("{{{{not valid yaml}}}}")
	err = os.WriteFile(filepath.Join(configDir, "config.yaml"), corruptData, 0644)
	require.NoError(t, err)

	cfg, err := Load()
	assert.Nil(t, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse config")
}

func TestDirectoryCreation(t *testing.T) {
	tmpDir := setupTestDir(t)

	nestedDir := filepath.Join(tmpDir, "pfs")
	assert.NoDirExists(t, nestedDir)

	cfg := &Config{
		OllamaBaseURL: "http://localhost:11434",
		OllamaModel:   "test-model",
		OfflineMode:   true,
		DebugLevel:    0,
	}

	err := cfg.Save()
	require.NoError(t, err)

	assert.DirExists(t, nestedDir)
	assert.FileExists(t, filepath.Join(nestedDir, "config.yaml"))
}

func TestParseEnvFile(t *testing.T) {
	tmpDir := t.TempDir()

	content := `# comment line
OLLAMA_BASE_URL=http://test:1234
OLLAMA_MODEL=test-model

OFFLINE_MODE=true
DEBUG_LEVEL=1
`
	envPath := filepath.Join(tmpDir, "test.env")
	err := os.WriteFile(envPath, []byte(content), 0644)
	require.NoError(t, err)

	result, err := parseEnvFile(envPath)
	require.NoError(t, err)

	assert.Equal(t, "http://test:1234", result["OLLAMA_BASE_URL"])
	assert.Equal(t, "test-model", result["OLLAMA_MODEL"])
	assert.Equal(t, "true", result["OFFLINE_MODE"])
	assert.Equal(t, "1", result["DEBUG_LEVEL"])
	assert.Equal(t, 4, len(result))
}

func TestMigrationWithPartialEnv(t *testing.T) {
	_ = setupTestDir(t)

	home := t.TempDir()
	t.Setenv("HOME", home)

	envContent := "OLLAMA_MODEL=partial-model\n"
	envPath := filepath.Join(home, ".pfs.env")
	err := os.WriteFile(envPath, []byte(envContent), 0644)
	require.NoError(t, err)

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "http://localhost:11434", cfg.OllamaBaseURL)
	assert.Equal(t, "partial-model", cfg.OllamaModel)
	assert.True(t, cfg.OfflineMode)
	assert.Equal(t, 0, cfg.DebugLevel)
}

func TestSaveOverwritesExisting(t *testing.T) {
	_ = setupTestDir(t)

	cfg1 := &Config{
		OllamaBaseURL: "http://first:1111",
		OllamaModel:   "first-model",
		OfflineMode:   true,
		DebugLevel:    0,
	}
	err := cfg1.Save()
	require.NoError(t, err)

	cfg2 := &Config{
		OllamaBaseURL: "http://second:2222",
		OllamaModel:   "second-model",
		OfflineMode:   false,
		DebugLevel:    1,
	}
	err = cfg2.Save()
	require.NoError(t, err)

	loaded, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "http://second:2222", loaded.OllamaBaseURL)
	assert.Equal(t, "second-model", loaded.OllamaModel)
	assert.False(t, loaded.OfflineMode)
	assert.Equal(t, 1, loaded.DebugLevel)
}

func TestYAMLRoundtrip(t *testing.T) {
	original := &Config{
		OllamaBaseURL: "http://roundtrip:4321",
		OllamaModel:   "roundtrip-model",
		OfflineMode:   true,
		DebugLevel:    3,
	}

	data, err := yaml.Marshal(original)
	require.NoError(t, err)

	var restored Config
	err = yaml.Unmarshal(data, &restored)
	require.NoError(t, err)

	assert.Equal(t, original.OllamaBaseURL, restored.OllamaBaseURL)
	assert.Equal(t, original.OllamaModel, restored.OllamaModel)
	assert.Equal(t, original.OfflineMode, restored.OfflineMode)
	assert.Equal(t, original.DebugLevel, restored.DebugLevel)
}
