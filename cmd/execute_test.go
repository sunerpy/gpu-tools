package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestExecuteHelper_returnsZero_whenCommandSucceeds(t *testing.T) {
	// Given
	root := newRootCmd()
	root.SetArgs([]string{"version"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	// When
	code := execute(root)

	// Then
	if code != 0 {
		t.Fatalf("expected zero exit code, got %d", code)
	}
}

func TestExecuteHelper_returnsOneAndPrintsError_whenCommandFails(t *testing.T) {
	// Given
	root := newRootCmd()
	stderr := &bytes.Buffer{}
	root.SetArgs([]string{"--output", "xml", "version"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(stderr)

	// When
	code := execute(root)

	// Then
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "default_output=xml") {
		t.Fatalf("expected stderr to contain validation error, got %q", stderr.String())
	}
}

func TestHandleExecuteError_usesExitErrorCode_whenWrapped(t *testing.T) {
	// Given
	stderr := &bytes.Buffer{}
	cause := errors.New("denied")
	err := errors.Join(NewExitError(9, cause))

	// When
	code := handleExecuteError(stderr, err)

	// Then
	if code != 9 {
		t.Fatalf("expected exit code 9, got %d", code)
	}
	if !strings.Contains(stderr.String(), "denied") {
		t.Fatalf("expected stderr to contain cause, got %q", stderr.String())
	}
}

func TestExecute_returnsWithoutExit_whenRootCommandSucceeds(t *testing.T) {
	// Given
	previous := rootCmd
	root := newRootCmd()
	root.SetArgs([]string{"version"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	rootCmd = root
	t.Cleanup(func() { rootCmd = previous })

	// When / Then
	Execute()
}
