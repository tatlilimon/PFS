package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	appName = "pfs"
	envFile = ".pfs.env"
)

type DebugLevel int

const (
	DebugLevelNone DebugLevel = iota
	DebugLevelBasic
	DebugLevelDetailed
)

type Config struct {
	OllamaBaseURL string `yaml:"ollama_base_url"`
	OllamaModel   string `yaml:"ollama_model"`
	OfflineMode   bool   `yaml:"offline_mode"`
	DebugLevel    int    `yaml:"debug_level"`
}

func defaultConfig() *Config {
	return &Config{
		OllamaBaseURL: "http://localhost:11434",
		OllamaModel:   "llama3.2",
		OfflineMode:   true,
		DebugLevel:    0,
	}
}

func configPath() (string, error) {
	dir := configDir
	if dir == "" {
		var err error
		dir, err = os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user config directory: %w", err)
		}
	}
	return filepath.Join(dir, appName, "config.yaml"), nil
}

func oldEnvPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	return filepath.Join(home, envFile), nil
}

var configDir string

func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	if data, err := os.ReadFile(path); err == nil {
		cfg := &Config{}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
		}
		return cfg, nil
	}

	oldPath, err := oldEnvPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(oldPath); err == nil {
		cfg, err := migrate(oldPath, path)
		if err != nil {
			return nil, fmt.Errorf("failed to migrate config from %s: %w", oldPath, err)
		}
		return cfg, nil
	}

	return defaultConfig(), nil
}

func (c *Config) Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory %s: %w", dir, err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", path, err)
	}
	return nil
}

func migrate(oldPath, newPath string) (*Config, error) {
	envMap, err := parseEnvFile(oldPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read legacy env file: %w", err)
	}

	cfg := defaultConfig()
	if v, ok := envMap["OLLAMA_BASE_URL"]; ok && v != "" {
		cfg.OllamaBaseURL = v
	}
	if v, ok := envMap["OLLAMA_MODEL"]; ok && v != "" {
		cfg.OllamaModel = v
	}
	if v, ok := envMap["OFFLINE_MODE"]; ok {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.OfflineMode = b
		}
	}
	if v, ok := envMap["DEBUG_LEVEL"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.DebugLevel = n
		}
	}

	dir := filepath.Dir(newPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(newPath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write config file: %w", err)
	}

	bakPath := oldPath + ".bak"
	if err := os.Rename(oldPath, bakPath); err != nil {
		return nil, fmt.Errorf("failed to rename old config to backup: %w", err)
	}

	return cfg, nil
}

func parseEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			result[key] = val
		}
	}
	return result, nil
}
