package core

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_returns_defaults_when_path_empty_or_missing(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "empty path", path: ""},
		{name: "missing file", path: filepath.Join(t.TempDir(), "missing.yaml")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a config path that should not be read.

			// When: the config loader is invoked.
			got, err := Load(tt.path)
			// Then: defaults are returned without error.
			if err != nil {
				t.Fatalf("Load(%q) returned error: %v", tt.path, err)
			}
			assertConfigEqual(t, &Config{
				DefaultOutput: "table",
				Backend:       "auto",
				ReportDir:     ".",
			}, got)
		})
	}
}

func TestLoad_parses_all_fields_when_yaml_is_valid(t *testing.T) {
	// Given: a valid YAML config file with every field set.
	path := writeConfigFile(t, `default_output: json
backend: nvidia-smi
report_dir: /tmp/gpu-reports
nvidia_smi_path: /usr/local/bin/nvidia-smi
`)

	// When: the config loader reads the file.
	got, err := Load(path)
	// Then: every YAML field is parsed into the config struct.
	if err != nil {
		t.Fatalf("Load(%q) returned error: %v", path, err)
	}
	assertConfigEqual(t, &Config{
		DefaultOutput: "json",
		Backend:       "nvidia-smi",
		ReportDir:     "/tmp/gpu-reports",
		NvidiaSmiPath: "/usr/local/bin/nvidia-smi",
	}, got)
}

func TestLoad_returns_typed_error_when_enum_fields_are_invalid(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantField string
		wantValue string
	}{
		{
			name:      "invalid default output",
			yaml:      "default_output: xml\nbackend: auto\nreport_dir: .\n",
			wantField: "default_output",
			wantValue: "xml",
		},
		{
			name:      "invalid backend",
			yaml:      "default_output: table\nbackend: cuda\nreport_dir: .\n",
			wantField: "backend",
			wantValue: "cuda",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a YAML config file with one invalid enum field.
			path := writeConfigFile(t, tt.yaml)

			// When: the config loader validates the parsed file.
			_, err := Load(path)

			// Then: callers can inspect the typed config error.
			if err == nil {
				t.Fatal("Load returned nil error")
			}
			var configErr *ConfigError
			if !errors.As(err, &configErr) {
				t.Fatalf("Load error type = %T, want *ConfigError", err)
			}
			if configErr.Field != tt.wantField {
				t.Fatalf("ConfigError.Field = %q, want %q", configErr.Field, tt.wantField)
			}
			if configErr.Value != tt.wantValue {
				t.Fatalf("ConfigError.Value = %q, want %q", configErr.Value, tt.wantValue)
			}
		})
	}
}

func TestLoad_returns_error_when_yaml_shape_is_invalid(t *testing.T) {
	// Given: a YAML file whose top-level shape cannot unmarshal into Config.
	path := writeConfigFile(t, "- not\n- a\n- mapping\n")

	// When: the config loader reads the file.
	_, err := Load(path)

	// Then: the YAML unmarshal error is propagated.
	if err == nil {
		t.Fatal("Load returned nil error")
	}
}

func TestDefaultConfigPath_uses_user_home_directory(t *testing.T) {
	// Given: a process home directory visible to os.UserHomeDir.
	home := t.TempDir()
	t.Setenv("HOME", home)

	// When: the default config path is requested.
	got, err := DefaultConfigPath()
	// Then: the path points at ~/.gpu-tools/config.yaml.
	if err != nil {
		t.Fatalf("DefaultConfigPath returned error: %v", err)
	}
	want := filepath.Join(home, ".gpu-tools", "config.yaml")
	if got != want {
		t.Fatalf("DefaultConfigPath = %q, want %q", got, want)
	}
}

func TestConfigError_Error_formats_field_and_value(t *testing.T) {
	got := (&ConfigError{Field: "backend", Value: "cuda"}).Error()
	want := "invalid config: backend=cuda"
	if got != want {
		t.Fatalf("ConfigError.Error() = %q, want %q", got, want)
	}
}

func TestConfig_Validate_returns_typed_error_when_config_is_nil(t *testing.T) {
	err := (*Config)(nil).Validate()
	if err == nil {
		t.Fatal("Validate returned nil error")
	}

	var configErr *ConfigError
	if !errors.As(err, &configErr) {
		t.Fatalf("Validate error type = %T, want *ConfigError", err)
	}
	if configErr.Field != "config" {
		t.Fatalf("ConfigError.Field = %q, want %q", configErr.Field, "config")
	}
}

func TestConfig_Validate_accepts_amd_backend(t *testing.T) {
	// Given: a config that selects the AMD rocm-smi backend explicitly.
	config := &Config{DefaultOutput: OutputTable, Backend: BackendAMD}

	// When: the config is validated.
	err := config.Validate()
	// Then: amd is accepted as a known backend.
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestLoad_returns_wrapped_error_when_path_is_directory(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("Load returned nil error")
	}

	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("Load error type = %T, want wrapped *os.PathError", err)
	}
}

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	return path
}

func assertConfigEqual(t *testing.T, want, got *Config) {
	t.Helper()

	if got == nil {
		t.Fatal("Config is nil")
	}
	if *got != *want {
		t.Fatalf("Config = %+v, want %+v", *got, *want)
	}
}
