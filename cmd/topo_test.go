package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sunerpy/gpu-tools/internal/topo"
)

func overrideTopoCollect(t *testing.T, fn func(context.Context, string) (*topo.Result, error)) {
	t.Helper()
	previous := topoCollect
	topoCollect = fn
	t.Cleanup(func() { topoCollect = previous })
}

func overridePlatform(t *testing.T, isLinux bool, osName string) {
	t.Helper()
	prevIsLinux := platformIsLinux
	prevOS := platformOS
	platformIsLinux = func() bool { return isLinux }
	platformOS = func() string { return osName }
	t.Cleanup(func() {
		platformIsLinux = prevIsLinux
		platformOS = prevOS
	})
}

func sampleTopoResult() *topo.Result {
	return &topo.Result{
		Matrix: topo.Matrix{
			GPUs: []string{"GPU0", "GPU1"},
			Cells: map[string]map[string]topo.Cell{
				"GPU0": {
					"GPU0": {Type: topo.LinkSelf},
					"GPU1": {Type: topo.LinkNVLink, Lanes: 12},
				},
				"GPU1": {
					"GPU0": {Type: topo.LinkNVLink, Lanes: 12},
					"GPU1": {Type: topo.LinkSelf},
				},
			},
			NICs: []topo.NICAffinity{
				{
					NIC: "NIC0",
					PerGPU: map[string]topo.Cell{
						"GPU0": {Type: topo.LinkPIX},
						"GPU1": {Type: topo.LinkSYS},
					},
				},
			},
		},
		Advice: []topo.Advice{
			{GPU: "GPU0", NIC: "NIC0", Link: topo.LinkPIX, Rating: topo.RatingGood},
			{GPU: "GPU1", NIC: "NIC0", Link: topo.LinkSYS, Rating: topo.RatingBad},
		},
	}
}

func TestTopoCommand_rendersTable_whenCollectSucceeds(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overrideTopoCollect(t, func(_ context.Context, smiPath string) (*topo.Result, error) {
		if smiPath != "" {
			t.Fatalf("expected empty smiPath from default config, got %q", smiPath)
		}
		return sampleTopoResult(), nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, stderr, err := executeCommand(newRootCmd(), "topo")
	// Then
	if err != nil {
		t.Fatalf("expected topo to succeed: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	for _, want := range []string{"Connectivity Matrix", "GPU0", "GPU1", "NIC0", "NV12", "Affinity Advice", "good", "bad"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected table output to contain %q, got:\n%s", want, stdout)
		}
	}
}

func TestTopoCommand_rendersJSON_whenOutputJSON(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overrideTopoCollect(t, func(context.Context, string) (*topo.Result, error) {
		return sampleTopoResult(), nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "--output", "json", "topo")
	// Then
	if err != nil {
		t.Fatalf("expected topo json to succeed: %v", err)
	}
	var decoded topo.Result
	if derr := json.Unmarshal([]byte(stdout), &decoded); derr != nil {
		t.Fatalf("expected valid JSON, got error %v for:\n%s", derr, stdout)
	}
	if len(decoded.Matrix.GPUs) != 2 {
		t.Fatalf("expected 2 GPUs in JSON, got %d", len(decoded.Matrix.GPUs))
	}
	if len(decoded.Advice) != 2 {
		t.Fatalf("expected 2 advice entries, got %d", len(decoded.Advice))
	}
	if !strings.Contains(stdout, "  ") {
		t.Fatalf("expected 2-space indented JSON, got:\n%s", stdout)
	}
}

func TestTopoCommand_rendersMarkdown_whenOutputMarkdown(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overrideTopoCollect(t, func(context.Context, string) (*topo.Result, error) {
		return sampleTopoResult(), nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "--output", "markdown", "topo")
	// Then
	if err != nil {
		t.Fatalf("expected topo markdown to succeed: %v", err)
	}
	for _, want := range []string{"## Connectivity Matrix", "| GPU0 | GPU1 |", "## Affinity Advice", "| GPU | NIC | Link | Rating |", "good", "bad"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected markdown output to contain %q, got:\n%s", want, stdout)
		}
	}
}

func TestTopoCommand_returnsExitCode2_whenToolMissing(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overrideTopoCollect(t, func(context.Context, string) (*topo.Result, error) {
		return nil, topo.ErrToolNotInstalled
	})
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "topo")

	// Then
	if err == nil {
		t.Fatalf("expected missing tool to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %d", exitErr.Code)
	}
	if !strings.Contains(err.Error(), "nvidia") {
		t.Fatalf("expected nvidia driver install hint, got %q", err.Error())
	}
}

func TestTopoCommand_returnsExitCode1_whenCollectFails(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overrideTopoCollect(t, func(context.Context, string) (*topo.Result, error) {
		return nil, errors.New("parse topo: boom")
	})
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "topo")

	// Then
	if err == nil {
		t.Fatalf("expected collect failure to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.Code)
	}
}

func TestTopoCommand_returnsExitCode1_whenResultNil(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overrideTopoCollect(t, func(context.Context, string) (*topo.Result, error) {
		return nil, nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "topo")

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

func TestTopoCommand_returnsExitCode2AndMessage_whenNotLinux(t *testing.T) {
	// Given
	called := false
	overridePlatform(t, false, "darwin")
	overrideTopoCollect(t, func(context.Context, string) (*topo.Result, error) {
		called = true
		return nil, nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "topo")

	// Then
	if err == nil {
		t.Fatalf("expected non-Linux to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %d", exitErr.Code)
	}
	if !strings.Contains(err.Error(), "gpu-tools topo requires Linux (uses nvidia-smi); current OS: darwin") {
		t.Fatalf("expected linux-required message, got %q", err.Error())
	}
	if called {
		t.Fatalf("expected topoCollect not to run on non-Linux")
	}
	if stdout != "" {
		t.Fatalf("expected no stdout for non-JSON unsupported platform, got %q", stdout)
	}
}

func TestTopoCommand_emitsUnsupportedJSON_whenNotLinuxAndOutputJSON(t *testing.T) {
	// Given
	overridePlatform(t, false, "windows")
	overrideTopoCollect(t, func(context.Context, string) (*topo.Result, error) {
		t.Fatalf("topoCollect must not run on non-Linux")
		return nil, nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "--output", "json", "topo")

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
	if payload.Platform != "windows" {
		t.Fatalf("expected platform windows, got %q", payload.Platform)
	}
	if payload.Reason != "requires Linux" {
		t.Fatalf("expected reason 'requires Linux', got %q", payload.Reason)
	}
	if len(payload.RequiredTools) != 1 || payload.RequiredTools[0] != "nvidia-smi" {
		t.Fatalf("expected required_tools [nvidia-smi], got %v", payload.RequiredTools)
	}
}

func TestRenderTopo_returnsError_whenOutputUnknown(t *testing.T) {
	// When
	err := renderTopo(&strings.Builder{}, "xml", sampleTopoResult())

	// Then
	if err == nil {
		t.Fatalf("expected unknown output format to fail")
	}
	if !strings.Contains(err.Error(), "unknown topo output format") {
		t.Fatalf("expected unknown format error, got %q", err.Error())
	}
}
