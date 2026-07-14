// Package health implements the read-only environment "doctor" probe layer.
//
// It runs a set of independent, non-destructive probes (peermem loaded?
// IOMMU=pt? ACS off? RDMA port active? etc.) and aggregates their results.
// This is the data layer only: probes report status, they never decide exit
// codes and never mutate any system state. The gpu-tools doctor command lives
// in a separate package and consumes DefaultProbes, Run, and Report.
package health

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/sunerpy/gpu-tools/internal/prereq"
)

// Status is the outcome of a single probe (or the aggregated report).
type Status string

const (
	// StatusOK means the probe found the expected healthy configuration.
	StatusOK Status = "ok"
	// StatusWarn means the probe found a non-fatal problem worth attention.
	StatusWarn Status = "warn"
	// StatusFail means the probe found a fatal problem.
	StatusFail Status = "fail"
	// StatusSkip means the probe could not run (missing dependency / permission).
	StatusSkip Status = "skip"
)

// Result is the outcome of one probe.
type Result struct {
	Name   string
	Status Status
	Detail string
	Hint   string
}

// Probe is a single independent, non-destructive environment check.
type Probe interface {
	Name() string
	Run(ctx context.Context) Result
}

// Report aggregates the results of a probe run.
type Report struct {
	Results []Result
	Overall Status
}

// Seams for the /proc probes; tests point these at temp files.
var (
	procModulesPath = "/proc/modules"
	procCmdlinePath = "/proc/cmdline"
)

// hintFor is the prereq-catalog lookup seam. Tests override it for determinism
// regardless of the host distro.
var hintFor = prereq.HintFor

// enrichHint returns the distro-aware prereq hint for binary when one exists,
// otherwise the inline fallback. It never blanks a hint.
func enrichHint(binary, inline string) string {
	if hint := hintFor(binary); hint != "" {
		return hint
	}
	return inline
}

// Aggregate reduces a set of results to a single overall status.
//
// Precedence: any fail -> fail; else any warn -> warn; else ok. skip is
// ignored, so an all-skip set (or an empty set) aggregates to ok.
func Aggregate(results []Result) Status {
	overall := StatusOK
	for _, r := range results {
		switch r.Status {
		case StatusFail:
			return StatusFail
		case StatusWarn:
			overall = StatusWarn
		case StatusOK, StatusSkip:
			// StatusOK keeps overall at least ok; StatusSkip is ignored.
		}
	}
	return overall
}

// Run executes every probe and collects all results. Each probe is
// independent: a failure or skip in one never prevents the others from
// running. Probes must never panic.
func Run(ctx context.Context, probes []Probe) Report {
	results := make([]Result, 0, len(probes))
	for _, p := range probes {
		results = append(results, p.Run(ctx))
	}
	return Report{Results: results, Overall: Aggregate(results)}
}

// DefaultProbes returns the standard six probes wired to runner. A nil runner
// falls back to the production os/exec runner.
func DefaultProbes(runner execRunner) []Probe {
	if runner == nil {
		runner = osExecRunner{}
	}
	return []Probe{
		nvidiaSmiProbe{runner: runner},
		peermemProbe{},
		iommuProbe{},
		acsProbe{runner: runner},
		rdmaPortProbe{runner: runner},
		linkLayerProbe{runner: runner},
	}
}

// nvidiaSmiProbe checks that the NVIDIA driver is present by listing GPUs.
type nvidiaSmiProbe struct {
	runner execRunner
}

func (p nvidiaSmiProbe) Name() string { return "nvidia-smi" }

func (p nvidiaSmiProbe) Run(ctx context.Context) Result {
	path, err := lookPath("nvidia-smi")
	if err != nil {
		return Result{Name: p.Name(), Status: StatusSkip, Detail: "nvidia-smi not found", Hint: enrichHint("nvidia-smi", "install NVIDIA driver")}
	}
	out, err := p.runner.Run(ctx, path, "-L")
	if err != nil {
		return Result{Name: p.Name(), Status: StatusSkip, Detail: "nvidia-smi failed", Hint: enrichHint("nvidia-smi", "install NVIDIA driver")}
	}
	count := 0
	for line := range strings.SplitSeq(string(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "GPU ") {
			count++
		}
	}
	if count == 0 {
		return Result{Name: p.Name(), Status: StatusWarn, Detail: "no GPUs listed", Hint: enrichHint("nvidia-smi", "install NVIDIA driver")}
	}
	return Result{Name: p.Name(), Status: StatusOK, Detail: pluralGPUs(count)}
}

func pluralGPUs(count int) string {
	if count == 1 {
		return "1 GPU detected"
	}
	return strconv.Itoa(count) + " GPUs detected"
}

// peermemProbe checks that the nvidia_peermem module is loaded.
type peermemProbe struct{}

func (p peermemProbe) Name() string { return "nvidia-peermem" }

func (p peermemProbe) Run(_ context.Context) Result {
	data, err := os.ReadFile(procModulesPath)
	if err != nil {
		return Result{Name: p.Name(), Status: StatusSkip, Detail: "cannot read " + procModulesPath}
	}
	if strings.Contains(string(data), "nvidia_peermem") {
		return Result{Name: p.Name(), Status: StatusOK, Detail: "nvidia_peermem loaded"}
	}
	return Result{Name: p.Name(), Status: StatusWarn, Detail: "nvidia_peermem not loaded", Hint: "modprobe nvidia-peermem"}
}

// iommuProbe checks the kernel command line for iommu=pt.
type iommuProbe struct{}

func (p iommuProbe) Name() string { return "iommu" }

func (p iommuProbe) Run(_ context.Context) Result {
	data, err := os.ReadFile(procCmdlinePath)
	if err != nil {
		return Result{Name: p.Name(), Status: StatusSkip, Detail: "cannot read " + procCmdlinePath}
	}
	cmdline := string(data)
	if strings.Contains(cmdline, "iommu=pt") {
		return Result{Name: p.Name(), Status: StatusOK, Detail: "iommu=pt set"}
	}
	if strings.Contains(cmdline, "iommu=") {
		return Result{Name: p.Name(), Status: StatusWarn, Detail: "iommu set but not pt", Hint: "add iommu=pt"}
	}
	return Result{Name: p.Name(), Status: StatusWarn, Detail: "iommu not set", Hint: "add iommu=pt"}
}

// acsProbe checks whether PCIe ACS is enabled (which blocks GPUDirect P2P).
type acsProbe struct {
	runner execRunner
}

func (p acsProbe) Name() string { return "acs" }

func (p acsProbe) Run(ctx context.Context) Result {
	path, err := lookPath("lspci")
	if err != nil {
		return Result{Name: p.Name(), Status: StatusSkip, Detail: "lspci not found", Hint: "run as root or check BIOS"}
	}
	out, err := p.runner.Run(ctx, path, "-vvv")
	if err != nil {
		return Result{Name: p.Name(), Status: StatusSkip, Detail: "lspci failed", Hint: "run as root or check BIOS"}
	}
	seenACSCtl := false
	for line := range strings.SplitSeq(string(out), "\n") {
		if !strings.Contains(line, "ACSCtl:") {
			continue
		}
		seenACSCtl = true
		if strings.Contains(line, "SrcValid+") {
			return Result{Name: p.Name(), Status: StatusWarn, Detail: "ACS enabled (SrcValid+)", Hint: "disable ACS for GPUDirect P2P"}
		}
	}
	if !seenACSCtl {
		return Result{Name: p.Name(), Status: StatusSkip, Detail: "no ACSCtl lines", Hint: "run as root or check BIOS"}
	}
	return Result{Name: p.Name(), Status: StatusOK, Detail: "ACS disabled (SrcValid-)"}
}

// rdmaPortProbe checks that at least one RDMA port is active.
type rdmaPortProbe struct {
	runner execRunner
}

func (p rdmaPortProbe) Name() string { return "rdma-port" }

func (p rdmaPortProbe) Run(ctx context.Context) Result {
	path, err := lookPath("ibv_devinfo")
	if err != nil {
		return Result{Name: p.Name(), Status: StatusSkip, Detail: "ibv_devinfo not found", Hint: enrichHint("ibv_devinfo", "install OFED/rdma-core")}
	}
	out, err := p.runner.Run(ctx, path)
	if err != nil {
		return Result{Name: p.Name(), Status: StatusSkip, Detail: "ibv_devinfo failed", Hint: enrichHint("ibv_devinfo", "install OFED/rdma-core")}
	}
	if strings.Contains(string(out), "PORT_ACTIVE") {
		return Result{Name: p.Name(), Status: StatusOK, Detail: "RDMA port active"}
	}
	return Result{Name: p.Name(), Status: StatusWarn, Detail: "no active RDMA port", Hint: "check cabling / bring the port up"}
}

// linkLayerProbe reports the RDMA link layer (InfiniBand vs RoCE).
type linkLayerProbe struct {
	runner execRunner
}

func (p linkLayerProbe) Name() string { return "link-layer" }

func (p linkLayerProbe) Run(ctx context.Context) Result {
	path, err := lookPath("ibstat")
	if err != nil {
		return Result{Name: p.Name(), Status: StatusSkip, Detail: "ibstat not found", Hint: enrichHint("ibstat", "install OFED/rdma-core")}
	}
	out, err := p.runner.Run(ctx, path)
	if err != nil {
		return Result{Name: p.Name(), Status: StatusSkip, Detail: "ibstat failed", Hint: enrichHint("ibstat", "install OFED/rdma-core")}
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		_, after, found := strings.Cut(line, "Link layer:")
		if !found {
			continue
		}
		value := strings.TrimSpace(after)
		switch value {
		case "InfiniBand":
			return Result{Name: p.Name(), Status: StatusOK, Detail: "InfiniBand"}
		case "Ethernet":
			return Result{Name: p.Name(), Status: StatusOK, Detail: "Ethernet (RoCE)"}
		default:
			return Result{Name: p.Name(), Status: StatusOK, Detail: value}
		}
	}
	return Result{Name: p.Name(), Status: StatusSkip, Detail: "no Link layer reported", Hint: enrichHint("ibstat", "install OFED/rdma-core")}
}
