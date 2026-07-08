package bench

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRun_parsesGPUBurnThroughput_whenOutputContainsGFLOPS(t *testing.T) {
	// Given
	runner := &fakeExecRunner{output: []byte("GPU 0: 8457.50 GFLOP/s\n")}
	overrideLookPath(t, func(name string) (string, error) { return "/usr/bin/" + name, nil })

	// When
	result, err := Run(context.Background(), runner, ToolGPUBurn, 3*time.Second)
	// Then
	if err != nil {
		t.Fatalf("expected gpu-burn run to succeed: %v", err)
	}
	if result.Tool != string(ToolGPUBurn) {
		t.Fatalf("expected tool %q, got %q", ToolGPUBurn, result.Tool)
	}
	if result.Duration != 3*time.Second {
		t.Fatalf("expected duration 3s, got %s", result.Duration)
	}
	if result.Throughput != 8457.50 || result.Unit != "GFLOP/s" {
		t.Fatalf("expected 8457.50 GFLOP/s, got %.2f %q", result.Throughput, result.Unit)
	}
	if result.RawLog != "GPU 0: 8457.50 GFLOP/s\n" {
		t.Fatalf("expected raw log to be preserved, got %q", result.RawLog)
	}
	if runner.name != "/usr/bin/gpu-burn" {
		t.Fatalf("expected resolved path to be executed, got %q", runner.name)
	}
	if !reflect.DeepEqual(runner.args, []string{"3"}) {
		t.Fatalf("expected gpu-burn duration arg, got %#v", runner.args)
	}
}

func TestRun_parsesNVBandwidthThroughput_whenOutputContainsGBPerSecond(t *testing.T) {
	// Given
	runner := &fakeExecRunner{output: []byte("Device to device copy bandwidth: 912.25 GB/s\n")}
	overrideLookPath(t, func(name string) (string, error) { return "/opt/bin/" + name, nil })

	// When
	result, err := Run(context.Background(), runner, ToolNVBandwidth, 10*time.Second)
	// Then
	if err != nil {
		t.Fatalf("expected nvbandwidth run to succeed: %v", err)
	}
	if result.Throughput != 912.25 || result.Unit != "GB/s" {
		t.Fatalf("expected 912.25 GB/s, got %.2f %q", result.Throughput, result.Unit)
	}
	if len(runner.args) != 0 {
		t.Fatalf("expected nvbandwidth default invocation to use no args, got %#v", runner.args)
	}
}

func TestRun_returnsZeroThroughput_whenOutputIsMalformed(t *testing.T) {
	// Given
	runner := &fakeExecRunner{output: []byte("benchmark completed without numeric throughput\n")}
	overrideLookPath(t, func(name string) (string, error) { return "/usr/local/bin/" + name, nil })

	// When
	result, err := Run(context.Background(), runner, ToolBandwidthTest, 2*time.Second)
	// Then
	if err != nil {
		t.Fatalf("expected malformed output to return a partial result without error: %v", err)
	}
	if result.Throughput != 0 {
		t.Fatalf("expected zero throughput, got %.2f", result.Throughput)
	}
	if result.Unit != "" {
		t.Fatalf("expected empty unit, got %q", result.Unit)
	}
	if result.RawLog != "benchmark completed without numeric throughput\n" {
		t.Fatalf("expected raw log to be preserved, got %q", result.RawLog)
	}
	if !reflect.DeepEqual(runner.args, []string{"--mode=quick", "--memory=pinned"}) {
		t.Fatalf("expected bandwidthTest safe default args, got %#v", runner.args)
	}
}

func TestRun_returnsErrToolNotInstalled_whenLookPathFails(t *testing.T) {
	// Given
	runner := &fakeExecRunner{output: []byte("unused")}
	overrideLookPath(t, func(string) (string, error) { return "", exec.ErrNotFound })

	// When
	result, err := Run(context.Background(), runner, ToolGPUBurn, time.Second)

	// Then
	if result != nil {
		t.Fatalf("expected no result for missing tool, got %#v", result)
	}
	if !errors.Is(err, ErrToolNotInstalled) {
		t.Fatalf("expected ErrToolNotInstalled, got %v", err)
	}
	if runner.called {
		t.Fatalf("expected runner not to be called when tool is missing")
	}
}

func TestRun_returnsUnknownToolError_whenToolIsUnsupported(t *testing.T) {
	// Given
	runner := &fakeExecRunner{output: []byte("unused")}

	// When
	_, err := Run(context.Background(), runner, Tool("bogus"), time.Second)

	// Then
	if err == nil || !strings.Contains(err.Error(), "unknown benchmark tool") {
		t.Fatalf("expected unknown benchmark tool error, got %v", err)
	}
	if runner.called {
		t.Fatalf("expected runner not to be called for unknown tool")
	}
}

func TestRun_returnsDurationError_whenDurationIsNotPositive(t *testing.T) {
	// Given
	runner := &fakeExecRunner{output: []byte("unused")}

	// When
	_, err := Run(context.Background(), runner, ToolGPUBurn, 0)

	// Then
	if err == nil || !strings.Contains(err.Error(), "must be positive") {
		t.Fatalf("expected positive duration error, got %v", err)
	}
	if runner.called {
		t.Fatalf("expected runner not to be called for invalid duration")
	}
}

func TestRun_returnsWrappedRunnerError_whenExecRunnerFails(t *testing.T) {
	// Given
	runErr := errors.New("runner failed")
	runner := &fakeExecRunner{err: runErr}
	overrideLookPath(t, func(name string) (string, error) { return "/bench/bin/" + name, nil })

	// When
	_, err := Run(context.Background(), runner, ToolGPUBurn, time.Second)

	// Then
	if !errors.Is(err, runErr) {
		t.Fatalf("expected wrapped runner error, got %v", err)
	}
	if !strings.Contains(err.Error(), "run benchmark tool gpu-burn") {
		t.Fatalf("expected benchmark tool context, got %v", err)
	}
}

func TestRun_usesDefaultRunner_whenRunnerIsNilAndToolLookupFails(t *testing.T) {
	// Given
	overrideLookPath(t, func(string) (string, error) { return "", exec.ErrNotFound })

	// When
	_, err := Run(context.Background(), nil, ToolGPUBurn, time.Second)

	// Then
	if !errors.Is(err, ErrToolNotInstalled) {
		t.Fatalf("expected ErrToolNotInstalled, got %v", err)
	}
}

func TestLookPath_returnsWrappedResultForDefaultImplementation(t *testing.T) {
	// Given
	missingBinary := "/nonexistent/gpu-tools-bench-missing-binary"

	// When
	path, err := lookPath(os.Args[0])
	_, missingErr := lookPath(missingBinary)

	// Then
	if err != nil {
		t.Fatalf("expected current test binary lookup to succeed: %v", err)
	}
	if path == "" {
		t.Fatalf("expected non-empty path for current test binary")
	}
	if missingErr == nil || !strings.Contains(missingErr.Error(), "look path "+missingBinary) {
		t.Fatalf("expected wrapped missing binary error, got %v", missingErr)
	}
}

func TestIsKnownTool_returnsExpectedKnownStatus(t *testing.T) {
	// Given
	tests := []struct {
		name string
		tool Tool
		want bool
	}{
		{name: "gpu burn", tool: ToolGPUBurn, want: true},
		{name: "nv bandwidth", tool: ToolNVBandwidth, want: true},
		{name: "bandwidth test", tool: ToolBandwidthTest, want: true},
		{name: "unknown", tool: Tool("bogus"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got := IsKnownTool(tt.tool)

			// Then
			if got != tt.want {
				t.Fatalf("expected IsKnownTool(%q) to be %t, got %t", tt.tool, tt.want, got)
			}
		})
	}
}

func TestArgsForTool_returnsExpectedArgs(t *testing.T) {
	// Given
	tests := []struct {
		name     string
		tool     Tool
		duration time.Duration
		want     []string
	}{
		{name: "gpu burn floors subsecond duration", tool: ToolGPUBurn, duration: 500 * time.Millisecond, want: []string{"1"}},
		{name: "gpu burn rounds down whole seconds", tool: ToolGPUBurn, duration: 2500 * time.Millisecond, want: []string{"2"}},
		{name: "nv bandwidth uses no args", tool: ToolNVBandwidth, duration: time.Second, want: nil},
		{name: "bandwidth test uses safe defaults", tool: ToolBandwidthTest, duration: time.Second, want: []string{"--mode=quick", "--memory=pinned"}},
		{name: "unknown uses nil args", tool: Tool("bogus"), duration: time.Second, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got := argsForTool(tt.tool, tt.duration)

			// Then
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("expected args %#v, got %#v", tt.want, got)
			}
		})
	}
}

func TestOsExecRunnerRun_returnsCommandStdout_whenCommandSucceeds(t *testing.T) {
	// Given
	runner := osExecRunner{}

	// When
	out, err := runner.Run(context.Background(), "echo", "bench-ok")
	// Then
	if err != nil {
		t.Fatalf("expected echo command to succeed: %v", err)
	}
	if strings.TrimSpace(string(out)) != "bench-ok" {
		t.Fatalf("expected echo output bench-ok, got %q", string(out))
	}
}

func TestOsExecRunnerRun_returnsError_whenCommandFails(t *testing.T) {
	// Given
	runner := osExecRunner{}
	missingBinary := "/nonexistent/gpu-tools-bench-missing-binary"

	// When
	out, err := runner.Run(context.Background(), missingBinary)

	// Then
	if out != nil {
		t.Fatalf("expected no stdout when command fails, got %q", string(out))
	}
	if err == nil || !strings.Contains(err.Error(), "run "+missingBinary) {
		t.Fatalf("expected wrapped exec error, got %v", err)
	}
}

func TestParseThroughput_returnsZeroAndEmptyUnit_whenLogDoesNotMatch(t *testing.T) {
	// Given
	rawLog := "benchmark completed with 123 units only"

	// When
	throughput, unit := parseThroughput(rawLog, bandwidthThroughputPattern, "GB/s")

	// Then
	if throughput != 0 {
		t.Fatalf("expected zero throughput, got %.2f", throughput)
	}
	if unit != "" {
		t.Fatalf("expected empty unit, got %q", unit)
	}
}

func TestParseThroughput_returnsZeroAndEmptyUnit_whenMatchedNumberOverflows(t *testing.T) {
	// Given
	rawLog := strings.Repeat("9", 400) + " GB/s"

	// When
	throughput, unit := parseThroughput(rawLog, bandwidthThroughputPattern, "GB/s")

	// Then
	if throughput != 0 {
		t.Fatalf("expected zero throughput for malformed number, got %.2f", throughput)
	}
	if unit != "" {
		t.Fatalf("expected empty unit for malformed number, got %q", unit)
	}
}

type fakeExecRunner struct {
	output []byte
	err    error
	name   string
	args   []string
	called bool
}

func (r *fakeExecRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.called = true
	r.name = name
	r.args = append([]string(nil), args...)
	if r.err != nil {
		return nil, r.err
	}
	return r.output, nil
}

func overrideLookPath(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	previous := lookPath
	lookPath = fn
	t.Cleanup(func() { lookPath = previous })
}
