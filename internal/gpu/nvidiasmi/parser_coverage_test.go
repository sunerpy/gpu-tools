package nvidiasmi

import (
	"reflect"
	"testing"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

func TestRequiredValue_returnsValueOrError_whenFieldPresentOrMissing(t *testing.T) {
	tests := []struct {
		name    string
		row     []string
		columns map[string]int
		field   string
		want    string
		wantErr bool
	}{
		{
			name:    "present field trimmed",
			row:     []string{"0", "  GPU-abc  "},
			columns: map[string]int{fieldIndex: 0, fieldUUID: 1},
			field:   fieldUUID,
			want:    "GPU-abc",
		},
		{
			name:    "missing field returns error",
			row:     []string{"0"},
			columns: map[string]int{fieldIndex: 0},
			field:   fieldUUID,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given / When
			got, err := requiredValue(tt.row, tt.columns, tt.field)

			// Then
			if tt.wantErr {
				requireError(t, err)
				return
			}
			requireNoError(t, err)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestParseRequiredInt_returnsIntOrError_whenValueValidEmptyOrNonNumeric(t *testing.T) {
	tests := []struct {
		name    string
		row     []string
		columns map[string]int
		field   string
		want    int
		wantErr bool
	}{
		{
			name:    "valid integer",
			row:     []string{"7"},
			columns: map[string]int{fieldIndex: 0},
			field:   fieldIndex,
			want:    7,
		},
		{
			name:    "empty string is non numeric",
			row:     []string{""},
			columns: map[string]int{fieldIndex: 0},
			field:   fieldIndex,
			wantErr: true,
		},
		{
			name:    "non numeric value",
			row:     []string{"abc"},
			columns: map[string]int{fieldIndex: 0},
			field:   fieldIndex,
			wantErr: true,
		},
		{
			name:    "missing field propagates required error",
			row:     []string{},
			columns: map[string]int{},
			field:   fieldIndex,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given / When
			got, err := parseRequiredInt(tt.row, tt.columns, tt.field)

			// Then
			if tt.wantErr {
				requireError(t, err)
				return
			}
			requireNoError(t, err)
			if got != tt.want {
				t.Fatalf("expected %d, got %d", tt.want, got)
			}
		})
	}
}

func TestParseOptionalString_returnsCleanedValue_whenValuePresentUnavailableOrMissing(t *testing.T) {
	tests := []struct {
		name    string
		row     []string
		columns map[string]int
		field   string
		want    string
		wantErr bool
	}{
		{
			name:    "normal value trimmed",
			row:     []string{"  NVIDIA A100  "},
			columns: map[string]int{fieldName: 0},
			field:   fieldName,
			want:    "NVIDIA A100",
		},
		{
			name:    "not available token becomes empty",
			row:     []string{"[N/A]"},
			columns: map[string]int{fieldName: 0},
			field:   fieldName,
			want:    "",
		},
		{
			name:    "not supported token becomes empty",
			row:     []string{"[Not Supported]"},
			columns: map[string]int{fieldName: 0},
			field:   fieldName,
			want:    "",
		},
		{
			name:    "empty string stays empty",
			row:     []string{""},
			columns: map[string]int{fieldName: 0},
			field:   fieldName,
			want:    "",
		},
		{
			name:    "field absent from columns returns empty",
			row:     []string{"NVIDIA A100"},
			columns: map[string]int{},
			field:   fieldName,
			want:    "",
		},
		{
			name:    "column index beyond row returns error",
			row:     []string{"only-one"},
			columns: map[string]int{fieldName: 5},
			field:   fieldName,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given / When
			got, err := parseOptionalString(tt.row, tt.columns, tt.field)

			// Then
			if tt.wantErr {
				requireError(t, err)
				return
			}
			requireNoError(t, err)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestParseOptionalPower_returnsMilliwattsOrError_whenValueValidUnavailableOrMalformed(t *testing.T) {
	tests := []struct {
		name    string
		row     []string
		columns map[string]int
		field   string
		want    uint32
		wantErr bool
	}{
		{
			name:    "valid float converts to milliwatts",
			row:     []string{"120.50"},
			columns: map[string]int{fieldPowerDraw: 0},
			field:   fieldPowerDraw,
			want:    120500,
		},
		{
			name:    "not available becomes zero",
			row:     []string{"[N/A]"},
			columns: map[string]int{fieldPowerDraw: 0},
			field:   fieldPowerDraw,
			want:    0,
		},
		{
			name:    "empty value becomes zero",
			row:     []string{""},
			columns: map[string]int{fieldPowerDraw: 0},
			field:   fieldPowerDraw,
			want:    0,
		},
		{
			name:    "field absent returns zero",
			row:     []string{"120.50"},
			columns: map[string]int{},
			field:   fieldPowerDraw,
			want:    0,
		},
		{
			name:    "malformed float returns error",
			row:     []string{"not-a-float"},
			columns: map[string]int{fieldPowerDraw: 0},
			field:   fieldPowerDraw,
			wantErr: true,
		},
		{
			name:    "column index beyond row returns error",
			row:     []string{"only-one"},
			columns: map[string]int{fieldPowerDraw: 3},
			field:   fieldPowerDraw,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given / When
			got, err := parseOptionalPower(tt.row, tt.columns, tt.field)

			// Then
			if tt.wantErr {
				requireError(t, err)
				return
			}
			requireNoError(t, err)
			if got != tt.want {
				t.Fatalf("expected %d, got %d", tt.want, got)
			}
		})
	}
}

func fullColumns() map[string]int {
	return map[string]int{
		fieldIndex:       0,
		fieldUUID:        1,
		fieldName:        2,
		fieldMemoryTotal: 3,
		fieldMemoryUsed:  4,
		fieldTemperature: 5,
		fieldPowerDraw:   6,
		fieldPowerLimit:  7,
		fieldClockGR:     8,
		fieldClockMem:    9,
		fieldUtilGPU:     10,
		fieldUtilMem:     11,
		fieldPState:      12,
		fieldDriver:      13,
	}
}

func TestApplyOptionalFields_populatesEveryField_whenRowHasAllValues(t *testing.T) {
	// Given
	row := []string{
		"0", "GPU-abc", "NVIDIA A100",
		"40960", "1024", "55",
		"120.50", "400.00", "1410", "1215",
		"75", "20", "P0", "535.129.03",
	}
	device := gpu.Device{Index: 0, UUID: "GPU-abc"}

	// When
	err := applyOptionalFields(row, fullColumns(), &device)

	// Then
	requireNoError(t, err)
	want := gpu.Device{
		Index:          0,
		UUID:           "GPU-abc",
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
	}
	if !reflect.DeepEqual(device, want) {
		t.Fatalf("unexpected device\nwant: %#v\n got: %#v", want, device)
	}
}

func TestApplyOptionalFields_leavesZeroValues_whenEveryOptionalFieldAbsent(t *testing.T) {
	// Given
	row := []string{"0", "GPU-abc"}
	columns := map[string]int{fieldIndex: 0, fieldUUID: 1}
	device := gpu.Device{Index: 0, UUID: "GPU-abc"}

	// When
	err := applyOptionalFields(row, columns, &device)

	// Then
	requireNoError(t, err)
	want := gpu.Device{Index: 0, UUID: "GPU-abc"}
	if !reflect.DeepEqual(device, want) {
		t.Fatalf("unexpected device\nwant: %#v\n got: %#v", want, device)
	}
}

func TestApplyOptionalFields_returnsError_whenAnOptionalFieldIsMalformed(t *testing.T) {
	tests := []struct {
		name   string
		field  string
		column int
	}{
		{name: "name column beyond row", field: fieldName, column: 9},
		{name: "memory total column beyond row", field: fieldMemoryTotal, column: 9},
		{name: "memory used column beyond row", field: fieldMemoryUsed, column: 9},
		{name: "temperature column beyond row", field: fieldTemperature, column: 9},
		{name: "power draw column beyond row", field: fieldPowerDraw, column: 9},
		{name: "power limit column beyond row", field: fieldPowerLimit, column: 9},
		{name: "graphics clock column beyond row", field: fieldClockGR, column: 9},
		{name: "memory clock column beyond row", field: fieldClockMem, column: 9},
		{name: "gpu util column beyond row", field: fieldUtilGPU, column: 9},
		{name: "memory util column beyond row", field: fieldUtilMem, column: 9},
		{name: "pstate column beyond row", field: fieldPState, column: 9},
		{name: "driver column beyond row", field: fieldDriver, column: 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			row := []string{"0", "GPU-abc"}
			columns := map[string]int{fieldIndex: 0, fieldUUID: 1, tt.field: tt.column}
			device := gpu.Device{Index: 0, UUID: "GPU-abc"}

			// When
			err := applyOptionalFields(row, columns, &device)

			// Then
			requireError(t, err)
		})
	}
}

func TestParseDevice_returnsPopulatedDevice_whenRowIsComplete(t *testing.T) {
	// Given
	row := []string{
		"3", "GPU-happy", "NVIDIA L40S",
		"46068", "2048", "60",
		"80.25", "350.00", "1800", "9001",
		"42", "11", "P2", "535.129.03",
	}

	// When
	device, err := parseDevice(row, fullColumns())

	// Then
	requireNoError(t, err)
	want := gpu.Device{
		Index:          3,
		UUID:           "GPU-happy",
		Name:           "NVIDIA L40S",
		MemoryTotal:    46068 * bytesPerMiB,
		MemoryUsed:     2048 * bytesPerMiB,
		Temperature:    60,
		PowerDraw:      80250,
		PowerLimit:     350000,
		ClockGraphics:  1800,
		ClockMem:       9001,
		UtilizationGPU: 42,
		UtilizationMem: 11,
		PState:         "P2",
		DriverVersion:  "535.129.03",
	}
	if !reflect.DeepEqual(device, want) {
		t.Fatalf("unexpected device\nwant: %#v\n got: %#v", want, device)
	}
}

func TestParseDevice_returnsError_whenRequiredOrOptionalFieldMalformed(t *testing.T) {
	tests := []struct {
		name    string
		row     []string
		columns map[string]int
	}{
		{
			name:    "malformed required index",
			row:     []string{"not-int", "GPU-abc"},
			columns: map[string]int{fieldIndex: 0, fieldUUID: 1},
		},
		{
			name:    "missing required uuid",
			row:     []string{"0"},
			columns: map[string]int{fieldIndex: 0, fieldUUID: 5},
		},
		{
			name:    "malformed optional field",
			row:     []string{"0", "GPU-abc", "bad-mem"},
			columns: map[string]int{fieldIndex: 0, fieldUUID: 1, fieldMemoryTotal: 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given / When
			device, err := parseDevice(tt.row, tt.columns)

			// Then
			requireError(t, err)
			if !reflect.DeepEqual(device, gpu.Device{}) {
				t.Fatalf("expected zero device, got %#v", device)
			}
		})
	}
}

func TestParseHelpField_extractsFieldName_whenLineMatchesOrNot(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantName string
		wantOK   bool
	}{
		{
			name:     "quoted field with description",
			line:     `"memory.total" - Total installed GPU memory.`,
			wantName: "memory.total",
			wantOK:   true,
		},
		{
			name:     "leading whitespace quoted field",
			line:     `    "power.draw" - Power draw.`,
			wantName: "power.draw",
			wantOK:   true,
		},
		{
			name:   "blank line not matched",
			line:   "   ",
			wantOK: false,
		},
		{
			name:   "line without leading quote not matched",
			line:   "index - GPU index without quotes",
			wantOK: false,
		},
		{
			name:   "line with only opening quote not matched",
			line:   `"unterminated field`,
			wantOK: false,
		},
		{
			name:   "empty quoted field not matched",
			line:   `"" - empty name`,
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given / When
			got, ok := parseHelpField(tt.line)

			// Then
			if ok != tt.wantOK {
				t.Fatalf("expected ok=%t, got %t", tt.wantOK, ok)
			}
			if ok && got != tt.wantName {
				t.Fatalf("expected field %q, got %q", tt.wantName, got)
			}
		})
	}
}
