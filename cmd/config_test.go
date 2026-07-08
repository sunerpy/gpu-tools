package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigCommand_printsHelp_whenNoSubcommandProvided(t *testing.T) {
	// Given
	root := newRootCmd()

	// When
	stdout, stderr, err := executeCommand(root, "config")
	// Then
	if err != nil {
		t.Fatalf("expected config help to succeed: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	for _, want := range []string{"Manage the gpu-tools YAML configuration", "init", "show"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected config help to contain %q, got:\n%s", want, stdout)
		}
	}
}

func TestConfigInit_returnsExitError_whenConfigExistsWithoutForce(t *testing.T) {
	// Given
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".gpu-tools", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("default_output: json\nbackend: auto\n"), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	// When
	_, _, err := executeCommand(newRootCmd(), "config", "init")

	// Then
	if err == nil {
		t.Fatalf("expected existing config to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.Code)
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already exists message, got %q", err.Error())
	}
}

func TestConfigInit_overwritesConfig_whenForceProvided(t *testing.T) {
	// Given
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".gpu-tools", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("default_output: json\nbackend: nvml\n"), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	// When
	stdout, stderr, err := executeCommand(newRootCmd(), "config", "init", "--force")
	// Then
	if err != nil {
		t.Fatalf("expected forced config init to succeed: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, path) {
		t.Fatalf("expected output to mention %q, got %q", path, stdout)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read overwritten config: %v", readErr)
	}
	if !strings.Contains(string(data), "default_output: table") {
		t.Fatalf("expected forced config to restore table default, got:\n%s", string(data))
	}
}

func TestConfigShow_appliesFlagOverrides_whenPersistentFlagsProvided(t *testing.T) {
	// Given
	root := newRootCmd()

	// When
	stdout, _, err := executeCommand(root, "--output", "json", "--backend", "nvidia-smi", "config", "show")
	// Then
	if err != nil {
		t.Fatalf("expected config show with overrides to succeed: %v", err)
	}
	for _, want := range []string{"default_output: json", "backend: nvidia-smi"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected config show to contain %q, got:\n%s", want, stdout)
		}
	}
}

func TestConfigShow_returnsLoadError_whenConfigFileIsInvalid(t *testing.T) {
	// Given
	tmp := t.TempDir()
	path := filepath.Join(tmp, "bad.yaml")
	if err := os.WriteFile(path, []byte("default_output: ["), 0o600); err != nil {
		t.Fatalf("write bad config: %v", err)
	}

	// When
	_, _, err := executeCommand(newRootCmd(), "--config", path, "config", "show")

	// Then
	if err == nil {
		t.Fatalf("expected invalid config file to fail")
	}
	if !strings.Contains(err.Error(), "parse config") {
		t.Fatalf("expected parse config error, got %q", err.Error())
	}
}

func TestConfigShow_returnsWriteError_whenOutputWriterFails(t *testing.T) {
	// Given
	root := newRootCmd()
	root.SetArgs([]string{"config", "show"})
	root.SetOut(failingWriter{})

	// When
	err := root.Execute()

	// Then
	if err == nil {
		t.Fatalf("expected config show writer failure")
	}
	if !strings.Contains(err.Error(), "writer failed") {
		t.Fatalf("expected writer failure, got %q", err.Error())
	}
}

func TestConfigShow_returnsEncodeError_whenYAMLMarshalFails(t *testing.T) {
	// Given
	previous := marshalYAML
	marshalYAML = func(any) ([]byte, error) {
		return nil, fmt.Errorf("encode failed")
	}
	t.Cleanup(func() { marshalYAML = previous })
	show := newConfigShowCmd()
	root := newRootCmd()
	root.AddCommand(show)

	// When
	err := show.RunE(show, nil)

	// Then
	if err == nil {
		t.Fatalf("expected config show encode failure")
	}
	if !strings.Contains(err.Error(), "encode config") {
		t.Fatalf("expected encode config error, got %q", err.Error())
	}
}

func TestConfigInit_returnsWriteError_whenOutputWriterFails(t *testing.T) {
	// Given
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := newRootCmd()
	root.SetArgs([]string{"config", "init"})
	root.SetOut(failingWriter{})

	// When
	err := root.Execute()

	// Then
	if err == nil {
		t.Fatalf("expected config init writer failure")
	}
	if !strings.Contains(err.Error(), "writer failed") {
		t.Fatalf("expected writer failure, got %q", err.Error())
	}
}

func TestConfigInit_returnsPathError_whenDefaultConfigPathCannotResolve(t *testing.T) {
	// Given
	t.Setenv("HOME", "")
	cmd := newConfigInitCmd()

	// When
	err := cmd.RunE(cmd, nil)

	// Then
	if err == nil {
		t.Fatalf("expected config init path failure")
	}
	if !strings.Contains(err.Error(), "resolve default config path") {
		t.Fatalf("expected path resolution error, got %q", err.Error())
	}
}

func TestWriteDefaultConfig_returnsDirectoryError_whenParentIsFile(t *testing.T) {
	// Given
	tmp := t.TempDir()
	parentFile := filepath.Join(tmp, "parent")
	if err := os.WriteFile(parentFile, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write parent fixture: %v", err)
	}

	// When
	err := writeDefaultConfig(filepath.Join(parentFile, "config.yaml"), false)

	// Then
	if err == nil {
		t.Fatalf("expected directory creation to fail")
	}
	if !strings.Contains(err.Error(), "stat config") {
		t.Fatalf("expected stat error, got %q", err.Error())
	}
}

func TestWriteDefaultConfig_returnsWriteError_whenPathIsDirectoryWithForce(t *testing.T) {
	// Given
	path := t.TempDir()

	// When
	err := writeDefaultConfig(path, true)

	// Then
	if err == nil {
		t.Fatalf("expected writing to directory to fail")
	}
	if !strings.Contains(err.Error(), "write config") {
		t.Fatalf("expected write config error, got %q", err.Error())
	}
}

func TestWriteDefaultConfig_returnsEncodeError_whenYAMLMarshalFails(t *testing.T) {
	// Given
	previous := marshalYAML
	marshalYAML = func(any) ([]byte, error) {
		return nil, fmt.Errorf("encode failed")
	}
	t.Cleanup(func() { marshalYAML = previous })

	// When
	err := writeDefaultConfig(filepath.Join(t.TempDir(), "config.yaml"), false)

	// Then
	if err == nil {
		t.Fatalf("expected default config encode failure")
	}
	if !strings.Contains(err.Error(), "encode default config") {
		t.Fatalf("expected encode default config error, got %q", err.Error())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("writer failed")
}
