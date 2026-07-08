package nvml

import (
	"errors"
	"reflect"
	"testing"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

type fakeNVMLLib struct {
	initFunc                  func() error
	shutdownFunc              func() error
	deviceCountFunc           func() (int, error)
	deviceHandleByIndexFunc   func(int) (uintptr, error)
	deviceNameFunc            func(uintptr) (string, error)
	deviceMemoryFunc          func(uintptr) (uint64, uint64, error)
	deviceTemperatureFunc     func(uintptr) (uint32, error)
	devicePowerUsageFunc      func(uintptr) (uint32, error)
	devicePowerLimitFunc      func(uintptr) (uint32, error)
	deviceClockGraphicsFunc   func(uintptr) (uint32, error)
	deviceClockMemFunc        func(uintptr) (uint32, error)
	deviceUtilizationFunc     func(uintptr) (uint32, uint32, error)
	deviceThrottleReasonsFunc func(uintptr) (uint64, error)
	deviceECCEnabledFunc      func(uintptr) (*bool, error)
	deviceMIGEnabledFunc      func(uintptr) (bool, error)
	driverVersionFunc         func() (string, error)
	cudaVersionFunc           func() (string, error)
}

func (f *fakeNVMLLib) Init() error {
	if f.initFunc != nil {
		return f.initFunc()
	}
	return nil
}

func (f *fakeNVMLLib) Shutdown() error {
	if f.shutdownFunc != nil {
		return f.shutdownFunc()
	}
	return nil
}

func (f *fakeNVMLLib) DeviceCount() (int, error) {
	if f.deviceCountFunc != nil {
		return f.deviceCountFunc()
	}
	return 2, nil
}

func (f *fakeNVMLLib) DeviceHandleByIndex(i int) (uintptr, error) {
	if f.deviceHandleByIndexFunc != nil {
		return f.deviceHandleByIndexFunc(i)
	}
	return uintptr(i + 100), nil
}

func (f *fakeNVMLLib) DeviceName(h uintptr) (string, error) {
	if f.deviceNameFunc != nil {
		return f.deviceNameFunc(h)
	}
	if h == 100 {
		return "NVIDIA A100-SXM4-40GB", nil
	}
	return "NVIDIA L40S", nil
}

func (f *fakeNVMLLib) DeviceMemory(h uintptr) (uint64, uint64, error) {
	if f.deviceMemoryFunc != nil {
		return f.deviceMemoryFunc(h)
	}
	if h == 100 {
		return 40 * 1024 * 1024 * 1024, 8 * 1024 * 1024 * 1024, nil
	}
	return 48 * 1024 * 1024 * 1024, 12 * 1024 * 1024 * 1024, nil
}

func (f *fakeNVMLLib) DeviceTemperature(h uintptr) (uint32, error) {
	if f.deviceTemperatureFunc != nil {
		return f.deviceTemperatureFunc(h)
	}
	return uint32(h - 45), nil
}

func (f *fakeNVMLLib) DevicePowerUsage(h uintptr) (uint32, error) {
	if f.devicePowerUsageFunc != nil {
		return f.devicePowerUsageFunc(h)
	}
	return uint32(h * 1000), nil
}

func (f *fakeNVMLLib) DevicePowerLimit(h uintptr) (uint32, error) {
	if f.devicePowerLimitFunc != nil {
		return f.devicePowerLimitFunc(h)
	}
	return uint32(h * 2000), nil
}

func (f *fakeNVMLLib) DeviceClockGraphics(h uintptr) (uint32, error) {
	if f.deviceClockGraphicsFunc != nil {
		return f.deviceClockGraphicsFunc(h)
	}
	return uint32(h + 1000), nil
}

func (f *fakeNVMLLib) DeviceClockMem(h uintptr) (uint32, error) {
	if f.deviceClockMemFunc != nil {
		return f.deviceClockMemFunc(h)
	}
	return uint32(h + 5000), nil
}

func (f *fakeNVMLLib) DeviceUtilization(h uintptr) (uint32, uint32, error) {
	if f.deviceUtilizationFunc != nil {
		return f.deviceUtilizationFunc(h)
	}
	return uint32(h - 25), uint32(h - 75), nil
}

func (f *fakeNVMLLib) DeviceThrottleReasons(h uintptr) (uint64, error) {
	if f.deviceThrottleReasonsFunc != nil {
		return f.deviceThrottleReasonsFunc(h)
	}
	if h == 100 {
		return 0x1 | 0x4 | 0x80, nil
	}
	return 0x2 | 0x20 | 0x100, nil
}

func (f *fakeNVMLLib) DeviceECCEnabled(h uintptr) (*bool, error) {
	if f.deviceECCEnabledFunc != nil {
		return f.deviceECCEnabledFunc(h)
	}
	return boolPtr(h == 100), nil
}

func (f *fakeNVMLLib) DeviceMIGEnabled(h uintptr) (bool, error) {
	if f.deviceMIGEnabledFunc != nil {
		return f.deviceMIGEnabledFunc(h)
	}
	return h == 101, nil
}

func (f *fakeNVMLLib) DriverVersion() (string, error) {
	if f.driverVersionFunc != nil {
		return f.driverVersionFunc()
	}
	return "535.129.03", nil
}

func (f *fakeNVMLLib) CudaVersion() (string, error) {
	if f.cudaVersionFunc != nil {
		return f.cudaVersionFunc()
	}
	return "12020", nil
}

func TestCollector_DeviceCount_returnsLibDeviceCount_whenLibSucceeds(t *testing.T) {
	// Given
	collector := newCollectorWithLib(&fakeNVMLLib{
		deviceCountFunc: func() (int, error) { return 2, nil },
	})

	// When
	count, err := collector.DeviceCount()

	// Then
	requireNoError(t, err)
	if count != 2 {
		t.Fatalf("expected device count 2, got %d", count)
	}
}

func TestCollector_Device_mapsAllFields_whenLibReturnsTwoDevices(t *testing.T) {
	// Given
	collector := newCollectorWithLib(&fakeNVMLLib{})

	// When
	first, firstErr := collector.Device(0)
	second, secondErr := collector.Device(1)

	// Then
	requireNoError(t, firstErr)
	requireDevice(t, first, gpu.Device{
		Index:           0,
		Name:            "NVIDIA A100-SXM4-40GB",
		MemoryTotal:     40 * 1024 * 1024 * 1024,
		MemoryUsed:      8 * 1024 * 1024 * 1024,
		Temperature:     55,
		PowerDraw:       100000,
		PowerLimit:      200000,
		ClockGraphics:   1100,
		ClockMem:        5100,
		UtilizationGPU:  75,
		UtilizationMem:  25,
		ThrottleReasons: []string{"gpu_idle", "sw_power_cap", "hw_power_brake_slowdown"},
		ECCEnabled:      boolPtr(true),
		DriverVersion:   "535.129.03",
		CudaVersion:     "12020",
	})
	requireNoError(t, secondErr)
	requireDevice(t, second, gpu.Device{
		Index:           1,
		Name:            "NVIDIA L40S",
		MemoryTotal:     48 * 1024 * 1024 * 1024,
		MemoryUsed:      12 * 1024 * 1024 * 1024,
		Temperature:     56,
		PowerDraw:       101000,
		PowerLimit:      202000,
		ClockGraphics:   1101,
		ClockMem:        5101,
		UtilizationGPU:  76,
		UtilizationMem:  26,
		ThrottleReasons: []string{"applications_clocks_setting", "sw_thermal_slowdown", "display_clock_setting"},
		ECCEnabled:      boolPtr(false),
		MIGEnabled:      true,
		DriverVersion:   "535.129.03",
		CudaVersion:     "12020",
	})
}

func TestCollector_Device_preservesNilECC_whenLibReportsUnsupportedECC(t *testing.T) {
	// Given
	collector := newCollectorWithLib(&fakeNVMLLib{
		deviceECCEnabledFunc: func(uintptr) (*bool, error) { return nil, nil },
	})

	// When
	device, err := collector.Device(0)

	// Then
	requireNoError(t, err)
	if device.ECCEnabled != nil {
		t.Fatalf("expected nil ECCEnabled, got %#v", *device.ECCEnabled)
	}
}

func TestCollector_Device_returnsError_whenEachLibCallFails(t *testing.T) {
	tests := []struct {
		name string
		lib  *fakeNVMLLib
	}{
		{name: "handle lookup", lib: &fakeNVMLLib{deviceHandleByIndexFunc: func(int) (uintptr, error) { return 0, errFakeNVML }}},
		{name: "name", lib: &fakeNVMLLib{deviceNameFunc: func(uintptr) (string, error) { return "", errFakeNVML }}},
		{name: "memory", lib: &fakeNVMLLib{deviceMemoryFunc: func(uintptr) (uint64, uint64, error) { return 0, 0, errFakeNVML }}},
		{name: "temperature", lib: &fakeNVMLLib{deviceTemperatureFunc: func(uintptr) (uint32, error) { return 0, errFakeNVML }}},
		{name: "power usage", lib: &fakeNVMLLib{devicePowerUsageFunc: func(uintptr) (uint32, error) { return 0, errFakeNVML }}},
		{name: "power limit", lib: &fakeNVMLLib{devicePowerLimitFunc: func(uintptr) (uint32, error) { return 0, errFakeNVML }}},
		{name: "graphics clock", lib: &fakeNVMLLib{deviceClockGraphicsFunc: func(uintptr) (uint32, error) { return 0, errFakeNVML }}},
		{name: "memory clock", lib: &fakeNVMLLib{deviceClockMemFunc: func(uintptr) (uint32, error) { return 0, errFakeNVML }}},
		{name: "utilization", lib: &fakeNVMLLib{deviceUtilizationFunc: func(uintptr) (uint32, uint32, error) { return 0, 0, errFakeNVML }}},
		{name: "throttle reasons", lib: &fakeNVMLLib{deviceThrottleReasonsFunc: func(uintptr) (uint64, error) { return 0, errFakeNVML }}},
		{name: "ecc enabled", lib: &fakeNVMLLib{deviceECCEnabledFunc: func(uintptr) (*bool, error) { return nil, errFakeNVML }}},
		{name: "mig enabled", lib: &fakeNVMLLib{deviceMIGEnabledFunc: func(uintptr) (bool, error) { return false, errFakeNVML }}},
		{name: "driver version", lib: &fakeNVMLLib{driverVersionFunc: func() (string, error) { return "", errFakeNVML }}},
		{name: "cuda version", lib: &fakeNVMLLib{cudaVersionFunc: func() (string, error) { return "", errFakeNVML }}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			collector := newCollectorWithLib(tt.lib)

			// When
			device, err := collector.Device(0)

			// Then
			if device != nil {
				t.Fatalf("expected nil device, got %#v", device)
			}
			requireErrorIs(t, err, errFakeNVML)
		})
	}
}

func TestCollector_DeviceCount_returnsError_whenLibCountFails(t *testing.T) {
	// Given
	collector := newCollectorWithLib(&fakeNVMLLib{
		deviceCountFunc: func() (int, error) { return 0, errFakeNVML },
	})

	// When
	count, err := collector.DeviceCount()

	// Then
	if count != 0 {
		t.Fatalf("expected count 0, got %d", count)
	}
	requireErrorIs(t, err, errFakeNVML)
}

func TestCollector_InitShutdownBackend_delegateToLibAndReturnBackendName(t *testing.T) {
	// Given
	var initCalled bool
	var shutdownCalled bool
	collector := newCollectorWithLib(&fakeNVMLLib{
		initFunc: func() error {
			initCalled = true
			return nil
		},
		shutdownFunc: func() error {
			shutdownCalled = true
			return nil
		},
	})

	// When
	initErr := collector.Init()
	shutdownErr := collector.Shutdown()
	backend := collector.Backend()

	// Then
	requireNoError(t, initErr)
	requireNoError(t, shutdownErr)
	if !initCalled {
		t.Fatal("expected Init to call lib Init")
	}
	if !shutdownCalled {
		t.Fatal("expected Shutdown to call lib Shutdown")
	}
	if backend != "nvml" {
		t.Fatalf("expected backend nvml, got %q", backend)
	}
}

func TestCollector_InitShutdown_returnErrors_whenLibFails(t *testing.T) {
	// Given
	collector := newCollectorWithLib(&fakeNVMLLib{
		initFunc:     func() error { return errFakeNVML },
		shutdownFunc: func() error { return errFakeNVML },
	})

	// When
	initErr := collector.Init()
	shutdownErr := collector.Shutdown()

	// Then
	requireErrorIs(t, initErr, errFakeNVML)
	requireErrorIs(t, shutdownErr, errFakeNVML)
}

var errFakeNVML = errors.New("fake nvml failure")

func boolPtr(v bool) *bool {
	return &v
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

func requireErrorIs(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("expected error %v to match %v", err, target)
	}
}
