package nvml

import (
	"fmt"

	"github.com/sunerpy/gpu-tools/internal/gpu"
	"github.com/sunerpy/gpu-tools/internal/gpu/procinfo"
)

const (
	backendName = "nvml"
	backendPrio = 10
)

var resolveProcess = procinfo.Resolve

type Collector struct {
	lib nvmlLib
}

var _ gpu.Collector = (*Collector)(nil)

func init() {
	gpu.Register(backendName, backendPrio, newCollector)
}

func newCollector() (gpu.Collector, error) {
	lib, err := newPuregoLib()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", gpu.ErrBackendUnavailable, err)
	}
	return newCollectorWithLib(lib), nil
}

func newCollectorWithLib(lib nvmlLib) *Collector {
	return &Collector{lib: lib}
}

func (c *Collector) Init() error {
	return c.lib.Init()
}

func (c *Collector) Shutdown() error {
	return c.lib.Shutdown()
}

func (c *Collector) DeviceCount() (int, error) {
	count, err := c.lib.DeviceCount()
	if err != nil {
		return 0, fmt.Errorf("nvml device count: %w", err)
	}
	return count, nil
}

func (c *Collector) Device(i int) (*gpu.Device, error) {
	handle, err := c.lib.DeviceHandleByIndex(i)
	if err != nil {
		return nil, fmt.Errorf("nvml device handle index %d: %w", i, err)
	}
	device := &gpu.Device{Index: i}
	if device.Name, err = c.lib.DeviceName(handle); err != nil {
		return nil, fmt.Errorf("nvml device name index %d: %w", i, err)
	}
	if device.MemoryTotal, device.MemoryUsed, err = c.lib.DeviceMemory(handle); err != nil {
		return nil, fmt.Errorf("nvml device memory index %d: %w", i, err)
	}
	if device.Temperature, err = c.lib.DeviceTemperature(handle); err != nil {
		return nil, fmt.Errorf("nvml device temperature index %d: %w", i, err)
	}
	if device.PowerDraw, err = c.lib.DevicePowerUsage(handle); err != nil {
		return nil, fmt.Errorf("nvml device power usage index %d: %w", i, err)
	}
	if device.PowerLimit, err = c.lib.DevicePowerLimit(handle); err != nil {
		return nil, fmt.Errorf("nvml device power limit index %d: %w", i, err)
	}
	if device.ClockGraphics, err = c.lib.DeviceClockGraphics(handle); err != nil {
		return nil, fmt.Errorf("nvml device graphics clock index %d: %w", i, err)
	}
	if device.ClockMem, err = c.lib.DeviceClockMem(handle); err != nil {
		return nil, fmt.Errorf("nvml device memory clock index %d: %w", i, err)
	}
	if device.UtilizationGPU, device.UtilizationMem, err = c.lib.DeviceUtilization(handle); err != nil {
		return nil, fmt.Errorf("nvml device utilization index %d: %w", i, err)
	}
	reasons, err := c.lib.DeviceThrottleReasons(handle)
	if err != nil {
		return nil, fmt.Errorf("nvml device throttle reasons index %d: %w", i, err)
	}
	device.ThrottleReasons = decodeThrottleReasons(reasons)
	if device.ECCEnabled, err = c.lib.DeviceECCEnabled(handle); err != nil {
		return nil, fmt.Errorf("nvml device ecc mode index %d: %w", i, err)
	}
	if device.MIGEnabled, err = c.lib.DeviceMIGEnabled(handle); err != nil {
		return nil, fmt.Errorf("nvml device mig mode index %d: %w", i, err)
	}
	if device.DriverVersion, err = c.lib.DriverVersion(); err != nil {
		return nil, fmt.Errorf("nvml driver version index %d: %w", i, err)
	}
	if device.CudaVersion, err = c.lib.CudaVersion(); err != nil {
		return nil, fmt.Errorf("nvml cuda version index %d: %w", i, err)
	}
	c.gatherOptionalMetrics(device, handle)
	device.Processes = c.gatherProcesses(handle)
	return device, nil
}

// gatherOptionalMetrics fills the encoder/decoder utilization and PCIe link
// fields best-effort: a missing NVML symbol (plan R6) or NOT_SUPPORTED leaves the
// field 0 and never fails Device(i), unlike the baseline metrics above.
// MemBandwidthUtil stays 0 for NVML in v1.1 (no portable single call).
func (c *Collector) gatherOptionalMetrics(device *gpu.Device, handle uintptr) {
	if enc, err := c.lib.DeviceEncoderUtil(handle); err == nil {
		device.EncoderUtil = enc
	}
	if dec, err := c.lib.DeviceDecoderUtil(handle); err == nil {
		device.DecoderUtil = dec
	}
	if gen, err := c.lib.DevicePCIeGen(handle); err == nil {
		device.PCIeGen = gen
	}
	if width, err := c.lib.DevicePCIeWidth(handle); err == nil {
		device.PCIeWidth = width
	}
}

// gatherProcesses collects compute and graphics processes best-effort: per plan
// B5 a process-list error or an unsupported driver leaves the slice empty and
// never fails Device(i). Name/User come from procinfo.Resolve via the
// resolveProcess seam.
func (c *Collector) gatherProcesses(handle uintptr) []gpu.GPUProcess {
	compute, err := c.lib.DeviceComputeProcesses(handle)
	if err != nil {
		return nil
	}
	graphics, err := c.lib.DeviceGraphicsProcesses(handle)
	if err != nil {
		return nil
	}
	processes := make([]gpu.GPUProcess, 0, len(compute)+len(graphics))
	processes = appendProcesses(processes, compute, "compute")
	processes = appendProcesses(processes, graphics, "graphics")
	if len(processes) == 0 {
		return nil
	}
	return processes
}

func appendProcesses(dst []gpu.GPUProcess, procs []ProcInfo, kind string) []gpu.GPUProcess {
	for _, proc := range procs {
		name, user := resolveProcess(int(proc.PID))
		dst = append(dst, gpu.GPUProcess{
			PID:        int(proc.PID),
			Name:       name,
			User:       user,
			UsedMemory: proc.UsedMemory,
			Type:       kind,
		})
	}
	return dst
}

func (c *Collector) Backend() string {
	return backendName
}

type throttleReason struct {
	bit  uint64
	name string
}

var throttleReasons = []throttleReason{
	{bit: 0x1, name: "gpu_idle"},
	{bit: 0x2, name: "applications_clocks_setting"},
	{bit: 0x4, name: "sw_power_cap"},
	{bit: 0x8, name: "hw_slowdown"},
	{bit: 0x10, name: "sync_boost"},
	{bit: 0x20, name: "sw_thermal_slowdown"},
	{bit: 0x40, name: "hw_thermal_slowdown"},
	{bit: 0x80, name: "hw_power_brake_slowdown"},
	{bit: 0x100, name: "display_clock_setting"},
}

func decodeThrottleReasons(mask uint64) []string {
	reasons := make([]string, 0, len(throttleReasons))
	for _, reason := range throttleReasons {
		if mask&reason.bit != 0 {
			reasons = append(reasons, reason.name)
		}
	}
	return reasons
}
