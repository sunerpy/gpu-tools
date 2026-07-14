package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/sunerpy/gpu-tools/internal/health"
)

func overrideHealthRun(t *testing.T, fn func(context.Context) health.Report) {
	t.Helper()
	previous := healthRun
	healthRun = fn
	t.Cleanup(func() { healthRun = previous })
}

func okReport() health.Report {
	results := []health.Result{
		{Name: "nvidia-smi", Status: health.StatusOK, Detail: "2 GPUs detected"},
		{Name: "nvidia-peermem", Status: health.StatusOK, Detail: "nvidia_peermem loaded"},
		{Name: "iommu", Status: health.StatusOK, Detail: "iommu=pt set"},
	}
	return health.Report{Results: results, Overall: health.StatusOK}
}

func failReport() health.Report {
	results := []health.Result{
		{Name: "nvidia-smi", Status: health.StatusOK, Detail: "1 GPU detected"},
		{Name: "iommu", Status: health.StatusFail, Detail: "iommu broken", Hint: "add iommu=pt"},
	}
	return health.Report{Results: results, Overall: health.StatusFail}
}

func warnReport() health.Report {
	results := []health.Result{
		{Name: "nvidia-peermem", Status: health.StatusWarn, Detail: "nvidia_peermem not loaded", Hint: "modprobe nvidia-peermem"},
	}
	return health.Report{Results: results, Overall: health.StatusWarn}
}

func TestDoctorCommand_rendersTable_whenAllOK(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overrideHealthRun(t, func(context.Context) health.Report { return okReport() })
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, stderr, err := executeCommand(newRootCmd(), "doctor")
	// Then
	if err != nil {
		t.Fatalf("expected doctor to succeed: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	for _, want := range []string{"Check", "Status", "Detail", "Hint", "nvidia-smi", "iommu=pt set", "Overall:", "ok"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected table output to contain %q, got:\n%s", want, stdout)
		}
	}
}

func TestDoctorCommand_rendersJSON_whenOutputJSON(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overrideHealthRun(t, func(context.Context) health.Report { return okReport() })
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "--output", "json", "doctor")
	// Then
	if err != nil {
		t.Fatalf("expected doctor json to succeed: %v", err)
	}
	var payload struct {
		Results []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
			Hint   string `json:"hint"`
		} `json:"results"`
		Overall string `json:"overall"`
	}
	if derr := json.Unmarshal([]byte(stdout), &payload); derr != nil {
		t.Fatalf("expected valid JSON, got error %v for:\n%s", derr, stdout)
	}
	if payload.Overall != "ok" {
		t.Fatalf("expected overall ok, got %q", payload.Overall)
	}
	if len(payload.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(payload.Results))
	}
	if payload.Results[0].Name != "nvidia-smi" {
		t.Fatalf("expected first result nvidia-smi, got %q", payload.Results[0].Name)
	}
	if !strings.Contains(stdout, "  ") {
		t.Fatalf("expected 2-space indented JSON, got:\n%s", stdout)
	}
}

func TestDoctorCommand_rendersMarkdown_whenOutputMarkdown(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overrideHealthRun(t, func(context.Context) health.Report { return okReport() })
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "--output", "markdown", "doctor")
	// Then
	if err != nil {
		t.Fatalf("expected doctor markdown to succeed: %v", err)
	}
	for _, want := range []string{"## Health Checks", "| Check | Status | Detail | Hint |", "| --- | --- | --- | --- |", "nvidia-smi", "Overall:", "ok"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected markdown output to contain %q, got:\n%s", want, stdout)
		}
	}
}

func TestDoctorCommand_exitsZero_whenFailAndNotStrict(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overrideHealthRun(t, func(context.Context) health.Report { return failReport() })
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "doctor")
	// Then
	if err != nil {
		t.Fatalf("expected exit 0 even with failing checks (no --strict), got: %v", err)
	}
	if !strings.Contains(stdout, "iommu broken") {
		t.Fatalf("expected report printed, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "fail") {
		t.Fatalf("expected overall fail printed, got:\n%s", stdout)
	}
}

func TestDoctorCommand_exitsOne_whenFailAndStrict_reportStillPrinted(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overrideHealthRun(t, func(context.Context) health.Report { return failReport() })
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "doctor", "--strict")

	// Then
	if err == nil {
		t.Fatalf("expected --strict + fail to error")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.Code)
	}
	if !strings.Contains(err.Error(), "health checks failed") {
		t.Fatalf("expected strict failure message, got %q", err.Error())
	}
	if !strings.Contains(stdout, "iommu broken") {
		t.Fatalf("expected report STILL printed to stdout before strict error, got:\n%s", stdout)
	}
}

func TestDoctorCommand_exitsZero_whenWarnAndStrict(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overrideHealthRun(t, func(context.Context) health.Report { return warnReport() })
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "doctor", "--strict")
	// Then
	if err != nil {
		t.Fatalf("expected warn-only + --strict to exit 0 (only fail triggers strict nonzero), got: %v", err)
	}
	if !strings.Contains(stdout, "nvidia_peermem not loaded") {
		t.Fatalf("expected report printed, got:\n%s", stdout)
	}
}

func TestDoctorCommand_returnsExitCode2AndMessage_whenNotLinux(t *testing.T) {
	// Given
	called := false
	overridePlatform(t, false, "darwin")
	overrideHealthRun(t, func(context.Context) health.Report {
		called = true
		return okReport()
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "doctor")

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
	if !strings.Contains(err.Error(), "gpu-tools doctor requires Linux (uses /proc, lspci, ibv_devinfo); current OS: darwin") {
		t.Fatalf("expected linux-required message, got %q", err.Error())
	}
	if called {
		t.Fatalf("expected healthRun not to run on non-Linux")
	}
	if stdout != "" {
		t.Fatalf("expected no stdout for non-JSON unsupported platform, got %q", stdout)
	}
}

func TestDoctorCommand_emitsUnsupportedJSON_whenNotLinuxAndOutputJSON(t *testing.T) {
	// Given
	overridePlatform(t, false, "windows")
	overrideHealthRun(t, func(context.Context) health.Report {
		t.Fatalf("healthRun must not run on non-Linux")
		return health.Report{}
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "--output", "json", "doctor")

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
	wantTools := []string{"nvidia-smi", "lspci", "ibv_devinfo", "ibstat"}
	if len(payload.RequiredTools) != len(wantTools) {
		t.Fatalf("expected required_tools %v, got %v", wantTools, payload.RequiredTools)
	}
	for i, want := range wantTools {
		if payload.RequiredTools[i] != want {
			t.Fatalf("expected required_tools[%d]=%q, got %q", i, want, payload.RequiredTools[i])
		}
	}
}

func TestRenderDoctor_returnsError_whenOutputUnknown(t *testing.T) {
	// When
	err := renderDoctor(&strings.Builder{}, "xml", okReport())

	// Then
	if err == nil {
		t.Fatalf("expected unknown output format to fail")
	}
	if !strings.Contains(err.Error(), "unknown doctor output format") {
		t.Fatalf("expected unknown format error, got %q", err.Error())
	}
}

// failWriter fails every write, exercising the render error-return branches.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestRenderDoctorTable_returnsError_whenWriteFails(t *testing.T) {
	if err := renderDoctorTable(failWriter{}, okReport()); err == nil {
		t.Fatalf("expected table render to propagate write error")
	}
}

func TestRenderDoctorMarkdown_returnsError_whenWriteFails(t *testing.T) {
	if err := renderDoctorMarkdown(failWriter{}, okReport()); err == nil {
		t.Fatalf("expected markdown render to propagate write error")
	}
}

func TestRenderDoctorJSON_returnsError_whenWriteFails(t *testing.T) {
	if err := renderDoctorJSON(failWriter{}, okReport()); err == nil {
		t.Fatalf("expected json render to propagate write error")
	}
}

func TestDoctorCommand_returnsRenderError_whenOutputWriterFails(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overrideHealthRun(t, func(context.Context) health.Report { return okReport() })
	t.Setenv("HOME", t.TempDir())
	root := newRootCmd()
	root.SetOut(failWriter{})
	root.SetArgs([]string{"doctor"})

	// When
	err := root.Execute()

	// Then
	if err == nil {
		t.Fatalf("expected render write failure to propagate")
	}
	if !strings.Contains(err.Error(), "render doctor report") {
		t.Fatalf("expected render doctor error, got %q", err.Error())
	}
}

func TestDoctorCommand_returnsConfigError_whenConfigResolutionFails(t *testing.T) {
	// Given a malformed output flag so resolvedConfig fails inside runDoctor.
	overridePlatform(t, true, "linux")
	overrideHealthRun(t, func(context.Context) health.Report {
		t.Fatalf("healthRun must not run when config resolution fails")
		return health.Report{}
	})
	root := &cobra.Command{Use: "gpu-tools"}
	child := &cobra.Command{Use: "doctor"}
	root.PersistentFlags().String(configFlag, "", "")
	root.PersistentFlags().Bool(outputFlag, false, "")
	root.PersistentFlags().String(backendFlag, "auto", "")
	if err := root.PersistentFlags().Set(outputFlag, "true"); err != nil {
		t.Fatalf("set output flag: %v", err)
	}
	root.AddCommand(child)

	// When
	err := runDoctor(child, false)

	// Then
	if err == nil {
		t.Fatalf("expected config resolution failure")
	}
	if !strings.Contains(err.Error(), "read --output") {
		t.Fatalf("expected output flag read error, got %q", err.Error())
	}
}

func TestDoctorUnsupported_returnsEncodeError_whenJSONWriterFails(t *testing.T) {
	// Given a non-Linux platform, JSON output, and a failing stdout writer.
	overridePlatform(t, false, "darwin")
	root := newRootCmd()
	root.SetOut(failWriter{})
	root.SetArgs([]string{"--output", "json", "doctor"})

	// When
	err := root.Execute()

	// Then
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %d", exitErr.Code)
	}
	if !strings.Contains(err.Error(), "encode unsupported-platform payload") {
		t.Fatalf("expected encode error, got %q", err.Error())
	}
}

func TestHealthRun_defaultSeam_returnsReport(t *testing.T) {
	// When the default seam runs the real probe set.
	report := healthRun(context.Background())

	// Then it produces an overall status without panicking.
	if report.Overall == "" {
		t.Fatalf("expected non-empty overall status from default healthRun")
	}
}
