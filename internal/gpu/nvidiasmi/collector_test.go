package nvidiasmi

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

type fakeRunner struct {
	out          []byte
	helpOut      []byte
	queryOut     []byte
	err          error
	computeErr   error
	name         string
	args         []string
	gpuQueryArgs []string
	calls        []fakeRunCall
}

type fakeRunCall struct {
	name string
	args []string
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.name = name
	r.args = append([]string(nil), args...)
	r.calls = append(r.calls, fakeRunCall{name: name, args: append([]string(nil), args...)})
	if r.err != nil {
		return nil, r.err
	}
	if len(args) == 1 && args[0] == "--help-query-gpu" {
		if r.helpOut == nil {
			return fullFieldHelp(), nil
		}
		return append([]byte(nil), r.helpOut...), nil
	}
	if len(args) == 1 && args[0] == "--help-query-compute-apps" {
		return fullComputeAppsHelp(), nil
	}
	if len(args) > 0 && strings.HasPrefix(args[0], "--query-compute-apps=") {
		if r.computeErr != nil {
			return nil, r.computeErr
		}
		return nil, nil
	}
	// --query-gpu=...
	r.gpuQueryArgs = append([]string(nil), args...)
	if r.queryOut != nil {
		return append([]byte(nil), r.queryOut...), nil
	}
	return append([]byte(nil), r.out...), nil
}

func fullFieldHelp() []byte {
	var builder strings.Builder
	for _, field := range wantedFields {
		builder.WriteString("\"")
		builder.WriteString(field)
		builder.WriteString("\" - supported field.\n")
	}
	return []byte(builder.String())
}

const reducedFieldHelp = `"index" - GPU index.
"uuid" - GPU UUID.
"name" - Product name.
"memory.total" - Total memory.
"memory.used" - Used memory.
"temperature.gpu" - GPU temperature.
"power.draw" - Power draw.
"power.limit" - Power limit.
"clocks.mem" - Memory clock.
"utilization.gpu" - GPU utilization.
"utilization.memory" - Memory utilization.
"pstate" - Performance state.
"driver_version" - Driver version.
`

func TestCollector_Device_parsesTwoGPUCSV_whenQuerySucceeds(t *testing.T) {
	// Given
	runner := &fakeRunner{out: []byte(strings.Join([]string{
		"0, GPU-111, NVIDIA A100-SXM4-40GB, 40960, 1024, 55, 120.50, 400.00, 1410, 1215, 75, 20, P0, 535.129.03, 30, 10, 4, 16",
		"1, GPU-222, NVIDIA L40S, 46068, 2048, 60, 80.25, 350.00, 1800, 9001, 42, 11, P2, 535.129.03, 25, 5, 3, 8",
	}, "\n"))}
	collector := newCollectorWithRunner(runner, "/usr/bin/nvidia-smi")

	// When
	device, err := collector.Device(1)

	// Then
	requireNoError(t, err)
	if runner.name != "/usr/bin/nvidia-smi" {
		t.Fatalf("expected runner name /usr/bin/nvidia-smi, got %q", runner.name)
	}
	if !reflect.DeepEqual(runner.gpuQueryArgs, queryArgs(wantedFields)) {
		t.Fatalf("expected args %#v, got %#v", queryArgs(wantedFields), runner.gpuQueryArgs)
	}
	requireDevice(t, device, gpu.Device{
		Index:          1,
		UUID:           "GPU-222",
		Name:           "NVIDIA L40S",
		MemoryTotal:    46068 * 1024 * 1024,
		MemoryUsed:     2048 * 1024 * 1024,
		Temperature:    60,
		PowerDraw:      80250,
		PowerLimit:     350000,
		ClockGraphics:  1800,
		ClockMem:       9001,
		UtilizationGPU: 42,
		UtilizationMem: 11,
		PState:         "P2",
		DriverVersion:  "535.129.03",
		EncoderUtil:    25,
		DecoderUtil:    5,
		PCIeGen:        3,
		PCIeWidth:      8,
	})
}

func TestCollector_Device_zeroesUnavailableFields_whenCSVUsesNAValues(t *testing.T) {
	// Given
	runner := &fakeRunner{out: []byte("0, GPU-NA, NVIDIA Test, [N/A], [Not Supported], , [N/A], [Not Supported], , [N/A], [N/A], [Not Supported], [N/A], , [N/A], [Not Supported], [N/A], \n")}
	collector := newCollectorWithRunner(runner, "nvidia-smi")

	// When
	device, err := collector.Device(0)

	// Then
	requireNoError(t, err)
	requireDevice(t, device, gpu.Device{
		Index: 0,
		UUID:  "GPU-NA",
		Name:  "NVIDIA Test",
	})
}

func TestCollector_Device_leavesUnsupportedFieldZero_whenHelpQueryOmitsGraphicsClock(t *testing.T) {
	// Given
	runner := &fakeRunner{
		helpOut:  []byte(reducedFieldHelp),
		queryOut: []byte("0, GPU-111, NVIDIA A100, 40960, 1024, 55, 120.50, 400.00, 1215, 75, 20, P0, 535.129.03\n"),
	}
	collector := newCollectorWithRunner(runner, "nvidia-smi")

	// When
	device, err := collector.Device(0)

	// Then
	requireNoError(t, err)
	expectedFields := []string{
		fieldIndex,
		fieldUUID,
		fieldName,
		fieldMemoryTotal,
		fieldMemoryUsed,
		fieldTemperature,
		fieldPowerDraw,
		fieldPowerLimit,
		fieldClockMem,
		fieldUtilGPU,
		fieldUtilMem,
		fieldPState,
		fieldDriver,
	}
	if !reflect.DeepEqual(runner.gpuQueryArgs, queryArgs(expectedFields)) {
		t.Fatalf("expected args %#v, got %#v", queryArgs(expectedFields), runner.gpuQueryArgs)
	}
	requireDevice(t, device, gpu.Device{
		Index:          0,
		UUID:           "GPU-111",
		Name:           "NVIDIA A100",
		MemoryTotal:    40960 * 1024 * 1024,
		MemoryUsed:     1024 * 1024 * 1024,
		Temperature:    55,
		PowerDraw:      120500,
		PowerLimit:     400000,
		ClockMem:       1215,
		UtilizationGPU: 75,
		UtilizationMem: 20,
		PState:         "P0",
		DriverVersion:  "535.129.03",
	})
}

func TestCollector_DeviceCount_cachesSupportedFields_whenCollectorQueriesTwice(t *testing.T) {
	// Given
	runner := &fakeRunner{out: []byte("0, GPU-111, NVIDIA A100, 40960, 1024, 55, 120.50, 400.00, 1410, 1215, 75, 20, P0, 535.129.03, 30, 10, 4, 16\n")}
	collector := newCollectorWithRunner(runner, "nvidia-smi")

	// When
	count, countErr := collector.DeviceCount()
	device, deviceErr := collector.Device(0)

	// Then
	requireNoError(t, countErr)
	requireNoError(t, deviceErr)
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}
	if device == nil {
		t.Fatal("expected device, got nil")
	}
	helpCalls := 0
	for _, call := range runner.calls {
		if reflect.DeepEqual(call.args, []string{"--help-query-gpu"}) {
			helpCalls++
		}
	}
	if helpCalls != 1 {
		t.Fatalf("expected one help-query call, got %d", helpCalls)
	}
}

func TestCollector_Device_returnsError_whenCSVRowHasWrongColumnCount(t *testing.T) {
	// Given
	runner := &fakeRunner{out: []byte("0, GPU-short, NVIDIA Test\n")}
	collector := newCollectorWithRunner(runner, "nvidia-smi")

	// When
	device, err := collector.Device(0)

	// Then
	if device != nil {
		t.Fatalf("expected nil device, got %#v", device)
	}
	requireError(t, err)
}

func TestCollector_Device_returnsError_whenCSVRowHasNonNumericField(t *testing.T) {
	// Given
	runner := &fakeRunner{out: []byte("0, GPU-bad, NVIDIA Test, bad, 1, 2, 3, 4, 5, 6, 7, 8, P0, 535.129.03\n")}
	collector := newCollectorWithRunner(runner, "nvidia-smi")

	// When
	device, err := collector.Device(0)

	// Then
	if device != nil {
		t.Fatalf("expected nil device, got %#v", device)
	}
	requireError(t, err)
}

func TestCollector_Device_returnsError_whenAnyNumericCSVFieldIsInvalid(t *testing.T) {
	tests := []struct {
		name   string
		column int
	}{
		{name: "index", column: 0},
		{name: "memory used", column: 4},
		{name: "temperature", column: 5},
		{name: "power draw", column: 6},
		{name: "power limit", column: 7},
		{name: "graphics clock", column: 8},
		{name: "memory clock", column: 9},
		{name: "gpu utilization", column: 10},
		{name: "memory utilization", column: 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			row := validRow()
			row[tt.column] = "bad"
			runner := &fakeRunner{out: []byte(strings.Join(row, ",") + "\n")}
			collector := newCollectorWithRunner(runner, "nvidia-smi")

			// When
			device, err := collector.Device(0)

			// Then
			if device != nil {
				t.Fatalf("expected nil device, got %#v", device)
			}
			requireError(t, err)
		})
	}
}

func TestCollector_Device_returnsError_whenCSVIsMalformed(t *testing.T) {
	// Given
	runner := &fakeRunner{out: []byte("\"unterminated")}
	collector := newCollectorWithRunner(runner, "nvidia-smi")

	// When
	device, err := collector.Device(0)

	// Then
	if device != nil {
		t.Fatalf("expected nil device, got %#v", device)
	}
	requireError(t, err)
}

func TestCollector_DeviceCount_returnsError_whenRunnerFailsForNonAvailabilityReason(t *testing.T) {
	// Given
	runner := &fakeRunner{err: errors.New("nvidia-smi failed")}
	collector := newCollectorWithRunner(runner, "nvidia-smi")

	// When
	count, err := collector.DeviceCount()

	// Then
	if count != 0 {
		t.Fatalf("expected count 0, got %d", count)
	}
	requireError(t, err)
	if errors.Is(err, gpu.ErrBackendUnavailable) {
		t.Fatalf("expected non-availability error, got %v", err)
	}
}

func TestCollector_DeviceCount_returnsParsedRowCount_whenQuerySucceeds(t *testing.T) {
	// Given
	runner := &fakeRunner{out: []byte(strings.Join([]string{
		"0, GPU-111, NVIDIA A100, 40960, 1024, 55, 120.50, 400.00, 1410, 1215, 75, 20, P0, 535.129.03, 30, 10, 4, 16",
		"1, GPU-222, NVIDIA L40S, 46068, 2048, 60, 80.25, 350.00, 1800, 9001, 42, 11, P2, 535.129.03, 25, 5, 3, 8",
	}, "\n"))}
	collector := newCollectorWithRunner(runner, "nvidia-smi")

	// When
	count, err := collector.DeviceCount()

	// Then
	requireNoError(t, err)
	if count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}
}

func TestCollector_Device_returnsError_whenIndexOutOfRange(t *testing.T) {
	// Given
	runner := &fakeRunner{out: []byte("0, GPU-111, NVIDIA A100, 40960, 1024, 55, 120.50, 400.00, 1410, 1215, 75, 20, P0, 535.129.03\n")}
	collector := newCollectorWithRunner(runner, "nvidia-smi")

	// When
	device, err := collector.Device(1)

	// Then
	if device != nil {
		t.Fatalf("expected nil device, got %#v", device)
	}
	requireError(t, err)
}

func TestCollector_DeviceCount_returnsErrBackendUnavailable_whenRunnerCannotFindBinary(t *testing.T) {
	// Given
	runner := &fakeRunner{err: exec.ErrNotFound}
	collector := newCollectorWithRunner(runner, "nvidia-smi")

	// When
	count, err := collector.DeviceCount()

	// Then
	if count != 0 {
		t.Fatalf("expected count 0, got %d", count)
	}
	requireErrorIs(t, err, gpu.ErrBackendUnavailable)
}

func TestNewCollector_returnsErrBackendUnavailable_whenBinaryMissingFromPath(t *testing.T) {
	// Given
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	// When
	collector, err := newCollector()

	// Then
	if collector != nil {
		t.Fatalf("expected nil collector, got %#v", collector)
	}
	requireErrorIs(t, err, gpu.ErrBackendUnavailable)
}

func TestNewCollector_usesConfiguredNvidiaSmiPath_whenConfigOverrideIsExecutable(t *testing.T) {
	// Given
	home := t.TempDir()
	smiPath := writeExecutable(t, filepath.Join(t.TempDir(), "custom-nvidia-smi"))
	writeConfig(t, home, "default_output: table\nbackend: auto\nnvidia_smi_path: "+smiPath+"\n")
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	// When
	collector, err := newCollector()

	// Then
	requireNoError(t, err)
	typed, ok := collector.(*Collector)
	if !ok {
		t.Fatalf("expected *Collector, got %T", collector)
	}
	if typed.smiPath != smiPath {
		t.Fatalf("expected smi path %q, got %q", smiPath, typed.smiPath)
	}
}

func TestNewCollector_usesPathLookup_whenConfigHasNoOverride(t *testing.T) {
	// Given
	home := t.TempDir()
	binDir := t.TempDir()
	smiPath := writeExecutable(t, filepath.Join(binDir, "nvidia-smi"))
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir)

	// When
	collector, err := newCollector()

	// Then
	requireNoError(t, err)
	typed, ok := collector.(*Collector)
	if !ok {
		t.Fatalf("expected *Collector, got %T", collector)
	}
	if typed.smiPath != smiPath {
		t.Fatalf("expected smi path %q, got %q", smiPath, typed.smiPath)
	}
}

func TestNewCollector_returnsConfigError_whenConfigIsInvalid(t *testing.T) {
	// Given
	home := t.TempDir()
	writeConfig(t, home, "default_output: invalid\nbackend: auto\n")
	t.Setenv("HOME", home)

	// When
	collector, err := newCollector()

	// Then
	if collector != nil {
		t.Fatalf("expected nil collector, got %#v", collector)
	}
	requireError(t, err)
}

func TestOsExecRunner_Run_returnsCommandOutput_whenCommandSucceeds(t *testing.T) {
	// Given
	runner := osExecRunner{}

	// When
	out, err := runner.Run(context.Background(), "echo", "smi-runner-ok")

	// Then
	requireNoError(t, err)
	if strings.TrimSpace(string(out)) != "smi-runner-ok" {
		t.Fatalf("expected echo output, got %q", string(out))
	}
}

func TestOsExecRunner_Run_returnsError_whenCommandFails(t *testing.T) {
	// Given
	runner := osExecRunner{}

	// When
	out, err := runner.Run(context.Background(), "false")

	// Then
	if out != nil {
		t.Fatalf("expected nil output, got %q", string(out))
	}
	requireError(t, err)
}

func TestCollector_InitShutdownBackend_returnStaticValues(t *testing.T) {
	// Given
	collector := newCollectorWithRunner(&fakeRunner{}, "nvidia-smi")

	// When
	initErr := collector.Init()
	shutdownErr := collector.Shutdown()
	backend := collector.Backend()

	// Then
	requireNoError(t, initErr)
	requireNoError(t, shutdownErr)
	if backend != "nvidia-smi" {
		t.Fatalf("expected backend nvidia-smi, got %q", backend)
	}
}

func requireDevice(t *testing.T, got *gpu.Device, want gpu.Device) {
	t.Helper()
	if got == nil {
		t.Fatal("expected device, got nil")
	}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("unexpected device\nwant: %#v\n got: %#v", want, *got)
	}
}

func validRow() []string {
	return []string{"0", "GPU-111", "NVIDIA A100", "40960", "1024", "55", "120.50", "400.00", "1410", "1215", "75", "20", "P0", "535.129.03"}
}

func writeExecutable(t *testing.T, path string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	return path
}

func writeConfig(t *testing.T, home, content string) {
	t.Helper()
	configDir := filepath.Join(home, ".gpu-tools")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func requireError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func requireErrorIs(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("expected error %v to match %v", err, target)
	}
}
