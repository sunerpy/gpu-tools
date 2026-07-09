package nvidiasmi

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sunerpy/gpu-tools/core"
	"github.com/sunerpy/gpu-tools/internal/gpu"
)

const (
	backendName = core.BackendNvidiaSMI
	backendPrio = 20
)

type Collector struct {
	runner          execRunner
	smiPath         string
	supportedFields map[string]bool
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
	fields, err := c.queryFields()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("%w: %w", gpu.ErrBackendUnavailable, err)
		}
		return nil, fmt.Errorf("discover nvidia-smi fields: %w", err)
	}
	out, err := c.runner.Run(context.Background(), c.smiPath, queryArgs(fields)...)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("%w: %w", gpu.ErrBackendUnavailable, err)
		}
		return nil, fmt.Errorf("query nvidia-smi: %w", err)
	}
	devices, err := parseDevices(out, fieldIndexes(fields))
	if err != nil {
		return nil, err
	}
	c.attachProcesses(devices)
	return devices, nil
}
