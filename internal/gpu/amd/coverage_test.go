package amd

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

func TestNewCollector_usesLookPathResult_whenRocmSMIExists(t *testing.T) {
	// Given
	originalLookPath := lookPath
	lookPath = func(name string) (string, error) {
		if name != "rocm-smi" {
			t.Fatalf("expected rocm-smi lookup, got %q", name)
		}
		return "/opt/rocm/bin/rocm-smi", nil
	}
	t.Cleanup(func() { lookPath = originalLookPath })

	// When
	collector, err := newCollector()

	// Then
	requireNoError(t, err)
	typed, ok := collector.(*Collector)
	if !ok {
		t.Fatalf("expected *Collector, got %T", collector)
	}
	if typed.smiPath != "/opt/rocm/bin/rocm-smi" {
		t.Fatalf("expected smi path override, got %q", typed.smiPath)
	}
}

func TestCollector_Device_returnsRunnerError_whenQueryFailsForNonAvailabilityReason(t *testing.T) {
	// Given
	runner := &fakeRunner{err: errors.New("rocm-smi failed")}
	collector := newCollectorWithRunner(runner, "rocm-smi")

	// When
	device, err := collector.Device(0)

	// Then
	if device != nil {
		t.Fatalf("expected nil device, got %#v", device)
	}
	requireError(t, err)
	if errors.Is(err, gpu.ErrBackendUnavailable) {
		t.Fatalf("expected non-availability error, got %v", err)
	}
}

func TestCollector_Device_returnsError_whenIndexOutOfRange(t *testing.T) {
	tests := []struct {
		name  string
		index int
	}{
		{name: "negative index", index: -1},
		{name: "past final device", index: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			runner := &fakeRunner{out: []byte(`{"card0":{}}`)}
			collector := newCollectorWithRunner(runner, "rocm-smi")

			// When
			device, err := collector.Device(tt.index)

			// Then
			if device != nil {
				t.Fatalf("expected nil device, got %#v", device)
			}
			requireError(t, err)
		})
	}
}

func TestParseDevices_returnsNil_whenOutputIsEmpty(t *testing.T) {
	// Given
	out := []byte(" \n\t ")

	// When
	devices, err := parseDevices(out)

	// Then
	requireNoError(t, err)
	if devices != nil {
		t.Fatalf("expected nil devices, got %#v", devices)
	}
}

func TestParseDevices_returnsError_whenCardKeyIsInvalid(t *testing.T) {
	// Given
	out := []byte(`{"gpu0":{}}`)

	// When
	devices, err := parseDevices(out)

	// Then
	if devices != nil {
		t.Fatalf("expected nil devices, got %#v", devices)
	}
	requireError(t, err)
}

func TestParseDevice_leavesMissingKeysZero_whenOnlyCardIndexExists(t *testing.T) {
	// Given
	card := rocmCard{}

	// When
	device, err := parseDevice("card7", card)

	// Then
	requireNoError(t, err)
	requireDevice(t, &device, gpu.Device{Index: 7})
}

func TestParseDevice_acceptsAlternateFieldNamesAndJSONNumbers(t *testing.T) {
	// Given
	card := rocmCard{
		"Device Name":          []byte(`"AMD Test GPU"`),
		"GPU Utilization (%)":  []byte(`12.6`),
		"Memory use (%)":       []byte(`"77.4 %"`),
		"Memory Total (B)":     []byte(`1024`),
		"Memory Used (MiB)":    []byte(`"2"`),
		"Edge Temperature (C)": []byte(`"50.5 C"`),
		"Power (W)":            []byte(`100.5`),
	}

	// When
	device, err := parseDevice("CARD2", card)

	// Then
	requireNoError(t, err)
	requireDevice(t, &device, gpu.Device{
		Index:          2,
		Name:           "AMD Test GPU",
		MemoryTotal:    1024,
		MemoryUsed:     2 * 1024 * 1024,
		Temperature:    51,
		PowerDraw:      100500,
		UtilizationGPU: 13,
		UtilizationMem: 77,
	})
}

func TestParseDevice_returnsError_whenMappedNumericFieldIsMalformed(t *testing.T) {
	tests := []struct {
		name string
		card rocmCard
	}{
		{name: "gpu utilization", card: rocmCard{"GPU use (%)": []byte(`"busy"`)}},
		{name: "memory utilization", card: rocmCard{"GPU memory use (%)": []byte(`"full"`)}},
		{name: "memory total", card: rocmCard{"VRAM Total Memory (MiB)": []byte(`"large"`)}},
		{name: "memory used", card: rocmCard{"VRAM Total Used Memory (MiB)": []byte(`"some"`)}},
		{name: "temperature", card: rocmCard{"Temperature (Sensor edge) (C)": []byte(`"hot"`)}},
		{name: "power", card: rocmCard{"Average Graphics Package Power (W)": []byte(`"many"`)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			_, err := parseDevice("card0", tt.card)

			// Then
			requireError(t, err)
		})
	}
}

func TestParseCardIndex_returnsError_whenSuffixIsMissingOrNonNumeric(t *testing.T) {
	tests := []string{"card", "cardx", "gpu0"}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			// When
			_, err := parseCardIndex(input)

			// Then
			requireError(t, err)
		})
	}
}

func TestReadHelpers_returnZero_whenKeysAreAbsentOrUnavailable(t *testing.T) {
	// Given
	card := rocmCard{"missing": []byte(`"[N/A]"`)}

	// When
	rounded, roundedErr := readRoundedUint32(card, []string{"absent"})
	memory, memoryErr := readMemory(card, []string{"missing"})
	power, powerErr := readPower(card, []string{"absent"})
	value, key, valueErr := readValue(card, []string{"absent"})

	// Then
	requireNoError(t, roundedErr)
	requireNoError(t, memoryErr)
	requireNoError(t, powerErr)
	requireNoError(t, valueErr)
	if rounded != 0 || memory != 0 || power != 0 || value != "" || key != "" {
		t.Fatalf("expected zero absent values, got rounded=%d memory=%d power=%d value=%q key=%q", rounded, memory, power, value, key)
	}
}

func TestReadValueAndReadString_handleUnsupportedJSONValues(t *testing.T) {
	// Given
	card := rocmCard{"bad": []byte(`true`)}

	// When
	text := readString(card, []string{"bad"})
	value, key, err := readValue(card, []string{"bad"})

	// Then
	if text != "" {
		t.Fatalf("expected readString to hide malformed value, got %q", text)
	}
	if value != "" || key != "bad" {
		t.Fatalf("expected failed key bad with empty value, got key=%q value=%q", key, value)
	}
	requireError(t, err)
}

func TestRawToString_convertsSupportedJSONScalars(t *testing.T) {
	tests := []struct {
		name    string
		raw     []byte
		want    string
		wantErr bool
	}{
		{name: "string", raw: []byte(`"42 MiB"`), want: "42 MiB"},
		{name: "number", raw: []byte(`42.5`), want: "42.5"},
		{name: "null", raw: []byte(`null`), want: ""},
		{name: "bool", raw: []byte(`false`), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got, err := rawToString(tt.raw)

			// Then
			if tt.wantErr {
				requireError(t, err)
				return
			}
			requireNoError(t, err)
			if got != tt.want {
				t.Fatalf("rawToString = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseNumeric_readsLeadingNumberAndRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    float64
		wantErr bool
	}{
		{name: "comma and suffix", input: "1,024 MiB", want: 1024},
		{name: "scientific", input: "-1.5e2W", want: -150},
		{name: "empty", input: " ", wantErr: true},
		{name: "no leading number", input: "watts", wantErr: true},
		{name: "invalid leading sign", input: "+watts", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got, err := parseNumeric(tt.input)

			// Then
			if tt.wantErr {
				requireError(t, err)
				return
			}
			requireNoError(t, err)
			if got != tt.want {
				t.Fatalf("parseNumeric = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemoryMultiplier_coversBinaryAndDecimalUnitBranches(t *testing.T) {
	tests := []struct {
		name  string
		field string
		value string
		want  float64
	}{
		{name: "gib field", field: "VRAM Total Memory (GiB)", want: bytesPerGiB},
		{name: "mib field", field: "VRAM Total Memory (MiB)", want: bytesPerMiB},
		{name: "kib field", field: "VRAM Total Memory (KiB)", want: bytesPerKiB},
		{name: "gb value", value: "1 GB", want: bytesPerGiB},
		{name: "mb value", value: "1 MB", want: bytesPerMiB},
		{name: "kb value", value: "1 KB", want: bytesPerKiB},
		{name: "bytes default", field: "VRAM Total Memory (B)", want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got := memoryMultiplier(tt.field, tt.value)

			// Then
			if got != tt.want {
				t.Fatalf("memoryMultiplier = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCleanAvailable_normalizesUnavailableSentinels(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "", want: ""},
		{input: " N/A ", want: ""},
		{input: "[N/A]", want: ""},
		{input: "Not Supported", want: ""},
		{input: "[Not Supported]", want: ""},
		{input: "42", want: "42"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			// When
			got := cleanAvailable(tt.input)

			// Then
			if got != tt.want {
				t.Fatalf("cleanAvailable = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOsExecRunner_Run_returnsCommandOutput_whenCommandSucceeds(t *testing.T) {
	// Given
	runner := osExecRunner{}

	// When
	out, err := runner.Run(context.Background(), "echo", "amd-runner-ok")

	// Then
	requireNoError(t, err)
	if strings.TrimSpace(string(out)) != "amd-runner-ok" {
		t.Fatalf("expected echo output, got %q", string(out))
	}
}

func TestOsExecRunner_Run_returnsError_whenCommandIsMissing(t *testing.T) {
	// Given
	runner := osExecRunner{}

	// When
	out, err := runner.Run(context.Background(), "gpu-tools-missing-amd-test-binary")

	// Then
	if out != nil {
		t.Fatalf("expected nil output, got %q", string(out))
	}
	requireError(t, err)
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("expected exec.ErrNotFound in chain, got %v", err)
	}
}
