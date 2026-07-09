package report

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
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
	// Enc/Dec (encoder/decoder %) and PCIe (genXwN link) are appended as two
	// compact columns so each device stays a single deterministic row.
	if _, err := fmt.Fprintln(tw, "Index\tName\tUUID\tMem(used/total)\tTemp\tPower(draw/limit)\tUtil(gpu/mem)\tEnc/Dec\tPCIe\tPState"); err != nil {
		return err
	}
	for _, device := range snap.Devices {
		if _, err := fmt.Fprintf(
			tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			device.Index,
			device.Name,
			device.UUID,
			formatMemory(device),
			formatTemperature(device),
			formatPower(device),
			formatUtilization(device),
			formatEncDec(device),
			formatPCIe(device),
			device.PState,
		); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	if err := renderProcessTable(&builder, snap); err != nil {
		return err
	}

	_, err := io.WriteString(w, builder.String())
	return err
}

func renderProcessTable(builder *strings.Builder, snap *Snapshot) error {
	procs := collectProcesses(snap)
	if len(procs) == 0 {
		return nil
	}

	builder.WriteString("\nGPU Processes\n")
	tw := tabwriter.NewWriter(builder, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "GPU\tPID\tType\tProcess\tUser\tMem"); err != nil {
		return err
	}
	for _, proc := range procs {
		if _, err := fmt.Fprintf(
			tw, "%d\t%d\t%s\t%s\t%s\t%s\n",
			proc.gpuIndex,
			proc.process.PID,
			proc.process.Type,
			proc.process.Name,
			proc.process.User,
			formatProcessMemory(proc.process),
		); err != nil {
			return err
		}
	}
	return tw.Flush()
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

	fmt.Fprintf(&builder, "| Index | Name | UUID | Mem(used/total) | Temp | Power(draw/limit) | Util(gpu/mem) | Enc/Dec | PCIe | PState |\n")
	fmt.Fprintf(&builder, "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, device := range snap.Devices {
		fmt.Fprintf(
			&builder, "| %d | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			device.Index,
			device.Name,
			device.UUID,
			formatMemory(device),
			formatTemperature(device),
			formatPower(device),
			formatUtilization(device),
			formatEncDec(device),
			formatPCIe(device),
			device.PState,
		)
	}

	renderProcessMarkdown(&builder, snap)

	_, err := io.WriteString(w, builder.String())
	return err
}

func renderProcessMarkdown(builder *strings.Builder, snap *Snapshot) {
	procs := collectProcesses(snap)
	if len(procs) == 0 {
		return
	}

	fmt.Fprintf(builder, "\n### GPU Processes\n\n")
	fmt.Fprintf(builder, "| GPU | PID | Type | Process | User | Mem |\n")
	fmt.Fprintf(builder, "| --- | --- | --- | --- | --- | --- |\n")
	for _, proc := range procs {
		fmt.Fprintf(
			builder, "| %d | %d | %s | %s | %s | %s |\n",
			proc.gpuIndex,
			proc.process.PID,
			proc.process.Type,
			proc.process.Name,
			proc.process.User,
			formatProcessMemory(proc.process),
		)
	}
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

func formatEncDec(device gpu.Device) string {
	return fmt.Sprintf("%d/%d%%", device.EncoderUtil, device.DecoderUtil)
}

func formatPCIe(device gpu.Device) string {
	return fmt.Sprintf("gen%dx%d", device.PCIeGen, device.PCIeWidth)
}

func formatProcessMemory(process gpu.GPUProcess) string {
	return fmt.Sprintf("%d MiB", bytesToMiB(process.UsedMemory))
}

type deviceProcess struct {
	gpuIndex int
	process  gpu.GPUProcess
}

// collectProcesses flattens every device's processes into a single slice sorted
// deterministically by (GPU index, PID) so table and markdown output never
// depend on device or process insertion order.
func collectProcesses(snap *Snapshot) []deviceProcess {
	var procs []deviceProcess
	for _, device := range snap.Devices {
		for _, process := range device.Processes {
			procs = append(procs, deviceProcess{gpuIndex: device.Index, process: process})
		}
	}
	sort.Slice(procs, func(i, j int) bool {
		if procs[i].gpuIndex != procs[j].gpuIndex {
			return procs[i].gpuIndex < procs[j].gpuIndex
		}
		return procs[i].process.PID < procs[j].process.PID
	})
	return procs
}
