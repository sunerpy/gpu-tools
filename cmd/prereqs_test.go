package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/sunerpy/gpu-tools/internal/prereq"
)

func overridePrereqChecks(t *testing.T, fn func() []prereq.CheckResult) {
	t.Helper()
	previous := prereqChecks
	prereqChecks = fn
	t.Cleanup(func() { prereqChecks = previous })
}

func samplePrereqChecks() []prereq.CheckResult {
	return []prereq.CheckResult{
		{
			Tool:    "nvidia-smi",
			Binary:  "nvidia-smi",
			Found:   true,
			Path:    "/usr/bin/nvidia-smi",
			Purpose: "NVIDIA GPU query/management",
			Install: "apt install nvidia-driver-<ver>",
		},
		{
			Tool:    "perftest",
			Binary:  "ib_write_bw",
			Found:   false,
			Path:    "",
			Purpose: "RDMA bandwidth/latency benchmark",
			Install: "apt install perftest",
		},
	}
}

func TestPrereqsCommand_rendersTable_whenChecksSucceed(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overridePrereqChecks(t, samplePrereqChecks)
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, stderr, err := executeCommand(newRootCmd(), "prereqs")
	// Then
	if err != nil {
		t.Fatalf("expected prereqs to succeed: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	for _, want := range []string{
		"Prerequisite Tools", "Tool", "Found", "Path", "Purpose", "Install",
		"nvidia-smi", "/usr/bin/nvidia-smi", "true",
		"perftest", "false", "apt install perftest",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected table output to contain %q, got:\n%s", want, stdout)
		}
	}
	// Found tools hide the install hint behind a dash.
	if !strings.Contains(stdout, "-") {
		t.Fatalf("expected a dash for the found tool's install cell, got:\n%s", stdout)
	}
}

func TestPrereqsCommand_rendersJSON_whenOutputJSON(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overridePrereqChecks(t, samplePrereqChecks)
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "--output", "json", "prereqs")
	// Then
	if err != nil {
		t.Fatalf("expected prereqs json to succeed: %v", err)
	}
	var decoded []struct {
		Tool    string `json:"tool"`
		Binary  string `json:"binary"`
		Found   bool   `json:"found"`
		Path    string `json:"path"`
		Purpose string `json:"purpose"`
		Install string `json:"install"`
	}
	if derr := json.Unmarshal([]byte(stdout), &decoded); derr != nil {
		t.Fatalf("expected valid JSON, got error %v for:\n%s", derr, stdout)
	}
	if len(decoded) != 2 {
		t.Fatalf("expected 2 checks in JSON, got %d", len(decoded))
	}
	if !decoded[0].Found || decoded[0].Path != "/usr/bin/nvidia-smi" {
		t.Fatalf("expected first check found with path, got %+v", decoded[0])
	}
	if decoded[1].Found || decoded[1].Install != "apt install perftest" {
		t.Fatalf("expected second check missing with install hint, got %+v", decoded[1])
	}
	if !strings.Contains(stdout, "  ") {
		t.Fatalf("expected 2-space indented JSON, got:\n%s", stdout)
	}
}

func TestPrereqsCommand_rendersMarkdown_whenOutputMarkdown(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overridePrereqChecks(t, samplePrereqChecks)
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "--output", "markdown", "prereqs")
	// Then
	if err != nil {
		t.Fatalf("expected prereqs markdown to succeed: %v", err)
	}
	for _, want := range []string{
		"## Prerequisite Tools",
		"| Tool | Found | Path | Purpose | Install |",
		"| nvidia-smi | true | /usr/bin/nvidia-smi |",
		"| perftest | false | - |",
		"apt install perftest",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected markdown output to contain %q, got:\n%s", want, stdout)
		}
	}
}

func TestPrereqsCommand_exitsZero_whenToolsMissing(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overridePrereqChecks(t, func() []prereq.CheckResult {
		return []prereq.CheckResult{
			{
				Tool:    "dcgm",
				Binary:  "dcgmi",
				Found:   false,
				Install: "install datacenter-gpu-manager",
			},
		}
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "prereqs")
	// Then: informational, so exit 0 even when everything is missing.
	if err != nil {
		t.Fatalf("expected prereqs to exit 0 even when tools missing, got: %v", err)
	}
	if !strings.Contains(stdout, "install datacenter-gpu-manager") {
		t.Fatalf("expected install hint for missing tool, got:\n%s", stdout)
	}
}

func TestPrereqsCommand_returnsExitCode2AndMessage_whenNotLinux(t *testing.T) {
	// Given
	called := false
	overridePlatform(t, false, "darwin")
	overridePrereqChecks(t, func() []prereq.CheckResult {
		called = true
		return nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "prereqs")

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
	if !strings.Contains(err.Error(), "gpu-tools prereqs requires Linux; current OS: darwin") {
		t.Fatalf("expected linux-required message, got %q", err.Error())
	}
	if called {
		t.Fatalf("expected prereqChecks not to run on non-Linux")
	}
	if stdout != "" {
		t.Fatalf("expected no stdout for non-JSON unsupported platform, got %q", stdout)
	}
}

func TestPrereqsCommand_emitsUnsupportedJSON_whenNotLinuxAndOutputJSON(t *testing.T) {
	// Given
	overridePlatform(t, false, "windows")
	overridePrereqChecks(t, func() []prereq.CheckResult {
		t.Fatalf("prereqChecks must not run on non-Linux")
		return nil
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "--output", "json", "prereqs")

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
	if len(payload.RequiredTools) != len(prereq.Tools) {
		t.Fatalf("expected %d required_tools, got %d", len(prereq.Tools), len(payload.RequiredTools))
	}
	if payload.RequiredTools[0] != "nvidia-smi" {
		t.Fatalf("expected first required tool nvidia-smi, got %q", payload.RequiredTools[0])
	}
}

func TestRenderPrereqs_returnsError_whenOutputUnknown(t *testing.T) {
	// When
	err := renderPrereqs(&strings.Builder{}, "xml", samplePrereqChecks())

	// Then
	if err == nil {
		t.Fatalf("expected unknown output format to fail")
	}
	if !strings.Contains(err.Error(), "unknown prereqs output format") {
		t.Fatalf("expected unknown format error, got %q", err.Error())
	}
}

func TestRenderPrereqs_propagatesWriteError(t *testing.T) {
	checks := samplePrereqChecks()
	for _, output := range []string{"table", "json", "markdown"} {
		if err := renderPrereqs(failingWriter{}, output, checks); err == nil {
			t.Fatalf("expected %s render to propagate write error", output)
		}
	}
}

func TestPrereqChecks_defaultSeamReturnsCatalog(t *testing.T) {
	got := prereqChecks()
	if len(got) != len(prereq.Tools) {
		t.Fatalf("default prereqChecks returned %d results, want %d", len(got), len(prereq.Tools))
	}
}

func TestPrereqsCommand_returnsRenderError_whenOutputWriterFails(t *testing.T) {
	// Given
	overridePlatform(t, true, "linux")
	overridePrereqChecks(t, samplePrereqChecks)
	t.Setenv("HOME", t.TempDir())
	root := newRootCmd()
	root.SetOut(failingWriter{})
	root.SetArgs([]string{"prereqs"})

	// When
	err := root.Execute()

	// Then
	if err == nil {
		t.Fatalf("expected render write failure to propagate")
	}
	if !strings.Contains(err.Error(), "render prereqs result") {
		t.Fatalf("expected render prereqs error, got %q", err.Error())
	}
}

func TestPrereqsCommand_returnsConfigError_whenConfigResolutionFails(t *testing.T) {
	// Given a malformed output flag so resolvedConfig fails inside runPrereqs.
	overridePlatform(t, true, "linux")
	overridePrereqChecks(t, func() []prereq.CheckResult {
		t.Fatalf("prereqChecks must not run when config resolution fails")
		return nil
	})
	root := &cobra.Command{Use: "gpu-tools"}
	child := &cobra.Command{Use: "prereqs"}
	root.PersistentFlags().String(configFlag, "", "")
	root.PersistentFlags().Bool(outputFlag, false, "")
	root.PersistentFlags().String(backendFlag, "auto", "")
	if err := root.PersistentFlags().Set(outputFlag, "true"); err != nil {
		t.Fatalf("set output flag: %v", err)
	}
	root.AddCommand(child)

	// When
	err := runPrereqs(child)

	// Then
	if err == nil {
		t.Fatalf("expected config resolution failure")
	}
	if !strings.Contains(err.Error(), "read --output") {
		t.Fatalf("expected output flag read error, got %q", err.Error())
	}
}

func TestPrereqsUnsupported_returnsEncodeError_whenJSONWriterFails(t *testing.T) {
	// Given a non-Linux platform, JSON output, and a failing stdout writer.
	overridePlatform(t, false, "darwin")
	root := newRootCmd()
	root.SetOut(failingWriter{})
	root.SetArgs([]string{"--output", "json", "prereqs"})

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

func TestInstallCell_showsDash_whenFoundOrNoHint(t *testing.T) {
	// Found tool -> dash regardless of install hint.
	if got := installCell(prereq.CheckResult{Found: true, Install: "apt install x"}); got != "-" {
		t.Fatalf("expected dash for found tool, got %q", got)
	}
	// Missing tool with no hint -> dash.
	if got := installCell(prereq.CheckResult{Found: false, Install: ""}); got != "-" {
		t.Fatalf("expected dash for missing tool without hint, got %q", got)
	}
	// Missing tool with hint -> the hint.
	if got := installCell(prereq.CheckResult{Found: false, Install: "apt install x"}); got != "apt install x" {
		t.Fatalf("expected hint for missing tool, got %q", got)
	}
}
