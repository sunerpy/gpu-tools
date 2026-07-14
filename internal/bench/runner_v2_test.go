package bench

import (
	"context"
	"errors"
	"math"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRunWithOptions_parsesPerftestAverageBandwidth_whenOutputContainsMultipleRows(t *testing.T) {
	// Given
	cudaDevice := 2
	stdout := strings.Join([]string{
		" #bytes     #iterations    BW peak[Gb/sec]    BW average[Gb/sec]   MsgRate[Mpps]",
		" 4096       1000           80.00              79.50                0.120",
		" 65536      1000           197.32             196.85               0.375",
		"",
	}, "\n")
	runner := &fakeExecRunnerV2{result: ExecResult{Stdout: []byte(stdout)}}
	overrideLookPath(t, func(name string) (string, error) { return "/usr/bin/" + name, nil })

	// When
	result, err := RunWithOptions(context.Background(), runner, ToolPerftest, Options{
		Duration:  5 * time.Second,
		Server:    "rdma-peer",
		UseCUDA:   &cudaDevice,
		ExtraArgs: []string{"--report_gbits", "--size=65536"},
	})
	// Then
	if err != nil {
		t.Fatalf("expected perftest run to succeed: %v", err)
	}
	if result.Tool != string(ToolPerftest) {
		t.Fatalf("expected tool %q, got %q", ToolPerftest, result.Tool)
	}
	if result.Duration != 5*time.Second {
		t.Fatalf("expected duration 5s, got %s", result.Duration)
	}
	assertFloatNear(t, result.Throughput, 196.85)
	if result.Unit != "Gb/s" {
		t.Fatalf("expected unit Gb/s, got %q", result.Unit)
	}
	if result.RawLog != stdout {
		t.Fatalf("expected raw stdout to be preserved, got %q", result.RawLog)
	}
	if runner.name != "/usr/bin/ib_write_bw" {
		t.Fatalf("expected ib_write_bw resolved path, got %q", runner.name)
	}
	wantArgs := []string{"rdma-peer", "--use_cuda=2", "--report_gbits", "--size=65536"}
	if !reflect.DeepEqual(runner.args, wantArgs) {
		t.Fatalf("expected args %#v, got %#v", wantArgs, runner.args)
	}
	if runner.env != nil {
		t.Fatalf("expected nil env without injected variables, got %#v", runner.env)
	}
}

func TestRunWithOptions_returnsServerError_whenPerftestServerIsMissing(t *testing.T) {
	// Given
	runner := &fakeExecRunnerV2{result: ExecResult{Stdout: []byte("unused")}}
	overrideLookPath(t, func(name string) (string, error) { return "/usr/bin/" + name, nil })

	// When
	result, err := RunWithOptions(context.Background(), runner, ToolPerftest, Options{})

	// Then
	if result != nil {
		t.Fatalf("expected no result when server is missing, got %#v", result)
	}
	if err == nil || !strings.Contains(err.Error(), "perftest requires --server") {
		t.Fatalf("expected missing server error, got %v", err)
	}
	if errors.Is(err, ErrToolNotInstalled) {
		t.Fatalf("expected missing server error not to wrap ErrToolNotInstalled")
	}
	if runner.called {
		t.Fatalf("expected runner not to be called when server is missing")
	}
}

func TestRunWithOptions_returnsErrToolNotInstalled_whenPerftestBinaryIsMissing(t *testing.T) {
	// Given
	runner := &fakeExecRunnerV2{result: ExecResult{Stdout: []byte("unused")}}
	overrideLookPath(t, func(string) (string, error) { return "", exec.ErrNotFound })

	// When
	result, err := RunWithOptions(context.Background(), runner, ToolPerftest, Options{Server: "rdma-peer"})

	// Then
	if result != nil {
		t.Fatalf("expected no result for missing perftest binary, got %#v", result)
	}
	if !errors.Is(err, ErrToolNotInstalled) {
		t.Fatalf("expected ErrToolNotInstalled, got %v", err)
	}
	if runner.called {
		t.Fatalf("expected runner not to be called when perftest binary is missing")
	}
}

func TestRunWithOptions_returnsZeroThroughput_whenPerftestOutputIsMalformed(t *testing.T) {
	// Given
	runner := &fakeExecRunnerV2{result: ExecResult{Stdout: []byte("benchmark completed without tabular throughput\n")}}
	overrideLookPath(t, func(name string) (string, error) { return "/bench/bin/" + name, nil })

	// When
	result, err := RunWithOptions(context.Background(), runner, ToolPerftest, Options{Server: "rdma-peer"})
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

func TestParsePerftestThroughput_returnsZeroAndEmptyUnit_whenAverageOverflows(t *testing.T) {
	// Given
	rawLog := "65536 1000 197.32 " + strings.Repeat("9", 400) + " 0.375"

	// When
	throughput, unit := parsePerftestThroughput(rawLog)

	// Then
	if throughput != 0 {
		t.Fatalf("expected zero throughput for overflow, got %.2f", throughput)
	}
	if unit != "" {
		t.Fatalf("expected empty unit for overflow, got %q", unit)
	}
}

type fakeExecRunnerV2 struct {
	result ExecResult
	err    error
	name   string
	args   []string
	env    []string
	called bool
}

func (r *fakeExecRunnerV2) RunResult(_ context.Context, env []string, name string, args ...string) (ExecResult, error) {
	r.called = true
	r.env = append([]string(nil), env...)
	r.name = name
	r.args = append([]string(nil), args...)
	if r.err != nil {
		return ExecResult{}, r.err
	}
	return r.result, nil
}

func assertFloatNear(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.0001 {
		t.Fatalf("expected %.4f, got %.4f", want, got)
	}
}
