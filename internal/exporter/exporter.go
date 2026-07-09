// Package exporter exposes a gpu.Collector as Prometheus metrics.
//
// The Exporter is a prometheus.Collector (Describe + Collect) that wraps a
// gpu.Collector built lazily on the first scrape via an injected factory. Per
// plan R4 it:
//
//   - guards GPU reads with a sync.Mutex so overlapping scrapes never race the
//     inner collector (bounded scrape freshness);
//   - holds its OWN *prometheus.Registry created via prometheus.NewRegistry so
//     repeated instances or tests never panic on duplicate registration and
//     the global default registry is never touched;
//   - emits gpu_tools_up 0 with NO device series (HTTP 200) when the backend is
//     unavailable (ErrBackendUnavailable / ErrNoBackend);
//   - never returns or propagates an error to promhttp: on a read error it logs
//     to the configured logger seam (stderr by default) and emits up 0.
package exporter

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

// Factory builds the underlying gpu.Collector. It matches the shape of
// gpu.DefaultFactory bound to a resolved config.
type Factory func() (gpu.Collector, error)

const (
	// upName is the only metric that carries the gpu_tools namespace; per-GPU
	// and per-process series use the plain gpu_ prefix per the plan T10 spec.
	upName     = "gpu_tools_up"
	powerScale = 1000.0 // NVML reports milliwatts; export watts.
)

var deviceLabels = []string{"index", "uuid", "name"}

// Exporter implements prometheus.Collector over a gpu.Collector.
type Exporter struct {
	factory Factory
	logger  io.Writer
	reg     *prometheus.Registry

	mu        sync.Mutex
	collector gpu.Collector

	up        *prometheus.Desc
	util      *prometheus.Desc
	memUsed   *prometheus.Desc
	memTotal  *prometheus.Desc
	temp      *prometheus.Desc
	powerDraw *prometheus.Desc
	powerCap  *prometheus.Desc
	clockGr   *prometheus.Desc
	clockMem  *prometheus.Desc
	encUtil   *prometheus.Desc
	decUtil   *prometheus.Desc
	procMem   *prometheus.Desc
}

var _ prometheus.Collector = (*Exporter)(nil)

// New builds an Exporter over factory, logging read errors to stderr, and
// registers it into its own registry.
func New(factory Factory) *Exporter {
	return NewWithLogger(factory, os.Stderr)
}

// NewWithLogger is the test seam: it lets callers capture the stderr log sink.
func NewWithLogger(factory Factory, logger io.Writer) *Exporter {
	e := &Exporter{
		factory: factory,
		logger:  logger,
		reg:     prometheus.NewRegistry(),
		up: prometheus.NewDesc(
			upName,
			"1 if a GPU backend is available and the last read succeeded, else 0.",
			nil, nil,
		),
		util: prometheus.NewDesc(
			"gpu_utilization_percent",
			"GPU core utilization in percent.", deviceLabels, nil,
		),
		memUsed: prometheus.NewDesc(
			"gpu_memory_used_bytes",
			"GPU memory used in bytes.", deviceLabels, nil,
		),
		memTotal: prometheus.NewDesc(
			"gpu_memory_total_bytes",
			"GPU memory total in bytes.", deviceLabels, nil,
		),
		temp: prometheus.NewDesc(
			"gpu_temperature_celsius",
			"GPU temperature in degrees Celsius.", deviceLabels, nil,
		),
		powerDraw: prometheus.NewDesc(
			"gpu_power_draw_watts",
			"GPU power draw in watts.", deviceLabels, nil,
		),
		powerCap: prometheus.NewDesc(
			"gpu_power_limit_watts",
			"GPU power limit in watts.", deviceLabels, nil,
		),
		clockGr: prometheus.NewDesc(
			"gpu_clock_graphics_mhz",
			"GPU graphics clock in MHz.", deviceLabels, nil,
		),
		clockMem: prometheus.NewDesc(
			"gpu_clock_mem_mhz",
			"GPU memory clock in MHz.", deviceLabels, nil,
		),
		encUtil: prometheus.NewDesc(
			"gpu_encoder_utilization_percent",
			"GPU hardware encoder utilization in percent.", deviceLabels, nil,
		),
		decUtil: prometheus.NewDesc(
			"gpu_decoder_utilization_percent",
			"GPU hardware decoder utilization in percent.", deviceLabels, nil,
		),
		procMem: prometheus.NewDesc(
			"gpu_process_used_memory_bytes",
			"GPU memory used by a process in bytes.",
			[]string{"index", "pid", "process_name", "type"}, nil,
		),
	}
	e.reg.MustRegister(e)
	return e
}

// Registry returns the Exporter's own registry (R4b). Serve /metrics via
// promhttp.HandlerFor(exp.Registry(), ...).
func (e *Exporter) Registry() *prometheus.Registry {
	return e.reg
}

// Describe implements prometheus.Collector. Sending descriptors here keeps the
// registry from treating the collector as unchecked.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.up
	ch <- e.util
	ch <- e.memUsed
	ch <- e.memTotal
	ch <- e.temp
	ch <- e.powerDraw
	ch <- e.powerCap
	ch <- e.clockGr
	ch <- e.clockMem
	ch <- e.encUtil
	ch <- e.decUtil
	ch <- e.procMem
}

// Collect implements prometheus.Collector. It NEVER propagates an error to
// promhttp (R4d): on any failure it logs and emits gpu_tools_up 0 with no
// device series. The whole read is mutex-guarded so overlapping scrapes never
// race the inner collector (R4a).
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mu.Lock()
	defer e.mu.Unlock()

	devices, err := e.readDevices()
	if err != nil {
		if errors.Is(err, gpu.ErrBackendUnavailable) || errors.Is(err, gpu.ErrNoBackend) {
			// Backend genuinely unavailable: expected on no-GPU hosts, quiet.
			ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, 0)
			return
		}
		e.logf("gpu-tools exporter: scrape read failed: %v", err)
		ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, 0)
		return
	}

	ch <- prometheus.MustNewConstMetric(e.up, prometheus.GaugeValue, 1)
	for i := range devices {
		e.emitDevice(ch, &devices[i])
	}
}

// readDevices lazily builds the inner collector, then snapshots every device.
// Callers hold e.mu.
func (e *Exporter) readDevices() ([]gpu.Device, error) {
	if e.collector == nil {
		collector, err := e.factory()
		if err != nil {
			return nil, err
		}
		if err := collector.Init(); err != nil {
			return nil, fmt.Errorf("initialize GPU collector: %w", err)
		}
		e.collector = collector
	}
	count, err := e.collector.DeviceCount()
	if err != nil {
		return nil, fmt.Errorf("count GPU devices: %w", err)
	}
	devices := make([]gpu.Device, 0, count)
	for i := range count {
		device, err := e.collector.Device(i)
		if err != nil {
			return nil, fmt.Errorf("read GPU device %d: %w", i, err)
		}
		if device == nil {
			return nil, fmt.Errorf("read GPU device %d: nil device", i)
		}
		devices = append(devices, *device)
	}
	return devices, nil
}

func (e *Exporter) emitDevice(ch chan<- prometheus.Metric, device *gpu.Device) {
	index := fmt.Sprintf("%d", device.Index)
	labels := []string{index, device.UUID, device.Name}
	gauge := func(desc *prometheus.Desc, value float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, value, labels...)
	}
	gauge(e.util, float64(device.UtilizationGPU))
	gauge(e.memUsed, float64(device.MemoryUsed))
	gauge(e.memTotal, float64(device.MemoryTotal))
	gauge(e.temp, float64(device.Temperature))
	gauge(e.powerDraw, float64(device.PowerDraw)/powerScale)
	gauge(e.powerCap, float64(device.PowerLimit)/powerScale)
	gauge(e.clockGr, float64(device.ClockGraphics))
	gauge(e.clockMem, float64(device.ClockMem))
	gauge(e.encUtil, float64(device.EncoderUtil))
	gauge(e.decUtil, float64(device.DecoderUtil))
	for _, proc := range device.Processes {
		ch <- prometheus.MustNewConstMetric(
			e.procMem, prometheus.GaugeValue, float64(proc.UsedMemory),
			index, fmt.Sprintf("%d", proc.PID), proc.Name, proc.Type,
		)
	}
}

func (e *Exporter) logf(format string, args ...any) {
	if e.logger == nil {
		return
	}
	_, _ = fmt.Fprintf(e.logger, format+"\n", args...)
}
