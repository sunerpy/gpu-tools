package amd

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"testing"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

type fakeRunner struct {
	out  []byte
	err  error
	name string
	args []string
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.name = name
	r.args = append([]string(nil), args...)
	if r.err != nil {
		return nil, r.err
	}
	return append([]byte(nil), r.out...), nil
}

func TestCollector_Device_parsesTwoCardJSON_whenQuerySucceeds(t *testing.T) {
	// Given
	runner := &fakeRunner{out: []byte(`{
		"card1": {
			"Card series": "AMD Instinct MI250X",
			"GPU use (%)": "88",
			"GPU memory use (%)": "41",
			"VRAM Total Memory (MiB)": "65536",
			"VRAM Total Used Memory (MiB)": "32768",
			"Temperature (Sensor edge) (C)": "62.6",
			"Average Graphics Package Power (W)": "280.4"
		},
		"card0": {
			"Product Name": "AMD Radeon Pro W7900",
			"GPU use (%)": "12",
			"GPU memory use (%)": "33",
			"VRAM Total Memory (GiB)": "48",
			"VRAM Total Used Memory (MiB)": "1024",
			"Temperature (Sensor edge) (C)": "45.4",
			"Average Graphics Package Power (W)": "30.0"
		}
	}`)}
	collector := newCollectorWithRunner(runner, "/usr/bin/rocm-smi")

	// When
	device, err := collector.Device(0)

	// Then
	requireNoError(t, err)
	if runner.name != "/usr/bin/rocm-smi" {
		t.Fatalf("expected runner name /usr/bin/rocm-smi, got %q", runner.name)
	}
	if !reflect.DeepEqual(runner.args, queryArgs) {
		t.Fatalf("expected args %#v, got %#v", queryArgs, runner.args)
	}
	requireDevice(t, device, gpu.Device{
		Index:          0,
		Name:           "AMD Radeon Pro W7900",
		MemoryTotal:    48 * 1024 * 1024 * 1024,
		MemoryUsed:     1024 * 1024 * 1024,
		Temperature:    45,
		PowerDraw:      30000,
		UtilizationGPU: 12,
		UtilizationMem: 33,
	})
}

func TestCollector_DeviceCount_returnsParsedCardCount_whenQuerySucceeds(t *testing.T) {
	// Given
	runner := &fakeRunner{out: []byte(`{"card1":{},"card0":{}}`)}
	collector := newCollectorWithRunner(runner, "rocm-smi")

	// When
	count, err := collector.DeviceCount()

	// Then
	requireNoError(t, err)
	if count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}
}

func TestCollector_Device_returnsSecondSortedCard_whenJSONKeysAreOutOfOrder(t *testing.T) {
	// Given
	runner := &fakeRunner{out: []byte(`{
		"card1":{"Card series":"AMD Instinct MI250X","VRAM Total Memory (MiB)":"65536","VRAM Total Used Memory (MiB)":"32768","Temperature (Sensor edge) (C)":"62.6","Average Graphics Package Power (W)":"280.4","GPU use (%)":"88"},
		"card0":{"Card series":"AMD Radeon Pro W7900"}
	}`)}
	collector := newCollectorWithRunner(runner, "rocm-smi")

	// When
	device, err := collector.Device(1)

	// Then
	requireNoError(t, err)
	requireDevice(t, device, gpu.Device{
		Index:          1,
		Name:           "AMD Instinct MI250X",
		MemoryTotal:    65536 * 1024 * 1024,
		MemoryUsed:     32768 * 1024 * 1024,
		Temperature:    63,
		PowerDraw:      280400,
		UtilizationGPU: 88,
	})
}

func TestCollector_Device_returnsError_whenJSONIsMalformed(t *testing.T) {
	// Given
	runner := &fakeRunner{out: []byte(`{"card0":`)}
	collector := newCollectorWithRunner(runner, "rocm-smi")

	// When
	device, err := collector.Device(0)

	// Then
	if device != nil {
		t.Fatalf("expected nil device, got %#v", device)
	}
	requireError(t, err)
}

func TestCollector_DeviceCount_returnsErrBackendUnavailable_whenRunnerCannotFindBinary(t *testing.T) {
	// Given
	runner := &fakeRunner{err: exec.ErrNotFound}
	collector := newCollectorWithRunner(runner, "rocm-smi")

	// When
	count, err := collector.DeviceCount()

	// Then
	if count != 0 {
		t.Fatalf("expected count 0, got %d", count)
	}
	requireErrorIs(t, err, gpu.ErrBackendUnavailable)
}

func TestNewCollector_returnsErrBackendUnavailable_whenRocmSMIMissingFromPath(t *testing.T) {
	// Given
	originalLookPath := lookPath
	lookPath = func(string) (string, error) {
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() { lookPath = originalLookPath })

	// When
	collector, err := newCollector()

	// Then
	if collector != nil {
		t.Fatalf("expected nil collector, got %#v", collector)
	}
	requireErrorIs(t, err, gpu.ErrBackendUnavailable)
}

func TestCollector_InitShutdownBackend_returnStaticValues(t *testing.T) {
	// Given
	collector := newCollectorWithRunner(&fakeRunner{}, "rocm-smi")

	// When
	initErr := collector.Init()
	shutdownErr := collector.Shutdown()
	backend := collector.Backend()

	// Then
	requireNoError(t, initErr)
	requireNoError(t, shutdownErr)
	if backend != "amd" {
		t.Fatalf("expected backend amd, got %q", backend)
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
