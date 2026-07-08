package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	OutputTable    = "table"
	OutputJSON     = "json"
	OutputMarkdown = "markdown"

	BackendAuto      = "auto"
	BackendNVML      = "nvml"
	BackendNvidiaSMI = "nvidia-smi"
)

type Config struct {
	DefaultOutput string `yaml:"default_output"`
	Backend       string `yaml:"backend"`
	ReportDir     string `yaml:"report_dir"`
	NvidiaSmiPath string `yaml:"nvidia_smi_path"`
}

type ConfigError struct {
	Field string
	Value string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("invalid config: %s=%s", e.Field, e.Value)
}

func Load(path string) (*Config, error) {
	config := defaultConfig()
	if path == "" {
		return config, nil
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return config, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return config, nil
}

func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home dir: %w", err)
	}
	return filepath.Join(home, ".gpu-tools", "config.yaml"), nil
}

func (c *Config) Validate() error {
	if c == nil {
		return &ConfigError{Field: "config", Value: "<nil>"}
	}
	if !isValidDefaultOutput(c.DefaultOutput) {
		return &ConfigError{Field: "default_output", Value: c.DefaultOutput}
	}
	if !isValidBackend(c.Backend) {
		return &ConfigError{Field: "backend", Value: c.Backend}
	}
	return nil
}

func defaultConfig() *Config {
	return &Config{
		DefaultOutput: OutputTable,
		Backend:       BackendAuto,
		ReportDir:     ".",
	}
}

func isValidDefaultOutput(value string) bool {
	switch value {
	case OutputTable, OutputJSON, OutputMarkdown:
		return true
	default:
		return false
	}
}

func isValidBackend(value string) bool {
	switch value {
	case BackendAuto, BackendNVML, BackendNvidiaSMI:
		return true
	default:
		return false
	}
}
