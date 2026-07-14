package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sunerpy/gpu-tools/core"
	"github.com/sunerpy/gpu-tools/internal/platform"
	"github.com/sunerpy/gpu-tools/internal/topo"
)

// topoCollect is the collection seam. Tests replace it to inject a fake
// topology result without a real nvidia-smi on the host.
var topoCollect = func(ctx context.Context, smiPath string) (*topo.Result, error) {
	return topo.Collect(ctx, nil, smiPath)
}

// platformIsLinux and platformOS are cmd-level indirection over the platform
// package so tests can override the platform gate without touching the
// platform package internals.
var (
	platformIsLinux = platform.IsLinux
	platformOS      = platform.CurrentOS
)

func newTopoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "topo",
		Short: "Show GPU/NIC connectivity matrix from nvidia-smi topo -m",
		Long: "Show the GPU/NIC connectivity matrix parsed from `nvidia-smi topo -m`, " +
			"plus per-NIC GPU affinity advice. Linux only; requires an installed NVIDIA driver.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTopo(cmd)
		},
	}
}

func runTopo(cmd *cobra.Command) error {
	if !platformIsLinux() {
		return topoUnsupported(cmd)
	}
	cfg, err := resolvedConfig(cmd)
	if err != nil {
		return err
	}
	result, err := topoCollect(cmd.Context(), cfg.NvidiaSmiPath)
	if err != nil {
		if errors.Is(err, topo.ErrToolNotInstalled) {
			return &ExitError{Code: 2, Err: fmt.Errorf("nvidia-smi not installed, install the NVIDIA driver and retry: %w", err)}
		}
		return NewExitError(1, err)
	}
	if result == nil {
		return NewExitError(1, fmt.Errorf("topo returned no result"))
	}
	if err := renderTopo(cmd.OutOrStdout(), cfg.DefaultOutput, result); err != nil {
		return fmt.Errorf("render topo result: %w", err)
	}
	return nil
}

// topoUnsupported handles the non-Linux path. For JSON output it emits the
// small unsupported-platform object to stdout; for every output mode it returns
// an exit-2 error carrying the clean message.
func topoUnsupported(cmd *cobra.Command) error {
	osName := platformOS()
	output, _ := cmd.Flags().GetString(outputFlag)
	if output == core.OutputJSON {
		payload := map[string]any{
			"supported":      false,
			"platform":       osName,
			"reason":         "requires Linux",
			"required_tools": []string{"nvidia-smi"},
		}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(payload); err != nil {
			return NewExitError(2, fmt.Errorf("encode unsupported-platform payload: %w", err))
		}
	}
	return &ExitError{Code: 2, Err: fmt.Errorf("gpu-tools topo requires Linux (uses nvidia-smi); current OS: %s", osName)}
}

func renderTopo(w io.Writer, output string, result *topo.Result) error {
	switch output {
	case core.OutputTable:
		return renderTopoTable(w, result)
	case core.OutputJSON:
		return renderTopoJSON(w, result)
	case core.OutputMarkdown:
		return renderTopoMarkdown(w, result)
	default:
		return fmt.Errorf("unknown topo output format %q", output)
	}
}

func renderTopoJSON(w io.Writer, result *topo.Result) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// cellText renders a single matrix cell for display; NVLink links include their
// lane count (e.g. NV12).
func cellText(cell topo.Cell) string {
	if cell.Type == topo.LinkNVLink {
		return fmt.Sprintf("NV%d", cell.Lanes)
	}
	if cell.Type == topo.LinkSelf {
		return "X"
	}
	return string(cell.Type)
}

func renderTopoTable(w io.Writer, result *topo.Result) error {
	var builder strings.Builder
	builder.WriteString("Connectivity Matrix\n")
	tw := tabwriter.NewWriter(&builder, 0, 0, 2, ' ', 0)

	header := "\t" + strings.Join(result.Matrix.GPUs, "\t")
	if _, err := fmt.Fprintln(tw, header); err != nil {
		return err
	}
	if err := writeTopoRows(tw, result); err != nil {
		return err
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	builder.WriteString("\nAffinity Advice\n")
	atw := tabwriter.NewWriter(&builder, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(atw, "GPU\tNIC\tLink\tRating"); err != nil {
		return err
	}
	for _, advice := range result.Advice {
		if _, err := fmt.Fprintf(atw, "%s\t%s\t%s\t%s\n", advice.GPU, advice.NIC, advice.Link, advice.Rating); err != nil {
			return err
		}
	}
	if err := atw.Flush(); err != nil {
		return err
	}

	_, err := io.WriteString(w, builder.String())
	return err
}

// writeTopoRows writes GPU rows followed by NIC rows to the matrix tabwriter.
func writeTopoRows(tw io.Writer, result *topo.Result) error {
	for _, gpu := range result.Matrix.GPUs {
		cells := make([]string, 0, len(result.Matrix.GPUs))
		for _, col := range result.Matrix.GPUs {
			cells = append(cells, cellText(result.Matrix.Cells[gpu][col]))
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\n", gpu, strings.Join(cells, "\t")); err != nil {
			return err
		}
	}
	for _, nic := range result.Matrix.NICs {
		cells := make([]string, 0, len(result.Matrix.GPUs))
		for _, col := range result.Matrix.GPUs {
			cells = append(cells, cellText(nic.PerGPU[col]))
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\n", nic.NIC, strings.Join(cells, "\t")); err != nil {
			return err
		}
	}
	return nil
}

func renderTopoMarkdown(w io.Writer, result *topo.Result) error {
	var builder strings.Builder
	fmt.Fprintln(&builder, "## Connectivity Matrix")
	fmt.Fprintln(&builder)

	fmt.Fprintf(&builder, "| | %s |\n", strings.Join(result.Matrix.GPUs, " | "))
	separators := make([]string, 0, len(result.Matrix.GPUs)+1)
	for range len(result.Matrix.GPUs) + 1 {
		separators = append(separators, "---")
	}
	fmt.Fprintf(&builder, "| %s |\n", strings.Join(separators, " | "))

	for _, gpu := range result.Matrix.GPUs {
		cells := make([]string, 0, len(result.Matrix.GPUs))
		for _, col := range result.Matrix.GPUs {
			cells = append(cells, cellText(result.Matrix.Cells[gpu][col]))
		}
		fmt.Fprintf(&builder, "| %s | %s |\n", gpu, strings.Join(cells, " | "))
	}
	for _, nic := range result.Matrix.NICs {
		cells := make([]string, 0, len(result.Matrix.GPUs))
		for _, col := range result.Matrix.GPUs {
			cells = append(cells, cellText(nic.PerGPU[col]))
		}
		fmt.Fprintf(&builder, "| %s | %s |\n", nic.NIC, strings.Join(cells, " | "))
	}

	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, "## Affinity Advice")
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, "| GPU | NIC | Link | Rating |")
	fmt.Fprintln(&builder, "| --- | --- | --- | --- |")
	for _, advice := range result.Advice {
		fmt.Fprintf(&builder, "| %s | %s | %s | %s |\n", advice.GPU, advice.NIC, advice.Link, advice.Rating)
	}

	_, err := io.WriteString(w, builder.String())
	return err
}

func init() {
	registerCommand(newTopoCmd)
}
