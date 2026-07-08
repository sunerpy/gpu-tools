package cmd

import (
	"strings"
	"testing"
)

func TestCompletionCommand_generatesScripts_whenShellIsSupported(t *testing.T) {
	tests := []struct {
		name  string
		shell string
		want  string
	}{
		{name: "bash", shell: "bash", want: "gpu-tools"},
		{name: "zsh", shell: "zsh", want: "compdef"},
		{name: "fish", shell: "fish", want: "complete"},
		{name: "powershell", shell: "powershell", want: "Register-ArgumentCompleter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			root := newRootCmd()

			// When
			stdout, stderr, err := executeCommand(root, "completion", tt.shell)
			// Then
			if err != nil {
				t.Fatalf("expected completion %s to succeed: %v", tt.shell, err)
			}
			if stderr != "" {
				t.Fatalf("expected empty stderr, got %q", stderr)
			}
			if !strings.Contains(stdout, tt.want) {
				t.Fatalf("expected completion output to contain %q, got:\n%s", tt.want, stdout)
			}
		})
	}
}

func TestCompletionCommand_returnsError_whenShellIsUnsupported(t *testing.T) {
	// Given
	root := newRootCmd()

	// When
	_, _, err := executeCommand(root, "completion", "tcsh")

	// Then
	if err == nil {
		t.Fatalf("expected unsupported shell to fail")
	}
	if !strings.Contains(err.Error(), "invalid argument") {
		t.Fatalf("expected cobra valid-args error, got %q", err.Error())
	}
}

func TestCompletionCommand_runERejectsUnsupportedShell_whenCalledDirectly(t *testing.T) {
	// Given
	cmd := newCompletionCmd()

	// When
	err := cmd.RunE(cmd, []string{"tcsh"})

	// Then
	if err == nil {
		t.Fatalf("expected direct RunE call to reject unsupported shell")
	}
	if !strings.Contains(err.Error(), "unsupported shell") {
		t.Fatalf("expected unsupported shell error, got %q", err.Error())
	}
}
