package bench

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestIsKnownTool_returnsTrueForPerftestAndNCCLTests(t *testing.T) {
	// Given
	tests := []Tool{ToolPerftest, ToolNCCLTests}

	for _, tool := range tests {
		t.Run(string(tool), func(t *testing.T) {
			// When
			known := IsKnownTool(tool)

			// Then
			if !known {
				t.Fatalf("expected %q to be a known tool", tool)
			}
		})
	}
}

func TestRun_preservesLegacyParsingAndOmitsGDRDMA_whenUsingExistingEntryPoint(t *testing.T) {
	// Given
	tests := []struct {
		name           string
		tool           Tool
		output         string
		duration       time.Duration
		wantThroughput float64
		wantUnit       string
		wantArgs       []string
	}{
		{name: "gpu burn", tool: ToolGPUBurn, output: "GPU 0: 8457.50 GFLOP/s\n", duration: 3 * time.Second, wantThroughput: 8457.50, wantUnit: "GFLOP/s", wantArgs: []string{"3"}},
		{name: "nv bandwidth", tool: ToolNVBandwidth, output: "Device to device copy bandwidth: 912.25 GB/s\n", duration: 10 * time.Second, wantThroughput: 912.25, wantUnit: "GB/s", wantArgs: nil},
		{name: "bandwidth test", tool: ToolBandwidthTest, output: "bandwidthTest throughput 123.45 GB/s\n", duration: 2 * time.Second, wantThroughput: 123.45, wantUnit: "GB/s", wantArgs: []string{"--mode=quick", "--memory=pinned"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			runner := &fakeExecRunner{output: []byte(tt.output)}
			overrideLookPath(t, func(name string) (string, error) { return "/legacy/bin/" + name, nil })

			// When
			result, err := Run(context.Background(), runner, tt.tool, tt.duration)
			// Then
			if err != nil {
				t.Fatalf("expected legacy run to succeed: %v", err)
			}
			assertFloatNear(t, result.Throughput, tt.wantThroughput)
			if result.Unit != tt.wantUnit {
				t.Fatalf("expected unit %q, got %q", tt.wantUnit, result.Unit)
			}
			if result.GDRDMA != "" {
				t.Fatalf("expected empty GDRDMA for legacy tool, got %q", result.GDRDMA)
			}
			if !reflect.DeepEqual(runner.args, tt.wantArgs) {
				t.Fatalf("expected args %#v, got %#v", tt.wantArgs, runner.args)
			}
			jsonBytes, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("expected legacy result to marshal: %v", err)
			}
			jsonLog := string(jsonBytes)
			if strings.Contains(jsonLog, "gdrdma") || strings.Contains(jsonLog, "GDRDMA") {
				t.Fatalf("expected GDRDMA to be omitted from legacy JSON, got %s", jsonLog)
			}
		})
	}
}

func TestRunWithOptions_preservesLegacyParsing_whenUsingOptionsEntryPoint(t *testing.T) {
	// Given
	runner := &fakeExecRunnerV2{result: ExecResult{Stdout: []byte("GPU 0: 8457.50 GFLOP/s\n")}}
	overrideLookPath(t, func(name string) (string, error) { return "/legacy/bin/" + name, nil })

	// When
	result, err := RunWithOptions(context.Background(), runner, ToolGPUBurn, Options{Duration: 3 * time.Second})
	// Then
	if err != nil {
		t.Fatalf("expected legacy RunWithOptions to succeed: %v", err)
	}
	assertFloatNear(t, result.Throughput, 8457.50)
	if result.Unit != "GFLOP/s" {
		t.Fatalf("expected unit GFLOP/s, got %q", result.Unit)
	}
	wantArgs := []string{"3"}
	if !reflect.DeepEqual(runner.args, wantArgs) {
		t.Fatalf("expected args %#v, got %#v", wantArgs, runner.args)
	}
	if runner.env != nil {
		t.Fatalf("expected nil env for legacy RunWithOptions, got %#v", runner.env)
	}
}

func TestRunWithOptions_returnsDurationError_whenLegacyDurationIsNotPositive(t *testing.T) {
	// Given
	runner := &fakeExecRunnerV2{result: ExecResult{Stdout: []byte("unused")}}

	// When
	result, err := RunWithOptions(context.Background(), runner, ToolGPUBurn, Options{})

	// Then
	if result != nil {
		t.Fatalf("expected no result for invalid duration, got %#v", result)
	}
	if err == nil || !strings.Contains(err.Error(), "must be positive") {
		t.Fatalf("expected positive duration error, got %v", err)
	}
	if runner.called {
		t.Fatalf("expected runner not to be called for invalid legacy duration")
	}
}

func TestRunWithOptions_returnsErrToolNotInstalled_whenLegacyBinaryIsMissing(t *testing.T) {
	// Given
	runner := &fakeExecRunnerV2{result: ExecResult{Stdout: []byte("unused")}}
	overrideLookPath(t, func(string) (string, error) { return "", exec.ErrNotFound })

	// When
	result, err := RunWithOptions(context.Background(), runner, ToolGPUBurn, Options{Duration: time.Second})

	// Then
	if result != nil {
		t.Fatalf("expected no result for missing legacy binary, got %#v", result)
	}
	if !errors.Is(err, ErrToolNotInstalled) {
		t.Fatalf("expected ErrToolNotInstalled, got %v", err)
	}
	if runner.called {
		t.Fatalf("expected runner not to be called when legacy binary is missing")
	}
}

func TestRunWithOptions_returnsWrappedRunnerError_whenLegacyRunnerFails(t *testing.T) {
	// Given
	runErr := errors.New("runner failed")
	runner := &fakeExecRunnerV2{err: runErr}
	overrideLookPath(t, func(name string) (string, error) { return "/legacy/bin/" + name, nil })

	// When
	result, err := RunWithOptions(context.Background(), runner, ToolGPUBurn, Options{Duration: time.Second})

	// Then
	if result != nil {
		t.Fatalf("expected no result for runner failure, got %#v", result)
	}
	if !errors.Is(err, runErr) {
		t.Fatalf("expected wrapped runner error, got %v", err)
	}
	if !strings.Contains(err.Error(), "run benchmark tool gpu-burn") {
		t.Fatalf("expected benchmark tool context, got %v", err)
	}
}

func TestRunWithOptions_returnsUnknownToolError_whenToolIsUnsupported(t *testing.T) {
	// Given
	runner := &fakeExecRunnerV2{result: ExecResult{Stdout: []byte("unused")}}

	// When
	result, err := RunWithOptions(context.Background(), runner, Tool("bogus"), Options{Duration: time.Second})

	// Then
	if result != nil {
		t.Fatalf("expected no result for unknown tool, got %#v", result)
	}
	if err == nil || !strings.Contains(err.Error(), "unknown benchmark tool") {
		t.Fatalf("expected unknown benchmark tool error, got %v", err)
	}
	if runner.called {
		t.Fatalf("expected runner not to be called for unknown tool")
	}
}

func TestRunWithOptions_usesDefaultRunner_whenRunnerIsNilAndToolLookupFails(t *testing.T) {
	// Given
	overrideLookPath(t, func(string) (string, error) { return "", exec.ErrNotFound })

	// When
	_, err := RunWithOptions(context.Background(), nil, ToolGPUBurn, Options{Duration: time.Second})

	// Then
	if !errors.Is(err, ErrToolNotInstalled) {
		t.Fatalf("expected ErrToolNotInstalled, got %v", err)
	}
}

func TestOsExecRunnerV2RunResult_capturesStdoutStderrAndEnv_whenCommandSucceeds(t *testing.T) {
	// Given
	runner := osExecRunnerV2{}

	// When
	result, err := runner.RunResult(context.Background(), []string{"GPU_TOOLS_BENCH_TEST_ENV=v2"}, "sh", "-c", "printf '%s' \"$GPU_TOOLS_BENCH_TEST_ENV\"; printf '%s' err >&2")
	// Then
	if err != nil {
		t.Fatalf("expected shell command to succeed: %v", err)
	}
	if string(result.Stdout) != "v2" {
		t.Fatalf("expected stdout v2, got %q", string(result.Stdout))
	}
	if string(result.Stderr) != "err" {
		t.Fatalf("expected stderr err, got %q", string(result.Stderr))
	}
}

func TestOsExecRunnerV2RunResult_returnsErrorAndCapturedOutput_whenCommandFails(t *testing.T) {
	// Given
	runner := osExecRunnerV2{}

	// When
	result, err := runner.RunResult(context.Background(), nil, "sh", "-c", "printf out; printf err >&2; exit 7")

	// Then
	if err == nil || !strings.Contains(err.Error(), "run sh") {
		t.Fatalf("expected wrapped shell error, got %v", err)
	}
	if string(result.Stdout) != "out" {
		t.Fatalf("expected captured stdout out, got %q", string(result.Stdout))
	}
	if string(result.Stderr) != "err" {
		t.Fatalf("expected captured stderr err, got %q", string(result.Stderr))
	}
}
