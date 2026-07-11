package cmd

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sunerpy/gpu-tools/core"
	"github.com/sunerpy/gpu-tools/internal/report"
)

func TestRenderReport_appendsMarkdownSummary_whenFormatIsMarkdown(t *testing.T) {
	// Given
	snapshot := &report.Snapshot{Devices: reportDevices()}

	// When
	rendered, err := renderReport(core.OutputMarkdown, snapshot)
	// Then
	if err != nil {
		t.Fatalf("expected markdown render to succeed: %v", err)
	}
	assertReportMarkdown(t, string(rendered))
}

func TestWithMarkdownSummary_insertsSeparatorNewline_whenRenderedHasNoTrailingNewline(t *testing.T) {
	// Given
	snapshot := &report.Snapshot{Devices: reportDevices()}

	// When
	got := withMarkdownSummary([]byte("body without newline"), snapshot)

	// Then
	text := string(got)
	if !strings.Contains(text, "body without newline\n\n## Summary") {
		t.Fatalf("expected summary to be separated by newlines, got:\n%s", text)
	}
}

func TestWriteReportFile_returnsCreateError_whenParentDirectoryIsMissing(t *testing.T) {
	// Given
	path := filepath.Join(t.TempDir(), "missing", "snapshot.md")

	// When
	err := writeReportFile(path, []byte("report"))

	// Then
	if err == nil {
		t.Fatalf("expected create file error")
	}
	if !strings.Contains(err.Error(), "create file") {
		t.Fatalf("expected create file error, got %q", err.Error())
	}
}

func TestHostname_returnsEmptyString_whenOSHostnameFails(t *testing.T) {
	// Given
	previous := osHostname
	osHostname = func() (string, error) {
		return "", errors.New("hostname failed")
	}
	t.Cleanup(func() { osHostname = previous })

	// When
	got := hostname()

	// Then
	if got != "" {
		t.Fatalf("hostname = %q, want empty string", got)
	}
}
