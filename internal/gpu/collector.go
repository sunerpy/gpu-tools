//go:generate mockgen -source=collector.go -destination=mocks/collector_mock.go -package=mocks

package gpu

import "errors"

var (
	ErrBackendUnavailable = errors.New("gpu backend unavailable")
	ErrNoBackend          = errors.New("no gpu backend available")
)

type Collector interface {
	Init() error
	Shutdown() error
	DeviceCount() (int, error)
	Device(i int) (*Device, error)
	Backend() string
}

type Device struct {
	Index           int
	UUID            string
	Name            string
	MemoryTotal     uint64
	MemoryUsed      uint64
	Temperature     uint32
	PowerDraw       uint32
	PowerLimit      uint32
	ClockGraphics   uint32
	ClockMem        uint32
	UtilizationGPU  uint32
	UtilizationMem  uint32
	ThrottleReasons []string
	ECCEnabled      *bool
	MIGEnabled      bool
	MIGDevices      []MIGDevice
	PState          string
	FanSpeed        *int
	DriverVersion   string
	CudaVersion     string
}

type MIGDevice struct {
	GIID        int
	CIID        int
	UUID        string
	MemoryTotal uint64
}
