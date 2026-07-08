package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/sunerpy/gpu-tools/version"
)

func TestRootCommand_printsHelpWithRegisteredSubcommands_whenHelpFlagProvided(t *testing.T) {
	// Given
	root := newRootCmd()

	// When
	stdout, stderr, err := executeCommand(root, "--help")
	// Then
	if err != nil {
		t.Fatalf("expected help to succeed: %v", err)
	}
	combined := stdout + stderr
	for _, want := range []string{"gpu-tools", "version", "config", "completion"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("expected help output to contain %q, got:\n%s", want, combined)
		}
	}
}

func TestRootCommand_printsHelpWithRegisteredSubcommands_whenNoSubcommandProvided(t *testing.T) {
	// Given
	root := newRootCmd()

	// When
	stdout, stderr, err := executeCommand(root)
	// Then
	if err != nil {
		t.Fatalf("expected root help to succeed: %v", err)
	}
	combined := stdout + stderr
	for _, want := range []string{"gpu-tools", "version", "config", "completion"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("expected root help output to contain %q, got:\n%s", want, combined)
		}
	}
}

func TestRootCommand_printsVersionInfo_whenVersionSubcommandRuns(t *testing.T) {
	// Given
	root := newRootCmd()

	// When
	stdout, stderr, err := executeCommand(root, "version")
	// Then
	if err != nil {
		t.Fatalf("expected version to succeed: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, version.Info()) {
		t.Fatalf("expected version output to contain version.Info(), got:\n%s", stdout)
	}
}

func TestRootCommand_returnsErrorWithoutExiting_whenUnknownFlagProvided(t *testing.T) {
	// Given
	root := newRootCmd()

	// When
	stdout, stderr, err := executeCommand(root, "--nope")

	// Then
	if err == nil {
		t.Fatalf("expected unknown flag to return an error")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("expected unknown flag error, got %q", err.Error())
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("expected no command output, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestConfigCommand_writesAndShowsDefaultConfig_whenHomeIsTemporary(t *testing.T) {
	// Given
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".gpu-tools", "config.yaml")

	// When
	initStdout, initStderr, initErr := executeCommand(newRootCmd(), "config", "init")

	// Then
	if initErr != nil {
		t.Fatalf("expected config init to succeed: %v", initErr)
	}
	if initStderr != "" {
		t.Fatalf("expected empty config init stderr, got %q", initStderr)
	}
	if !strings.Contains(initStdout, configPath) {
		t.Fatalf("expected config init output to mention %q, got %q", configPath, initStdout)
	}
	data, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("expected config file to exist: %v", readErr)
	}
	fileText := string(data)
	for _, want := range []string{"default_output: table", "backend: auto", "report_dir: ."} {
		if !strings.Contains(fileText, want) {
			t.Fatalf("expected config file to contain %q, got:\n%s", want, fileText)
		}
	}

	// When
	showStdout, showStderr, showErr := executeCommand(newRootCmd(), "config", "show")

	// Then
	if showErr != nil {
		t.Fatalf("expected config show to succeed: %v", showErr)
	}
	if showStderr != "" {
		t.Fatalf("expected empty config show stderr, got %q", showStderr)
	}
	for _, want := range []string{"default_output: table", "backend: auto", "report_dir: ."} {
		if !strings.Contains(showStdout, want) {
			t.Fatalf("expected config show output to contain %q, got:\n%s", want, showStdout)
		}
	}
}

func TestRootCommand_validatesPersistentFlags_whenInvalidValuesProvided(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "invalid output", args: []string{"--output", "xml", "version"}, want: "default_output=xml"},
		{name: "invalid backend", args: []string{"--backend", "cuda", "version"}, want: "backend=cuda"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			root := newRootCmd()

			// When
			_, _, err := executeCommand(root, tt.args...)

			// Then
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error to contain %q, got %q", tt.want, err.Error())
			}
		})
	}
}

func TestRootCommand_usesConfigDefaultOutput_whenConfigFlagProvided(t *testing.T) {
	// Given
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.yaml")
	configText := "default_output: markdown\nbackend: nvml\nreport_dir: reports\nnvidia_smi_path: /usr/bin/nvidia-smi\n"
	if err := os.WriteFile(configPath, []byte(configText), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	root := newRootCmd()

	// When
	_, _, err := executeCommand(root, "--config", configPath, "version")
	// Then
	if err != nil {
		t.Fatalf("expected config-backed default flags to validate: %v", err)
	}
}

func TestRootCommand_fallsBackToTableDefaults_whenDefaultConfigIsInvalid(t *testing.T) {
	// Given
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".gpu-tools", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("default_output: xml\nbackend: auto\n"), 0o600); err != nil {
		t.Fatalf("write invalid default config: %v", err)
	}
	root := newRootCmd()

	// When
	outputFlag := root.PersistentFlags().Lookup("output")
	backendFlag := root.PersistentFlags().Lookup("backend")

	// Then
	if outputFlag == nil || outputFlag.DefValue != "table" {
		t.Fatalf("expected fallback output default table, got %#v", outputFlag)
	}
	if backendFlag == nil || backendFlag.DefValue != "auto" {
		t.Fatalf("expected fallback backend default auto, got %#v", backendFlag)
	}
}

func TestRootCommand_fallsBackToTableDefaults_whenDefaultConfigPathCannotResolve(t *testing.T) {
	// Given
	t.Setenv("HOME", "")

	// When
	root := newRootCmd()
	outputFlag := root.PersistentFlags().Lookup("output")
	backendFlag := root.PersistentFlags().Lookup("backend")

	// Then
	if outputFlag == nil || outputFlag.DefValue != "table" {
		t.Fatalf("expected fallback output default table, got %#v", outputFlag)
	}
	if backendFlag == nil || backendFlag.DefValue != "auto" {
		t.Fatalf("expected fallback backend default auto, got %#v", backendFlag)
	}
}

func TestResolvedConfig_returnsFlagReadError_whenCommandHasNoPersistentFlags(t *testing.T) {
	// Given
	cmd := &cobra.Command{Use: "empty"}

	// When
	_, err := resolvedConfig(cmd)

	// Then
	if err == nil {
		t.Fatalf("expected missing config flag to fail")
	}
	if !strings.Contains(err.Error(), "read --config") {
		t.Fatalf("expected config flag read error, got %q", err.Error())
	}
}

func TestResolvedConfig_returnsOutputFlagReadError_whenOutputFlagHasWrongType(t *testing.T) {
	// Given
	root := &cobra.Command{Use: "root"}
	child := &cobra.Command{Use: "child"}
	root.PersistentFlags().String(configFlag, "", "")
	root.PersistentFlags().Bool(outputFlag, false, "")
	root.PersistentFlags().String(backendFlag, "auto", "")
	if err := root.PersistentFlags().Set(outputFlag, "true"); err != nil {
		t.Fatalf("set output flag: %v", err)
	}
	root.AddCommand(child)

	// When
	_, err := resolvedConfig(child)

	// Then
	if err == nil {
		t.Fatalf("expected output flag read failure")
	}
	if !strings.Contains(err.Error(), "read --output") {
		t.Fatalf("expected output flag read error, got %q", err.Error())
	}
}

func TestResolvedConfig_returnsBackendFlagReadError_whenBackendFlagHasWrongType(t *testing.T) {
	// Given
	root := &cobra.Command{Use: "root"}
	child := &cobra.Command{Use: "child"}
	root.PersistentFlags().String(configFlag, "", "")
	root.PersistentFlags().String(outputFlag, "table", "")
	root.PersistentFlags().Bool(backendFlag, false, "")
	if err := root.PersistentFlags().Set(backendFlag, "true"); err != nil {
		t.Fatalf("set backend flag: %v", err)
	}
	root.AddCommand(child)

	// When
	_, err := resolvedConfig(child)

	// Then
	if err == nil {
		t.Fatalf("expected backend flag read failure")
	}
	if !strings.Contains(err.Error(), "read --backend") {
		t.Fatalf("expected backend flag read error, got %q", err.Error())
	}
}

type commandExecutor interface {
	SetOut(io.Writer)
	SetErr(io.Writer)
	SetArgs([]string)
	Execute() error
}

func executeCommand(root commandExecutor, args ...string) (string, string, error) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}
