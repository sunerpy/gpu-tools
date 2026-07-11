package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/sunerpy/gpu-tools/core"
	"github.com/sunerpy/gpu-tools/internal/gpu"
	"github.com/sunerpy/gpu-tools/internal/report"
)

func TestDetectCommand_rendersTableWithDevices_whenCollectorSucceeds(t *testing.T) {
	// Given
	collector := newFakeCollector([]gpu.Device{
		{Index: 0, UUID: "GPU-0", Name: "RTX 4090", MemoryTotal: 24 * 1024 * 1024, MemoryUsed: 12 * 1024 * 1024, Temperature: 42, PowerDraw: 100_000, PowerLimit: 450_000, UtilizationGPU: 10, UtilizationMem: 20, PState: "P0"},
		{Index: 1, UUID: "GPU-1", Name: "RTX 6000 Ada", MemoryTotal: 48 * 1024 * 1024, MemoryUsed: 16 * 1024 * 1024, Temperature: 39, PowerDraw: 90_000, PowerLimit: 300_000, UtilizationGPU: 30, UtilizationMem: 40, PState: "P2"},
	})
	overrideGPUFactory(t, collector, nil)
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, stderr, err := executeCommand(newRootCmd(), "detect", "-o", "table")
	// Then
	if err != nil {
		t.Fatalf("expected detect table to succeed: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	for _, want := range []string{"RTX 4090", "RTX 6000 Ada"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected table output to contain %q, got:\n%s", want, stdout)
		}
	}
	if !collector.initCalled || !collector.shutdownCalled {
		t.Fatalf("expected Init and Shutdown to be called, got init=%v shutdown=%v", collector.initCalled, collector.shutdownCalled)
	}
}

func TestDetectCommand_rendersValidJSON_whenJSONOutputRequested(t *testing.T) {
	// Given
	collector := newFakeCollector([]gpu.Device{{Index: 0, UUID: "GPU-json", Name: "JSON GPU"}})
	overrideGPUFactory(t, collector, nil)
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "detect", "-o", "json")
	// Then
	if err != nil {
		t.Fatalf("expected detect json to succeed: %v", err)
	}
	var snapshot report.Snapshot
	if err := json.Unmarshal([]byte(stdout), &snapshot); err != nil {
		t.Fatalf("expected valid JSON snapshot, got error %v and output:\n%s", err, stdout)
	}
	if snapshot.Backend != "fake" {
		t.Fatalf("expected backend fake, got %q", snapshot.Backend)
	}
	if len(snapshot.Devices) != 1 || snapshot.Devices[0].Name != "JSON GPU" {
		t.Fatalf("expected JSON GPU device, got %#v", snapshot.Devices)
	}
}

func TestDetectCommand_returnsExitError_whenBackendUnavailable(t *testing.T) {
	// Given
	overrideGPUFactory(t, nil, gpu.ErrBackendUnavailable)
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "detect")

	// Then
	if err == nil {
		t.Fatalf("expected backend unavailable to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.Code)
	}
	if !strings.Contains(err.Error(), "no NVIDIA GPU detected") {
		t.Fatalf("expected friendly no NVIDIA message, got %q", err.Error())
	}
}

func TestDetectCommand_returnsExitError_whenNoBackendAvailable(t *testing.T) {
	// Given
	overrideGPUFactory(t, nil, gpu.ErrNoBackend)
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "detect")

	// Then
	if err == nil {
		t.Fatalf("expected no backend to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.Code)
	}
	if !strings.Contains(err.Error(), "no NVIDIA GPU detected") {
		t.Fatalf("expected friendly no NVIDIA message, got %q", err.Error())
	}
}

func TestDetectCommand_returnsExitError_whenFactoryReturnsUnexpectedError(t *testing.T) {
	// Given
	overrideGPUFactory(t, nil, errors.New("factory failed"))
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "detect")

	// Then
	if err == nil {
		t.Fatalf("expected factory error to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.Code)
	}
	if !strings.Contains(err.Error(), "select GPU backend") {
		t.Fatalf("expected backend selection message, got %q", err.Error())
	}
}

func TestDetectCommand_returnsConfigError_whenRunWithoutRootFlags(t *testing.T) {
	// Given
	cmd := newDetectCmd()

	// When
	err := runDetect(cmd)

	// Then
	if err == nil {
		t.Fatalf("expected missing root flags to fail")
	}
	if !strings.Contains(err.Error(), "read --config") {
		t.Fatalf("expected config flag read error, got %q", err.Error())
	}
}

func TestDetectCommand_returnsWatchFlagError_whenWatchFlagIsMissing(t *testing.T) {
	// Given
	t.Setenv("HOME", t.TempDir())
	root := newRootCmd()
	cmd := &cobra.Command{Use: "detect"}
	root.AddCommand(cmd)

	// When
	err := runDetect(cmd)

	// Then
	if err == nil {
		t.Fatalf("expected missing watch flag to fail")
	}
	if !strings.Contains(err.Error(), "read --watch") {
		t.Fatalf("expected watch flag error, got %q", err.Error())
	}
}

func TestDetectCommand_returnsWrappedError_whenCollectorFlowFails(t *testing.T) {
	tests := []struct {
		name          string
		collector     *fakeCollector
		failingWriter bool
		want          string
	}{
		{name: "init fails", collector: &fakeCollector{initErr: errors.New("init failed")}, want: "initialize GPU collector"},
		{name: "device count fails", collector: &fakeCollector{countErr: errors.New("count failed")}, want: "count GPU devices"},
		{name: "device read fails", collector: &fakeCollector{devices: []gpu.Device{{Index: 0}}, deviceErr: errors.New("device failed")}, want: "read GPU device 0"},
		{name: "device read returns nil", collector: &fakeCollector{devices: []gpu.Device{{Index: 0}}, nilDevice: true}, want: "read GPU device 0"},
		{name: "render fails", collector: newFakeCollector([]gpu.Device{{Index: 0, Name: "writer GPU"}}), failingWriter: true, want: "render detect snapshot"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			overrideGPUFactory(t, tt.collector, nil)
			t.Setenv("HOME", t.TempDir())

			// When
			err := executeDetectFailureCase(tt.failingWriter)

			// Then
			if err == nil {
				t.Fatalf("expected detect to fail")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error to contain %q, got %q", tt.want, err.Error())
			}
		})
	}
}

func TestRegisteredBackends_includeNVMLAndNvidiaSMI_whenRootPackageImportsBackends(t *testing.T) {
	for _, backend := range []string{core.BackendNVML, core.BackendNvidiaSMI} {
		// When
		_, err := gpu.Select(backend)

		// Then
		if _, ok := errors.AsType[*gpu.UnknownBackendError](err); ok {
			t.Fatalf("expected backend %q to be registered, got %v", backend, err)
		}
	}
}

type fakeCollector struct {
	devices        []gpu.Device
	initErr        error
	countErr       error
	deviceErr      error
	nilDevice      bool
	initCalled     bool
	shutdownCalled bool
}

func newFakeCollector(devices []gpu.Device) *fakeCollector {
	return &fakeCollector{devices: devices}
}

func (c *fakeCollector) Init() error {
	c.initCalled = true
	return c.initErr
}

func (c *fakeCollector) Shutdown() error {
	c.shutdownCalled = true
	return nil
}

func (c *fakeCollector) DeviceCount() (int, error) {
	if c.countErr != nil {
		return 0, c.countErr
	}
	return len(c.devices), nil
}

func (c *fakeCollector) Device(i int) (*gpu.Device, error) {
	if c.deviceErr != nil {
		return nil, c.deviceErr
	}
	if c.nilDevice {
		return nil, nil
	}
	if i < 0 || i >= len(c.devices) {
		return nil, fmt.Errorf("device index %d out of range", i)
	}
	device := c.devices[i]
	return &device, nil
}

func (c *fakeCollector) Backend() string {
	return "fake"
}

func overrideGPUFactory(t *testing.T, collector gpu.Collector, factoryErr error) {
	t.Helper()
	previous := gpu.DefaultFactory
	gpu.DefaultFactory = func(core.Config) (gpu.Collector, error) {
		return collector, factoryErr
	}
	t.Cleanup(func() { gpu.DefaultFactory = previous })
}

func executeDetectFailureCase(failingOutput bool) error {
	root := newRootCmd()
	if failingOutput {
		root.SetOut(failingWriter{})
		root.SetArgs([]string{"detect"})
		return root.Execute()
	}
	_, _, err := executeCommand(root, "detect")
	return err
}
