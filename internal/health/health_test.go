package health

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// fakeRunner dispatches canned output by the base name of the executed
// command. Each tool key maps to its stdout and error; an unrecognized tool
// returns an error so a probe never silently reads another tool's output.
type fakeRunner struct {
	outputs map[string][]byte
	errs    map[string]error
	calls   []fakeRunCall
}

type fakeRunCall struct {
	name string
	args []string
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, fakeRunCall{name: name, args: append([]string(nil), args...)})
	key := filepath.Base(name)
	if err, ok := r.errs[key]; ok && err != nil {
		return nil, err
	}
	if out, ok := r.outputs[key]; ok {
		return append([]byte(nil), out...), nil
	}
	return nil, errors.New("unexpected tool: " + key)
}

func overrideLookPath(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	previous := lookPath
	lookPath = fn
	t.Cleanup(func() { lookPath = previous })
}

// lookPathAll resolves every tool to /usr/bin/<name>.
func lookPathAll(name string) (string, error) { return "/usr/bin/" + name, nil }

// lookPathMissing fails to resolve the named tool while resolving all others.
func lookPathMissing(missing string) func(string) (string, error) {
	return func(name string) (string, error) {
		if name == missing {
			return "", exec.ErrNotFound
		}
		return "/usr/bin/" + name, nil
	}
}

func overrideHintFor(t *testing.T, fn func(string) string) {
	t.Helper()
	previous := hintFor
	hintFor = fn
	t.Cleanup(func() { hintFor = previous })
}

func overrideProcModules(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "modules")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write proc modules: %v", err)
	}
	previous := procModulesPath
	procModulesPath = path
	t.Cleanup(func() { procModulesPath = previous })
}

func overrideProcModulesPath(t *testing.T, path string) {
	t.Helper()
	previous := procModulesPath
	procModulesPath = path
	t.Cleanup(func() { procModulesPath = previous })
}

func overrideProcCmdline(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cmdline")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write proc cmdline: %v", err)
	}
	previous := procCmdlinePath
	procCmdlinePath = path
	t.Cleanup(func() { procCmdlinePath = previous })
}

func overrideProcCmdlinePath(t *testing.T, path string) {
	t.Helper()
	previous := procCmdlinePath
	procCmdlinePath = path
	t.Cleanup(func() { procCmdlinePath = previous })
}

func TestNvidiaSmiProbe(t *testing.T) {
	// Given
	tests := []struct {
		name       string
		lookPath   func(string) (string, error)
		output     []byte
		err        error
		wantStatus Status
	}{
		{
			name:       "ok when GPUs listed",
			lookPath:   lookPathAll,
			output:     []byte("GPU 0: NVIDIA A100 (UUID: GPU-111)\nGPU 1: NVIDIA L40S (UUID: GPU-222)\n"),
			wantStatus: StatusOK,
		},
		{
			name:       "warn when no GPUs listed",
			lookPath:   lookPathAll,
			output:     []byte("No devices were found\n"),
			wantStatus: StatusWarn,
		},
		{
			name:       "skip when lookPath fails",
			lookPath:   lookPathMissing("nvidia-smi"),
			wantStatus: StatusSkip,
		},
		{
			name:       "skip when exec fails",
			lookPath:   lookPathAll,
			err:        errors.New("boom"),
			wantStatus: StatusSkip,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrideLookPath(t, tt.lookPath)
			runner := &fakeRunner{
				outputs: map[string][]byte{"nvidia-smi": tt.output},
				errs:    map[string]error{"nvidia-smi": tt.err},
			}

			// When
			got := nvidiaSmiProbe{runner: runner}.Run(context.Background())

			// Then
			if got.Status != tt.wantStatus {
				t.Fatalf("expected status %q, got %q (%s)", tt.wantStatus, got.Status, got.Detail)
			}
			if got.Name != "nvidia-smi" {
				t.Fatalf("expected name nvidia-smi, got %q", got.Name)
			}
		})
	}
}

func TestNvidiaSmiProbe_countsSingleGPU(t *testing.T) {
	// Given
	overrideLookPath(t, lookPathAll)
	runner := &fakeRunner{outputs: map[string][]byte{"nvidia-smi": []byte("GPU 0: NVIDIA A100 (UUID: GPU-111)\n")}}

	// When
	got := nvidiaSmiProbe{runner: runner}.Run(context.Background())

	// Then
	if got.Status != StatusOK {
		t.Fatalf("expected ok, got %q", got.Status)
	}
	if got.Detail != "1 GPU detected" {
		t.Fatalf("expected singular detail, got %q", got.Detail)
	}
}

func TestPeermemProbe(t *testing.T) {
	t.Run("ok when peermem loaded", func(t *testing.T) {
		// Given
		overrideProcModules(t, "nvidia_peermem 16384 0 - Live 0x0000000000000000\nnvidia 12345 1 - Live 0x0\n")

		// When
		got := peermemProbe{}.Run(context.Background())

		// Then
		if got.Status != StatusOK {
			t.Fatalf("expected ok, got %q", got.Status)
		}
	})

	t.Run("warn when peermem absent", func(t *testing.T) {
		// Given
		overrideProcModules(t, "nvidia 12345 1 - Live 0x0\n")

		// When
		got := peermemProbe{}.Run(context.Background())

		// Then
		if got.Status != StatusWarn {
			t.Fatalf("expected warn, got %q", got.Status)
		}
		if got.Hint != "modprobe nvidia-peermem" {
			t.Fatalf("expected modprobe hint, got %q", got.Hint)
		}
	})

	t.Run("skip when file unreadable", func(t *testing.T) {
		// Given
		overrideProcModulesPath(t, filepath.Join(t.TempDir(), "does-not-exist"))

		// When
		got := peermemProbe{}.Run(context.Background())

		// Then
		if got.Status != StatusSkip {
			t.Fatalf("expected skip, got %q", got.Status)
		}
	})
}

func TestIommuProbe(t *testing.T) {
	tests := []struct {
		name       string
		cmdline    string
		wantStatus Status
	}{
		{name: "ok when iommu=pt", cmdline: "BOOT_IMAGE=/vmlinuz intel_iommu=on iommu=pt quiet\n", wantStatus: StatusOK},
		{name: "warn when iommu set but not pt", cmdline: "BOOT_IMAGE=/vmlinuz iommu=on quiet\n", wantStatus: StatusWarn},
		{name: "warn when iommu absent", cmdline: "BOOT_IMAGE=/vmlinuz quiet\n", wantStatus: StatusWarn},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			overrideProcCmdline(t, tt.cmdline)

			// When
			got := iommuProbe{}.Run(context.Background())

			// Then
			if got.Status != tt.wantStatus {
				t.Fatalf("expected %q, got %q (%s)", tt.wantStatus, got.Status, got.Detail)
			}
		})
	}

	t.Run("skip when file unreadable", func(t *testing.T) {
		// Given
		overrideProcCmdlinePath(t, filepath.Join(t.TempDir(), "does-not-exist"))

		// When
		got := iommuProbe{}.Run(context.Background())

		// Then
		if got.Status != StatusSkip {
			t.Fatalf("expected skip, got %q", got.Status)
		}
	})
}

func TestAcsProbe(t *testing.T) {
	tests := []struct {
		name       string
		lookPath   func(string) (string, error)
		output     []byte
		err        error
		wantStatus Status
	}{
		{
			name:       "warn when ACS enabled",
			lookPath:   lookPathAll,
			output:     []byte("\t\tACSCtl:\tSrcValid+ TransBlk- ReqRedir- CmpltRedir- UpstreamFwd- EgressCtrl- DirectTrans-\n"),
			wantStatus: StatusWarn,
		},
		{
			name:       "ok when ACS disabled",
			lookPath:   lookPathAll,
			output:     []byte("\t\tACSCtl:\tSrcValid- TransBlk- ReqRedir- CmpltRedir- UpstreamFwd- EgressCtrl- DirectTrans-\n"),
			wantStatus: StatusOK,
		},
		{
			name:       "skip when no ACSCtl lines (non-root)",
			lookPath:   lookPathAll,
			output:     []byte("00:00.0 Host bridge: Intel Corporation Device\n\t\tControl: I/O+ Mem+\n"),
			wantStatus: StatusSkip,
		},
		{
			name:       "skip when empty output",
			lookPath:   lookPathAll,
			output:     []byte(""),
			wantStatus: StatusSkip,
		},
		{
			name:       "skip when lookPath fails",
			lookPath:   lookPathMissing("lspci"),
			wantStatus: StatusSkip,
		},
		{
			name:       "skip when exec fails",
			lookPath:   lookPathAll,
			err:        errors.New("permission denied"),
			wantStatus: StatusSkip,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrideLookPath(t, tt.lookPath)
			runner := &fakeRunner{
				outputs: map[string][]byte{"lspci": tt.output},
				errs:    map[string]error{"lspci": tt.err},
			}

			// When
			got := acsProbe{runner: runner}.Run(context.Background())

			// Then
			if got.Status != tt.wantStatus {
				t.Fatalf("expected %q, got %q (%s)", tt.wantStatus, got.Status, got.Detail)
			}
		})
	}
}

func TestRdmaPortProbe(t *testing.T) {
	tests := []struct {
		name       string
		lookPath   func(string) (string, error)
		output     []byte
		err        error
		wantStatus Status
	}{
		{
			name:       "ok when port active",
			lookPath:   lookPathAll,
			output:     []byte("hca_id:\tmlx5_0\n\t\tport:\t1\n\t\t\tstate:\t\t\tPORT_ACTIVE (4)\n"),
			wantStatus: StatusOK,
		},
		{
			name:       "warn when only port down",
			lookPath:   lookPathAll,
			output:     []byte("hca_id:\tmlx5_0\n\t\tport:\t1\n\t\t\tstate:\t\t\tPORT_DOWN (1)\n"),
			wantStatus: StatusWarn,
		},
		{
			name:       "skip when lookPath fails",
			lookPath:   lookPathMissing("ibv_devinfo"),
			wantStatus: StatusSkip,
		},
		{
			name:       "skip when exec fails",
			lookPath:   lookPathAll,
			err:        errors.New("boom"),
			wantStatus: StatusSkip,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrideLookPath(t, tt.lookPath)
			runner := &fakeRunner{
				outputs: map[string][]byte{"ibv_devinfo": tt.output},
				errs:    map[string]error{"ibv_devinfo": tt.err},
			}

			// When
			got := rdmaPortProbe{runner: runner}.Run(context.Background())

			// Then
			if got.Status != tt.wantStatus {
				t.Fatalf("expected %q, got %q (%s)", tt.wantStatus, got.Status, got.Detail)
			}
		})
	}
}

func TestLinkLayerProbe(t *testing.T) {
	tests := []struct {
		name       string
		lookPath   func(string) (string, error)
		output     []byte
		err        error
		wantStatus Status
		wantDetail string
	}{
		{
			name:       "ok InfiniBand",
			lookPath:   lookPathAll,
			output:     []byte("CA 'mlx5_0'\n\tPort 1:\n\t\tLink layer: InfiniBand\n"),
			wantStatus: StatusOK,
			wantDetail: "InfiniBand",
		},
		{
			name:       "ok Ethernet maps to RoCE",
			lookPath:   lookPathAll,
			output:     []byte("CA 'mlx5_0'\n\tPort 1:\n\t\tLink layer: Ethernet\n"),
			wantStatus: StatusOK,
			wantDetail: "Ethernet (RoCE)",
		},
		{
			name:       "ok unknown link layer preserved",
			lookPath:   lookPathAll,
			output:     []byte("CA 'mlx5_0'\n\tPort 1:\n\t\tLink layer: Unknown\n"),
			wantStatus: StatusOK,
			wantDetail: "Unknown",
		},
		{
			name:       "skip when no link layer reported",
			lookPath:   lookPathAll,
			output:     []byte("CA 'mlx5_0'\n\tPort 1:\n\t\tState: Active\n"),
			wantStatus: StatusSkip,
		},
		{
			name:       "skip when lookPath fails",
			lookPath:   lookPathMissing("ibstat"),
			wantStatus: StatusSkip,
		},
		{
			name:       "skip when exec fails",
			lookPath:   lookPathAll,
			err:        errors.New("boom"),
			wantStatus: StatusSkip,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrideLookPath(t, tt.lookPath)
			runner := &fakeRunner{
				outputs: map[string][]byte{"ibstat": tt.output},
				errs:    map[string]error{"ibstat": tt.err},
			}

			// When
			got := linkLayerProbe{runner: runner}.Run(context.Background())

			// Then
			if got.Status != tt.wantStatus {
				t.Fatalf("expected %q, got %q (%s)", tt.wantStatus, got.Status, got.Detail)
			}
			if tt.wantDetail != "" && got.Detail != tt.wantDetail {
				t.Fatalf("expected detail %q, got %q", tt.wantDetail, got.Detail)
			}
		})
	}
}

func TestAggregate(t *testing.T) {
	tests := []struct {
		name    string
		results []Result
		want    Status
	}{
		{name: "empty is ok", results: nil, want: StatusOK},
		{name: "all skip is ok", results: []Result{{Status: StatusSkip}, {Status: StatusSkip}}, want: StatusOK},
		{name: "ok with skip is ok", results: []Result{{Status: StatusOK}, {Status: StatusSkip}}, want: StatusOK},
		{name: "warn beats ok", results: []Result{{Status: StatusOK}, {Status: StatusWarn}, {Status: StatusSkip}}, want: StatusWarn},
		{name: "fail beats warn", results: []Result{{Status: StatusWarn}, {Status: StatusFail}, {Status: StatusOK}}, want: StatusFail},
		{name: "fail beats everything", results: []Result{{Status: StatusOK}, {Status: StatusSkip}, {Status: StatusWarn}, {Status: StatusFail}}, want: StatusFail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got := Aggregate(tt.results)

			// Then
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestRun_collectsAllResults_withMixedStatuses(t *testing.T) {
	// Given
	overrideLookPath(t, lookPathAll)
	overrideProcModules(t, "nvidia_peermem 1 0 - Live 0x0\n")
	overrideProcCmdline(t, "BOOT_IMAGE=/vmlinuz iommu=pt\n")
	runner := &fakeRunner{
		outputs: map[string][]byte{
			"nvidia-smi":  []byte("GPU 0: NVIDIA A100 (UUID: GPU-111)\n"),
			"lspci":       []byte("\t\tACSCtl:\tSrcValid+ TransBlk-\n"),
			"ibv_devinfo": []byte("state:\tPORT_ACTIVE (4)\n"),
			"ibstat":      []byte("Link layer: InfiniBand\n"),
		},
	}
	probes := DefaultProbes(runner)

	// When
	report := Run(context.Background(), probes)

	// Then
	if len(report.Results) != 6 {
		t.Fatalf("expected 6 results, got %d", len(report.Results))
	}
	// nvidia ok, peermem ok, iommu ok, acs warn, rdma ok, link ok -> overall warn.
	if report.Overall != StatusWarn {
		t.Fatalf("expected overall warn, got %q", report.Overall)
	}
	byName := map[string]Status{}
	for _, r := range report.Results {
		byName[r.Name] = r.Status
	}
	want := map[string]Status{
		"nvidia-smi":     StatusOK,
		"nvidia-peermem": StatusOK,
		"iommu":          StatusOK,
		"acs":            StatusWarn,
		"rdma-port":      StatusOK,
		"link-layer":     StatusOK,
	}
	if !reflect.DeepEqual(byName, want) {
		t.Fatalf("expected %#v, got %#v", want, byName)
	}
}

func TestRun_isIndependent_whenSomeProbesSkip(t *testing.T) {
	// Given: nvidia-smi missing (skip) must not block the others.
	overrideLookPath(t, lookPathMissing("nvidia-smi"))
	overrideProcModules(t, "nvidia 1 0 - Live 0x0\n") // peermem absent -> warn
	overrideProcCmdline(t, "BOOT_IMAGE=/vmlinuz iommu=pt\n")
	runner := &fakeRunner{
		outputs: map[string][]byte{
			"lspci":       []byte("\t\tACSCtl:\tSrcValid-\n"),
			"ibv_devinfo": []byte("state:\tPORT_ACTIVE (4)\n"),
			"ibstat":      []byte("Link layer: Ethernet\n"),
		},
	}
	probes := DefaultProbes(runner)

	// When
	report := Run(context.Background(), probes)

	// Then
	if len(report.Results) != 6 {
		t.Fatalf("expected 6 results, got %d", len(report.Results))
	}
	if report.Results[0].Status != StatusSkip {
		t.Fatalf("expected nvidia-smi skip, got %q", report.Results[0].Status)
	}
	// peermem warn dominates -> overall warn.
	if report.Overall != StatusWarn {
		t.Fatalf("expected overall warn, got %q", report.Overall)
	}
}

func TestDefaultProbes_usesOsExecRunner_whenRunnerNil(t *testing.T) {
	// When
	probes := DefaultProbes(nil)

	// Then
	if len(probes) != 6 {
		t.Fatalf("expected 6 probes, got %d", len(probes))
	}
	names := make([]string, len(probes))
	for i, p := range probes {
		names[i] = p.Name()
	}
	want := []string{"nvidia-smi", "nvidia-peermem", "iommu", "acs", "rdma-port", "link-layer"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("expected %#v, got %#v", want, names)
	}
}

func TestOsExecRunnerRun_returnsStdout_whenCommandSucceeds(t *testing.T) {
	// Given
	runner := osExecRunner{}

	// When
	out, err := runner.Run(context.Background(), "echo", "health-ok")
	// Then
	if err != nil {
		t.Fatalf("expected echo to succeed: %v", err)
	}
	if string(out) != "health-ok\n" {
		t.Fatalf("expected echo output, got %q", string(out))
	}
}

func TestOsExecRunnerRun_returnsError_whenCommandFails(t *testing.T) {
	// Given
	runner := osExecRunner{}
	missing := "/nonexistent/gpu-tools-health-missing-binary"

	// When
	out, err := runner.Run(context.Background(), missing)

	// Then
	if out != nil {
		t.Fatalf("expected no stdout on failure, got %q", string(out))
	}
	if err == nil {
		t.Fatalf("expected error for missing binary")
	}
}

func TestProbeHintEnrichment_usesPrereqHint_whenNonEmpty(t *testing.T) {
	// Given: hintFor returns a distro-aware install string per binary.
	overrideHintFor(t, func(binary string) string {
		switch binary {
		case "nvidia-smi":
			return "apt install nvidia-driver-XYZ"
		case "ibv_devinfo":
			return "apt install ibverbs-utils rdma-core"
		case "ibstat":
			return "apt install infiniband-diags"
		default:
			return ""
		}
	})

	tests := []struct {
		name     string
		probe    Probe
		wantHint string
	}{
		{
			name:     "nvidia-smi skip enriched",
			probe:    nvidiaSmiProbe{runner: &fakeRunner{}},
			wantHint: "apt install nvidia-driver-XYZ",
		},
		{
			name:     "rdma-port skip enriched",
			probe:    rdmaPortProbe{runner: &fakeRunner{}},
			wantHint: "apt install ibverbs-utils rdma-core",
		},
		{
			name:     "link-layer skip enriched",
			probe:    linkLayerProbe{runner: &fakeRunner{}},
			wantHint: "apt install infiniband-diags",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: the tool resolves but exec fails -> skip with a hint.
			overrideLookPath(t, lookPathAll)

			// When
			got := tt.probe.Run(context.Background())

			// Then
			if got.Status != StatusSkip {
				t.Fatalf("expected skip, got %q", got.Status)
			}
			if got.Hint != tt.wantHint {
				t.Fatalf("expected enriched hint %q, got %q", tt.wantHint, got.Hint)
			}
		})
	}
}

func TestProbeHintEnrichment_fallsBackToInlineHint_whenPrereqEmpty(t *testing.T) {
	// Given: hintFor always returns "" (unknown binary), so the inline hint wins.
	overrideHintFor(t, func(string) string { return "" })
	overrideLookPath(t, lookPathAll)

	tests := []struct {
		name     string
		probe    Probe
		wantHint string
	}{
		{
			name:     "nvidia-smi keeps inline hint",
			probe:    nvidiaSmiProbe{runner: &fakeRunner{}},
			wantHint: "install NVIDIA driver",
		},
		{
			name:     "rdma-port keeps inline hint",
			probe:    rdmaPortProbe{runner: &fakeRunner{}},
			wantHint: "install OFED/rdma-core",
		},
		{
			name:     "link-layer keeps inline hint",
			probe:    linkLayerProbe{runner: &fakeRunner{}},
			wantHint: "install OFED/rdma-core",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got := tt.probe.Run(context.Background())

			// Then
			if got.Status != StatusSkip {
				t.Fatalf("expected skip, got %q", got.Status)
			}
			if got.Hint != tt.wantHint {
				t.Fatalf("expected inline hint %q, got %q", tt.wantHint, got.Hint)
			}
		})
	}
}

func TestEnrichHint_replacesAndFallsBack(t *testing.T) {
	overrideHintFor(t, func(binary string) string {
		if binary == "known" {
			return "distro-aware hint"
		}
		return ""
	})

	if got := enrichHint("known", "inline"); got != "distro-aware hint" {
		t.Fatalf("expected replacement with prereq hint, got %q", got)
	}
	if got := enrichHint("unknown", "inline"); got != "inline" {
		t.Fatalf("expected fallback to inline, got %q", got)
	}
}

func TestLookPath_resolvesAndWrapsErrors(t *testing.T) {
	// When
	path, err := lookPath(os.Args[0])
	_, missingErr := lookPath("/nonexistent/gpu-tools-health-missing-binary")

	// Then
	if err != nil {
		t.Fatalf("expected current test binary lookup to succeed: %v", err)
	}
	if path == "" {
		t.Fatalf("expected non-empty path")
	}
	if missingErr == nil {
		t.Fatalf("expected error for missing binary")
	}
}
