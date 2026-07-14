// Package prereq detects whether prerequisite external tools are installed and
// provides platform-appropriate install guidance. It bundles nothing: the
// Install hints are advisory strings that point at official sources. The
// catalog and helpers are designed to be importable so the doctor command can
// reuse the same guidance via HintFor.
package prereq

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
)

// Tool describes a single prerequisite external tool and its per-distro install
// guidance. Install maps a distro family key ("debian"/"rhel"/"source"/
// "generic") to a one-line install hint.
type Tool struct {
	Name    string            `json:"name"`
	Binary  string            `json:"binary"`
	Purpose string            `json:"purpose"`
	Install map[string]string `json:"install"`
}

// CheckResult is the detection result for one Tool. Install holds the single
// hint chosen for the current distro, so consumers need not re-run detection.
type CheckResult struct {
	Tool    string `json:"tool"`
	Binary  string `json:"binary"`
	Found   bool   `json:"found"`
	Path    string `json:"path"`
	Purpose string `json:"purpose"`
	Install string `json:"install"`
}

// Tools is the prerequisite catalog. These are guidance strings only; nothing
// is bundled or installed.
var Tools = []Tool{
	{
		Name:    "nvidia-smi",
		Binary:  "nvidia-smi",
		Purpose: "NVIDIA GPU query/management",
		Install: map[string]string{
			"debian":  "apt install nvidia-driver-<ver>",
			"rhel":    "dnf install nvidia-driver",
			"generic": "install the NVIDIA driver (nvidia.com/download) or use --gpus in containers",
		},
	},
	{
		Name:    "ibv_devinfo",
		Binary:  "ibv_devinfo",
		Purpose: "RDMA device/port info",
		Install: map[string]string{
			"debian": "apt install ibverbs-utils rdma-core",
			"rhel":   "dnf install libibverbs-utils rdma-core",
			"source": "install MLNX_OFED / DOCA-OFED from NVIDIA Networking",
		},
	},
	{
		Name:    "ibstat",
		Binary:  "ibstat",
		Purpose: "InfiniBand/RoCE port state",
		Install: map[string]string{
			"debian": "apt install infiniband-diags",
			"rhel":   "dnf install infiniband-diags",
			"source": "MLNX_OFED / DOCA-OFED",
		},
	},
	{
		Name:    "perftest",
		Binary:  "ib_write_bw",
		Purpose: "RDMA bandwidth/latency benchmark",
		Install: map[string]string{
			"debian": "apt install perftest",
			"rhel":   "dnf install perftest",
			"source": "build github.com/linux-rdma/perftest (--use_cuda for GDR)",
		},
	},
	{
		Name:    "nccl-tests",
		Binary:  "all_reduce_perf",
		Purpose: "NCCL collective bandwidth (busbw)",
		Install: map[string]string{
			"source":  "build github.com/NVIDIA/nccl-tests (make MPI=1 ...)",
			"generic": "build github.com/NVIDIA/nccl-tests (make MPI=1 ...)",
		},
	},
	{
		Name:    "dcgm",
		Binary:  "dcgmi",
		Purpose: "Data Center GPU Manager diagnostics",
		Install: map[string]string{
			"debian": "install datacenter-gpu-manager",
			"rhel":   "install datacenter-gpu-manager",
			"source": "docs.nvidia.com/datacenter/dcgm",
		},
	},
}

// lookPath is the binary-lookup seam. Tests replace it to simulate present or
// missing tools without depending on the host PATH.
var lookPath = func(name string) (string, error) {
	return exec.LookPath(name)
}

// osReleasePath is the distro-detection file-path seam, mirroring procRoot in
// internal/gpu/procinfo. Tests point it at a temp fixture.
var osReleasePath = "/etc/os-release"

// DetectDistro reads the ID field of /etc/os-release and maps it to a distro
// family: "debian", "rhel", or "generic". It also consults ID_LIKE so
// downstream derivatives (e.g. ubuntu, rocky) resolve to their family. If the
// file is missing or unreadable, it returns "generic".
func DetectDistro() string {
	file, err := os.Open(osReleasePath)
	if err != nil {
		return "generic"
	}
	defer func() { _ = file.Close() }()

	var id, idLike string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "ID="):
			id = parseOSReleaseValue(strings.TrimPrefix(line, "ID="))
		case strings.HasPrefix(line, "ID_LIKE="):
			idLike = parseOSReleaseValue(strings.TrimPrefix(line, "ID_LIKE="))
		}
	}

	if family := distroFamily(id); family != "" {
		return family
	}
	for _, like := range strings.Fields(idLike) {
		if family := distroFamily(like); family != "" {
			return family
		}
	}
	return "generic"
}

// parseOSReleaseValue strips surrounding quotes from an os-release value.
func parseOSReleaseValue(v string) string {
	v = strings.TrimSpace(v)
	v = strings.Trim(v, `"'`)
	return strings.ToLower(v)
}

// distroFamily maps a single os-release id token to a family, or "" if unknown.
func distroFamily(id string) string {
	switch id {
	case "debian", "ubuntu", "linuxmint", "pop", "raspbian":
		return "debian"
	case "rhel", "centos", "fedora", "rocky", "almalinux", "ol", "oracle", "amzn":
		return "rhel"
	default:
		return ""
	}
}

// selectHint picks the best install hint for a tool given a distro family,
// falling back to "source" then "generic". Returns "" if none apply.
func selectHint(install map[string]string, distro string) string {
	for _, key := range []string{distro, "source", "generic"} {
		if hint, ok := install[key]; ok && hint != "" {
			return hint
		}
	}
	return ""
}

// Check runs lookPath on each tool's binary and records whether it was found,
// its resolved path, and the install hint chosen for the current distro.
func Check() []CheckResult {
	distro := DetectDistro()
	checks := make([]CheckResult, 0, len(Tools))
	for _, tool := range Tools {
		path, err := lookPath(tool.Binary)
		found := err == nil && path != ""
		if !found {
			path = ""
		}
		checks = append(checks, CheckResult{
			Tool:    tool.Name,
			Binary:  tool.Binary,
			Found:   found,
			Path:    path,
			Purpose: tool.Purpose,
			Install: selectHint(tool.Install, distro),
		})
	}
	return checks
}

// HintFor returns the best install hint for a given binary using the current
// distro. It returns "" if the binary is not in the catalog. Used by the doctor
// command to enrich its fail/warn hints.
func HintFor(binary string) string {
	distro := DetectDistro()
	for _, tool := range Tools {
		if tool.Binary == binary {
			return selectHint(tool.Install, distro)
		}
	}
	return ""
}
