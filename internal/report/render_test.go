package report

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sunerpy/gpu-tools/internal/gpu"
)

func Test_TableRenderer_Render_matches_golden_when_devices_present(t *testing.T) {
	// Given
	snap := fixedSnapshot()

	// When
	got := renderString(t, TableRenderer{}, snap)

	// Then
	want := readGolden(t, "table.golden")
	if got != want {
		t.Fatalf("table output mismatch\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func Test_JSONRenderer_Render_matches_golden_when_devices_present(t *testing.T) {
	// Given
	snap := fixedSnapshot()

	// When
	got := renderString(t, JSONRenderer{}, snap)

	// Then
	want := readGolden(t, "snapshot.json.golden")
	if got != want {
		t.Fatalf("json output mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func Test_MarkdownRenderer_Render_matches_golden_when_devices_present(t *testing.T) {
	// Given
	snap := fixedSnapshot()

	// When
	got := renderString(t, MarkdownRenderer{}, snap)

	// Then
	want := readGolden(t, "snapshot.md.golden")
	if got != want {
		t.Fatalf("markdown output mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func Test_Renderers_Render_empty_snapshot_without_devices(t *testing.T) {
	tests := []struct {
		name     string
		renderer Renderer
		want     string
	}{
		{
			name:     "table",
			renderer: TableRenderer{},
			want:     "no devices\n",
		},
		{
			name:     "json",
			renderer: JSONRenderer{},
			want: `{
  "Host": "empty-host",
  "Timestamp": "2026-01-02T03:04:05Z",
  "Backend": "none",
  "Devices": []
}
`,
		},
		{
			name:     "markdown",
			renderer: MarkdownRenderer{},
			want: "## GPU Snapshot\n\n" +
				"- Host: `empty-host`\n" +
				"- Backend: `none`\n" +
				"- Timestamp: `2026-01-02T03:04:05Z`\n\n" +
				"No devices.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			snap := emptySnapshot()

			// When
			got := renderString(t, tt.renderer, snap)

			// Then
			if got != tt.want {
				t.Fatalf("%s empty output mismatch\nwant:\n%q\ngot:\n%q", tt.name, tt.want, got)
			}
		})
	}
}

func Test_RendererFor_returns_renderer_when_format_known(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   Renderer
	}{
		{name: "table", format: "table", want: TableRenderer{}},
		{name: "json", format: "json", want: JSONRenderer{}},
		{name: "markdown", format: "markdown", want: MarkdownRenderer{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got, err := RendererFor(tt.format)
			// Then
			if err != nil {
				t.Fatalf("RendererFor(%q) unexpected error: %v", tt.format, err)
			}
			if got == nil {
				t.Fatalf("RendererFor(%q) returned nil renderer", tt.format)
			}
			if renderString(t, got, emptySnapshot()) != renderString(t, tt.want, emptySnapshot()) {
				t.Fatalf("RendererFor(%q) returned wrong renderer", tt.format)
			}
		})
	}
}

func Test_RendererFor_returns_unknown_format_error_when_format_unknown(t *testing.T) {
	// When
	renderer, err := RendererFor("yaml")

	// Then
	if renderer != nil {
		t.Fatalf("RendererFor returned renderer for unknown format: %T", renderer)
	}
	var formatErr *UnknownFormatError
	if !errors.As(err, &formatErr) {
		t.Fatalf("RendererFor error type = %T, want *UnknownFormatError", err)
	}
	if formatErr.Format != "yaml" {
		t.Fatalf("UnknownFormatError.Format = %q, want yaml", formatErr.Format)
	}
	if err.Error() != `unknown output format "yaml"` {
		t.Fatalf("UnknownFormatError text = %q", err.Error())
	}
}

func Test_Renderers_return_writer_error_when_writer_fails(t *testing.T) {
	tests := []struct {
		name     string
		renderer Renderer
	}{
		{name: "table", renderer: TableRenderer{}},
		{name: "json", renderer: JSONRenderer{}},
		{name: "markdown", renderer: MarkdownRenderer{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			writer := failingWriter{}

			// When
			err := tt.renderer.Render(writer, fixedSnapshot())

			// Then
			if !errors.Is(err, errWriteFailed) {
				t.Fatalf("Render error = %v, want errWriteFailed", err)
			}
		})
	}
}

func renderString(t *testing.T, renderer Renderer, snap *Snapshot) string {
	t.Helper()

	var buf bytes.Buffer
	if err := renderer.Render(&buf, snap); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func readGolden(t *testing.T, name string) string {
	t.Helper()

	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return string(data)
}

func fixedSnapshot() *Snapshot {
	eccEnabled := true
	eccDisabled := false
	fanSpeed := 68

	return &Snapshot{
		Host:      "gpu-host-01",
		Timestamp: fixedTimestamp(),
		Backend:   "nvml",
		Devices: []gpu.Device{
			{
				Index:           0,
				UUID:            "GPU-aaaaaaaa-bbbb-cccc-dddd-eeeeeeee0000",
				Name:            "NVIDIA RTX 6000 Ada",
				MemoryTotal:     24 * 1024 * 1024 * 1024,
				MemoryUsed:      2 * 1024 * 1024 * 1024,
				Temperature:     63,
				PowerDraw:       215500,
				PowerLimit:      300000,
				ClockGraphics:   2100,
				ClockMem:        5001,
				UtilizationGPU:  72,
				UtilizationMem:  41,
				ThrottleReasons: []string{"sw_power_cap", "gpu_idle"},
				ECCEnabled:      &eccEnabled,
				MIGEnabled:      false,
				PState:          "P0",
				FanSpeed:        &fanSpeed,
				DriverVersion:   "555.42.02",
				CudaVersion:     "12.5",
			},
			{
				Index:           1,
				UUID:            "GPU-aaaaaaaa-bbbb-cccc-dddd-eeeeeeee0001",
				Name:            "NVIDIA L4",
				MemoryTotal:     24 * 1024 * 1024 * 1024,
				MemoryUsed:      1024 * 1024 * 1024,
				Temperature:     51,
				PowerDraw:       72000,
				PowerLimit:      72000,
				ClockGraphics:   1590,
				ClockMem:        6251,
				UtilizationGPU:  15,
				UtilizationMem:  8,
				ThrottleReasons: []string{},
				ECCEnabled:      &eccDisabled,
				MIGEnabled:      true,
				MIGDevices: []gpu.MIGDevice{
					{
						GIID:        3,
						CIID:        7,
						UUID:        "MIG-GPU-aaaaaaaa-bbbb-cccc-dddd-eeeeeeee0001/3/7",
						MemoryTotal: 5 * 1024 * 1024 * 1024,
					},
				},
				PState:        "P8",
				DriverVersion: "555.42.02",
				CudaVersion:   "12.5",
			},
		},
	}
}

func emptySnapshot() *Snapshot {
	return &Snapshot{
		Host:      "empty-host",
		Timestamp: fixedTimestamp(),
		Backend:   "none",
		Devices:   []gpu.Device{},
	}
}

func fixedTimestamp() time.Time {
	return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
}

var errWriteFailed = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write(_ []byte) (int, error) {
	return 0, errWriteFailed
}

func Test_Renderers_never_emit_ansi_escape_sequences(t *testing.T) {
	tests := []struct {
		name     string
		renderer Renderer
	}{
		{name: "table", renderer: TableRenderer{}},
		{name: "json", renderer: JSONRenderer{}},
		{name: "markdown", renderer: MarkdownRenderer{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got := renderString(t, tt.renderer, fixedSnapshot())

			// Then
			if strings.Contains(got, "\x1b[") {
				t.Fatalf("%s output contains ANSI escape sequence: %q", tt.name, got)
			}
		})
	}
}
