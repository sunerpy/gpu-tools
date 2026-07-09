//go:build linux || darwin

package nvml

import (
	"fmt"
	"strconv"

	"github.com/ebitengine/purego"
)

const (
	nvmlLibraryName          = "libnvidia-ml.so.1"
	nvmlSuccess              = 0
	nvmlErrorNotSupported    = 3
	nvmlErrorInsufficientSz  = 7
	nvmlTemperatureGPU       = 0
	nvmlClockGraphics        = 0
	nvmlClockMem             = 2
	nvmlStringBufferLength   = 96
	nvmlProcessQueryMaxRetry = 8
)

type nvmlProcessInfo struct {
	pid               uint32
	usedGpuMemory     uint64
	gpuInstanceId     uint32 //nolint:unused // ABI mirror of nvmlProcessInfo_t v3 layout; required for correct pointer decode even though unread
	computeInstanceId uint32 //nolint:unused // ABI mirror of nvmlProcessInfo_t v3 layout; required for correct pointer decode even though unread
}

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
	nvmlDeviceGetComputeRunningProcesses      func(uintptr, *uint32, *nvmlProcessInfo) uint32
	nvmlDeviceGetGraphicsRunningProcesses     func(uintptr, *uint32, *nvmlProcessInfo) uint32
	nvmlDeviceGetEncoderUtilization           func(uintptr, *uint32, *uint32) uint32
	nvmlDeviceGetDecoderUtilization           func(uintptr, *uint32, *uint32) uint32
	nvmlDeviceGetCurrPcieLinkGeneration       func(uintptr, *uint32) uint32
	nvmlDeviceGetCurrPcieLinkWidth            func(uintptr, *uint32) uint32
	nvmlSystemGetDriverVersion                func([]byte, uint32) uint32
	nvmlSystemGetCudaDriverVersion            func(*int32) uint32
}

func newPuregoLib() (nvmlLib, error) {
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
	l.registerOptional(&l.nvmlDeviceGetComputeRunningProcesses, "nvmlDeviceGetComputeRunningProcesses_v3")
	l.registerOptional(&l.nvmlDeviceGetGraphicsRunningProcesses, "nvmlDeviceGetGraphicsRunningProcesses_v3")
	l.registerOptional(&l.nvmlDeviceGetEncoderUtilization, "nvmlDeviceGetEncoderUtilization")
	l.registerOptional(&l.nvmlDeviceGetDecoderUtilization, "nvmlDeviceGetDecoderUtilization")
	l.registerOptional(&l.nvmlDeviceGetCurrPcieLinkGeneration, "nvmlDeviceGetCurrPcieLinkGeneration")
	l.registerOptional(&l.nvmlDeviceGetCurrPcieLinkWidth, "nvmlDeviceGetCurrPcieLinkWidth")
	purego.RegisterLibFunc(&l.nvmlSystemGetDriverVersion, l.handle, "nvmlSystemGetDriverVersion")
	purego.RegisterLibFunc(&l.nvmlSystemGetCudaDriverVersion, l.handle, "nvmlSystemGetCudaDriverVersion")
}

// registerOptional binds fn only if symbol resolves via Dlsym. purego's
// RegisterLibFunc PANICS on an absent symbol, and the _v3 process symbols do not
// exist on older drivers (plan R6); probing first keeps fn nil so the caller
// treats the feature as unsupported and yields an empty process list instead of
// crashing.
func (l *puregoLib) registerOptional(fn any, symbol string) {
	if _, err := purego.Dlsym(l.handle, symbol); err != nil {
		return
	}
	purego.RegisterLibFunc(fn, l.handle, symbol)
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

func (l *puregoLib) DeviceComputeProcesses(h uintptr) ([]ProcInfo, error) {
	return runningProcesses(l.nvmlDeviceGetComputeRunningProcesses, h)
}

func (l *puregoLib) DeviceGraphicsProcesses(h uintptr) ([]ProcInfo, error) {
	return runningProcesses(l.nvmlDeviceGetGraphicsRunningProcesses, h)
}

func (l *puregoLib) DeviceEncoderUtil(h uintptr) (uint32, error) {
	return optionalUtil(l.nvmlDeviceGetEncoderUtilization, h)
}

func (l *puregoLib) DeviceDecoderUtil(h uintptr) (uint32, error) {
	return optionalUtil(l.nvmlDeviceGetDecoderUtilization, h)
}

func (l *puregoLib) DevicePCIeGen(h uintptr) (uint32, error) {
	return optionalLink(l.nvmlDeviceGetCurrPcieLinkGeneration, h)
}

func (l *puregoLib) DevicePCIeWidth(h uintptr) (uint32, error) {
	return optionalLink(l.nvmlDeviceGetCurrPcieLinkWidth, h)
}

// optionalUtil handles the NVML util calls whose second out-param is a sampling
// period this collector discards. A nil fn (missing symbol, plan R6) or
// NOT_SUPPORTED yields (0, nil) so Device(i) never fails.
func optionalUtil(fn func(uintptr, *uint32, *uint32) uint32, h uintptr) (uint32, error) {
	if fn == nil {
		return 0, nil
	}
	var util, samplingPeriod uint32
	switch code := fn(h, &util, &samplingPeriod); code {
	case nvmlSuccess:
		return util, nil
	case nvmlErrorNotSupported:
		return 0, nil
	default:
		return 0, nvmlReturn(code)
	}
}

// optionalLink handles the single-out-param NVML PCIe link calls with the same
// missing-symbol / NOT_SUPPORTED to (0, nil) best-effort contract as optionalUtil.
func optionalLink(fn func(uintptr, *uint32) uint32, h uintptr) (uint32, error) {
	if fn == nil {
		return 0, nil
	}
	var value uint32
	switch code := fn(h, &value); code {
	case nvmlSuccess:
		return value, nil
	case nvmlErrorNotSupported:
		return 0, nil
	default:
		return 0, nvmlReturn(code)
	}
}

// runningProcesses drives NVML's count-in/out protocol: an absent symbol (fn ==
// nil, unsupported driver) or NOT_SUPPORTED yields an empty slice with no error
// (best-effort). Call with count=0 to learn the required size (INSUFFICIENT_SIZE
// == 7), allocate, then call again; a shrinking count is retried a bounded
// number of times to tolerate a race where processes appear between calls.
func runningProcesses(fn func(uintptr, *uint32, *nvmlProcessInfo) uint32, h uintptr) ([]ProcInfo, error) {
	if fn == nil {
		return nil, nil
	}
	var count uint32
	for range nvmlProcessQueryMaxRetry {
		code := fn(h, &count, nil)
		switch code {
		case nvmlSuccess:
			return nil, nil
		case nvmlErrorNotSupported:
			return nil, nil
		case nvmlErrorInsufficientSz:
		default:
			return nil, nvmlReturn(code)
		}
		buf := make([]nvmlProcessInfo, count)
		code = fn(h, &count, &buf[0])
		switch code {
		case nvmlSuccess:
			infos := make([]ProcInfo, count)
			for i := range infos {
				infos[i] = ProcInfo{PID: buf[i].pid, UsedMemory: buf[i].usedGpuMemory}
			}
			return infos, nil
		case nvmlErrorNotSupported:
			return nil, nil
		case nvmlErrorInsufficientSz:
			continue
		default:
			return nil, nvmlReturn(code)
		}
	}
	return nil, nil
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
