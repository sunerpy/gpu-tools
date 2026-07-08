package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sunerpy/gpu-tools/core"
	"github.com/sunerpy/gpu-tools/internal/gpu"
	"github.com/sunerpy/gpu-tools/internal/report"
)

func TestReportCommand_writesMarkdownFileInReportDir_whenOutOmitted(t *testing.T) {
	// Given
	reportDir := t.TempDir()
	configPath := writeReportConfig(t, reportDir, core.OutputTable)
	collector := newFakeCollector(reportDevices())
	overrideGPUFactory(t, collector, nil)

	// When
	stdout, stderr, err := executeCommand(newRootCmd(), "--config", configPath, "report")
	// Then
	if err != nil {
		t.Fatalf("expected report to succeed: %v", err)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("expected no command output, got stdout=%q stderr=%q", stdout, stderr)
	}
	data := readOnlyGeneratedReport(t, reportDir)
	assertReportMarkdown(t, string(data))
	if !collector.initCalled || !collector.shutdownCalled {
		t.Fatalf("expected Init and Shutdown to be called, got init=%v shutdown=%v", collector.initCalled, collector.shutdownCalled)
	}
}

func TestReportCommand_writesMarkdownToStdout_whenOutIsDash(t *testing.T) {
	// Given
	configPath := writeReportConfig(t, t.TempDir(), core.OutputTable)
	overrideGPUFactory(t, newFakeCollector(reportDevices()), nil)

	// When
	stdout, stderr, err := executeCommand(newRootCmd(), "--config", configPath, "report", "--out", "-")
	// Then
	if err != nil {
		t.Fatalf("expected report stdout to succeed: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	assertReportMarkdown(t, stdout)
}

func TestReportCommand_writesMarkdownToExplicitPath_whenOutIsFile(t *testing.T) {
	// Given
	configPath := writeReportConfig(t, t.TempDir(), core.OutputTable)
	outputPath := filepath.Join(t.TempDir(), "snapshot.md")
	overrideGPUFactory(t, newFakeCollector(reportDevices()), nil)

	// When
	stdout, stderr, err := executeCommand(newRootCmd(), "--config", configPath, "report", "--out", outputPath)
	// Then
	if err != nil {
		t.Fatalf("expected report file output to succeed: %v", err)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("expected no command output, got stdout=%q stderr=%q", stdout, stderr)
	}
	data, readErr := os.ReadFile(outputPath)
	if readErr != nil {
		t.Fatalf("read explicit report: %v", readErr)
	}
	assertReportMarkdown(t, string(data))
}

func TestReportCommand_writesJSON_whenOutputFlagIsChanged(t *testing.T) {
	// Given
	configPath := writeReportConfig(t, t.TempDir(), core.OutputTable)
	overrideGPUFactory(t, newFakeCollector(reportDevices()), nil)

	// When
	stdout, stderr, err := executeCommand(newRootCmd(), "--config", configPath, "--output", "json", "report", "--out", "-")
	// Then
	if err != nil {
		t.Fatalf("expected report json to succeed: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	var snapshot report.Snapshot
	if err := json.Unmarshal([]byte(stdout), &snapshot); err != nil {
		t.Fatalf("expected valid JSON snapshot, got %v and output:\n%s", err, stdout)
	}
	if snapshot.Backend != "fake" {
		t.Fatalf("expected fake backend, got %q", snapshot.Backend)
	}
	if len(snapshot.Devices) != 2 {
		t.Fatalf("expected two devices, got %#v", snapshot.Devices)
	}
}

func TestReportCommand_returnsExitError_whenOutputPathCannotBeCreated(t *testing.T) {
	// Given
	configPath := writeReportConfig(t, t.TempDir(), core.OutputTable)
	overrideGPUFactory(t, newFakeCollector(reportDevices()), nil)
	outputPath := filepath.Join(t.TempDir(), "missing", "snapshot.md")

	// When
	_, _, err := executeCommand(newRootCmd(), "--config", configPath, "report", "--out", outputPath)

	// Then
	assertExitErrorCodeOne(t, err)
	if !strings.Contains(err.Error(), "write report") {
		t.Fatalf("expected write report error, got %q", err.Error())
	}
}

func TestReportCommand_returnsExitError_whenBackendUnavailable(t *testing.T) {
	// Given
	configPath := writeReportConfig(t, t.TempDir(), core.OutputTable)
	overrideGPUFactory(t, nil, gpu.ErrBackendUnavailable)

	// When
	_, _, err := executeCommand(newRootCmd(), "--config", configPath, "report")

	// Then
	assertExitErrorCodeOne(t, err)
	if !strings.Contains(err.Error(), "no NVIDIA GPU detected") {
		t.Fatalf("expected friendly no NVIDIA message, got %q", err.Error())
	}
}

func TestReportCommand_returnsWrappedError_whenCollectorFlowFails(t *testing.T) {
	tests := []struct {
		name      string
		collector *fakeCollector
		want      string
	}{
		{name: "init fails", collector: &fakeCollector{initErr: errors.New("init failed")}, want: "initialize GPU collector"},
		{name: "device count fails", collector: &fakeCollector{countErr: errors.New("count failed")}, want: "count GPU devices"},
		{name: "device read fails", collector: &fakeCollector{devices: reportDevices(), deviceErr: errors.New("device failed")}, want: "read GPU device 0"},
		{name: "device read returns nil", collector: &fakeCollector{devices: reportDevices(), nilDevice: true}, want: "read GPU device 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			configPath := writeReportConfig(t, t.TempDir(), core.OutputTable)
			overrideGPUFactory(t, tt.collector, nil)

			// When
			_, _, err := executeCommand(newRootCmd(), "--config", configPath, "report")

			// Then
			if err == nil {
				t.Fatalf("expected report to fail")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error to contain %q, got %q", tt.want, err.Error())
			}
		})
	}
}

func TestReportCommand_returnsConfigError_whenRunWithoutRootFlags(t *testing.T) {
	// Given
	cmd := newReportCmd()

	// When
	err := runReport(cmd, "-")

	// Then
	if err == nil {
		t.Fatalf("expected missing root flags to fail")
	}
	if !strings.Contains(err.Error(), "read --config") {
		t.Fatalf("expected config flag read error, got %q", err.Error())
	}
}

func TestReportCommand_returnsExitError_whenStdoutWriterFails(t *testing.T) {
	// Given
	configPath := writeReportConfig(t, t.TempDir(), core.OutputTable)
	overrideGPUFactory(t, newFakeCollector(reportDevices()), nil)
	root := newRootCmd()
	root.SetArgs([]string{"--config", configPath, "report", "--out", "-"})
	root.SetOut(failingWriter{})

	// When
	err := root.Execute()

	// Then
	assertExitErrorCodeOne(t, err)
	if !strings.Contains(err.Error(), "write report stdout") {
		t.Fatalf("expected stdout write error, got %q", err.Error())
	}
}

func TestRenderReport_returnsRendererError_whenFormatIsUnknown(t *testing.T) {
	// Given
	snapshot := &report.Snapshot{}

	// When
	_, err := renderReport("xml", snapshot)

	// Then
	if err == nil {
		t.Fatalf("expected unknown renderer to fail")
	}
	if !strings.Contains(err.Error(), "select report renderer") {
		t.Fatalf("expected renderer selection error, got %q", err.Error())
	}
}

func writeReportConfig(t *testing.T, reportDir, output string) string {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	configText := strings.Join([]string{
		"default_output: " + output,
		"backend: auto",
		"report_dir: " + reportDir,
		"",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(configText), 0o600); err != nil {
		t.Fatalf("write report config: %v", err)
	}
	return configPath
}

func reportDevices() []gpu.Device {
	return []gpu.Device{
		{Index: 0, UUID: "GPU-report-0", Name: "RTX 4090", MemoryTotal: 24 * 1024 * 1024, MemoryUsed: 12 * 1024 * 1024, Temperature: 42, PowerDraw: 100_000, PowerLimit: 450_000, UtilizationGPU: 10, UtilizationMem: 20, PState: "P0"},
		{Index: 1, UUID: "GPU-report-1", Name: "RTX 6000 Ada", MemoryTotal: 48 * 1024 * 1024, MemoryUsed: 16 * 1024 * 1024, Temperature: 55, PowerDraw: 90_000, PowerLimit: 300_000, UtilizationGPU: 30, UtilizationMem: 40, PState: "P2"},
	}
}

func readOnlyGeneratedReport(t *testing.T, reportDir string) []byte {
	t.Helper()
	entries, err := os.ReadDir(reportDir)
	if err != nil {
		t.Fatalf("read report dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one report file, got %d", len(entries))
	}
	name := entries[0].Name()
	if !strings.HasPrefix(name, "gpu-report-") || !strings.HasSuffix(name, ".md") {
		t.Fatalf("expected gpu-report-*.md file, got %q", name)
	}
	data, err := os.ReadFile(filepath.Join(reportDir, name))
	if err != nil {
		t.Fatalf("read generated report: %v", err)
	}
	return data
}

func assertReportMarkdown(t *testing.T, text string) {
	t.Helper()
	for _, want := range []string{
		"## GPU Snapshot",
		"## Summary",
		"- Device count: `2`",
		"- Aggregate memory total: `72 MiB`",
		"- Max temperature: `55°C`",
		"| Index | Name | UUID | Mem(used/total) | Temp | Power(draw/limit) | Util(gpu/mem) | PState |",
		"| 0 | RTX 4090 | GPU-report-0 | 12/24 MiB | 42°C | 100.0/450.0 W | 10/20% | P0 |",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected markdown to contain %q, got:\n%s", want, text)
		}
	}
}

func assertExitErrorCodeOne(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected command to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.Code)
	}
}
