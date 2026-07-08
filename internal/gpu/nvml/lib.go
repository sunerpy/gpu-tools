package nvml

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
	DriverVersion() (string, error)
	CudaVersion() (string, error)
}
