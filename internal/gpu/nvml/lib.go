package nvml

// ProcInfo mirrors the relevant fields of NVML's nvmlProcessInfo_t: the PID of a
// process using the device and the device memory it holds. A backend that cannot
// enumerate processes (missing _v3 symbol or NOT_SUPPORTED) returns an empty
// slice with no error so per-process usage stays best-effort.
type ProcInfo struct {
	PID        uint32
	UsedMemory uint64
}

type nvmlLib interface {
	Init() error
	Shutdown() error
	DeviceCount() (int, error)
	DeviceHandleByIndex(i int) (uintptr, error)
	DeviceName(h uintptr) (string, error)
	DeviceMemory(h uintptr) (total, used uint64, err error)
	DeviceTemperature(h uintptr) (uint32, error)
	DevicePowerUsage(h uintptr) (uint32, error)
	DevicePowerLimit(h uintptr) (uint32, error)
	DeviceClockGraphics(h uintptr) (uint32, error)
	DeviceClockMem(h uintptr) (uint32, error)
	DeviceUtilization(h uintptr) (gpuUtil, memUtil uint32, err error)
	DeviceThrottleReasons(h uintptr) (uint64, error)
	DeviceECCEnabled(h uintptr) (*bool, error)
	DeviceMIGEnabled(h uintptr) (bool, error)
	DeviceComputeProcesses(h uintptr) ([]ProcInfo, error)
	DeviceGraphicsProcesses(h uintptr) ([]ProcInfo, error)
	DeviceEncoderUtil(h uintptr) (uint32, error)
	DeviceDecoderUtil(h uintptr) (uint32, error)
	DevicePCIeGen(h uintptr) (uint32, error)
	DevicePCIeWidth(h uintptr) (uint32, error)
	DriverVersion() (string, error)
	CudaVersion() (string, error)
}
