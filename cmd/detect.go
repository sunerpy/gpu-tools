package cmd

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/sunerpy/gpu-tools/internal/gpu"
	"github.com/sunerpy/gpu-tools/internal/report"
)

func newDetectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "detect",
		Short: "Detect local NVIDIA GPU inventory",
		Long:  "Detect local NVIDIA GPUs and render a point-in-time inventory snapshot.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDetect(cmd)
		},
	}
}

func runDetect(cmd *cobra.Command) (err error) {
	cfg, err := resolvedConfig(cmd)
	if err != nil {
		return err
	}
	collector, err := gpu.DefaultFactory(*cfg)
	if err != nil {
		return detectFactoryError(err)
	}
	if err := collector.Init(); err != nil {
		return fmt.Errorf("initialize GPU collector: %w", err)
	}
	defer func() {
		err = errors.Join(err, collector.Shutdown())
	}()

	count, err := collector.DeviceCount()
	if err != nil {
		return fmt.Errorf("count GPU devices: %w", err)
	}
	devices := make([]gpu.Device, 0, count)
	for i := range count {
		device, err := collector.Device(i)
		if err != nil {
			return fmt.Errorf("read GPU device %d: %w", i, err)
		}
		if device == nil {
			return fmt.Errorf("read GPU device %d: nil device", i)
		}
		devices = append(devices, *device)
	}

	renderer, err := report.RendererFor(cfg.DefaultOutput)
	if err != nil {
		return fmt.Errorf("select detect renderer: %w", err)
	}
	host, _ := os.Hostname()
	snapshot := report.Snapshot{
		Host:      host,
		Timestamp: time.Now(),
		Backend:   collector.Backend(),
		Devices:   devices,
	}
	if err := renderer.Render(cmd.OutOrStdout(), &snapshot); err != nil {
		return fmt.Errorf("render detect snapshot: %w", err)
	}
	return nil
}

func detectFactoryError(err error) error {
	if errors.Is(err, gpu.ErrBackendUnavailable) || errors.Is(err, gpu.ErrNoBackend) {
		return NewExitError(1, fmt.Errorf("no NVIDIA GPU detected: %w", err))
	}
	return NewExitError(1, fmt.Errorf("select GPU backend: %w", err))
}

func init() {
	registerCommand(newDetectCmd)
}
