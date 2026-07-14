package bench

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestRunWithOptions_parsesNCCLTestsBusBandwidthFromMaxSizeRow_whenOutputContainsMultipleRows(t *testing.T) {
	// Given
	stdout := strings.Join([]string{
		"#      size    count  type  redop  time   algbw   busbw  error",
		"     8388608  2097152 float   sum  1234.5  6.79   12.34  0e+00",
		"  1073741824  268435456 float   sum  2345.6  45.6   47.1  0e+00",
		"",
	}, "\n")
	runner := &fakeExecRunnerV2{result: ExecResult{Stdout: []byte(stdout)}}
	overrideLookPath(t, func(name string) (string, error) { return "/opt/nccl-tests/" + name, nil })

	// When
	result, err := RunWithOptions(context.Background(), runner, ToolNCCLTests, Options{ExtraArgs: []string{"-n", "20"}})
	// Then
	if err != nil {
		t.Fatalf("expected nccl-tests run to succeed: %v", err)
	}
	assertFloatNear(t, result.Throughput, 47.1)
	if result.Unit != "GB/s" {
		t.Fatalf("expected unit GB/s, got %q", result.Unit)
	}
	if result.GDRDMA != "" {
		t.Fatalf("expected empty GDRDMA without debug, got %q", result.GDRDMA)
	}
	if runner.name != "/opt/nccl-tests/all_reduce_perf" {
		t.Fatalf("expected all_reduce_perf resolved path, got %q", runner.name)
	}
	wantArgs := []string{"-b", "8", "-e", "8G", "-f", "2", "-g", "8", "-n", "20"}
	if !reflect.DeepEqual(runner.args, wantArgs) {
		t.Fatalf("expected args %#v, got %#v", wantArgs, runner.args)
	}
	if runner.env != nil {
		t.Fatalf("expected nil env without debug, got %#v", runner.env)
	}
}

func TestRunWithOptions_setsGDRDMATrue_whenNCCLDebugShowsIBAndGDRDMA(t *testing.T) {
	// Given
	runner := &fakeExecRunnerV2{
		result: ExecResult{
			Stdout: []byte("8388608 2097152 float sum 1234.5 6.79 12.34 0e+00\n"),
			Stderr: []byte("NCCL INFO NET/IB : GPU Direct RDMA enabled via GDRDMA\n"),
		},
	}
	overrideLookPath(t, func(name string) (string, error) { return "/usr/local/bin/" + name, nil })

	// When
	result, err := RunWithOptions(context.Background(), runner, ToolNCCLTests, Options{GPUs: 4, NCCLDebug: true})
	// Then
	if err != nil {
		t.Fatalf("expected nccl-tests debug run to succeed: %v", err)
	}
	if result.GDRDMA != "true" {
		t.Fatalf("expected GDRDMA true, got %q", result.GDRDMA)
	}
	if !reflect.DeepEqual(runner.env, []string{"NCCL_DEBUG=INFO"}) {
		t.Fatalf("expected NCCL_DEBUG env, got %#v", runner.env)
	}
	wantArgs := []string{"-b", "8", "-e", "8G", "-f", "2", "-g", "4"}
	if !reflect.DeepEqual(runner.args, wantArgs) {
		t.Fatalf("expected args %#v, got %#v", wantArgs, runner.args)
	}
}

func TestRunWithOptions_setsGDRDMAFalse_whenNCCLDebugUsesSocket(t *testing.T) {
	// Given
	runner := &fakeExecRunnerV2{
		result: ExecResult{
			Stdout: []byte("8388608 2097152 float sum 1234.5 6.79 12.34 0e+00\n"),
			Stderr: []byte("NCCL INFO NET/Socket : Using eth0\n"),
		},
	}
	overrideLookPath(t, func(name string) (string, error) { return "/usr/local/bin/" + name, nil })

	// When
	result, err := RunWithOptions(context.Background(), runner, ToolNCCLTests, Options{NCCLDebug: true})
	// Then
	if err != nil {
		t.Fatalf("expected nccl-tests socket debug run to succeed: %v", err)
	}
	if result.GDRDMA != "false" {
		t.Fatalf("expected GDRDMA false, got %q", result.GDRDMA)
	}
	if !reflect.DeepEqual(runner.env, []string{"NCCL_DEBUG=INFO"}) {
		t.Fatalf("expected NCCL_DEBUG env, got %#v", runner.env)
	}
}

func TestRunWithOptions_setsGDRDMAUnknown_whenNCCLDebugHasNoTransportSignal(t *testing.T) {
	// Given
	runner := &fakeExecRunnerV2{
		result: ExecResult{
			Stdout: []byte("8388608 2097152 float sum 1234.5 6.79 12.34 0e+00\n"),
			Stderr: []byte("NCCL INFO launch mode parallel\n"),
		},
	}
	overrideLookPath(t, func(name string) (string, error) { return "/usr/local/bin/" + name, nil })

	// When
	result, err := RunWithOptions(context.Background(), runner, ToolNCCLTests, Options{NCCLDebug: true})
	// Then
	if err != nil {
		t.Fatalf("expected nccl-tests debug run to succeed: %v", err)
	}
	if result.GDRDMA != "unknown" {
		t.Fatalf("expected GDRDMA unknown, got %q", result.GDRDMA)
	}
}

func TestRunWithOptions_returnsErrToolNotInstalled_whenNCCLTestsBinaryIsMissing(t *testing.T) {
	// Given
	runner := &fakeExecRunnerV2{result: ExecResult{Stdout: []byte("unused")}}
	overrideLookPath(t, func(string) (string, error) { return "", exec.ErrNotFound })

	// When
	result, err := RunWithOptions(context.Background(), runner, ToolNCCLTests, Options{})

	// Then
	if result != nil {
		t.Fatalf("expected no result for missing nccl-tests binary, got %#v", result)
	}
	if !errors.Is(err, ErrToolNotInstalled) {
		t.Fatalf("expected ErrToolNotInstalled, got %v", err)
	}
	if runner.called {
		t.Fatalf("expected runner not to be called when nccl-tests binary is missing")
	}
}

func TestRunWithOptions_returnsWrappedRunnerError_whenNCCLTestsRunnerFails(t *testing.T) {
	// Given
	runErr := errors.New("all_reduce_perf failed")
	runner := &fakeExecRunnerV2{err: runErr}
	overrideLookPath(t, func(name string) (string, error) { return "/usr/local/bin/" + name, nil })

	// When
	result, err := RunWithOptions(context.Background(), runner, ToolNCCLTests, Options{})

	// Then
	if result != nil {
		t.Fatalf("expected no result for nccl-tests runner failure, got %#v", result)
	}
	if !errors.Is(err, runErr) {
		t.Fatalf("expected wrapped runner error, got %v", err)
	}
	if !strings.Contains(err.Error(), "run benchmark tool nccl-tests") {
		t.Fatalf("expected nccl-tests context, got %v", err)
	}
}

func TestRunWithOptions_returnsZeroThroughput_whenNCCLTestsOutputIsMalformed(t *testing.T) {
	// Given
	runner := &fakeExecRunnerV2{result: ExecResult{Stdout: []byte("8388608 2097152 float sum 1234.5 6.79 nope 0e+00\n")}}
	overrideLookPath(t, func(name string) (string, error) { return "/bench/bin/" + name, nil })

	// When
	result, err := RunWithOptions(context.Background(), runner, ToolNCCLTests, Options{})
	// Then
	if err != nil {
		t.Fatalf("expected malformed output to return partial result without error: %v", err)
	}
	if result.Throughput != 0 {
		t.Fatalf("expected zero throughput, got %.2f", result.Throughput)
	}
	if result.Unit != "" {
		t.Fatalf("expected empty unit, got %q", result.Unit)
	}
}
