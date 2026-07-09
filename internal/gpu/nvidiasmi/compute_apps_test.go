package nvidiasmi

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

// computeAppsRunner is a dedicated fake that distinguishes the three nvidia-smi
// invocations the collector makes: --help-query-gpu, --query-gpu=...,
// --help-query-compute-apps, and --query-compute-apps=... . Each has its own
// canned output and error so process attribution can be driven deterministically.
type computeAppsRunner struct {
	gpuHelpOut     []byte
	gpuQueryOut    []byte
	appsHelpOut    []byte
	appsHelpErr    error
	appsQueryOut   []byte
	appsQueryErr   error
	appsQueryArgs  []string
	appsHelpCalls  int
	appsQueryCalls int
}

func (r *computeAppsRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	if len(args) == 1 && args[0] == "--help-query-gpu" {
		if r.gpuHelpOut == nil {
			return fullFieldHelp(), nil
		}
		return append([]byte(nil), r.gpuHelpOut...), nil
	}
	if len(args) == 1 && args[0] == "--help-query-compute-apps" {
		r.appsHelpCalls++
		if r.appsHelpErr != nil {
			return nil, r.appsHelpErr
		}
		return append([]byte(nil), r.appsHelpOut...), nil
	}
	if len(args) > 0 && strings.HasPrefix(args[0], "--query-compute-apps=") {
		r.appsQueryCalls++
		r.appsQueryArgs = append([]string(nil), args...)
		if r.appsQueryErr != nil {
			return nil, r.appsQueryErr
		}
		return append([]byte(nil), r.appsQueryOut...), nil
	}
	// --query-gpu=...
	return append([]byte(nil), r.gpuQueryOut...), nil
}

func fullComputeAppsHelp() []byte {
	return []byte(`"gpu_uuid" - GPU UUID.
"pid" - Process ID.
"process_name" - Process name.
"used_memory" - Used GPU memory.
`)
}

func withFakeProcessResolver(t *testing.T) {
	t.Helper()
	restore := resolveProcess
	t.Cleanup(func() { resolveProcess = restore })
	resolveProcess = func(pid int) (string, string) {
		switch pid {
		case 1234:
			return "python3", "alice"
		case 5678:
			return "trainer", "bob"
		default:
			return "", ""
		}
	}
}

func TestCollector_Device_attachesComputeProcessesByExactUUID_whenAppsQuerySucceeds(t *testing.T) {
	// Given: two GPUs and two compute apps, one on each GPU by exact uuid.
	withFakeProcessResolver(t)
	runner := &computeAppsRunner{
		gpuQueryOut: []byte(strings.Join([]string{
			"0, GPU-AAA, NVIDIA A100, 40960, 1024, 55, 120.50, 400.00, 1410, 1215, 75, 20, P0, 535.129.03, 10, 5, 4, 16",
			"1, GPU-BBB, NVIDIA L40S, 46068, 2048, 60, 80.25, 350.00, 1800, 9001, 42, 11, P2, 535.129.03, 20, 8, 3, 8",
		}, "\n") + "\n"),
		appsHelpOut: fullComputeAppsHelp(),
		appsQueryOut: []byte(strings.Join([]string{
			"GPU-AAA, 1234, python3, 512",
			"GPU-BBB, 5678, trainer, 1024",
		}, "\n") + "\n"),
	}
	collector := newCollectorWithRunner(runner, "nvidia-smi")

	// When
	gpu0, err0 := collector.Device(0)
	gpu1, err1 := collector.Device(1)

	// Then
	requireNoError(t, err0)
	requireNoError(t, err1)
	requireProcesses(t, gpu0.Processes, []gpu.GPUProcess{
		{PID: 1234, Name: "python3", User: "alice", UsedMemory: 512 * bytesPerMiB, Type: "compute"},
	})
	requireProcesses(t, gpu1.Processes, []gpu.GPUProcess{
		{PID: 5678, Name: "trainer", User: "bob", UsedMemory: 1024 * bytesPerMiB, Type: "compute"},
	})
	if r := runner.appsQueryArgs; len(r) == 0 || !strings.Contains(r[0], "gpu_uuid") {
		t.Fatalf("expected compute-apps query to include gpu_uuid, got %#v", r)
	}
}

func TestCollector_Device_attachesMultipleProcessesToSameGPU_whenTwoAppsShareUUID(t *testing.T) {
	// Given: two compute apps on the SAME gpu uuid.
	withFakeProcessResolver(t)
	runner := &computeAppsRunner{
		gpuQueryOut: []byte("0, GPU-AAA, NVIDIA A100, 40960, 1024, 55, 120.50, 400.00, 1410, 1215, 75, 20, P0, 535.129.03, 10, 5, 4, 16\n"),
		appsHelpOut: fullComputeAppsHelp(),
		appsQueryOut: []byte(strings.Join([]string{
			"GPU-AAA, 1234, python3, 512",
			"GPU-AAA, 5678, trainer, 1024",
		}, "\n") + "\n"),
	}
	collector := newCollectorWithRunner(runner, "nvidia-smi")

	// When
	device, err := collector.Device(0)

	// Then
	requireNoError(t, err)
	requireProcesses(t, device.Processes, []gpu.GPUProcess{
		{PID: 1234, Name: "python3", User: "alice", UsedMemory: 512 * bytesPerMiB, Type: "compute"},
		{PID: 5678, Name: "trainer", User: "bob", UsedMemory: 1024 * bytesPerMiB, Type: "compute"},
	})
}

func TestCollector_Device_dropsProcessOnUnknownUUID_whenNoDeviceMatches(t *testing.T) {
	// Given: an app referencing a uuid that matches no device.
	withFakeProcessResolver(t)
	runner := &computeAppsRunner{
		gpuQueryOut:  []byte("0, GPU-AAA, NVIDIA A100, 40960, 1024, 55, 120.50, 400.00, 1410, 1215, 75, 20, P0, 535.129.03, 10, 5, 4, 16\n"),
		appsHelpOut:  fullComputeAppsHelp(),
		appsQueryOut: []byte("GPU-GHOST, 1234, python3, 512\n"),
	}
	collector := newCollectorWithRunner(runner, "nvidia-smi")

	// When
	device, err := collector.Device(0)

	// Then
	requireNoError(t, err)
	if len(device.Processes) != 0 {
		t.Fatalf("expected no processes for unknown uuid, got %#v", device.Processes)
	}
}

func TestCollector_Device_returnsEmptyProcesses_whenComputeAppsQueryErrors(t *testing.T) {
	// Given: an old driver whose compute-apps query fails; devices must still return.
	withFakeProcessResolver(t)
	runner := &computeAppsRunner{
		gpuQueryOut:  []byte("0, GPU-AAA, NVIDIA A100, 40960, 1024, 55, 120.50, 400.00, 1410, 1215, 75, 20, P0, 535.129.03, 10, 5, 4, 16\n"),
		appsHelpOut:  fullComputeAppsHelp(),
		appsQueryErr: errors.New("Function Not Found"),
	}
	collector := newCollectorWithRunner(runner, "nvidia-smi")

	// When
	device, err := collector.Device(0)

	// Then
	requireNoError(t, err)
	if len(device.Processes) != 0 {
		t.Fatalf("expected empty processes when compute-apps query errors, got %#v", device.Processes)
	}
}

func TestCollector_Device_skipsAttribution_whenComputeAppsHelpOmitsGPUUUID(t *testing.T) {
	// Given: help for compute-apps lists pid/process_name/used_memory but NOT
	// gpu_uuid — attribution must be skipped entirely (no query, no guessing).
	withFakeProcessResolver(t)
	runner := &computeAppsRunner{
		gpuQueryOut: []byte("0, GPU-AAA, NVIDIA A100, 40960, 1024, 55, 120.50, 400.00, 1410, 1215, 75, 20, P0, 535.129.03, 10, 5, 4, 16\n"),
		appsHelpOut: []byte(`"pid" - Process ID.
"process_name" - Process name.
"used_memory" - Used GPU memory.
`),
	}
	collector := newCollectorWithRunner(runner, "nvidia-smi")

	// When
	device, err := collector.Device(0)

	// Then
	requireNoError(t, err)
	if len(device.Processes) != 0 {
		t.Fatalf("expected empty processes when gpu_uuid unsupported, got %#v", device.Processes)
	}
	if runner.appsQueryCalls != 0 {
		t.Fatalf("expected no compute-apps query when gpu_uuid unsupported, got %d calls", runner.appsQueryCalls)
	}
}

func TestCollector_Device_skipsAttribution_whenComputeAppsHelpErrors(t *testing.T) {
	// Given: --help-query-compute-apps itself fails (very old driver). Degrade
	// to no attribution without failing the collection.
	withFakeProcessResolver(t)
	runner := &computeAppsRunner{
		gpuQueryOut: []byte("0, GPU-AAA, NVIDIA A100, 40960, 1024, 55, 120.50, 400.00, 1410, 1215, 75, 20, P0, 535.129.03, 10, 5, 4, 16\n"),
		appsHelpErr: errors.New("invalid option --help-query-compute-apps"),
	}
	collector := newCollectorWithRunner(runner, "nvidia-smi")

	// When
	device, err := collector.Device(0)

	// Then
	requireNoError(t, err)
	if len(device.Processes) != 0 {
		t.Fatalf("expected empty processes when compute-apps help errors, got %#v", device.Processes)
	}
	if runner.appsQueryCalls != 0 {
		t.Fatalf("expected no compute-apps query when help errors, got %d calls", runner.appsQueryCalls)
	}
}

func TestCollector_Device_dropsProcessWithMalformedRow_whenAttributingApps(t *testing.T) {
	// Given: a malformed app row (bad pid / short row) is dropped, valid ones kept.
	withFakeProcessResolver(t)
	runner := &computeAppsRunner{
		gpuQueryOut: []byte("0, GPU-AAA, NVIDIA A100, 40960, 1024, 55, 120.50, 400.00, 1410, 1215, 75, 20, P0, 535.129.03, 10, 5, 4, 16\n"),
		appsHelpOut: fullComputeAppsHelp(),
		appsQueryOut: []byte(strings.Join([]string{
			"GPU-AAA, not-a-pid, bad, 512",
			"GPU-AAA, 1234, python3, notmem",
			"GPU-AAA, 5678, trainer, 1024",
		}, "\n") + "\n"),
	}
	collector := newCollectorWithRunner(runner, "nvidia-smi")

	// When
	device, err := collector.Device(0)

	// Then
	requireNoError(t, err)
	requireProcesses(t, device.Processes, []gpu.GPUProcess{
		{PID: 5678, Name: "trainer", User: "bob", UsedMemory: 1024 * bytesPerMiB, Type: "compute"},
	})
}

func TestParseComputeAppsHelp_detectsGPUUUIDSupport(t *testing.T) {
	tests := []struct {
		name string
		help []byte
		want bool
	}{
		{name: "gpu_uuid present", help: fullComputeAppsHelp(), want: true},
		{name: "gpu_uuid absent", help: []byte("\"pid\" - Process ID.\n\"used_memory\" - mem.\n"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given / When
			got := parseSupportedFields(tt.help)[fieldGPUUUID]

			// Then
			if got != tt.want {
				t.Fatalf("expected gpu_uuid support=%t, got %t", tt.want, got)
			}
		})
	}
}

func TestParseComputeApps_returnsNilOrDropsRows_whenInputEmptyMalformedOrInvalid(t *testing.T) {
	tests := []struct {
		name    string
		out     []byte
		wantLen int
	}{
		{name: "blank input yields nil", out: []byte("  \n\t"), wantLen: 0},
		{name: "malformed csv yields nil", out: []byte("\"unterminated"), wantLen: 0},
		{name: "wrong column count dropped", out: []byte("GPU-AAA, 1234, python3\n"), wantLen: 0},
		{name: "empty uuid dropped", out: []byte(" , 1234, python3, 512\n"), wantLen: 0},
		{name: "valid row kept", out: []byte("GPU-AAA, 1234, python3, 512\n"), wantLen: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given / When
			apps := parseComputeApps(tt.out)

			// Then
			if len(apps) != tt.wantLen {
				t.Fatalf("expected %d apps, got %d: %#v", tt.wantLen, len(apps), apps)
			}
		})
	}
}

func requireProcesses(t *testing.T, got, want []gpu.GPUProcess) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %d processes, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("process %d mismatch\nwant: %#v\n got: %#v", i, want[i], got[i])
		}
	}
}
