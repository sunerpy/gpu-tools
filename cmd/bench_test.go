package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sunerpy/gpu-tools/internal/bench"
)

func TestBenchCommand_returnsExitCode2_whenBenchmarkToolIsMissing(t *testing.T) {
	// Given
	overrideBenchRun(t, func(context.Context, bench.Tool, time.Duration) (*bench.BenchResult, error) {
		return nil, bench.ErrToolNotInstalled
	})
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "bench", "--tool", "gpu-burn")

	// Then
	if err == nil {
		t.Fatalf("expected missing benchmark tool to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %d", exitErr.Code)
	}
	if !strings.Contains(err.Error(), "benchmark tool \"gpu-burn\" not installed") {
		t.Fatalf("expected install hint, got %q", err.Error())
	}
}

func TestBenchCommand_returnsError_whenToolIsUnknown(t *testing.T) {
	// Given
	called := false
	overrideBenchRun(t, func(context.Context, bench.Tool, time.Duration) (*bench.BenchResult, error) {
		called = true
		return nil, nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "bench", "--tool", "bogus")

	// Then
	if err == nil {
		t.Fatalf("expected unknown tool to fail")
	}
	if !strings.Contains(err.Error(), "unknown benchmark tool \"bogus\"") {
		t.Fatalf("expected unknown tool error, got %q", err.Error())
	}
	if called {
		t.Fatalf("expected benchRun not to be called for unknown tools")
	}
}

func TestBenchCommand_rendersTable_whenBenchmarkSucceeds(t *testing.T) {
	// Given
	overrideBenchRun(t, func(_ context.Context, tool bench.Tool, duration time.Duration) (*bench.BenchResult, error) {
		if tool != bench.ToolGPUBurn {
			t.Fatalf("expected gpu-burn tool, got %q", tool)
		}
		if duration != 5*time.Second {
			t.Fatalf("expected duration 5s, got %s", duration)
		}
		return &bench.BenchResult{Tool: string(tool), Duration: duration, Throughput: 123.45, Unit: "GFLOP/s", RawLog: "raw output"}, nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, stderr, err := executeCommand(newRootCmd(), "bench", "--tool", "gpu-burn", "--duration", "5s")
	// Then
	if err != nil {
		t.Fatalf("expected bench table to succeed: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	for _, want := range []string{"gpu-burn", "5s", "123.45", "GFLOP/s"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected table output to contain %q, got:\n%s", want, stdout)
		}
	}
}

func TestBenchCommand_rendersJSON_whenJSONOutputRequested(t *testing.T) {
	// Given
	overrideBenchRun(t, func(_ context.Context, tool bench.Tool, duration time.Duration) (*bench.BenchResult, error) {
		return &bench.BenchResult{Tool: string(tool), Duration: duration, Throughput: 456.75, Unit: "GB/s", RawLog: "json raw"}, nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, stderr, err := executeCommand(newRootCmd(), "bench", "--tool", "nvbandwidth", "--duration", "7s", "--output", "json")
	// Then
	if err != nil {
		t.Fatalf("expected bench json to succeed: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	var result bench.BenchResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("expected valid JSON result, got error %v and output:\n%s", err, stdout)
	}
	if result.Tool != "nvbandwidth" || result.Throughput != 456.75 || result.Unit != "GB/s" || result.RawLog != "json raw" {
		t.Fatalf("unexpected JSON result: %#v", result)
	}
}

func TestBenchCommand_returnsExitCode1_whenBenchmarkRunFails(t *testing.T) {
	// Given
	overrideBenchRun(t, func(context.Context, bench.Tool, time.Duration) (*bench.BenchResult, error) {
		return nil, errors.New("benchmark failed")
	})
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "bench", "--tool", "bandwidthTest")

	// Then
	if err == nil {
		t.Fatalf("expected benchmark failure")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.Code)
	}
}

func TestBenchCommand_returnsExitCode1_whenBenchmarkReturnsNilResult(t *testing.T) {
	// Given
	overrideBenchRun(t, func(context.Context, bench.Tool, time.Duration) (*bench.BenchResult, error) {
		return nil, nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "bench", "--tool", "gpu-burn")

	// Then
	if err == nil {
		t.Fatalf("expected nil benchmark result to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.Code)
	}
	if !strings.Contains(err.Error(), "returned no result") {
		t.Fatalf("expected nil result message, got %q", err.Error())
	}
}

func TestBenchCommand_rendersMarkdown_whenMarkdownOutputRequested(t *testing.T) {
	// Given
	overrideBenchRun(t, func(_ context.Context, tool bench.Tool, duration time.Duration) (*bench.BenchResult, error) {
		return &bench.BenchResult{Tool: string(tool), Duration: duration, Throughput: 88.5, Unit: "GB/s", RawLog: "markdown raw"}, nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "bench", "--tool", "bandwidthTest", "--duration", "9s", "--output", "markdown")
	// Then
	if err != nil {
		t.Fatalf("expected bench markdown to succeed: %v", err)
	}
	for _, want := range []string{"## Benchmark Result", "bandwidthTest", "88.50 GB/s"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected markdown output to contain %q, got:\n%s", want, stdout)
		}
	}
}

func TestBenchCommand_returnsRenderError_whenOutputWriterFails(t *testing.T) {
	// Given
	overrideBenchRun(t, func(_ context.Context, tool bench.Tool, duration time.Duration) (*bench.BenchResult, error) {
		return &bench.BenchResult{Tool: string(tool), Duration: duration, Throughput: 1, Unit: "GB/s", RawLog: "raw"}, nil
	})
	t.Setenv("HOME", t.TempDir())
	root := newRootCmd()
	root.SetOut(failingWriter{})
	root.SetArgs([]string{"bench", "--tool", "nvbandwidth"})

	// When
	err := root.Execute()

	// Then
	if err == nil {
		t.Fatalf("expected render failure")
	}
	if !strings.Contains(err.Error(), "render benchmark result") {
		t.Fatalf("expected render error, got %q", err.Error())
	}
}

func TestBenchRenderer_returnsError_whenOutputFormatIsUnknown(t *testing.T) {
	// Given
	result := &bench.BenchResult{Tool: "gpu-burn", Duration: time.Second}

	// When
	err := renderBenchResult(failingWriter{}, "xml", result)

	// Then
	if err == nil {
		t.Fatalf("expected unknown output format to fail")
	}
	if !strings.Contains(err.Error(), "unknown benchmark output format \"xml\"") {
		t.Fatalf("expected unknown format error, got %q", err.Error())
	}
}

func TestRenderBenchTable_returnsWriterError_whenOutputWriterFails(t *testing.T) {
	// Given
	result := &bench.BenchResult{Tool: "gpu-burn", Duration: time.Second, Throughput: 42.5, Unit: "GB/s", RawLog: "raw"}

	// When
	err := renderBenchTable(failingWriter{}, result)

	// Then
	if err == nil {
		t.Fatalf("expected writer failure")
	}
	if !strings.Contains(err.Error(), "writer failed") {
		t.Fatalf("expected writer failure, got %q", err.Error())
	}
}

func TestRenderBenchTable_returnsWriterError_whenGDRDMASetAndWriterFails(t *testing.T) {
	// Given
	result := &bench.BenchResult{Tool: "nccl-tests", Duration: time.Second, Throughput: 1, Unit: "GB/s", RawLog: "raw", GDRDMA: "true"}

	// When
	err := renderBenchTable(failingWriter{}, result)

	// Then
	if err == nil {
		t.Fatalf("expected writer failure")
	}
}

func TestRenderBenchTable_rendersGDRDMAColumn_whenSet(t *testing.T) {
	// Given
	result := &bench.BenchResult{Tool: "nccl-tests", Duration: time.Second, Throughput: 1, Unit: "GB/s", RawLog: "raw", GDRDMA: "unknown"}
	var buf strings.Builder

	// When
	if err := renderBenchTable(&buf, result); err != nil {
		t.Fatalf("expected table render to succeed: %v", err)
	}

	// Then
	if !strings.Contains(buf.String(), "GDRDMA") || !strings.Contains(buf.String(), "unknown") {
		t.Fatalf("expected GDRDMA column, got:\n%s", buf.String())
	}
}

func TestFormatBenchThroughput_omitsUnit_whenUnitIsEmpty(t *testing.T) {
	// Given
	result := &bench.BenchResult{Throughput: 12.345}

	// When
	got := formatBenchThroughput(result)

	// Then
	if got != "12.35" {
		t.Fatalf("formatBenchThroughput = %q, want %q", got, "12.35")
	}
}

func TestBenchCommand_regressionLegacyTable_hasNoGDRDMAColumn(t *testing.T) {
	// Given
	overrideBenchRun(t, func(_ context.Context, tool bench.Tool, duration time.Duration) (*bench.BenchResult, error) {
		return &bench.BenchResult{Tool: string(tool), Duration: duration, Throughput: 10, Unit: "GB/s", RawLog: "raw"}, nil
	})
	failIfBenchRunOptionsCalled(t)
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "bench", "--tool", "gpu-burn", "--duration", "5s")
	// Then
	if err != nil {
		t.Fatalf("expected legacy bench to succeed: %v", err)
	}
	if strings.Contains(stdout, "GDRDMA") {
		t.Fatalf("expected no GDRDMA column for legacy tool, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Tool\t") && !strings.Contains(stdout, "Tool ") {
		t.Fatalf("expected legacy table header, got:\n%s", stdout)
	}
}

func TestBenchCommand_regressionLegacyJSON_hasNoGDRDMAField(t *testing.T) {
	// Given
	overrideBenchRun(t, func(_ context.Context, tool bench.Tool, duration time.Duration) (*bench.BenchResult, error) {
		return &bench.BenchResult{Tool: string(tool), Duration: duration, Throughput: 20, Unit: "GB/s", RawLog: "raw"}, nil
	})
	failIfBenchRunOptionsCalled(t)
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "bench", "--tool", "nvbandwidth", "--output", "json")
	// Then
	if err != nil {
		t.Fatalf("expected legacy json to succeed: %v", err)
	}
	if strings.Contains(stdout, "gdrdma") {
		t.Fatalf("expected no gdrdma field for legacy tool, got:\n%s", stdout)
	}
}

func TestBenchCommand_perftest_passesOptions(t *testing.T) {
	// Given
	var gotTool bench.Tool
	var gotOpts bench.Options
	overrideBenchRunOptions(t, func(_ context.Context, tool bench.Tool, opts bench.Options) (*bench.BenchResult, error) {
		gotTool = tool
		gotOpts = opts
		return &bench.BenchResult{Tool: string(tool), Duration: opts.Duration, Throughput: 100, Unit: "Gb/s", RawLog: "raw"}, nil
	})
	failIfBenchRunCalled(t)
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "bench", "--tool", "perftest", "--server", "1.2.3.4", "--use-cuda", "0", "--extra-args", "-s", "--extra-args", "65536")
	// Then
	if err != nil {
		t.Fatalf("expected perftest to succeed: %v", err)
	}
	if gotTool != bench.ToolPerftest {
		t.Fatalf("expected perftest tool, got %q", gotTool)
	}
	if gotOpts.Server != "1.2.3.4" {
		t.Fatalf("expected Server 1.2.3.4, got %q", gotOpts.Server)
	}
	if gotOpts.UseCUDA == nil || *gotOpts.UseCUDA != 0 {
		t.Fatalf("expected UseCUDA=0, got %v", gotOpts.UseCUDA)
	}
	if len(gotOpts.ExtraArgs) != 2 || gotOpts.ExtraArgs[0] != "-s" || gotOpts.ExtraArgs[1] != "65536" {
		t.Fatalf("expected ExtraArgs [-s 65536], got %v", gotOpts.ExtraArgs)
	}
}

func TestBenchCommand_perftest_useCUDANotSet_leavesUseCUDANil(t *testing.T) {
	// Given
	var gotOpts bench.Options
	overrideBenchRunOptions(t, func(_ context.Context, tool bench.Tool, opts bench.Options) (*bench.BenchResult, error) {
		gotOpts = opts
		return &bench.BenchResult{Tool: string(tool), Duration: opts.Duration, RawLog: "raw"}, nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "bench", "--tool", "perftest", "--server", "1.2.3.4")
	// Then
	if err != nil {
		t.Fatalf("expected perftest to succeed: %v", err)
	}
	if gotOpts.UseCUDA != nil {
		t.Fatalf("expected UseCUDA nil when flag not set, got %v", *gotOpts.UseCUDA)
	}
}

func TestBenchCommand_perftest_missingServer_returnsExitCode1(t *testing.T) {
	// Given
	overrideBenchRunOptions(t, func(_ context.Context, _ bench.Tool, _ bench.Options) (*bench.BenchResult, error) {
		return nil, errors.New("perftest requires --server")
	})
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "bench", "--tool", "perftest")

	// Then
	if err == nil {
		t.Fatalf("expected missing server to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.Code)
	}
}

func TestBenchCommand_perftest_missingBinary_returnsExitCode2(t *testing.T) {
	// Given
	overrideBenchRunOptions(t, func(_ context.Context, _ bench.Tool, _ bench.Options) (*bench.BenchResult, error) {
		return nil, bench.ErrToolNotInstalled
	})
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "bench", "--tool", "perftest", "--server", "1.2.3.4")

	// Then
	if err == nil {
		t.Fatalf("expected missing binary to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %d", exitErr.Code)
	}
}

func TestBenchCommand_ncclTests_passesOptions_andRendersGDRDMA(t *testing.T) {
	// Given
	var gotOpts bench.Options
	overrideBenchRunOptions(t, func(_ context.Context, tool bench.Tool, opts bench.Options) (*bench.BenchResult, error) {
		gotOpts = opts
		return &bench.BenchResult{Tool: string(tool), Duration: opts.Duration, Throughput: 42, Unit: "GB/s", RawLog: "raw", GDRDMA: "true"}, nil
	})
	failIfBenchRunCalled(t)
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "bench", "--tool", "nccl-tests", "--gpus", "4", "--nccl-debug")
	// Then
	if err != nil {
		t.Fatalf("expected nccl-tests to succeed: %v", err)
	}
	if gotOpts.GPUs != 4 {
		t.Fatalf("expected GPUs 4, got %d", gotOpts.GPUs)
	}
	if !gotOpts.NCCLDebug {
		t.Fatalf("expected NCCLDebug true")
	}
	if !strings.Contains(stdout, "GDRDMA") || !strings.Contains(stdout, "true") {
		t.Fatalf("expected GDRDMA line in table, got:\n%s", stdout)
	}
}

func TestBenchCommand_ncclTests_defaultGPUs_isEight(t *testing.T) {
	// Given
	var gotOpts bench.Options
	overrideBenchRunOptions(t, func(_ context.Context, tool bench.Tool, opts bench.Options) (*bench.BenchResult, error) {
		gotOpts = opts
		return &bench.BenchResult{Tool: string(tool), Duration: opts.Duration, RawLog: "raw"}, nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "bench", "--tool", "nccl-tests")
	// Then
	if err != nil {
		t.Fatalf("expected nccl-tests to succeed: %v", err)
	}
	if gotOpts.GPUs != 8 {
		t.Fatalf("expected default GPUs 8, got %d", gotOpts.GPUs)
	}
}

func TestBenchCommand_ncclTests_rendersGDRDMAInMarkdown(t *testing.T) {
	// Given
	overrideBenchRunOptions(t, func(_ context.Context, tool bench.Tool, opts bench.Options) (*bench.BenchResult, error) {
		return &bench.BenchResult{Tool: string(tool), Duration: opts.Duration, Throughput: 5, Unit: "GB/s", RawLog: "raw", GDRDMA: "false"}, nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "bench", "--tool", "nccl-tests", "--nccl-debug", "--output", "markdown")
	// Then
	if err != nil {
		t.Fatalf("expected nccl-tests markdown to succeed: %v", err)
	}
	if !strings.Contains(stdout, "- GDRDMA: `false`") {
		t.Fatalf("expected GDRDMA markdown line, got:\n%s", stdout)
	}
}

func TestBenchCommand_legacyToolWithNewFlag_returnsExitCode1(t *testing.T) {
	// Given
	failIfBenchRunCalled(t)
	failIfBenchRunOptionsCalled(t)
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "bench", "--tool", "gpu-burn", "--server", "x")

	// Then
	if err == nil {
		t.Fatalf("expected legacy tool with new flag to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.Code)
	}
	if !strings.Contains(err.Error(), "--server is only valid with --tool perftest or nccl-tests") {
		t.Fatalf("expected guard message, got %q", err.Error())
	}
}

func TestBenchCommand_perftestWithNCCLFlag_returnsExitCode1(t *testing.T) {
	// Given
	failIfBenchRunOptionsCalled(t)
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "bench", "--tool", "perftest", "--server", "1.2.3.4", "--gpus", "4")

	// Then
	if err == nil {
		t.Fatalf("expected perftest with nccl flag to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.Code)
	}
	if !strings.Contains(err.Error(), "--gpus is only valid with --tool nccl-tests") {
		t.Fatalf("expected guard message, got %q", err.Error())
	}
}

func TestBenchCommand_ncclTestsWithPerftestFlag_returnsExitCode1(t *testing.T) {
	// Given
	failIfBenchRunOptionsCalled(t)
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "bench", "--tool", "nccl-tests", "--server", "1.2.3.4")

	// Then
	if err == nil {
		t.Fatalf("expected nccl-tests with perftest flag to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.Code)
	}
	if !strings.Contains(err.Error(), "--server is only valid with --tool perftest") {
		t.Fatalf("expected guard message, got %q", err.Error())
	}
}

func TestBenchCommand_perftestHelp_listsNewFlags(t *testing.T) {
	// Given
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "bench", "--tool", "perftest", "--help")
	// Then
	if err != nil {
		t.Fatalf("expected perftest help to succeed: %v", err)
	}
	for _, want := range []string{"--server", "--use-cuda", "--extra-args", "perftest", "nccl-tests"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected help to contain %q, got:\n%s", want, stdout)
		}
	}
}

func TestBenchCommand_ncclTestsHelp_listsNewFlags(t *testing.T) {
	// Given
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "bench", "--tool", "nccl-tests", "--help")
	// Then
	if err != nil {
		t.Fatalf("expected nccl-tests help to succeed: %v", err)
	}
	for _, want := range []string{"--gpus", "--nccl-debug", "--extra-args"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected help to contain %q, got:\n%s", want, stdout)
		}
	}
}

func TestBenchCommand_ncclTests_returnsExitCode1_whenNilResult(t *testing.T) {
	// Given
	overrideBenchRunOptions(t, func(context.Context, bench.Tool, bench.Options) (*bench.BenchResult, error) {
		return nil, nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "bench", "--tool", "nccl-tests")

	// Then
	if err == nil {
		t.Fatalf("expected nil result to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.Code)
	}
}

func TestBenchCommand_perftest_returnsExitCode2AndMessage_whenNotLinux(t *testing.T) {
	// Given
	overridePlatform(t, false, "darwin")
	failIfBenchRunOptionsCalled(t)
	failIfBenchRunCalled(t)
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "bench", "--tool", "perftest", "--server", "1.2.3.4")

	// Then
	if err == nil {
		t.Fatalf("expected non-Linux perftest to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %d", exitErr.Code)
	}
	if !strings.Contains(err.Error(), "gpu-tools bench --tool perftest requires Linux (uses ib_write_bw); current OS: darwin") {
		t.Fatalf("expected linux-required message, got %q", err.Error())
	}
	if stdout != "" {
		t.Fatalf("expected no stdout for non-JSON unsupported platform, got %q", stdout)
	}
}

func TestBenchCommand_perftest_emitsUnsupportedJSON_whenNotLinuxAndOutputJSON(t *testing.T) {
	// Given
	overridePlatform(t, false, "darwin")
	failIfBenchRunOptionsCalled(t)
	failIfBenchRunCalled(t)
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "--output", "json", "bench", "--tool", "perftest", "--server", "1.2.3.4")

	// Then
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %d", exitErr.Code)
	}
	var payload struct {
		Supported     bool     `json:"supported"`
		Platform      string   `json:"platform"`
		Reason        string   `json:"reason"`
		RequiredTools []string `json:"required_tools"`
	}
	if derr := json.Unmarshal([]byte(stdout), &payload); derr != nil {
		t.Fatalf("expected valid JSON on stdout, got error %v for:\n%s", derr, stdout)
	}
	if payload.Supported {
		t.Fatalf("expected supported=false")
	}
	if payload.Platform != "darwin" {
		t.Fatalf("expected platform darwin, got %q", payload.Platform)
	}
	if payload.Reason != "requires Linux" {
		t.Fatalf("expected reason 'requires Linux', got %q", payload.Reason)
	}
	if len(payload.RequiredTools) != 1 || payload.RequiredTools[0] != "ib_write_bw" {
		t.Fatalf("expected required_tools [ib_write_bw], got %v", payload.RequiredTools)
	}
}

func TestBenchCommand_ncclTests_emitsUnsupportedJSON_whenNotLinuxAndOutputJSON(t *testing.T) {
	// Given
	overridePlatform(t, false, "darwin")
	failIfBenchRunOptionsCalled(t)
	failIfBenchRunCalled(t)
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "--output", "json", "bench", "--tool", "nccl-tests")

	// Then
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %d", exitErr.Code)
	}
	var payload struct {
		Supported     bool     `json:"supported"`
		Platform      string   `json:"platform"`
		Reason        string   `json:"reason"`
		RequiredTools []string `json:"required_tools"`
	}
	if derr := json.Unmarshal([]byte(stdout), &payload); derr != nil {
		t.Fatalf("expected valid JSON on stdout, got error %v for:\n%s", derr, stdout)
	}
	if payload.Supported {
		t.Fatalf("expected supported=false")
	}
	if payload.Platform != "darwin" {
		t.Fatalf("expected platform darwin, got %q", payload.Platform)
	}
	if payload.Reason != "requires Linux" {
		t.Fatalf("expected reason 'requires Linux', got %q", payload.Reason)
	}
	if len(payload.RequiredTools) != 1 || payload.RequiredTools[0] != "all_reduce_perf" {
		t.Fatalf("expected required_tools [all_reduce_perf], got %v", payload.RequiredTools)
	}
}

func TestBenchCommand_regressionLegacyTool_notPlatformGated_whenNotLinux(t *testing.T) {
	// Given
	called := false
	overrideBenchRun(t, func(_ context.Context, tool bench.Tool, duration time.Duration) (*bench.BenchResult, error) {
		called = true
		return &bench.BenchResult{Tool: string(tool), Duration: duration, Throughput: 10, Unit: "GFLOP/s", RawLog: "raw"}, nil
	})
	failIfBenchRunOptionsCalled(t)
	overridePlatform(t, false, "darwin")
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "bench", "--tool", "gpu-burn", "--duration", "5s")
	// Then
	if err != nil {
		t.Fatalf("expected legacy gpu-burn to run on non-Linux: %v", err)
	}
	if !called {
		t.Fatalf("expected benchRun to be called for legacy tool on non-Linux")
	}
	if strings.Contains(stdout, "requires Linux") {
		t.Fatalf("expected no 'requires Linux' gate for legacy tool, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "gpu-burn") {
		t.Fatalf("expected legacy bench output, got:\n%s", stdout)
	}
}

func overrideBenchRun(t *testing.T, fn func(context.Context, bench.Tool, time.Duration) (*bench.BenchResult, error)) {
	t.Helper()
	previous := benchRun
	benchRun = fn
	t.Cleanup(func() { benchRun = previous })
}

func overrideBenchRunOptions(t *testing.T, fn func(context.Context, bench.Tool, bench.Options) (*bench.BenchResult, error)) {
	t.Helper()
	previous := benchRunOptions
	benchRunOptions = fn
	t.Cleanup(func() { benchRunOptions = previous })
}

func failIfBenchRunCalled(t *testing.T) {
	t.Helper()
	overrideBenchRun(t, func(context.Context, bench.Tool, time.Duration) (*bench.BenchResult, error) {
		t.Fatalf("expected benchRun not to be called")
		return nil, nil
	})
}

func failIfBenchRunOptionsCalled(t *testing.T) {
	t.Helper()
	overrideBenchRunOptions(t, func(context.Context, bench.Tool, bench.Options) (*bench.BenchResult, error) {
		t.Fatalf("expected benchRunOptions not to be called")
		return nil, nil
	})
}
