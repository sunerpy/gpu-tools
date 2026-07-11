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

func overrideBenchRun(t *testing.T, fn func(context.Context, bench.Tool, time.Duration) (*bench.BenchResult, error)) {
	t.Helper()
	previous := benchRun
	benchRun = fn
	t.Cleanup(func() { benchRun = previous })
}
