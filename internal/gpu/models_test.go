package gpu

import "testing"

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
