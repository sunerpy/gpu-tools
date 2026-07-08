package nvidiasmi

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"

	"github.com/sunerpy/gpu-tools/core"
	"github.com/sunerpy/gpu-tools/internal/gpu"
)

const (
	backendName   = core.BackendNvidiaSMI
	backendPrio   = 20
	expectedCols  = 14
	bytesPerMiB   = 1024 * 1024
	milliwattUnit = 1000
)

var queryArgs = []string{
	"--query-gpu=index,uuid,name,memory.total,memory.used,temperature.gpu,power.draw,power.limit,clocks.gr,clocks.mem,utilization.gpu,utilization.memory,pstate,driver_version",
	"--format=csv,noheader,nounits",
}

type Collector struct {
	runner  execRunner
	smiPath string
}

var _ gpu.Collector = (*Collector)(nil)

func init() {
	gpu.Register(backendName, backendPrio, newCollector)
}

func newCollector() (gpu.Collector, error) {
	path, err := configuredPath()
	if err != nil {
		return nil, err
	}
	return newCollectorWithRunner(osExecRunner{}, path), nil
}

func newCollectorWithRunner(runner execRunner, smiPath string) *Collector {
	return &Collector{runner: runner, smiPath: smiPath}
}

func (c *Collector) Init() error {
	return nil
}

func (c *Collector) Shutdown() error {
	return nil
}

func (c *Collector) DeviceCount() (int, error) {
	devices, err := c.devices()
	if err != nil {
		return 0, err
	}
	return len(devices), nil
}

func (c *Collector) Device(i int) (*gpu.Device, error) {
	devices, err := c.devices()
	if err != nil {
		return nil, err
	}
	if i < 0 || i >= len(devices) {
		return nil, fmt.Errorf("device index %d out of range", i)
	}
	return &devices[i], nil
}

func (c *Collector) Backend() string {
	return backendName
}

func configuredPath() (string, error) {
	configPath, err := core.DefaultConfigPath()
	if err != nil {
		return "", fmt.Errorf("default config path: %w", err)
	}
	config, err := core.Load(configPath)
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	if strings.TrimSpace(config.NvidiaSmiPath) != "" {
		path, err := lookPath(config.NvidiaSmiPath)
		if err != nil {
			return "", fmt.Errorf("%w: %w", gpu.ErrBackendUnavailable, err)
		}
		return path, nil
	}
	path, err := lookPath("nvidia-smi")
	if err != nil {
		return "", fmt.Errorf("%w: %w", gpu.ErrBackendUnavailable, err)
	}
	return path, nil
}

func (c *Collector) devices() ([]gpu.Device, error) {
	out, err := c.runner.Run(context.Background(), c.smiPath, queryArgs...)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("%w: %w", gpu.ErrBackendUnavailable, err)
		}
		return nil, fmt.Errorf("query nvidia-smi: %w", err)
	}
	return parseDevices(out)
}

func parseDevices(out []byte) ([]gpu.Device, error) {
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
		device, err := parseDevice(row)
		if err != nil {
			return nil, fmt.Errorf("parse row %d: %w", rowIndex, err)
		}
		devices = append(devices, device)
	}
	return devices, nil
}

func parseDevice(row []string) (gpu.Device, error) {
	if len(row) != expectedCols {
		return gpu.Device{}, fmt.Errorf("expected %d columns, got %d", expectedCols, len(row))
	}
	index, err := parseRequiredInt(row[0], "index")
	if err != nil {
		return gpu.Device{}, err
	}
	device := gpu.Device{
		Index:         index,
		UUID:          clean(row[1]),
		Name:          clean(row[2]),
		PState:        cleanAvailable(row[12]),
		DriverVersion: cleanAvailable(row[13]),
	}
	if device.MemoryTotal, err = parseMemory(row[3], "memory.total"); err != nil {
		return gpu.Device{}, err
	}
	if device.MemoryUsed, err = parseMemory(row[4], "memory.used"); err != nil {
		return gpu.Device{}, err
	}
	if device.Temperature, err = parseUint32(row[5], "temperature.gpu"); err != nil {
		return gpu.Device{}, err
	}
	if device.PowerDraw, err = parsePower(row[6], "power.draw"); err != nil {
		return gpu.Device{}, err
	}
	if device.PowerLimit, err = parsePower(row[7], "power.limit"); err != nil {
		return gpu.Device{}, err
	}
	if device.ClockGraphics, err = parseUint32(row[8], "clocks.gr"); err != nil {
		return gpu.Device{}, err
	}
	if device.ClockMem, err = parseUint32(row[9], "clocks.mem"); err != nil {
		return gpu.Device{}, err
	}
	if device.UtilizationGPU, err = parseUint32(row[10], "utilization.gpu"); err != nil {
		return gpu.Device{}, err
	}
	if device.UtilizationMem, err = parseUint32(row[11], "utilization.memory"); err != nil {
		return gpu.Device{}, err
	}
	return device, nil
}

func parseRequiredInt(value, field string) (int, error) {
	normalized := clean(value)
	parsed, err := strconv.Atoi(normalized)
	if err != nil {
		return 0, fmt.Errorf("%s %q: %w", field, normalized, err)
	}
	return parsed, nil
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
