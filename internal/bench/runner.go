package bench

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

var ErrToolNotInstalled = errors.New("benchmark tool not installed")

type execRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type ExecResult struct {
	Stdout []byte
	Stderr []byte
}

type execRunnerV2 interface {
	RunResult(ctx context.Context, env []string, name string, args ...string) (ExecResult, error)
}

type osExecRunner struct{}

func (r osExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("run %s: %w", name, err)
	}
	return out, nil
}

type osExecRunnerV2 struct{}

func (r osExecRunnerV2) RunResult(ctx context.Context, env []string, name string, args ...string) (ExecResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	err := cmd.Run()
	result := ExecResult{Stdout: outBuf.Bytes(), Stderr: errBuf.Bytes()}
	if err != nil {
		return result, fmt.Errorf("run %s: %w", name, err)
	}
	return result, nil
}

type Tool string

const (
	ToolGPUBurn       Tool = "gpu-burn"
	ToolNVBandwidth   Tool = "nvbandwidth"
	ToolBandwidthTest Tool = "bandwidthTest"
	ToolPerftest      Tool = "perftest"
	ToolNCCLTests     Tool = "nccl-tests"
)

type BenchResult struct {
	Tool       string
	Duration   time.Duration
	Throughput float64
	Unit       string
	RawLog     string
	GDRDMA     string `json:"gdrdma,omitempty"`
}

type Options struct {
	Duration  time.Duration
	Server    string
	UseCUDA   *int
	GPUs      int
	NCCLDebug bool
	ExtraArgs []string
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
	defaultRunnerV2            any = osExecRunnerV2{}
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

func RunWithOptions(ctx context.Context, runner execRunnerV2, tool Tool, opts Options) (*BenchResult, error) {
	if !IsKnownTool(tool) {
		return nil, fmt.Errorf("unknown benchmark tool %q", tool)
	}
	if runner == nil {
		runner = defaultRunnerV2.(execRunnerV2)
	}

	switch tool {
	case ToolGPUBurn, ToolNVBandwidth, ToolBandwidthTest:
		return runLegacyWithOptions(ctx, runner, tool, opts)
	case ToolPerftest:
		return runPerftest(ctx, runner, opts)
	case ToolNCCLTests:
		return runNCCLTests(ctx, runner, opts)
	default:
		return nil, fmt.Errorf("unknown benchmark tool %q", tool)
	}
}

func IsKnownTool(tool Tool) bool {
	switch tool {
	case ToolGPUBurn, ToolNVBandwidth, ToolBandwidthTest, ToolPerftest, ToolNCCLTests:
		return true
	default:
		return false
	}
}

func argsForTool(tool Tool, duration time.Duration) []string {
	switch tool {
	case ToolGPUBurn:
		seconds := int(duration / time.Second)
		seconds = max(seconds, 1)
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
