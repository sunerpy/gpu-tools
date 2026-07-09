package nvidiasmi

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

type deviceEdgeRunner struct {
	helpOut  []byte
	queryOut []byte
	queryErr error
	calls    []fakeRunCall
}

func (r *deviceEdgeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, fakeRunCall{name: name, args: append([]string(nil), args...)})
	if len(args) == 1 && args[0] == "--help-query-gpu" {
		if r.helpOut == nil {
			return fullFieldHelp(), nil
		}
		return append([]byte(nil), r.helpOut...), nil
	}
	if r.queryErr != nil {
		return nil, r.queryErr
	}
	return append([]byte(nil), r.queryOut...), nil
}

func TestCollector_devices_returnsQueryError_whenNvidiaSmiQueryFails(t *testing.T) {
	tests := []struct {
		name            string
		queryErr        error
		wantUnavailable bool
	}{
		{name: "wraps non availability query error", queryErr: errors.New("driver reset")},
		{name: "maps missing binary to backend unavailable", queryErr: exec.ErrNotFound, wantUnavailable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			runner := &deviceEdgeRunner{queryErr: tt.queryErr}
			collector := newCollectorWithRunner(runner, "nvidia-smi")

			// When
			devices, err := collector.devices()

			// Then
			if devices != nil {
				t.Fatalf("expected nil devices, got %#v", devices)
			}
			requireError(t, err)
			if errors.Is(err, gpu.ErrBackendUnavailable) != tt.wantUnavailable {
				t.Fatalf("expected backend unavailable=%t, got error %v", tt.wantUnavailable, err)
			}
			if !tt.wantUnavailable && !strings.Contains(err.Error(), "query nvidia-smi") {
				t.Fatalf("expected query wrapper, got %v", err)
			}
		})
	}
}

func TestCollector_devices_returnsEmptyDevices_whenNvidiaSmiOutputIsEmpty(t *testing.T) {
	// Given
	runner := &deviceEdgeRunner{queryOut: []byte(" \n\t")}
	collector := newCollectorWithRunner(runner, "nvidia-smi")

	// When
	devices, err := collector.devices()

	// Then
	requireNoError(t, err)
	if len(devices) != 0 {
		t.Fatalf("expected no devices, got %#v", devices)
	}
}

func TestConfiguredPath_returnsErrBackendUnavailable_whenConfiguredOverrideIsMissing(t *testing.T) {
	// Given
	home := t.TempDir()
	missingPath := filepath.Join(t.TempDir(), "missing-nvidia-smi")
	writeConfig(t, home, "default_output: table\nbackend: auto\nnvidia_smi_path: "+missingPath+"\n")
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	// When
	path, err := configuredPath()

	// Then
	if path != "" {
		t.Fatalf("expected empty path, got %q", path)
	}
	requireErrorIs(t, err, gpu.ErrBackendUnavailable)
}

func TestConfiguredPath_returnsErrBackendUnavailable_whenDefaultBinaryIsMissing(t *testing.T) {
	// Given
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	// When
	path, err := configuredPath()

	// Then
	if path != "" {
		t.Fatalf("expected empty path, got %q", path)
	}
	requireErrorIs(t, err, gpu.ErrBackendUnavailable)
}
