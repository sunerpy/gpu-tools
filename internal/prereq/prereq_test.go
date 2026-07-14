package prereq

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func overrideLookPath(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	prev := lookPath
	lookPath = fn
	t.Cleanup(func() { lookPath = prev })
}

func overrideOSReleasePath(t *testing.T, path string) {
	t.Helper()
	prev := osReleasePath
	osReleasePath = path
	t.Cleanup(func() { osReleasePath = prev })
}

func writeOSRelease(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "os-release")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write os-release fixture: %v", err)
	}
	return path
}

func TestDetectDistro(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "debian by id", content: "ID=debian\n", want: "debian"},
		{name: "ubuntu resolves to debian", content: "ID=ubuntu\nVERSION_ID=\"22.04\"\n", want: "debian"},
		{name: "rhel by id", content: "ID=\"rhel\"\n", want: "rhel"},
		{name: "rocky resolves to rhel", content: "ID=rocky\n", want: "rhel"},
		{name: "id_like fallback to rhel", content: "ID=customos\nID_LIKE=\"fedora rhel\"\n", want: "rhel"},
		{name: "id_like fallback to debian", content: "ID=customos\nID_LIKE=ubuntu\n", want: "debian"},
		{name: "unknown id returns generic", content: "ID=plan9\n", want: "generic"},
		{name: "empty file returns generic", content: "", want: "generic"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrideOSReleasePath(t, writeOSRelease(t, tt.content))
			if got := DetectDistro(); got != tt.want {
				t.Fatalf("DetectDistro() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectDistro_returnsGeneric_whenFileMissing(t *testing.T) {
	overrideOSReleasePath(t, filepath.Join(t.TempDir(), "does-not-exist"))
	if got := DetectDistro(); got != "generic" {
		t.Fatalf("DetectDistro() = %q, want generic when file missing", got)
	}
}

func TestCheck_marksFoundAndPicksHintPerDistro(t *testing.T) {
	tests := []struct {
		name          string
		osRelease     string
		presentBinary string
		presentPath   string
		wantHints     map[string]string
	}{
		{
			name:          "debian picks debian hints",
			osRelease:     "ID=debian\n",
			presentBinary: "nvidia-smi",
			presentPath:   "/usr/bin/nvidia-smi",
			wantHints: map[string]string{
				"nvidia-smi":  "apt install nvidia-driver-<ver>",
				"ibv_devinfo": "apt install ibverbs-utils rdma-core",
				"ib_write_bw": "apt install perftest",
				// nccl-tests has no debian key -> falls back to source
				"all_reduce_perf": "build github.com/NVIDIA/nccl-tests (make MPI=1 ...)",
			},
		},
		{
			name:          "rhel picks rhel hints",
			osRelease:     "ID=rhel\n",
			presentBinary: "ibstat",
			presentPath:   "/usr/sbin/ibstat",
			wantHints: map[string]string{
				"nvidia-smi":  "dnf install nvidia-driver",
				"ibv_devinfo": "dnf install libibverbs-utils rdma-core",
				"dcgmi":       "install datacenter-gpu-manager",
			},
		},
		{
			name:          "generic falls back to source or generic",
			osRelease:     "ID=plan9\n",
			presentBinary: "",
			presentPath:   "",
			wantHints: map[string]string{
				// nvidia-smi has generic
				"nvidia-smi": "install the NVIDIA driver (nvidia.com/download) or use --gpus in containers",
				// ibv_devinfo has no generic -> source
				"ibv_devinfo": "install MLNX_OFED / DOCA-OFED from NVIDIA Networking",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrideOSReleasePath(t, writeOSRelease(t, tt.osRelease))
			overrideLookPath(t, func(name string) (string, error) {
				if tt.presentBinary != "" && name == tt.presentBinary {
					return tt.presentPath, nil
				}
				return "", errors.New("not found")
			})

			checks := Check()
			if len(checks) != len(Tools) {
				t.Fatalf("Check() returned %d results, want %d", len(checks), len(Tools))
			}

			byBinary := make(map[string]CheckResult, len(checks))
			for _, c := range checks {
				byBinary[c.Binary] = c
			}

			for binary, wantHint := range tt.wantHints {
				got, ok := byBinary[binary]
				if !ok {
					t.Fatalf("no check result for binary %q", binary)
				}
				if got.Install != wantHint {
					t.Fatalf("binary %q Install = %q, want %q", binary, got.Install, wantHint)
				}
			}

			present, ok := byBinary[tt.presentBinary]
			if tt.presentBinary != "" {
				if !ok {
					t.Fatalf("present binary %q missing from results", tt.presentBinary)
				}
				if !present.Found {
					t.Fatalf("binary %q expected Found=true", tt.presentBinary)
				}
				if present.Path != tt.presentPath {
					t.Fatalf("binary %q Path = %q, want %q", tt.presentBinary, present.Path, tt.presentPath)
				}
			}

			for _, c := range checks {
				if c.Binary == tt.presentBinary {
					continue
				}
				if c.Found {
					t.Fatalf("binary %q expected Found=false, got true", c.Binary)
				}
				if c.Path != "" {
					t.Fatalf("binary %q expected empty Path, got %q", c.Binary, c.Path)
				}
			}
		})
	}
}

func TestCheck_ignoresEmptyPathWithoutError(t *testing.T) {
	overrideOSReleasePath(t, writeOSRelease(t, "ID=debian\n"))
	overrideLookPath(t, func(string) (string, error) {
		return "", nil
	})
	for _, c := range Check() {
		if c.Found {
			t.Fatalf("binary %q expected Found=false when path empty", c.Binary)
		}
	}
}

func TestHintFor(t *testing.T) {
	overrideOSReleasePath(t, writeOSRelease(t, "ID=debian\n"))
	tests := []struct {
		name   string
		binary string
		want   string
	}{
		{name: "known binary returns debian hint", binary: "nvidia-smi", want: "apt install nvidia-driver-<ver>"},
		{name: "known binary falling back to source", binary: "all_reduce_perf", want: "build github.com/NVIDIA/nccl-tests (make MPI=1 ...)"},
		{name: "unknown binary returns empty", binary: "totally-unknown", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HintFor(tt.binary); got != tt.want {
				t.Fatalf("HintFor(%q) = %q, want %q", tt.binary, got, tt.want)
			}
		})
	}
}

func TestSelectHint_returnsEmpty_whenNoKeysApply(t *testing.T) {
	if got := selectHint(map[string]string{"rhel": "x"}, "debian"); got != "" {
		t.Fatalf("selectHint with no matching keys = %q, want empty", got)
	}
}

func TestLookPath_defaultResolvesFromPATH(t *testing.T) {
	// The default seam wraps exec.LookPath; "go" exists in the test PATH.
	if _, err := lookPath("go"); err != nil {
		t.Fatalf("default lookPath(go) unexpectedly failed: %v", err)
	}
	if _, err := lookPath("gpu-tools-nonexistent-binary-xyz"); err == nil {
		t.Fatalf("default lookPath expected error for missing binary")
	}
}
