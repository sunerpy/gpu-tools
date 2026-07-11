package nvidiasmi

import (
	"strings"
	"testing"
)

func TestCollector_Device_returnsOutOfRangeError_whenIndexIsOutsideParsedDevices(t *testing.T) {
	tests := []struct {
		name  string
		index int
	}{
		{name: "negative index", index: -1},
		{name: "past final device", index: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			runner := &fakeRunner{out: []byte("0, GPU-111, NVIDIA A100, 40960, 1024, 55, 120.50, 400.00, 1410, 1215, 75, 20, P0, 535.129.03, 30, 10, 4, 16\n")}
			collector := newCollectorWithRunner(runner, "nvidia-smi")

			// When
			device, err := collector.Device(tt.index)

			// Then
			if device != nil {
				t.Fatalf("expected nil device, got %#v", device)
			}
			requireError(t, err)
			if !strings.Contains(err.Error(), "out of range") {
				t.Fatalf("expected out of range error, got %q", err.Error())
			}
		})
	}
}
