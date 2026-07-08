package bench

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

var ErrToolNotInstalled = errors.New("benchmark tool not installed")

type execRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type osExecRunner struct{}

func (r osExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("run %s: %w", name, err)
	}
	return out, nil
}

type Tool string

const (
	ToolGPUBurn       Tool = "gpu-burn"
	ToolNVBandwidth   Tool = "nvbandwidth"
	ToolBandwidthTest Tool = "bandwidthTest"
)

type BenchResult struct {
	Tool       string
	Duration   time.Duration
	Throughput float64
	Unit       string
	RawLog     string
}

var lookPath = func(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("look path %s: %w", name, err)
	}
	return path, nil
}

var (
	gpuBurnThroughputPattern       = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*GFLOP/s`)
	bandwidthThroughputPattern     = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*GB/s`)
	bandwidthTestDefaultArgs       = []string{"--mode=quick", "--memory=pinned"}
	nvBandwidthDefaultArgs         = []string{}
	defaultRunner              any = osExecRunner{}
)

func Run(ctx context.Context, runner execRunner, tool Tool, duration time.Duration) (*BenchResult, error) {
	if !IsKnownTool(tool) {
		return nil, fmt.Errorf("unknown benchmark tool %q", tool)
	}
	if duration <= 0 {
		return nil, fmt.Errorf("benchmark duration must be positive")
	}
	if runner == nil {
		runner = defaultRunner.(execRunner)
	}
	path, err := lookPath(string(tool))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrToolNotInstalled, tool)
	}

	args := argsForTool(tool, duration)
	out, err := runner.Run(ctx, path, args...)
	if err != nil {
		return nil, fmt.Errorf("run benchmark tool %s: %w", tool, err)
	}
	return parseResult(tool, duration, string(out)), nil
}

func IsKnownTool(tool Tool) bool {
	switch tool {
	case ToolGPUBurn, ToolNVBandwidth, ToolBandwidthTest:
		return true
	default:
		return false
	}
}

func argsForTool(tool Tool, duration time.Duration) []string {
	switch tool {
	case ToolGPUBurn:
		seconds := int(duration / time.Second)
		if seconds < 1 {
			seconds = 1
		}
		return []string{strconv.Itoa(seconds)}
	case ToolNVBandwidth:
		return append([]string(nil), nvBandwidthDefaultArgs...)
	case ToolBandwidthTest:
		return append([]string(nil), bandwidthTestDefaultArgs...)
	default:
		return nil
	}
}

func parseResult(tool Tool, duration time.Duration, rawLog string) *BenchResult {
	result := &BenchResult{Tool: string(tool), Duration: duration, RawLog: rawLog}
	switch tool {
	case ToolGPUBurn:
		result.Throughput, result.Unit = parseThroughput(rawLog, gpuBurnThroughputPattern, "GFLOP/s")
	case ToolNVBandwidth, ToolBandwidthTest:
		result.Throughput, result.Unit = parseThroughput(rawLog, bandwidthThroughputPattern, "GB/s")
	}
	return result
}

func parseThroughput(rawLog string, pattern *regexp.Regexp, unit string) (float64, string) {
	matches := pattern.FindStringSubmatch(rawLog)
	if len(matches) != 2 {
		return 0, ""
	}
	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, ""
	}
	return value, unit
}
