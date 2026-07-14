package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sunerpy/gpu-tools/internal/rdma"
)

func overrideRDMACollect(t *testing.T, fn func(context.Context) (*rdma.Result, error)) {
	t.Helper()
	previous := rdmaCollect
	rdmaCollect = fn
	t.Cleanup(func() { rdmaCollect = previous })
}

func sampleRDMAResult() *rdma.Result {
	return &rdma.Result{
		Devices: []rdma.Device{
			{
				Name:      "mlx5_0",
				NodeGUID:  "0x1234567890abcdef",
				FWVersion: "20.31.1014",
				Ports: []rdma.Port{
					{Num: 1, State: "PORT_ACTIVE", Rate: "200", LinkLayer: "InfiniBand"},
				},
			},
			{
				Name:      "mlx5_1",
				NodeGUID:  "0xfedcba0987654321",
				FWVersion: "20.31.1014",
				Ports: []rdma.Port{
					{Num: 1, State: "PORT_ACTIVE", Rate: "100", LinkLayer: "Ethernet"},
					{Num: 2, State: "PORT_DOWN", Rate: "0", LinkLayer: "Ethernet"},
				},
			},
		},
	}
}

func TestRDMACommand_rendersTable_whenCollectSucceeds(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overrideRDMACollect(t, func(context.Context) (*rdma.Result, error) {
		return sampleRDMAResult(), nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, stderr, err := executeCommand(newRootCmd(), "rdma")
	// Then
	if err != nil {
		t.Fatalf("expected rdma to succeed: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	for _, want := range []string{
		"mlx5_0", "mlx5_1", "0x1234567890abcdef", "20.31.1014",
		"PORT_ACTIVE", "PORT_DOWN", "InfiniBand", "Ethernet", "200", "100",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected table output to contain %q, got:\n%s", want, stdout)
		}
	}
}

func TestRDMACommand_rendersJSON_whenOutputJSON(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overrideRDMACollect(t, func(context.Context) (*rdma.Result, error) {
		return sampleRDMAResult(), nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "--output", "json", "rdma")
	// Then
	if err != nil {
		t.Fatalf("expected rdma json to succeed: %v", err)
	}
	var decoded struct {
		Devices []struct {
			Name      string `json:"name"`
			NodeGUID  string `json:"node_guid"`
			FWVersion string `json:"fw_version"`
			Ports     []struct {
				Num       int    `json:"num"`
				State     string `json:"state"`
				Rate      string `json:"rate"`
				LinkLayer string `json:"link_layer"`
			} `json:"ports"`
		} `json:"devices"`
	}
	if derr := json.Unmarshal([]byte(stdout), &decoded); derr != nil {
		t.Fatalf("expected valid JSON, got error %v for:\n%s", derr, stdout)
	}
	if len(decoded.Devices) != 2 {
		t.Fatalf("expected 2 devices in JSON, got %d", len(decoded.Devices))
	}
	if decoded.Devices[0].Name != "mlx5_0" {
		t.Fatalf("expected first device mlx5_0, got %q", decoded.Devices[0].Name)
	}
	if len(decoded.Devices[1].Ports) != 2 {
		t.Fatalf("expected 2 ports on mlx5_1, got %d", len(decoded.Devices[1].Ports))
	}
	if decoded.Devices[1].Ports[0].LinkLayer != "Ethernet" {
		t.Fatalf("expected Ethernet link layer, got %q", decoded.Devices[1].Ports[0].LinkLayer)
	}
	if !strings.Contains(stdout, "  ") {
		t.Fatalf("expected 2-space indented JSON, got:\n%s", stdout)
	}
}

func TestRDMACommand_rendersMarkdown_whenOutputMarkdown(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overrideRDMACollect(t, func(context.Context) (*rdma.Result, error) {
		return sampleRDMAResult(), nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "--output", "markdown", "rdma")
	// Then
	if err != nil {
		t.Fatalf("expected rdma markdown to succeed: %v", err)
	}
	for _, want := range []string{
		"## mlx5_0", "## mlx5_1",
		"| Port | State | Rate | Link Layer |",
		"| --- | --- | --- | --- |",
		"InfiniBand", "Ethernet",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected markdown output to contain %q, got:\n%s", want, stdout)
		}
	}
}

func TestRDMACommand_returnsExitCode2_whenToolMissing(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overrideRDMACollect(t, func(context.Context) (*rdma.Result, error) {
		return nil, rdma.ErrToolNotInstalled
	})
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "rdma")

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
	if !strings.Contains(err.Error(), "rdma-core") {
		t.Fatalf("expected OFED/rdma-core install hint, got %q", err.Error())
	}
}

func TestRDMACommand_returnsExitCode1_whenCollectFails(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overrideRDMACollect(t, func(context.Context) (*rdma.Result, error) {
		return nil, errors.New("parse ibstat: boom")
	})
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "rdma")

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

func TestRDMACommand_returnsExitCode1_whenResultNil(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overrideRDMACollect(t, func(context.Context) (*rdma.Result, error) {
		return nil, nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "rdma")

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

func TestRDMACommand_returnsExitCode2AndMessage_whenNotLinux(t *testing.T) {
	// Given
	called := false
	overridePlatform(t, false, "darwin")
	overrideRDMACollect(t, func(context.Context) (*rdma.Result, error) {
		called = true
		return nil, nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "rdma")

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
	if !strings.Contains(err.Error(), "gpu-tools rdma requires Linux (uses ibv_devinfo/ibstat); current OS: darwin") {
		t.Fatalf("expected linux-required message, got %q", err.Error())
	}
	if called {
		t.Fatalf("expected rdmaCollect not to run on non-Linux")
	}
	if stdout != "" {
		t.Fatalf("expected no stdout for non-JSON unsupported platform, got %q", stdout)
	}
}

func TestRDMACommand_emitsUnsupportedJSON_whenNotLinuxAndOutputJSON(t *testing.T) {
	// Given
	overridePlatform(t, false, "windows")
	overrideRDMACollect(t, func(context.Context) (*rdma.Result, error) {
		t.Fatalf("rdmaCollect must not run on non-Linux")
		return nil, nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "--output", "json", "rdma")

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
	if len(payload.RequiredTools) != 2 ||
		payload.RequiredTools[0] != "ibv_devinfo" ||
		payload.RequiredTools[1] != "ibstat" {
		t.Fatalf("expected required_tools [ibv_devinfo ibstat], got %v", payload.RequiredTools)
	}
}

func TestRenderRDMATable_handlesEmptyDevices(t *testing.T) {
	// Given
	var builder strings.Builder

	// When
	err := renderRDMATable(&builder, &rdma.Result{})
	// Then
	if err != nil {
		t.Fatalf("expected empty-device table to succeed: %v", err)
	}
	if !strings.Contains(builder.String(), "no RDMA devices found") {
		t.Fatalf("expected empty-device notice, got:\n%s", builder.String())
	}
}

func TestRenderRDMAMarkdown_handlesEmptyDevices(t *testing.T) {
	// Given
	var builder strings.Builder

	// When
	err := renderRDMAMarkdown(&builder, &rdma.Result{})
	// Then
	if err != nil {
		t.Fatalf("expected empty-device markdown to succeed: %v", err)
	}
	out := builder.String()
	if !strings.Contains(out, "# RDMA Devices") || !strings.Contains(out, "No RDMA devices found.") {
		t.Fatalf("expected empty-device markdown notice, got:\n%s", out)
	}
}

func TestRenderRDMA_returnsError_whenOutputUnknown(t *testing.T) {
	// When
	err := renderRDMA(&strings.Builder{}, "xml", sampleRDMAResult())

	// Then
	if err == nil {
		t.Fatalf("expected unknown output format to fail")
	}
	if !strings.Contains(err.Error(), "unknown rdma output format") {
		t.Fatalf("expected unknown format error, got %q", err.Error())
	}
}
