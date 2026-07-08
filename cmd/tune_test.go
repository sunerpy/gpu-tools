package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

func TestTuneCommand_rendersTableWithRecommendations_whenCollectorSucceeds(t *testing.T) {
	// Given
	eccEnabled := true
	collector := newFakeCollector([]gpu.Device{{
		Index:           0,
		UUID:            "GPU-tune-0",
		Name:            "RTX Tune",
		PowerDraw:       100_000,
		PowerLimit:      200_000,
		ECCEnabled:      &eccEnabled,
		ThrottleReasons: []string{"sw_power_cap"},
		Temperature:     90,
	}})
	overrideGPUFactory(t, collector, nil)
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, stderr, err := executeCommand(newRootCmd(), "tune", "-o", "table")
	// Then
	if err != nil {
		t.Fatalf("expected tune table to succeed: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	for _, want := range []string{"RTX Tune", "warning", "High temperature", "Power headroom available", "Power-cap throttle active"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected table output to contain %q, got:\n%s", want, stdout)
		}
	}
	if !collector.initCalled || !collector.shutdownCalled {
		t.Fatalf("expected Init and Shutdown to be called, got init=%v shutdown=%v", collector.initCalled, collector.shutdownCalled)
	}
}

func TestTuneCommand_rendersValidJSON_whenJSONOutputRequested(t *testing.T) {
	// Given
	collector := newFakeCollector([]gpu.Device{{Index: 1, UUID: "GPU-json", Name: "JSON GPU", Temperature: 85}})
	overrideGPUFactory(t, collector, nil)
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, stderr, err := executeCommand(newRootCmd(), "tune", "-o", "json")
	// Then
	if err != nil {
		t.Fatalf("expected tune json to succeed: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	var report struct {
		Devices []struct {
			Index           int    `json:"index"`
			UUID            string `json:"uuid"`
			Name            string `json:"name"`
			Recommendations []struct {
				Severity string `json:"severity"`
				Title    string `json:"title"`
			} `json:"recommendations"`
		} `json:"devices"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("expected valid tune JSON, got error %v and output:\n%s", err, stdout)
	}
	if len(report.Devices) != 1 || report.Devices[0].Name != "JSON GPU" {
		t.Fatalf("expected JSON GPU device, got %#v", report.Devices)
	}
	got := report.Devices[0].Recommendations
	if len(got) != 1 || got[0].Severity != "warning" || got[0].Title != "High temperature" {
		t.Fatalf("expected high temperature warning, got %#v", got)
	}
}

func TestTuneCommand_rendersMarkdownWithNoRecommendations_whenDeviceIsHealthy(t *testing.T) {
	// Given
	collector := newFakeCollector([]gpu.Device{{Index: 2, UUID: "GPU-md", Name: "Markdown GPU", Temperature: 40}})
	overrideGPUFactory(t, collector, nil)
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "tune", "-o", "markdown")
	// Then
	if err != nil {
		t.Fatalf("expected tune markdown to succeed: %v", err)
	}
	for _, want := range []string{"# GPU tuning recommendations", "Markdown GPU", "No recommendations."} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected markdown output to contain %q, got:\n%s", want, stdout)
		}
	}
}

func TestTuneCommand_returnsExitError_whenBackendUnavailable(t *testing.T) {
	// Given
	overrideGPUFactory(t, nil, gpu.ErrBackendUnavailable)
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "tune")

	// Then
	if err == nil {
		t.Fatalf("expected backend unavailable to fail")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.Code)
	}
	if !strings.Contains(err.Error(), "no NVIDIA GPU detected") {
		t.Fatalf("expected friendly no NVIDIA message, got %q", err.Error())
	}
}

func TestTuneCommand_returnsWrappedError_whenCollectorFlowFails(t *testing.T) {
	tests := []struct {
		name      string
		collector *fakeCollector
		want      string
	}{
		{name: "init fails", collector: &fakeCollector{initErr: errors.New("init failed")}, want: "initialize GPU collector"},
		{name: "device count fails", collector: &fakeCollector{countErr: errors.New("count failed")}, want: "count GPU devices"},
		{name: "device read fails", collector: &fakeCollector{devices: []gpu.Device{{Index: 0}}, deviceErr: errors.New("device failed")}, want: "read GPU device 0"},
		{name: "device read returns nil", collector: &fakeCollector{devices: []gpu.Device{{Index: 0}}, nilDevice: true}, want: "read GPU device 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			overrideGPUFactory(t, tt.collector, nil)
			t.Setenv("HOME", t.TempDir())

			// When
			_, _, err := executeCommand(newRootCmd(), "tune")

			// Then
			if err == nil {
				t.Fatalf("expected tune to fail")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error to contain %q, got %q", tt.want, err.Error())
			}
		})
	}
}

func TestTuneCommand_returnsWrappedError_whenRenderFails(t *testing.T) {
	// Given
	collector := newFakeCollector([]gpu.Device{{Index: 0, Name: "writer GPU", Temperature: 90}})
	overrideGPUFactory(t, collector, nil)
	t.Setenv("HOME", t.TempDir())
	root := newRootCmd()
	root.SetOut(failingWriter{})
	root.SetArgs([]string{"tune"})

	// When
	err := root.Execute()

	// Then
	if err == nil {
		t.Fatalf("expected tune render to fail")
	}
	if !strings.Contains(err.Error(), "render tune recommendations") {
		t.Fatalf("expected render error, got %q", err.Error())
	}
}

func TestRunTune_returnsConfigError_whenRunWithoutRootFlags(t *testing.T) {
	// Given
	cmd := newTuneCmd()

	// When
	err := runTune(cmd)

	// Then
	if err == nil {
		t.Fatalf("expected missing root flags to fail")
	}
	if !strings.Contains(err.Error(), "read --config") {
		t.Fatalf("expected config flag read error, got %q", err.Error())
	}
}

func TestRenderTuneReport_returnsError_whenFormatUnsupported(t *testing.T) {
	// When
	err := renderTuneReport(&bytes.Buffer{}, "xml", tuneReport{})

	// Then
	if err == nil {
		t.Fatalf("expected unsupported format to fail")
	}
	if !strings.Contains(err.Error(), "unsupported tune output") {
		t.Fatalf("expected unsupported output error, got %q", err.Error())
	}
}

func TestRenderTuneReport_rendersTableNoRecommendationRow_whenDeviceIsHealthy(t *testing.T) {
	// Given
	report := tuneReport{Devices: []tuneDeviceReport{{Index: 4, Name: "Healthy GPU"}}}
	buf := &bytes.Buffer{}

	// When
	err := renderTuneReport(buf, "table", report)
	// Then
	if err != nil {
		t.Fatalf("expected table render to succeed: %v", err)
	}
	for _, want := range []string{"Healthy GPU", "No recommendations.", "No tuning actions suggested."} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("expected table output to contain %q, got:\n%s", want, buf.String())
		}
	}
}

func TestRenderTuneReport_returnsWriterError_whenOutputWriteFails(t *testing.T) {
	tests := []struct {
		name   string
		format string
		report tuneReport
	}{
		{name: "table header", format: "table"},
		{name: "json encode", format: "json"},
		{name: "markdown header", format: "markdown"},
		{name: "table no recommendations row", format: "table", report: tuneReport{Devices: []tuneDeviceReport{{Index: 0}}}},
		{name: "table recommendation row", format: "table", report: tuneReport{Devices: []tuneDeviceReport{{Index: 0, Recommendations: []tuneRecommendationReport{{Title: "T"}}}}}},
		{name: "markdown device heading", format: "markdown", report: tuneReport{Devices: []tuneDeviceReport{{Index: 0}}}},
		{name: "markdown no recommendations row", format: "markdown", report: tuneReport{Devices: []tuneDeviceReport{{Index: 0}}}},
		{name: "markdown recommendation row", format: "markdown", report: tuneReport{Devices: []tuneDeviceReport{{Index: 0, Recommendations: []tuneRecommendationReport{{Title: "T"}}}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			writer := failAfterWriter{allowedWrites: allowedWritesForTuneRenderError(tt.name)}

			// When
			err := renderTuneReport(&writer, tt.format, tt.report)

			// Then
			if err == nil {
				t.Fatalf("expected writer error")
			}
		})
	}
}

func TestTuneDeviceLabel_prefersNameThenUUIDThenIndex(t *testing.T) {
	tests := []struct {
		name   string
		device tuneDeviceReport
		want   string
	}{
		{name: "name", device: tuneDeviceReport{Index: 1, UUID: "GPU-1", Name: "Named GPU"}, want: "GPU 1 Named GPU"},
		{name: "uuid", device: tuneDeviceReport{Index: 2, UUID: "GPU-2"}, want: "GPU 2 GPU-2"},
		{name: "index", device: tuneDeviceReport{Index: 3}, want: "GPU 3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got := tuneDeviceLabel(tt.device)

			// Then
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

type failAfterWriter struct {
	allowedWrites int
	writes        int
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.writes >= w.allowedWrites {
		return 0, errors.New("write failed")
	}
	w.writes++
	return len(p), nil
}

func allowedWritesForTuneRenderError(name string) int {
	switch name {
	case "table header", "json encode", "markdown header":
		return 0
	case "table no recommendations row", "table recommendation row", "markdown device heading":
		return 1
	case "markdown no recommendations row", "markdown recommendation row":
		return 2
	default:
		return 0
	}
}
