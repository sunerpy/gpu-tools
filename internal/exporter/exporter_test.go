package exporter

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

// fakeCollector is a hand-written gpu.Collector fake for exporter tests. It
// records how many times DeviceCount is called so overlapping-scrape guarding
// (R4a) can be asserted.
type fakeCollector struct {
	mu sync.Mutex

	devices   []gpu.Device
	countErr  error
	deviceErr error
	backend   string

	initCalled     bool
	shutdownCalled bool
	countCalls     int

	initErr   error
	nilDevice bool
}

func (f *fakeCollector) Init() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.initCalled = true
	return f.initErr
}

func (f *fakeCollector) Shutdown() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shutdownCalled = true
	return nil
}

func (f *fakeCollector) DeviceCount() (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.countCalls++
	if f.countErr != nil {
		return 0, f.countErr
	}
	return len(f.devices), nil
}

func (f *fakeCollector) Device(i int) (*gpu.Device, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deviceErr != nil {
		return nil, f.deviceErr
	}
	if f.nilDevice {
		return nil, nil
	}
	if i < 0 || i >= len(f.devices) {
		return nil, errors.New("device index out of range")
	}
	device := f.devices[i]
	return &device, nil
}

func (f *fakeCollector) Backend() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.backend == "" {
		return "fake"
	}
	return f.backend
}

func twoGPUDevices() []gpu.Device {
	return []gpu.Device{
		{
			Index: 0, UUID: "GPU-aaaa", Name: "RTX 4090",
			MemoryTotal: 24 * 1024 * 1024 * 1024, MemoryUsed: 12 * 1024 * 1024 * 1024,
			Temperature: 42, PowerDraw: 100_000, PowerLimit: 450_000,
			ClockGraphics: 2100, ClockMem: 10501,
			UtilizationGPU: 10, UtilizationMem: 20,
			EncoderUtil: 5, DecoderUtil: 7,
			Processes: []gpu.GPUProcess{
				{PID: 111, Name: "python", Type: "compute", UsedMemory: 2 * 1024 * 1024 * 1024},
			},
		},
		{
			Index: 1, UUID: "GPU-bbbb", Name: "RTX 6000 Ada",
			MemoryTotal: 48 * 1024 * 1024 * 1024, MemoryUsed: 16 * 1024 * 1024 * 1024,
			Temperature: 55, PowerDraw: 90_000, PowerLimit: 300_000,
			ClockGraphics: 1800, ClockMem: 9000,
			UtilizationGPU: 30, UtilizationMem: 40,
			EncoderUtil: 1, DecoderUtil: 2,
			Processes: []gpu.GPUProcess{
				{PID: 222, Name: "ffmpeg", Type: "graphics", UsedMemory: 512 * 1024 * 1024},
			},
		},
	}
}

// scrape serves the exporter's own registry over httptest and returns the body.
func scrape(t *testing.T, exp *Exporter) string {
	t.Helper()
	handler := promhttp.HandlerFor(exp.Registry(), promhttp.HandlerOpts{})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("scrape GET failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read scrape body: %v", err)
	}
	return string(body)
}

func TestExporter_emitsPerGPUandProcessSeries_whenTwoGPUFake(t *testing.T) {
	// Given
	fake := &fakeCollector{devices: twoGPUDevices(), backend: "fake"}
	exp := New(func() (gpu.Collector, error) { return fake, nil })

	// When
	body := scrape(t, exp)

	// Then
	wants := []string{
		"gpu_tools_up 1",
		`gpu_utilization_percent{index="0",name="RTX 4090",uuid="GPU-aaaa"} 10`,
		`gpu_utilization_percent{index="1",name="RTX 6000 Ada",uuid="GPU-bbbb"} 30`,
		`gpu_memory_used_bytes{index="0",name="RTX 4090",uuid="GPU-aaaa"} 1.2884901888e+10`,
		`gpu_memory_total_bytes{index="0",name="RTX 4090",uuid="GPU-aaaa"} 2.5769803776e+10`,
		`gpu_temperature_celsius{index="0",name="RTX 4090",uuid="GPU-aaaa"} 42`,
		`gpu_power_draw_watts{index="0",name="RTX 4090",uuid="GPU-aaaa"} 100`,
		`gpu_power_limit_watts{index="0",name="RTX 4090",uuid="GPU-aaaa"} 450`,
		`gpu_clock_graphics_mhz{index="0",name="RTX 4090",uuid="GPU-aaaa"} 2100`,
		`gpu_clock_mem_mhz{index="0",name="RTX 4090",uuid="GPU-aaaa"} 10501`,
		`gpu_encoder_utilization_percent{index="0",name="RTX 4090",uuid="GPU-aaaa"} 5`,
		`gpu_decoder_utilization_percent{index="0",name="RTX 4090",uuid="GPU-aaaa"} 7`,
		`gpu_process_used_memory_bytes{index="0",pid="111",process_name="python",type="compute"} 2.147483648e+09`,
		`gpu_process_used_memory_bytes{index="1",pid="222",process_name="ffmpeg",type="graphics"} 5.36870912e+08`,
	}
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Fatalf("expected metrics to contain %q, got:\n%s", want, body)
		}
	}
	if !fake.initCalled {
		t.Fatalf("expected Init to be called")
	}
}

func TestExporter_emitsUpZeroAndNoDeviceSeries_whenBackendUnavailable(t *testing.T) {
	// Given
	exp := New(func() (gpu.Collector, error) {
		return nil, gpu.ErrBackendUnavailable
	})

	// When
	body := scrape(t, exp)

	// Then
	if !strings.Contains(body, "gpu_tools_up 0") {
		t.Fatalf("expected gpu_tools_up 0, got:\n%s", body)
	}
	if strings.Contains(body, "gpu_utilization_percent") {
		t.Fatalf("expected no gpu_utilization_percent series, got:\n%s", body)
	}
}

func TestExporter_emitsUpZeroAndNeverErrors_whenReadFails(t *testing.T) {
	// Given: factory ok but DeviceCount fails on scrape
	fake := &fakeCollector{countErr: errors.New("read boom")}
	var logs bytes.Buffer
	exp := NewWithLogger(func() (gpu.Collector, error) { return fake, nil }, &logs)

	// When
	body := scrape(t, exp)

	// Then: up 0, no device series, HTTP 200 (scrape() asserts 200), error logged to stderr seam
	if !strings.Contains(body, "gpu_tools_up 0") {
		t.Fatalf("expected gpu_tools_up 0 on read error, got:\n%s", body)
	}
	if strings.Contains(body, "gpu_utilization_percent") {
		t.Fatalf("expected no device series on read error, got:\n%s", body)
	}
	if logs.Len() == 0 {
		t.Fatalf("expected read error to be logged to the logger seam")
	}
}

func TestExporter_emitsUpZero_whenInitFails(t *testing.T) {
	// Given: factory succeeds but Init returns an error.
	fake := &fakeCollector{devices: twoGPUDevices(), initErr: errors.New("init boom")}
	var logs bytes.Buffer
	exp := NewWithLogger(func() (gpu.Collector, error) { return fake, nil }, &logs)

	// When
	body := scrape(t, exp)

	// Then
	if !strings.Contains(body, "gpu_tools_up 0") {
		t.Fatalf("expected gpu_tools_up 0 on init error, got:\n%s", body)
	}
	if logs.Len() == 0 {
		t.Fatalf("expected init error to be logged")
	}
}

func TestExporter_emitsUpZero_whenDeviceReadFails(t *testing.T) {
	// Given: DeviceCount reports 1 but Device returns an error.
	fake := &fakeCollector{devices: twoGPUDevices()[:1], deviceErr: errors.New("device boom")}
	var logs bytes.Buffer
	exp := NewWithLogger(func() (gpu.Collector, error) { return fake, nil }, &logs)

	// When
	body := scrape(t, exp)

	// Then
	if !strings.Contains(body, "gpu_tools_up 0") {
		t.Fatalf("expected gpu_tools_up 0 on device read error, got:\n%s", body)
	}
	if logs.Len() == 0 {
		t.Fatalf("expected device read error to be logged")
	}
}

func TestExporter_emitsUpZero_whenNilDevice(t *testing.T) {
	// Given: DeviceCount reports 1 but Device returns a nil device.
	fake := &fakeCollector{devices: twoGPUDevices()[:1], nilDevice: true}
	var logs bytes.Buffer
	exp := NewWithLogger(func() (gpu.Collector, error) { return fake, nil }, &logs)

	// When
	body := scrape(t, exp)

	// Then
	if !strings.Contains(body, "gpu_tools_up 0") {
		t.Fatalf("expected gpu_tools_up 0 on nil device, got:\n%s", body)
	}
	if logs.Len() == 0 {
		t.Fatalf("expected nil device error to be logged")
	}
}

func TestExporter_doesNotPanic_whenLoggerNil(t *testing.T) {
	// Given: nil logger + a read error exercises the logf nil guard.
	fake := &fakeCollector{countErr: errors.New("read boom")}
	exp := NewWithLogger(func() (gpu.Collector, error) { return fake, nil }, nil)

	// When
	body := scrape(t, exp)

	// Then
	if !strings.Contains(body, "gpu_tools_up 0") {
		t.Fatalf("expected gpu_tools_up 0, got:\n%s", body)
	}
}

func TestExporter_usesOwnRegistry_whenBuiltTwice(t *testing.T) {
	// Given/When: two exporters built back-to-back must not panic on duplicate
	// registration (R4b: own NewRegistry, never the global default).
	fake := &fakeCollector{devices: twoGPUDevices()}
	first := New(func() (gpu.Collector, error) { return fake, nil })
	second := New(func() (gpu.Collector, error) { return fake, nil })

	// Then
	if first.Registry() == second.Registry() {
		t.Fatalf("expected each exporter to hold its own registry")
	}
	_ = scrape(t, first)
	_ = scrape(t, second)
}
