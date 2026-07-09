package nvidiasmi

import (
	"reflect"
	"testing"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

// reducedFieldHelpNoEncodePCIe lists every field EXCEPT the T5 additions
// (utilization.encoder, utilization.decoder, pcie.link.gen.current,
// pcie.link.width.current), simulating an older driver whose
// --help-query-gpu omits them.
func reducedFieldHelpNoEncodePCIe() []byte {
	var out []byte
	for _, field := range wantedFields {
		switch field {
		case fieldEncoderUtil, fieldDecoderUtil, fieldPCIeGen, fieldPCIeWidth:
			continue
		default:
			out = append(out, '"')
			out = append(out, field...)
			out = append(out, "\" - supported field.\n"...)
		}
	}
	return out
}

func TestCollector_Device_mapsEncoderDecoderPCIe_whenHelpListsThem(t *testing.T) {
	// Given: help lists all wanted fields (fullFieldHelp includes T5 fields),
	// and the CSV supplies encoder/decoder/pcie values in the trailing columns.
	runner := &fakeRunner{
		out:        []byte("0, GPU-111, NVIDIA A100, 40960, 1024, 55, 120.50, 400.00, 1410, 1215, 75, 20, P0, 535.129.03, 33, 12, 4, 16\n"),
		computeErr: errUnsupportedComputeApps,
	}
	collector := newCollectorWithRunner(runner, "nvidia-smi")

	// When
	device, err := collector.Device(0)

	// Then
	requireNoError(t, err)
	requireDevice(t, device, gpu.Device{
		Index:          0,
		UUID:           "GPU-111",
		Name:           "NVIDIA A100",
		MemoryTotal:    40960 * bytesPerMiB,
		MemoryUsed:     1024 * bytesPerMiB,
		Temperature:    55,
		PowerDraw:      120500,
		PowerLimit:     400000,
		ClockGraphics:  1410,
		ClockMem:       1215,
		UtilizationGPU: 75,
		UtilizationMem: 20,
		PState:         "P0",
		DriverVersion:  "535.129.03",
		EncoderUtil:    33,
		DecoderUtil:    12,
		PCIeGen:        4,
		PCIeWidth:      16,
	})
}

func TestCollector_Device_leavesEncoderDecoderPCIeZero_whenHelpOmitsThem(t *testing.T) {
	// Given: an older driver whose help omits the T5 fields entirely. The query
	// must not request them, and the CSV only carries the 14 legacy columns.
	runner := &fakeRunner{
		helpOut:    reducedFieldHelpNoEncodePCIe(),
		queryOut:   []byte("0, GPU-111, NVIDIA A100, 40960, 1024, 55, 120.50, 400.00, 1410, 1215, 75, 20, P0, 535.129.03\n"),
		computeErr: errUnsupportedComputeApps,
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
		fieldClockGR,
		fieldClockMem,
		fieldUtilGPU,
		fieldUtilMem,
		fieldPState,
		fieldDriver,
	}
	if !reflect.DeepEqual(runner.gpuQueryArgs, queryArgs(expectedFields)) {
		t.Fatalf("expected gpu query args %#v, got %#v", queryArgs(expectedFields), runner.gpuQueryArgs)
	}
	requireDevice(t, device, gpu.Device{
		Index:          0,
		UUID:           "GPU-111",
		Name:           "NVIDIA A100",
		MemoryTotal:    40960 * bytesPerMiB,
		MemoryUsed:     1024 * bytesPerMiB,
		Temperature:    55,
		PowerDraw:      120500,
		PowerLimit:     400000,
		ClockGraphics:  1410,
		ClockMem:       1215,
		UtilizationGPU: 75,
		UtilizationMem: 20,
		PState:         "P0",
		DriverVersion:  "535.129.03",
	})
}

func TestApplyOptionalFields_populatesEncoderDecoderPCIe_whenColumnsPresent(t *testing.T) {
	// Given
	row := []string{
		"0", "GPU-abc", "NVIDIA A100",
		"40960", "1024", "55",
		"120.50", "400.00", "1410", "1215",
		"75", "20", "P0", "535.129.03",
		"33", "12", "4", "16",
	}
	columns := fullColumns()
	columns[fieldEncoderUtil] = 14
	columns[fieldDecoderUtil] = 15
	columns[fieldPCIeGen] = 16
	columns[fieldPCIeWidth] = 17
	device := gpu.Device{Index: 0, UUID: "GPU-abc"}

	// When
	err := applyOptionalFields(row, columns, &device)

	// Then
	requireNoError(t, err)
	if device.EncoderUtil != 33 || device.DecoderUtil != 12 {
		t.Fatalf("expected encoder 33 decoder 12, got %d/%d", device.EncoderUtil, device.DecoderUtil)
	}
	if device.PCIeGen != 4 || device.PCIeWidth != 16 {
		t.Fatalf("expected pcie gen 4 width 16, got %d/%d", device.PCIeGen, device.PCIeWidth)
	}
}
