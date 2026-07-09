package gpu

import (
	"encoding/json"
	"testing"
)

func TestDevice_exposesGPUFieldsWithDocumentedUnits(t *testing.T) {
	// Given
	eccEnabled := true
	fanSpeed := 42
	device := Device{
		Index:           1,
		UUID:            "GPU-uuid",
		Name:            "NVIDIA Test GPU",
		MemoryTotal:     80 * 1024 * 1024 * 1024,
		MemoryUsed:      12 * 1024 * 1024 * 1024,
		Temperature:     65,
		PowerDraw:       250_000,
		PowerLimit:      300_000,
		ClockGraphics:   1_410,
		ClockMem:        5_001,
		UtilizationGPU:  91,
		UtilizationMem:  73,
		ThrottleReasons: []string{"power", "thermal"},
		ECCEnabled:      &eccEnabled,
		MIGEnabled:      true,
		MIGDevices: []MIGDevice{
			{GIID: 7, CIID: 3, UUID: "MIG-uuid", MemoryTotal: 10 * 1024 * 1024 * 1024},
		},
		PState:        "P0",
		FanSpeed:      &fanSpeed,
		DriverVersion: "555.42.02",
		CudaVersion:   "12.5",
	}

	// When
	mig := device.MIGDevices[0]

	// Then
	if device.MemoryTotal != 80*1024*1024*1024 {
		t.Fatalf("memory total should be bytes, got %d", device.MemoryTotal)
	}
	if device.Temperature != 65 {
		t.Fatalf("temperature should be Celsius, got %d", device.Temperature)
	}
	if device.PowerDraw != 250_000 {
		t.Fatalf("power draw should be milliwatts, got %d", device.PowerDraw)
	}
	if device.ClockGraphics != 1_410 || device.ClockMem != 5_001 {
		t.Fatalf("clocks should be MHz, got graphics=%d mem=%d", device.ClockGraphics, device.ClockMem)
	}
	if device.UtilizationGPU != 91 || device.UtilizationMem != 73 {
		t.Fatalf("utilization should be percent, got gpu=%d mem=%d", device.UtilizationGPU, device.UtilizationMem)
	}
	if device.ECCEnabled == nil || !*device.ECCEnabled {
		t.Fatalf("expected ECC enabled pointer to be true, got %#v", device.ECCEnabled)
	}
	if device.FanSpeed == nil || *device.FanSpeed != 42 {
		t.Fatalf("expected fan speed pointer to be 42, got %#v", device.FanSpeed)
	}
	if mig.GIID != 7 || mig.CIID != 3 || mig.UUID != "MIG-uuid" || mig.MemoryTotal != 10*1024*1024*1024 {
		t.Fatalf("unexpected MIG device: %#v", mig)
	}
}

func TestDevice_JSONRoundTrip_preservesProcessAndRicherMetricFields(t *testing.T) {
	// Given
	var zeroProcess GPUProcess
	if zeroProcess.PID != 0 || zeroProcess.Name != "" || zeroProcess.User != "" || zeroProcess.UsedMemory != 0 || zeroProcess.Type != "" {
		t.Fatalf("unexpected GPUProcess zero value: %#v", zeroProcess)
	}
	device := Device{
		Index:            2,
		UUID:             "GPU-json",
		Name:             "JSON GPU",
		MemoryTotal:      24 * 1024 * 1024 * 1024,
		MemoryUsed:       6 * 1024 * 1024 * 1024,
		EncoderUtil:      17,
		DecoderUtil:      23,
		PCIeGen:          4,
		PCIeWidth:        16,
		MemBandwidthUtil: 88,
		Processes: []GPUProcess{
			{PID: 1001, Name: "trainer", User: "alice", UsedMemory: 4 * 1024 * 1024 * 1024, Type: "compute"},
			{PID: 1002, Name: "compositor", User: "bob", UsedMemory: 512 * 1024 * 1024, Type: "graphics"},
		},
	}

	// When
	payload, err := json.Marshal(device)
	if err != nil {
		t.Fatalf("marshal device: %v", err)
	}
	var got Device
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal device: %v", err)
	}

	// Then
	if got.EncoderUtil != device.EncoderUtil || got.DecoderUtil != device.DecoderUtil {
		t.Fatalf("unexpected encoder/decoder utilization after JSON round trip: %#v", got)
	}
	if got.PCIeGen != device.PCIeGen || got.PCIeWidth != device.PCIeWidth {
		t.Fatalf("unexpected PCIe fields after JSON round trip: %#v", got)
	}
	if got.MemBandwidthUtil != device.MemBandwidthUtil {
		t.Fatalf("unexpected memory bandwidth utilization after JSON round trip: %#v", got)
	}
	if len(got.Processes) != 2 {
		t.Fatalf("expected 2 processes after JSON round trip, got %#v", got.Processes)
	}
	if got.Processes[0] != device.Processes[0] || got.Processes[1] != device.Processes[1] {
		t.Fatalf("unexpected processes after JSON round trip: %#v", got.Processes)
	}
}

func TestVendor_constantsExposeCurrentAndFutureGPUVendors(t *testing.T) {
	// When
	vendors := []Vendor{VendorNVIDIA, VendorAMD, VendorIntel}

	// Then
	if vendors[0] != Vendor("nvidia") {
		t.Fatalf("expected nvidia vendor, got %q", vendors[0])
	}
	if vendors[1] != Vendor("amd") {
		t.Fatalf("expected amd vendor, got %q", vendors[1])
	}
	if vendors[2] != Vendor("intel") {
		t.Fatalf("expected intel vendor, got %q", vendors[2])
	}
}
