package amd

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

const (
	backendName = "amd"
	backendPrio = 30
)

var queryArgs = []string{
	"--showid",
	"--showproductname",
	"--showuse",
	"--showmemuse",
	"--showtemp",
	"--showpower",
	"--json",
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
	path, err := lookPath("rocm-smi")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", gpu.ErrBackendUnavailable, err)
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

func (c *Collector) devices() ([]gpu.Device, error) {
	out, err := c.runner.Run(context.Background(), c.smiPath, queryArgs...)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("%w: %w", gpu.ErrBackendUnavailable, err)
		}
		return nil, fmt.Errorf("query rocm-smi: %w", err)
	}
	return parseDevices(out)
}
