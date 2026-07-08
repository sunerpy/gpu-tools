package cmd

import (
	"errors"
	"testing"
)

func TestExitError_wrapsCauseAndCode_whenConstructed(t *testing.T) {
	// Given
	cause := errors.New("boom")

	// When
	err := NewExitError(7, cause)

	// Then
	if err.Code != 7 {
		t.Fatalf("expected code 7, got %d", err.Code)
	}
	if err.Error() != "boom" {
		t.Fatalf("expected error message from cause, got %q", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Fatalf("expected ExitError to unwrap cause")
	}
}

func TestExitError_defaultsCodeAndMessage_whenInputsAreEmpty(t *testing.T) {
	// Given / When
	err := NewExitError(0, nil)

	// Then
	if err.Code != 1 {
		t.Fatalf("expected default code 1, got %d", err.Code)
	}
	if err.Error() != "exit with code 1" {
		t.Fatalf("expected fallback message, got %q", err.Error())
	}
	if err.Unwrap() != nil {
		t.Fatalf("expected nil unwrap for nil cause")
	}
}

func TestExitError_isNilSafe_whenMethodCalledOnNilPointer(t *testing.T) {
	// Given
	var err *ExitError

	// When / Then
	if err.Error() != "exit with code 1" {
		t.Fatalf("expected nil-safe message, got %q", err.Error())
	}
	if err.Unwrap() != nil {
		t.Fatalf("expected nil-safe unwrap")
	}
}

func TestExitError_errorDefaultsZeroCode_whenConstructedDirectly(t *testing.T) {
	// Given
	err := &ExitError{}

	// When / Then
	if err.Error() != "exit with code 1" {
		t.Fatalf("expected zero code fallback, got %q", err.Error())
	}
}
