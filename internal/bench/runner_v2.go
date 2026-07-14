package bench

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	perftestDataLinePattern = regexp.MustCompile(`(?m)^\s*\d+\s+\d+\s+[0-9]+(?:\.[0-9]+)?\s+([0-9]+(?:\.[0-9]+)?)\s+[0-9]+(?:\.[0-9]+)?(?:\s|$)`)
	ncclTestsDefaultArgs    = []string{"-b", "8", "-e", "8G", "-f", "2"}
)

func runLegacyWithOptions(ctx context.Context, runner execRunnerV2, tool Tool, opts Options) (*BenchResult, error) {
	if opts.Duration <= 0 {
		return nil, fmt.Errorf("benchmark duration must be positive")
	}
	path, err := lookPath(string(tool))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrToolNotInstalled, tool)
	}

	result, err := runner.RunResult(ctx, nil, path, argsForTool(tool, opts.Duration)...)
	if err != nil {
		return nil, fmt.Errorf("run benchmark tool %s: %w", tool, err)
	}
	return parseResult(tool, opts.Duration, string(result.Stdout)), nil
}

func runPerftest(ctx context.Context, runner execRunnerV2, opts Options) (*BenchResult, error) {
	if opts.Server == "" {
		return nil, errors.New("perftest requires --server")
	}
	path, err := lookPath("ib_write_bw")
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrToolNotInstalled, ToolPerftest)
	}

	args := []string{opts.Server}
	if opts.UseCUDA != nil {
		args = append(args, fmt.Sprintf("--use_cuda=%d", *opts.UseCUDA))
	}
	args = append(args, opts.ExtraArgs...)
	result, err := runner.RunResult(ctx, nil, path, args...)
	if err != nil {
		return nil, fmt.Errorf("run benchmark tool %s: %w", ToolPerftest, err)
	}
	benchResult := &BenchResult{Tool: string(ToolPerftest), Duration: opts.Duration, RawLog: string(result.Stdout)}
	benchResult.Throughput, benchResult.Unit = parsePerftestThroughput(benchResult.RawLog)
	return benchResult, nil
}

func runNCCLTests(ctx context.Context, runner execRunnerV2, opts Options) (*BenchResult, error) {
	path, err := lookPath("all_reduce_perf")
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrToolNotInstalled, ToolNCCLTests)
	}

	gpus := opts.GPUs
	if gpus == 0 {
		gpus = 8
	}
	args := append([]string(nil), ncclTestsDefaultArgs...)
	args = append(args, "-g", strconv.Itoa(gpus))
	args = append(args, opts.ExtraArgs...)
	var env []string
	if opts.NCCLDebug {
		env = []string{"NCCL_DEBUG=INFO"}
	}
	result, err := runner.RunResult(ctx, env, path, args...)
	if err != nil {
		return nil, fmt.Errorf("run benchmark tool %s: %w", ToolNCCLTests, err)
	}
	benchResult := &BenchResult{Tool: string(ToolNCCLTests), Duration: opts.Duration, RawLog: string(result.Stdout)}
	benchResult.Throughput, benchResult.Unit = parseNCCLTestsThroughput(benchResult.RawLog)
	if opts.NCCLDebug {
		benchResult.GDRDMA = parseGDRDMAStatus(string(result.Stderr))
	}
	return benchResult, nil
}

// parsePerftestThroughput takes the last ib_write_bw data row's BW average column.
func parsePerftestThroughput(rawLog string) (float64, string) {
	matches := perftestDataLinePattern.FindAllStringSubmatch(rawLog, -1)
	if len(matches) == 0 {
		return 0, ""
	}
	value, err := strconv.ParseFloat(matches[len(matches)-1][1], 64)
	if err != nil {
		return 0, ""
	}
	return value, "Gb/s"
}

// parseNCCLTestsThroughput takes the max-size all_reduce_perf row's busbw column.
func parseNCCLTestsThroughput(rawLog string) (float64, string) {
	var maxSize uint64
	var busBandwidth float64
	found := false
	for line := range strings.SplitSeq(rawLog, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		size, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		value, err := strconv.ParseFloat(fields[6], 64)
		if err != nil {
			continue
		}
		if !found || size > maxSize {
			found = true
			maxSize = size
			busBandwidth = value
		}
	}
	if !found {
		return 0, ""
	}
	return busBandwidth, "GB/s"
}

func parseGDRDMAStatus(rawLog string) string {
	lowerLog := strings.ToLower(rawLog)
	hasGDRDMA := strings.Contains(lowerLog, "gdrdma") || strings.Contains(lowerLog, "gpu direct rdma")
	if hasGDRDMA && strings.Contains(lowerLog, "net/ib") {
		return "true"
	}
	if strings.Contains(lowerLog, "net/socket") && !hasGDRDMA {
		return "false"
	}
	return "unknown"
}
