package amd

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

const (
	bytesPerKiB   = 1024
	bytesPerMiB   = 1024 * bytesPerKiB
	bytesPerGiB   = 1024 * bytesPerMiB
	milliwattUnit = 1000
)

var (
	nameKeys = []string{
		"Card series",
		"Card Series",
		"Product Name",
		"Card SKU",
		"Card Model",
		"GPU model",
		"Device Name",
	}
	gpuUseKeys = []string{
		"GPU use (%)",
		"GPU Use (%)",
		"GPU use %",
		"GPU Utilization (%)",
	}
	memUsePercentKeys = []string{
		"GPU memory use (%)",
		"GPU Memory use (%)",
		"GPU memory use %",
		"Memory use (%)",
	}
	memoryTotalKeys = []string{
		"VRAM Total Memory (B)",
		"VRAM Total Memory (KiB)",
		"VRAM Total Memory (MiB)",
		"VRAM Total Memory (GiB)",
		"GPU memory total (B)",
		"GPU memory total (MiB)",
		"Memory Total (B)",
		"Memory Total (MiB)",
	}
	memoryUsedKeys = []string{
		"VRAM Total Used Memory (B)",
		"VRAM Total Used Memory (KiB)",
		"VRAM Total Used Memory (MiB)",
		"VRAM Total Used Memory (GiB)",
		"GPU memory used (B)",
		"GPU memory used (MiB)",
		"Memory Used (B)",
		"Memory Used (MiB)",
	}
	temperatureKeys = []string{
		"Temperature (Sensor edge) (C)",
		"Temperature (Sensor Edge) (C)",
		"Temperature (edge) (C)",
		"Edge Temperature (C)",
	}
	powerKeys = []string{
		"Average Graphics Package Power (W)",
		"Average Socket Power (W)",
		"Current Socket Graphics Package Power (W)",
		"Power (W)",
	}
)

type rocmCard map[string]json.RawMessage

func parseDevices(out []byte) ([]gpu.Device, error) {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}
	cards := map[string]rocmCard{}
	if err := json.Unmarshal([]byte(trimmed), &cards); err != nil {
		return nil, fmt.Errorf("parse rocm-smi json: %w", err)
	}
	devices := make([]gpu.Device, 0, len(cards))
	for key, card := range cards {
		device, err := parseDevice(key, card)
		if err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Index < devices[j].Index
	})
	return devices, nil
}

func parseDevice(cardKey string, card rocmCard) (gpu.Device, error) {
	index, err := parseCardIndex(cardKey)
	if err != nil {
		return gpu.Device{}, err
	}
	device := gpu.Device{
		Index: index,
		Name:  readString(card, nameKeys),
	}
	if device.UtilizationGPU, err = readRoundedUint32(card, gpuUseKeys); err != nil {
		return gpu.Device{}, err
	}
	if device.UtilizationMem, err = readRoundedUint32(card, memUsePercentKeys); err != nil {
		return gpu.Device{}, err
	}
	if device.MemoryTotal, err = readMemory(card, memoryTotalKeys); err != nil {
		return gpu.Device{}, err
	}
	if device.MemoryUsed, err = readMemory(card, memoryUsedKeys); err != nil {
		return gpu.Device{}, err
	}
	if device.Temperature, err = readRoundedUint32(card, temperatureKeys); err != nil {
		return gpu.Device{}, err
	}
	if device.PowerDraw, err = readPower(card, powerKeys); err != nil {
		return gpu.Device{}, err
	}
	return device, nil
}

func parseCardIndex(cardKey string) (int, error) {
	indexText, ok := strings.CutPrefix(strings.ToLower(strings.TrimSpace(cardKey)), "card")
	if !ok || indexText == "" {
		return 0, fmt.Errorf("invalid rocm-smi card key %q", cardKey)
	}
	index, err := strconv.Atoi(indexText)
	if err != nil {
		return 0, fmt.Errorf("invalid rocm-smi card key %q: %w", cardKey, err)
	}
	return index, nil
}

func readString(card rocmCard, keys []string) string {
	value, _, err := readValue(card, keys)
	if err != nil {
		return ""
	}
	return value
}

func readRoundedUint32(card rocmCard, keys []string) (uint32, error) {
	value, key, err := readValue(card, keys)
	if err != nil || value == "" {
		return 0, err
	}
	parsed, err := parseNumeric(value)
	if err != nil {
		return 0, fmt.Errorf("%s %q: %w", key, value, err)
	}
	return uint32(math.Round(parsed)), nil
}

func readMemory(card rocmCard, keys []string) (uint64, error) {
	value, key, err := readValue(card, keys)
	if err != nil || value == "" {
		return 0, err
	}
	parsed, err := parseNumeric(value)
	if err != nil {
		return 0, fmt.Errorf("%s %q: %w", key, value, err)
	}
	return uint64(math.Round(parsed * memoryMultiplier(key, value))), nil
}

func readPower(card rocmCard, keys []string) (uint32, error) {
	value, key, err := readValue(card, keys)
	if err != nil || value == "" {
		return 0, err
	}
	parsed, err := parseNumeric(value)
	if err != nil {
		return 0, fmt.Errorf("%s %q: %w", key, value, err)
	}
	return uint32(math.Round(parsed * milliwattUnit)), nil
}

func readValue(card rocmCard, keys []string) (string, string, error) {
	for _, key := range keys {
		raw, ok := card[key]
		if !ok {
			continue
		}
		value, err := rawToString(raw)
		if err != nil {
			return "", key, fmt.Errorf("%s: %w", key, err)
		}
		return cleanAvailable(value), key, nil
	}
	return "", "", nil
}

func rawToString(raw json.RawMessage) (string, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		return strconv.FormatFloat(number, 'f', -1, 64), nil
	}
	if string(raw) == "null" {
		return "", nil
	}
	return "", fmt.Errorf("unsupported json value %s", string(raw))
}

func parseNumeric(value string) (float64, error) {
	trimmed := strings.TrimSpace(strings.ReplaceAll(value, ",", ""))
	if trimmed == "" {
		return 0, strconv.ErrSyntax
	}
	end := 0
	for _, r := range trimmed {
		if !isNumberRune(r) {
			break
		}
		end += len(string(r))
	}
	if end == 0 {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseFloat(trimmed[:end], 64)
}

func isNumberRune(r rune) bool {
	return unicode.IsDigit(r) || r == '.' || r == '-' || r == '+' || r == 'e' || r == 'E'
}

func memoryMultiplier(field, value string) float64 {
	unitSource := strings.ToLower(field + " " + value)
	switch {
	case strings.Contains(unitSource, "gib"):
		return bytesPerGiB
	case strings.Contains(unitSource, "mib"):
		return bytesPerMiB
	case strings.Contains(unitSource, "kib"):
		return bytesPerKiB
	case strings.Contains(unitSource, "gb"):
		return bytesPerGiB
	case strings.Contains(unitSource, "mb"):
		return bytesPerMiB
	case strings.Contains(unitSource, "kb"):
		return bytesPerKiB
	default:
		return 1
	}
}

func cleanAvailable(value string) string {
	normalized := strings.TrimSpace(value)
	switch normalized {
	case "", "N/A", "[N/A]", "Not Supported", "[Not Supported]":
		return ""
	default:
		return normalized
	}
}
