package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/sunerpy/gpu-tools/core"
	"github.com/sunerpy/gpu-tools/internal/gpu"
)

// Snapshot is the immutable input rendered by report output formats.
type Snapshot struct {
	Host      string
	Timestamp time.Time
	Backend   string
	Devices   []gpu.Device
}

// Renderer writes a deterministic, ANSI-free report representation.
type Renderer interface {
	Render(w io.Writer, snap *Snapshot) error
}

// TableRenderer renders an ANSI-free text table.
type TableRenderer struct{}

// JSONRenderer renders indented JSON with two-space indentation.
type JSONRenderer struct{}

// MarkdownRenderer renders a GitHub-flavored Markdown report.
type MarkdownRenderer struct{}

// UnknownFormatError reports an unsupported output renderer format.
type UnknownFormatError struct {
	Format string
}

func (e *UnknownFormatError) Error() string {
	return fmt.Sprintf("unknown output format %q", e.Format)
}

// RendererFor returns the renderer selected by the configured output format.
func RendererFor(format string) (Renderer, error) {
	switch format {
	case core.OutputTable:
		return TableRenderer{}, nil
	case core.OutputJSON:
		return JSONRenderer{}, nil
	case core.OutputMarkdown:
		return MarkdownRenderer{}, nil
	default:
		return nil, &UnknownFormatError{Format: format}
	}
}

func (TableRenderer) Render(w io.Writer, snap *Snapshot) error {
	if len(snap.Devices) == 0 {
		_, err := io.WriteString(w, "no devices\n")
		return err
	}

	var builder strings.Builder
	tw := tabwriter.NewWriter(&builder, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "Index\tName\tUUID\tMem(used/total)\tTemp\tPower(draw/limit)\tUtil(gpu/mem)\tPState"); err != nil {
		return err
	}
	for _, device := range snap.Devices {
		if _, err := fmt.Fprintf(
			tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			device.Index,
			device.Name,
			device.UUID,
			formatMemory(device),
			formatTemperature(device),
			formatPower(device),
			formatUtilization(device),
			device.PState,
		); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	_, err := io.WriteString(w, builder.String())
	return err
}

func (JSONRenderer) Render(w io.Writer, snap *Snapshot) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(snap)
}

func (MarkdownRenderer) Render(w io.Writer, snap *Snapshot) error {
	var builder strings.Builder
	fmt.Fprintf(&builder, "## GPU Snapshot\n\n")
	fmt.Fprintf(&builder, "- Host: `%s`\n", snap.Host)
	fmt.Fprintf(&builder, "- Backend: `%s`\n", snap.Backend)
	fmt.Fprintf(&builder, "- Timestamp: `%s`\n\n", snap.Timestamp.Format(time.RFC3339))
	if len(snap.Devices) == 0 {
		fmt.Fprintf(&builder, "No devices.\n")
		_, err := io.WriteString(w, builder.String())
		return err
	}

	fmt.Fprintf(&builder, "| Index | Name | UUID | Mem(used/total) | Temp | Power(draw/limit) | Util(gpu/mem) | PState |\n")
	fmt.Fprintf(&builder, "| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, device := range snap.Devices {
		fmt.Fprintf(
			&builder, "| %d | %s | %s | %s | %s | %s | %s | %s |\n",
			device.Index,
			device.Name,
			device.UUID,
			formatMemory(device),
			formatTemperature(device),
			formatPower(device),
			formatUtilization(device),
			device.PState,
		)
	}

	_, err := io.WriteString(w, builder.String())
	return err
}

// Human-readable renderers present memory in integer MiB and power in W with one decimal.
func formatMemory(device gpu.Device) string {
	return fmt.Sprintf("%d/%d MiB", bytesToMiB(device.MemoryUsed), bytesToMiB(device.MemoryTotal))
}

func bytesToMiB(bytes uint64) uint64 {
	return bytes / 1024 / 1024
}

func formatTemperature(device gpu.Device) string {
	return fmt.Sprintf("%d°C", device.Temperature)
}

func formatPower(device gpu.Device) string {
	return fmt.Sprintf("%.1f/%.1f W", milliWattsToWatts(device.PowerDraw), milliWattsToWatts(device.PowerLimit))
}

func milliWattsToWatts(milliWatts uint32) float64 {
	return float64(milliWatts) / 1000
}

func formatUtilization(device gpu.Device) string {
	return fmt.Sprintf("%d/%d%%", device.UtilizationGPU, device.UtilizationMem)
}
