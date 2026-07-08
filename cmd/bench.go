package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/sunerpy/gpu-tools/core"
	"github.com/sunerpy/gpu-tools/internal/bench"
)

const (
	benchToolFlag     = "tool"
	benchDurationFlag = "duration"
)

var benchRun = func(ctx context.Context, tool bench.Tool, duration time.Duration) (*bench.BenchResult, error) {
	return bench.Run(ctx, nil, tool, duration)
}

func newBenchCmd() *cobra.Command {
	var toolFlag string
	var duration time.Duration
	cmd := &cobra.Command{
		Use:   "bench",
		Short: "Run an external GPU benchmark tool",
		Long:  "Run a supported external GPU benchmark tool and render the parsed result.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBench(cmd, toolFlag, duration)
		},
	}
	cmd.Flags().StringVar(&toolFlag, benchToolFlag, string(bench.ToolGPUBurn), "benchmark tool: gpu-burn, nvbandwidth, or bandwidthTest")
	cmd.Flags().DurationVar(&duration, benchDurationFlag, 60*time.Second, "benchmark duration")
	return cmd
}

func runBench(cmd *cobra.Command, toolFlag string, duration time.Duration) error {
	cfg, err := resolvedConfig(cmd)
	if err != nil {
		return err
	}
	tool := bench.Tool(toolFlag)
	if !bench.IsKnownTool(tool) {
		return fmt.Errorf("unknown benchmark tool %q", toolFlag)
	}
	result, err := benchRun(cmd.Context(), tool, duration)
	if err != nil {
		if errors.Is(err, bench.ErrToolNotInstalled) {
			return &ExitError{Code: 2, Err: fmt.Errorf("benchmark tool %q not installed, install it and retry", toolFlag)}
		}
		return NewExitError(1, err)
	}
	if result == nil {
		return NewExitError(1, fmt.Errorf("benchmark tool %q returned no result", toolFlag))
	}
	if err := renderBenchResult(cmd.OutOrStdout(), cfg.DefaultOutput, result); err != nil {
		return fmt.Errorf("render benchmark result: %w", err)
	}
	return nil
}

func renderBenchResult(w io.Writer, output string, result *bench.BenchResult) error {
	switch output {
	case core.OutputTable:
		return renderBenchTable(w, result)
	case core.OutputJSON:
		return renderBenchJSON(w, result)
	case core.OutputMarkdown:
		return renderBenchMarkdown(w, result)
	default:
		return fmt.Errorf("unknown benchmark output format %q", output)
	}
}

func renderBenchTable(w io.Writer, result *bench.BenchResult) error {
	var builder strings.Builder
	tw := tabwriter.NewWriter(&builder, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "Tool\tDuration\tThroughput\tRawLogBytes"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", result.Tool, result.Duration, formatBenchThroughput(result), len(result.RawLog)); err != nil {
		return err
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := io.WriteString(w, builder.String())
	return err
}

func renderBenchJSON(w io.Writer, result *bench.BenchResult) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func renderBenchMarkdown(w io.Writer, result *bench.BenchResult) error {
	var builder strings.Builder
	fmt.Fprintln(&builder, "## Benchmark Result")
	fmt.Fprintln(&builder)
	fmt.Fprintf(&builder, "- Tool: `%s`\n", result.Tool)
	fmt.Fprintf(&builder, "- Duration: `%s`\n", result.Duration)
	fmt.Fprintf(&builder, "- Throughput: `%s`\n", formatBenchThroughput(result))
	fmt.Fprintf(&builder, "- RawLogBytes: `%d`\n", len(result.RawLog))
	_, err := io.WriteString(w, builder.String())
	return err
}

func formatBenchThroughput(result *bench.BenchResult) string {
	if result.Unit == "" {
		return fmt.Sprintf("%.2f", result.Throughput)
	}
	return fmt.Sprintf("%.2f %s", result.Throughput, result.Unit)
}

func init() {
	registerCommand(newBenchCmd)
}
