package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/sunerpy/gpu-tools/core"
	"github.com/sunerpy/gpu-tools/internal/gpu"
	"github.com/sunerpy/gpu-tools/internal/tune"
)

type tuneReport struct {
	Devices []tuneDeviceReport `json:"devices"`
}

type tuneDeviceReport struct {
	Index           int                        `json:"index"`
	UUID            string                     `json:"uuid"`
	Name            string                     `json:"name"`
	Recommendations []tuneRecommendationReport `json:"recommendations"`
}

type tuneRecommendationReport struct {
	Severity        tune.Severity `json:"severity"`
	Title           string        `json:"title"`
	Detail          string        `json:"detail"`
	SuggestedAction string        `json:"suggested_action"`
}

func newTuneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tune",
		Short: "Show read-only GPU tuning recommendations",
		Long:  "Show advisory GPU tuning recommendations without changing hardware settings.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTune(cmd)
		},
	}
}

func runTune(cmd *cobra.Command) (err error) {
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

	devices, err := collectTuneDevices(collector)
	if err != nil {
		return err
	}
	report := newTuneReportFromDevices(devices)
	if err := renderTuneReport(cmd.OutOrStdout(), cfg.DefaultOutput, report); err != nil {
		return fmt.Errorf("render tune recommendations: %w", err)
	}
	return nil
}

func collectTuneDevices(collector gpu.Collector) ([]gpu.Device, error) {
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

func newTuneReportFromDevices(devices []gpu.Device) tuneReport {
	report := tuneReport{Devices: make([]tuneDeviceReport, 0, len(devices))}
	for _, device := range devices {
		report.Devices = append(report.Devices, tuneDeviceReport{
			Index:           device.Index,
			UUID:            device.UUID,
			Name:            device.Name,
			Recommendations: tuneRecommendationsForDevice(device),
		})
	}
	return report
}

func tuneRecommendationsForDevice(device gpu.Device) []tuneRecommendationReport {
	recommendations := tune.Evaluate(device)
	items := make([]tuneRecommendationReport, 0, len(recommendations))
	for _, recommendation := range recommendations {
		items = append(items, tuneRecommendationReport{
			Severity:        recommendation.Severity,
			Title:           recommendation.Title,
			Detail:          recommendation.Detail,
			SuggestedAction: recommendation.SuggestedAction,
		})
	}
	return items
}

func renderTuneReport(w io.Writer, format string, report tuneReport) error {
	switch format {
	case core.OutputTable:
		return renderTuneTable(w, report)
	case core.OutputJSON:
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	case core.OutputMarkdown:
		return renderTuneMarkdown(w, report)
	default:
		return fmt.Errorf("unsupported tune output %q", format)
	}
}

func renderTuneTable(w io.Writer, report tuneReport) error {
	if _, err := fmt.Fprintln(w, "GPU\tSeverity\tRecommendation\tSuggested Action"); err != nil {
		return err
	}
	for _, device := range report.Devices {
		if len(device.Recommendations) == 0 {
			if _, err := fmt.Fprintf(w, "%s\tinfo\tNo recommendations.\tNo tuning actions suggested.\n", tuneDeviceLabel(device)); err != nil {
				return err
			}
			continue
		}
		for _, recommendation := range device.Recommendations {
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", tuneDeviceLabel(device), recommendation.Severity, recommendation.Title, recommendation.SuggestedAction); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderTuneMarkdown(w io.Writer, report tuneReport) error {
	if _, err := fmt.Fprintln(w, "# GPU tuning recommendations"); err != nil {
		return err
	}
	for _, device := range report.Devices {
		if _, err := fmt.Fprintf(w, "\n## %s\n\n", tuneDeviceLabel(device)); err != nil {
			return err
		}
		if len(device.Recommendations) == 0 {
			if _, err := fmt.Fprintln(w, "No recommendations."); err != nil {
				return err
			}
			continue
		}
		for _, recommendation := range device.Recommendations {
			if _, err := fmt.Fprintf(w, "- **%s** `%s`: %s Suggested action: %s\n", recommendation.Title, recommendation.Severity, recommendation.Detail, recommendation.SuggestedAction); err != nil {
				return err
			}
		}
	}
	return nil
}

func tuneDeviceLabel(device tuneDeviceReport) string {
	if device.Name != "" {
		return fmt.Sprintf("GPU %d %s", device.Index, device.Name)
	}
	if device.UUID != "" {
		return fmt.Sprintf("GPU %d %s", device.Index, device.UUID)
	}
	return fmt.Sprintf("GPU %d", device.Index)
}

func init() {
	registerCommand(newTuneCmd)
}
