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
	benchToolFlag      = "tool"
	benchDurationFlag  = "duration"
	benchServerFlag    = "server"
	benchUseCUDAFlag   = "use-cuda"
	benchGPUsFlag      = "gpus"
	benchNCCLDebugFlag = "nccl-debug"
	benchExtraArgsFlag = "extra-args"
)

var benchRun = func(ctx context.Context, tool bench.Tool, duration time.Duration) (*bench.BenchResult, error) {
	return bench.Run(ctx, nil, tool, duration)
}

var benchRunOptions = func(ctx context.Context, tool bench.Tool, opts bench.Options) (*bench.BenchResult, error) {
	return bench.RunWithOptions(ctx, nil, tool, opts)
}

type benchFlags struct {
	tool      string
	duration  time.Duration
	server    string
	useCUDA   int
	gpus      int
	ncclDebug bool
	extraArgs []string
}

func newBenchCmd() *cobra.Command {
	flags := &benchFlags{}
	cmd := &cobra.Command{
		Use:   "bench",
		Short: "Run an external GPU benchmark tool",
		Long:  "Run a supported external GPU benchmark tool and render the parsed result.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBench(cmd, flags)
		},
	}
	cmd.Flags().StringVar(&flags.tool, benchToolFlag, string(bench.ToolGPUBurn), "benchmark tool: gpu-burn, nvbandwidth, bandwidthTest, perftest, or nccl-tests")
	cmd.Flags().DurationVar(&flags.duration, benchDurationFlag, 60*time.Second, "benchmark duration")
	cmd.Flags().StringVar(&flags.server, benchServerFlag, "", "perftest server address (required for --tool perftest)")
	cmd.Flags().IntVar(&flags.useCUDA, benchUseCUDAFlag, 0, "perftest CUDA device index (--tool perftest)")
	cmd.Flags().IntVar(&flags.gpus, benchGPUsFlag, 8, "nccl-tests GPU count (--tool nccl-tests)")
	cmd.Flags().BoolVar(&flags.ncclDebug, benchNCCLDebugFlag, false, "nccl-tests NCCL_DEBUG=INFO with GDRDMA detection (--tool nccl-tests)")
	cmd.Flags().StringArrayVar(&flags.extraArgs, benchExtraArgsFlag, nil, "extra benchmark arguments, repeatable (--tool perftest or nccl-tests)")
	return cmd
}

func runBench(cmd *cobra.Command, flags *benchFlags) error {
	cfg, err := resolvedConfig(cmd)
	if err != nil {
		return err
	}
	tool := bench.Tool(flags.tool)
	if !bench.IsKnownTool(tool) {
		return fmt.Errorf("unknown benchmark tool %q", flags.tool)
	}
	if err = validateBenchFlags(cmd, tool); err != nil {
		return err
	}
	if tool == bench.ToolPerftest || tool == bench.ToolNCCLTests {
		if !platformIsLinux() {
			return benchUnsupported(cmd, tool)
		}
	}
	result, err := dispatchBench(cmd, tool, flags)
	if err != nil {
		if errors.Is(err, bench.ErrToolNotInstalled) {
			return &ExitError{Code: 2, Err: fmt.Errorf("benchmark tool %q not installed, install it and retry", flags.tool)}
		}
		return NewExitError(1, err)
	}
	if result == nil {
		return NewExitError(1, fmt.Errorf("benchmark tool %q returned no result", flags.tool))
	}
	if err := renderBenchResult(cmd.OutOrStdout(), cfg.DefaultOutput, result); err != nil {
		return fmt.Errorf("render benchmark result: %w", err)
	}
	return nil
}

func validateBenchFlags(cmd *cobra.Command, tool bench.Tool) error {
	newFlags := []struct {
		name          string
		validPerftest bool
		validNCCL     bool
	}{
		{benchServerFlag, true, false},
		{benchUseCUDAFlag, true, false},
		{benchGPUsFlag, false, true},
		{benchNCCLDebugFlag, false, true},
		{benchExtraArgsFlag, true, true},
	}
	for _, f := range newFlags {
		if !cmd.Flags().Changed(f.name) {
			continue
		}
		switch tool {
		case bench.ToolPerftest:
			if !f.validPerftest {
				return NewExitError(1, fmt.Errorf("--%s is only valid with --tool nccl-tests", f.name))
			}
		case bench.ToolNCCLTests:
			if !f.validNCCL {
				return NewExitError(1, fmt.Errorf("--%s is only valid with --tool perftest", f.name))
			}
		default:
			return NewExitError(1, fmt.Errorf("--%s is only valid with --tool perftest or nccl-tests", f.name))
		}
	}
	return nil
}

// benchUnsupported handles the non-Linux path for the Linux-only bench tools
// (perftest, nccl-tests). For JSON output it emits the small
// unsupported-platform object to stdout; for every output mode it returns an
// exit-2 error carrying the clean message. It mirrors topoUnsupported.
func benchUnsupported(cmd *cobra.Command, tool bench.Tool) error {
	osName := platformOS()
	binary := benchRequiredBinary(tool)
	output, _ := cmd.Flags().GetString(outputFlag)
	if output == core.OutputJSON {
		payload := map[string]any{
			"supported":      false,
			"platform":       osName,
			"reason":         "requires Linux",
			"required_tools": []string{binary},
		}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(payload); err != nil {
			return NewExitError(2, fmt.Errorf("encode unsupported-platform payload: %w", err))
		}
	}
	return &ExitError{Code: 2, Err: fmt.Errorf("gpu-tools bench --tool %s requires Linux (uses %s); current OS: %s", tool, binary, osName)}
}

// benchRequiredBinary maps a Linux-only bench tool to the external binary it wraps.
func benchRequiredBinary(tool bench.Tool) string {
	if tool == bench.ToolNCCLTests {
		return "all_reduce_perf"
	}
	return "ib_write_bw"
}

func dispatchBench(cmd *cobra.Command, tool bench.Tool, flags *benchFlags) (*bench.BenchResult, error) {
	switch tool {
	case bench.ToolPerftest, bench.ToolNCCLTests:
		opts := bench.Options{
			Duration:  flags.duration,
			Server:    flags.server,
			GPUs:      flags.gpus,
			NCCLDebug: flags.ncclDebug,
			ExtraArgs: flags.extraArgs,
		}
		if cmd.Flags().Changed(benchUseCUDAFlag) {
			useCUDA := flags.useCUDA
			opts.UseCUDA = &useCUDA
		}
		return benchRunOptions(cmd.Context(), tool, opts)
	default:
		return benchRun(cmd.Context(), tool, flags.duration)
	}
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
	header := "Tool\tDuration\tThroughput\tRawLogBytes"
	row := fmt.Sprintf("%s\t%s\t%s\t%d", result.Tool, result.Duration, formatBenchThroughput(result), len(result.RawLog))
	if result.GDRDMA != "" {
		header += "\tGDRDMA"
		row += "\t" + result.GDRDMA
	}
	if _, err := fmt.Fprintln(tw, header); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(tw, row); err != nil {
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
	if result.GDRDMA != "" {
		fmt.Fprintf(&builder, "- GDRDMA: `%s`\n", result.GDRDMA)
	}
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
