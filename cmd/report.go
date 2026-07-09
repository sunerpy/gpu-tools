package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/sunerpy/gpu-tools/core"
	"github.com/sunerpy/gpu-tools/internal/gpu"
	"github.com/sunerpy/gpu-tools/internal/report"
)

const reportOutFlag = "out"

func newReportCmd() *cobra.Command {
	var outPath string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Write a point-in-time GPU snapshot report artifact",
		Long:  "Collect local NVIDIA GPU inventory and render a point-in-time snapshot report artifact.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReport(cmd, outPath)
		},
	}
	cmd.Flags().StringVar(&outPath, reportOutFlag, "", "report output path, or - for stdout")
	return cmd
}

func runReport(cmd *cobra.Command, outPath string) (err error) {
	cfg, err := resolvedConfig(cmd)
	if err != nil {
		return err
	}
	collector, err := gpu.DefaultFactory(*cfg)
	if err != nil {
		return detectFactoryError(err)
	}
	if err = collector.Init(); err != nil {
		return fmt.Errorf("initialize GPU collector: %w", err)
	}
	defer func() {
		err = errors.Join(err, collector.Shutdown())
	}()

	devices, err := collectReportDevices(collector)
	if err != nil {
		return err
	}
	snapshot := report.Snapshot{
		Host:      hostname(),
		Timestamp: time.Now(),
		Backend:   collector.Backend(),
		Devices:   devices,
	}
	format := reportOutputFormat(cmd, cfg)
	rendered, err := renderReport(format, &snapshot)
	if err != nil {
		return err
	}
	target := reportOutputTarget(outPath, cfg, snapshot.Timestamp)
	if err := writeReport(cmd, target, rendered); err != nil {
		return NewExitError(1, err)
	}
	return nil
}

func collectReportDevices(collector gpu.Collector) ([]gpu.Device, error) {
	count, err := collector.DeviceCount()
	if err != nil {
		return nil, fmt.Errorf("count GPU devices: %w", err)
	}
	devices := make([]gpu.Device, 0, count)
	for i := range count {
		device, err := collector.Device(i)
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

func reportOutputFormat(cmd *cobra.Command, cfg *core.Config) string {
	if cmd.Root().PersistentFlags().Changed(outputFlag) {
		return cfg.DefaultOutput
	}
	return core.OutputMarkdown
}

func renderReport(format string, snapshot *report.Snapshot) ([]byte, error) {
	renderer, err := report.RendererFor(format)
	if err != nil {
		return nil, fmt.Errorf("select report renderer: %w", err)
	}
	var output bytes.Buffer
	if format == core.OutputMarkdown {
		if err := renderer.Render(&output, snapshot); err != nil {
			return nil, fmt.Errorf("render report snapshot: %w", err)
		}
		return withMarkdownSummary(output.Bytes(), snapshot), nil
	}
	if err := renderer.Render(&output, snapshot); err != nil {
		return nil, fmt.Errorf("render report snapshot: %w", err)
	}
	return output.Bytes(), nil
}

func withMarkdownSummary(rendered []byte, snapshot *report.Snapshot) []byte {
	var output bytes.Buffer
	output.Write(rendered)
	if len(rendered) > 0 && rendered[len(rendered)-1] != '\n' {
		output.WriteByte('\n')
	}
	fmt.Fprintf(&output, "\n## Summary\n\n")
	fmt.Fprintf(&output, "- Device count: `%d`\n", len(snapshot.Devices))
	fmt.Fprintf(&output, "- Aggregate memory total: `%d MiB`\n", totalMemoryMiB(snapshot.Devices))
	fmt.Fprintf(&output, "- Max temperature: `%d°C`\n", maxTemperature(snapshot.Devices))
	return output.Bytes()
}

func totalMemoryMiB(devices []gpu.Device) uint64 {
	var total uint64
	for _, device := range devices {
		total += device.MemoryTotal
	}
	return total / 1024 / 1024
}

func maxTemperature(devices []gpu.Device) uint32 {
	var maxTemp uint32
	for _, device := range devices {
		if device.Temperature > maxTemp {
			maxTemp = device.Temperature
		}
	}
	return maxTemp
}

func reportOutputTarget(outPath string, cfg *core.Config, timestamp time.Time) string {
	if outPath != "" {
		return outPath
	}
	name := fmt.Sprintf("gpu-report-%s.md", timestamp.Format("20060102-150405"))
	return filepath.Join(cfg.ReportDir, name)
}

func writeReport(cmd *cobra.Command, target string, data []byte) error {
	if target == "-" {
		_, err := cmd.OutOrStdout().Write(data)
		if err != nil {
			return fmt.Errorf("write report stdout: %w", err)
		}
		return nil
	}
	if err := writeReportFile(target, data); err != nil {
		return fmt.Errorf("write report %s: %w", target, err)
	}
	return nil
}

func writeReportFile(path string, data []byte) (err error) {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func hostname() string {
	host, err := os.Hostname()
	if err != nil {
		return ""
	}
	return host
}

func init() {
	registerCommand(newReportCmd)
}
