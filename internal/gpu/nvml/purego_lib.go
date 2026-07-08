package nvml

import (
	"fmt"
	"strconv"

	"github.com/ebitengine/purego"
)

const (
	nvmlLibraryName        = "libnvidia-ml.so.1"
	nvmlSuccess            = 0
	nvmlTemperatureGPU     = 0
	nvmlClockGraphics      = 0
	nvmlClockMem           = 2
	nvmlStringBufferLength = 96
)

type nvmlMemory struct {
	total uint64
	free  uint64 //nolint:unused // ABI mirror of nvmlMemory_t C layout; required for correct pointer decode even though unread
	used  uint64
}

type nvmlUtilization struct {
	gpu    uint32
	memory uint32
}

type nvmlReturn uint32

func (r nvmlReturn) Error() string {
	return fmt.Sprintf("nvml return code %d", uint32(r))
}

type puregoLib struct {
	handle                                    uintptr
	nvmlInit                                  func() uint32
	nvmlShutdown                              func() uint32
	nvmlDeviceGetCount                        func(*uint32) uint32
	nvmlDeviceGetHandleByIndex                func(uint32, *uintptr) uint32
	nvmlDeviceGetName                         func(uintptr, []byte, uint32) uint32
	nvmlDeviceGetMemoryInfo                   func(uintptr, *nvmlMemory) uint32
	nvmlDeviceGetTemperature                  func(uintptr, uint32, *uint32) uint32
	nvmlDeviceGetPowerUsage                   func(uintptr, *uint32) uint32
	nvmlDeviceGetEnforcedPowerLimit           func(uintptr, *uint32) uint32
	nvmlDeviceGetClockInfo                    func(uintptr, uint32, *uint32) uint32
	nvmlDeviceGetUtilizationRates             func(uintptr, *nvmlUtilization) uint32
	nvmlDeviceGetCurrentClocksThrottleReasons func(uintptr, *uint64) uint32
	nvmlDeviceGetEccMode                      func(uintptr, *uint32, *uint32) uint32
	nvmlDeviceGetMigMode                      func(uintptr, *uint32, *uint32) uint32
	nvmlSystemGetDriverVersion                func([]byte, uint32) uint32
	nvmlSystemGetCudaDriverVersion            func(*int32) uint32
}

func newPuregoLib() (*puregoLib, error) {
	handle, err := purego.Dlopen(nvmlLibraryName, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("dlopen %s: %w", nvmlLibraryName, err)
	}
	lib := &puregoLib{handle: handle}
	lib.register()
	return lib, nil
}

func (l *puregoLib) register() {
	purego.RegisterLibFunc(&l.nvmlInit, l.handle, "nvmlInit_v2")
	purego.RegisterLibFunc(&l.nvmlShutdown, l.handle, "nvmlShutdown")
	purego.RegisterLibFunc(&l.nvmlDeviceGetCount, l.handle, "nvmlDeviceGetCount_v2")
	purego.RegisterLibFunc(&l.nvmlDeviceGetHandleByIndex, l.handle, "nvmlDeviceGetHandleByIndex_v2")
	purego.RegisterLibFunc(&l.nvmlDeviceGetName, l.handle, "nvmlDeviceGetName")
	purego.RegisterLibFunc(&l.nvmlDeviceGetMemoryInfo, l.handle, "nvmlDeviceGetMemoryInfo")
	purego.RegisterLibFunc(&l.nvmlDeviceGetTemperature, l.handle, "nvmlDeviceGetTemperature")
	purego.RegisterLibFunc(&l.nvmlDeviceGetPowerUsage, l.handle, "nvmlDeviceGetPowerUsage")
	purego.RegisterLibFunc(&l.nvmlDeviceGetEnforcedPowerLimit, l.handle, "nvmlDeviceGetEnforcedPowerLimit")
	purego.RegisterLibFunc(&l.nvmlDeviceGetClockInfo, l.handle, "nvmlDeviceGetClockInfo")
	purego.RegisterLibFunc(&l.nvmlDeviceGetUtilizationRates, l.handle, "nvmlDeviceGetUtilizationRates")
	purego.RegisterLibFunc(&l.nvmlDeviceGetCurrentClocksThrottleReasons, l.handle, "nvmlDeviceGetCurrentClocksThrottleReasons")
	purego.RegisterLibFunc(&l.nvmlDeviceGetEccMode, l.handle, "nvmlDeviceGetEccMode")
	purego.RegisterLibFunc(&l.nvmlDeviceGetMigMode, l.handle, "nvmlDeviceGetMigMode")
	purego.RegisterLibFunc(&l.nvmlSystemGetDriverVersion, l.handle, "nvmlSystemGetDriverVersion")
	purego.RegisterLibFunc(&l.nvmlSystemGetCudaDriverVersion, l.handle, "nvmlSystemGetCudaDriverVersion")
}

func (l *puregoLib) Init() error {
	return nvmlError(l.nvmlInit())
}

func (l *puregoLib) Shutdown() error {
	return nvmlError(l.nvmlShutdown())
}

func (l *puregoLib) DeviceCount() (int, error) {
	var count uint32
	if err := nvmlError(l.nvmlDeviceGetCount(&count)); err != nil {
		return 0, err
	}
	return int(count), nil
}

func (l *puregoLib) DeviceHandleByIndex(i int) (uintptr, error) {
	var handle uintptr
	if err := nvmlError(l.nvmlDeviceGetHandleByIndex(uint32(i), &handle)); err != nil {
		return 0, err
	}
	return handle, nil
}

func (l *puregoLib) DeviceName(h uintptr) (string, error) {
	buf := make([]byte, nvmlStringBufferLength)
	if err := nvmlError(l.nvmlDeviceGetName(h, buf, uint32(len(buf)))); err != nil {
		return "", err
	}
	return cString(buf), nil
}

func (l *puregoLib) DeviceMemory(h uintptr) (uint64, uint64, error) {
	var memory nvmlMemory
	if err := nvmlError(l.nvmlDeviceGetMemoryInfo(h, &memory)); err != nil {
		return 0, 0, err
	}
	return memory.total, memory.used, nil
}

func (l *puregoLib) DeviceTemperature(h uintptr) (uint32, error) {
	var temperature uint32
	if err := nvmlError(l.nvmlDeviceGetTemperature(h, nvmlTemperatureGPU, &temperature)); err != nil {
		return 0, err
	}
	return temperature, nil
}

func (l *puregoLib) DevicePowerUsage(h uintptr) (uint32, error) {
	var power uint32
	if err := nvmlError(l.nvmlDeviceGetPowerUsage(h, &power)); err != nil {
		return 0, err
	}
	return power, nil
}

func (l *puregoLib) DevicePowerLimit(h uintptr) (uint32, error) {
	var limit uint32
	if err := nvmlError(l.nvmlDeviceGetEnforcedPowerLimit(h, &limit)); err != nil {
		return 0, err
	}
	return limit, nil
}

func (l *puregoLib) DeviceClockGraphics(h uintptr) (uint32, error) {
	var clock uint32
	if err := nvmlError(l.nvmlDeviceGetClockInfo(h, nvmlClockGraphics, &clock)); err != nil {
		return 0, err
	}
	return clock, nil
}

func (l *puregoLib) DeviceClockMem(h uintptr) (uint32, error) {
	var clock uint32
	if err := nvmlError(l.nvmlDeviceGetClockInfo(h, nvmlClockMem, &clock)); err != nil {
		return 0, err
	}
	return clock, nil
}

func (l *puregoLib) DeviceUtilization(h uintptr) (uint32, uint32, error) {
	var utilization nvmlUtilization
	if err := nvmlError(l.nvmlDeviceGetUtilizationRates(h, &utilization)); err != nil {
		return 0, 0, err
	}
	return utilization.gpu, utilization.memory, nil
}

func (l *puregoLib) DeviceThrottleReasons(h uintptr) (uint64, error) {
	var reasons uint64
	if err := nvmlError(l.nvmlDeviceGetCurrentClocksThrottleReasons(h, &reasons)); err != nil {
		return 0, err
	}
	return reasons, nil
}

func (l *puregoLib) DeviceECCEnabled(h uintptr) (*bool, error) {
	var current uint32
	var pending uint32
	if err := nvmlError(l.nvmlDeviceGetEccMode(h, &current, &pending)); err != nil {
		return nil, err
	}
	enabled := current != 0
	return &enabled, nil
}

func (l *puregoLib) DeviceMIGEnabled(h uintptr) (bool, error) {
	var current uint32
	var pending uint32
	if err := nvmlError(l.nvmlDeviceGetMigMode(h, &current, &pending)); err != nil {
		return false, err
	}
	return current != 0, nil
}

func (l *puregoLib) DriverVersion() (string, error) {
	buf := make([]byte, nvmlStringBufferLength)
	if err := nvmlError(l.nvmlSystemGetDriverVersion(buf, uint32(len(buf)))); err != nil {
		return "", err
	}
	return cString(buf), nil
}

func (l *puregoLib) CudaVersion() (string, error) {
	var version int32
	if err := nvmlError(l.nvmlSystemGetCudaDriverVersion(&version)); err != nil {
		return "", err
	}
	return strconv.Itoa(int(version)), nil
}

func nvmlError(code uint32) error {
	if code != nvmlSuccess {
		return nvmlReturn(code)
	}
	return nil
}

func cString(buf []byte) string {
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}
