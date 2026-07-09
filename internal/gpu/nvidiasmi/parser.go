package nvidiasmi

import (
	"encoding/csv"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

const (
	bytesPerMiB   = 1024 * 1024
	milliwattUnit = 1000
)

func parseDevices(out []byte, columns map[string]int) ([]gpu.Device, error) {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}
	reader := csv.NewReader(strings.NewReader(trimmed))
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse nvidia-smi csv: %w", err)
	}
	devices := make([]gpu.Device, 0, len(rows))
	for rowIndex, row := range rows {
		device, err := parseDevice(row, columns)
		if err != nil {
			return nil, fmt.Errorf("parse row %d: %w", rowIndex, err)
		}
		devices = append(devices, device)
	}
	return devices, nil
}

func parseDevice(row []string, columns map[string]int) (gpu.Device, error) {
	index, err := parseRequiredInt(row, columns, fieldIndex)
	if err != nil {
		return gpu.Device{}, err
	}
	uuid, err := requiredValue(row, columns, fieldUUID)
	if err != nil {
		return gpu.Device{}, err
	}
	device := gpu.Device{Index: index, UUID: uuid}
	if err := applyOptionalFields(row, columns, &device); err != nil {
		return gpu.Device{}, err
	}
	return device, nil
}

func applyOptionalFields(row []string, columns map[string]int, device *gpu.Device) error {
	var err error
	if device.Name, err = parseOptionalString(row, columns, fieldName); err != nil {
		return err
	}
	if device.MemoryTotal, err = parseOptionalMemory(row, columns, fieldMemoryTotal); err != nil {
		return err
	}
	if device.MemoryUsed, err = parseOptionalMemory(row, columns, fieldMemoryUsed); err != nil {
		return err
	}
	if device.Temperature, err = parseOptionalUint32(row, columns, fieldTemperature); err != nil {
		return err
	}
	if device.PowerDraw, err = parseOptionalPower(row, columns, fieldPowerDraw); err != nil {
		return err
	}
	if device.PowerLimit, err = parseOptionalPower(row, columns, fieldPowerLimit); err != nil {
		return err
	}
	if device.ClockGraphics, err = parseOptionalUint32(row, columns, fieldClockGR); err != nil {
		return err
	}
	if device.ClockMem, err = parseOptionalUint32(row, columns, fieldClockMem); err != nil {
		return err
	}
	if device.UtilizationGPU, err = parseOptionalUint32(row, columns, fieldUtilGPU); err != nil {
		return err
	}
	if device.UtilizationMem, err = parseOptionalUint32(row, columns, fieldUtilMem); err != nil {
		return err
	}
	if device.PState, err = parseOptionalString(row, columns, fieldPState); err != nil {
		return err
	}
	if device.DriverVersion, err = parseOptionalString(row, columns, fieldDriver); err != nil {
		return err
	}
	if device.EncoderUtil, err = parseOptionalUint32(row, columns, fieldEncoderUtil); err != nil {
		return err
	}
	if device.DecoderUtil, err = parseOptionalUint32(row, columns, fieldDecoderUtil); err != nil {
		return err
	}
	if device.PCIeGen, err = parseOptionalUint32(row, columns, fieldPCIeGen); err != nil {
		return err
	}
	if device.PCIeWidth, err = parseOptionalUint32(row, columns, fieldPCIeWidth); err != nil {
		return err
	}
	return nil
}

func requiredValue(row []string, columns map[string]int, field string) (string, error) {
	value, ok, err := valueForField(row, columns, field)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("missing mandatory field %s", field)
	}
	return clean(value), nil
}

func parseRequiredInt(row []string, columns map[string]int, field string) (int, error) {
	value, err := requiredValue(row, columns, field)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s %q: %w", field, value, err)
	}
	return parsed, nil
}

func parseOptionalString(row []string, columns map[string]int, field string) (string, error) {
	value, ok, err := valueForField(row, columns, field)
	if err != nil || !ok {
		return "", err
	}
	return cleanAvailable(value), nil
}

func parseOptionalMemory(row []string, columns map[string]int, field string) (uint64, error) {
	value, ok, err := valueForField(row, columns, field)
	if err != nil || !ok {
		return 0, err
	}
	return parseMemory(value, field)
}

func parseOptionalUint32(row []string, columns map[string]int, field string) (uint32, error) {
	value, ok, err := valueForField(row, columns, field)
	if err != nil || !ok {
		return 0, err
	}
	return parseUint32(value, field)
}

func parseOptionalPower(row []string, columns map[string]int, field string) (uint32, error) {
	value, ok, err := valueForField(row, columns, field)
	if err != nil || !ok {
		return 0, err
	}
	return parsePower(value, field)
}

func valueForField(row []string, columns map[string]int, field string) (string, bool, error) {
	index, ok := columns[field]
	if !ok {
		return "", false, nil
	}
	if index >= len(row) {
		return "", false, fmt.Errorf("field %s column %d missing from %d-column row", field, index, len(row))
	}
	return row[index], true, nil
}

func parseMemory(value, field string) (uint64, error) {
	normalized := cleanAvailable(value)
	if normalized == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(normalized, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s %q: %w", field, normalized, err)
	}
	return parsed * bytesPerMiB, nil
}

func parseUint32(value, field string) (uint32, error) {
	normalized := cleanAvailable(value)
	if normalized == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(normalized, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s %q: %w", field, normalized, err)
	}
	return uint32(parsed), nil
}

func parsePower(value, field string) (uint32, error) {
	normalized := cleanAvailable(value)
	if normalized == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseFloat(normalized, 64)
	if err != nil {
		return 0, fmt.Errorf("%s %q: %w", field, normalized, err)
	}
	return uint32(math.Round(parsed * milliwattUnit)), nil
}

func cleanAvailable(value string) string {
	normalized := clean(value)
	switch normalized {
	case "", "[N/A]", "[Not Supported]":
		return ""
	default:
		return normalized
	}
}

func clean(value string) string {
	return strings.TrimSpace(value)
}
