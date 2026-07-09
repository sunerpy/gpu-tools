package nvidiasmi

import (
	"context"
	"encoding/csv"
	"errors"
	"strconv"
	"strings"

	"github.com/sunerpy/gpu-tools/internal/gpu"
	"github.com/sunerpy/gpu-tools/internal/gpu/procinfo"
)

// fieldGPUUUID is the --query-compute-apps column that ties a process to its
// device. Per plan R2 attribution is by EXACT gpu_uuid match only.
const fieldGPUUUID = "gpu_uuid"

// processType is the Type tag attached to every process discovered through
// --query-compute-apps.
const processType = "compute"

// computeAppsFields is the fixed column order requested from
// --query-compute-apps. gpu_uuid is first so attribution is unambiguous.
var computeAppsFields = []string{
	fieldGPUUUID,
	"pid",
	"process_name",
	"used_memory",
}

// errUnsupportedComputeApps marks the degrade path: the driver does not expose
// gpu_uuid for compute apps, so attribution is skipped entirely (no guessing).
var errUnsupportedComputeApps = errors.New("nvidia-smi compute-apps gpu_uuid unsupported")

// resolveProcess is the seam onto procinfo.Resolve so tests can inject
// deterministic name/user resolution without a real /proc.
var resolveProcess = procinfo.Resolve

// computeApp is one parsed --query-compute-apps row before attribution.
type computeApp struct {
	uuid       string
	pid        int
	name       string
	usedMemory uint64
}

// attachProcesses queries per-process compute apps best-effort and attaches
// each to its device by EXACT gpu_uuid match (plan R2). It never returns an
// error: any failure — unsupported driver, missing gpu_uuid, query error,
// malformed rows — leaves devices with empty Processes rather than failing the
// whole collection.
func (c *Collector) attachProcesses(devices []gpu.Device) {
	apps, err := c.computeApps()
	if err != nil {
		return
	}
	byUUID := make(map[string]int, len(devices))
	for i := range devices {
		byUUID[devices[i].UUID] = i
	}
	for _, app := range apps {
		idx, ok := byUUID[app.uuid]
		if !ok {
			continue
		}
		name, user := resolveProcess(app.pid)
		devices[idx].Processes = append(devices[idx].Processes, gpu.GPUProcess{
			PID:        app.pid,
			Name:       name,
			User:       user,
			UsedMemory: app.usedMemory,
			Type:       processType,
		})
	}
}

// computeApps probes for gpu_uuid support, runs the compute-apps query, and
// parses the rows. It returns errUnsupportedComputeApps (or the underlying
// error) whenever attribution must be skipped.
func (c *Collector) computeApps() ([]computeApp, error) {
	supported, err := supportedFields(c.runner, c.smiPath, "--help-query-compute-apps")
	if err != nil {
		return nil, err
	}
	if !supported[fieldGPUUUID] {
		return nil, errUnsupportedComputeApps
	}
	out, err := c.runner.Run(context.Background(), c.smiPath, computeAppsArgs()...)
	if err != nil {
		return nil, err
	}
	return parseComputeApps(out), nil
}

func computeAppsArgs() []string {
	return []string{
		"--query-compute-apps=" + strings.Join(computeAppsFields, ","),
		"--format=csv,noheader,nounits",
	}
}

// parseComputeApps parses the compute-apps CSV. Malformed rows (wrong column
// count, non-numeric pid/memory) are dropped, never fatal — the query is
// best-effort.
func parseComputeApps(out []byte) []computeApp {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil
	}
	reader := csv.NewReader(strings.NewReader(trimmed))
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil
	}
	apps := make([]computeApp, 0, len(rows))
	for _, row := range rows {
		app, ok := parseComputeAppRow(row)
		if ok {
			apps = append(apps, app)
		}
	}
	return apps
}

func parseComputeAppRow(row []string) (computeApp, bool) {
	if len(row) != len(computeAppsFields) {
		return computeApp{}, false
	}
	uuid := clean(row[0])
	if uuid == "" {
		return computeApp{}, false
	}
	pid, err := strconv.Atoi(clean(row[1]))
	if err != nil {
		return computeApp{}, false
	}
	usedMemory, err := parseMemory(row[3], "used_memory")
	if err != nil {
		return computeApp{}, false
	}
	return computeApp{
		uuid:       uuid,
		pid:        pid,
		name:       cleanAvailable(row[2]),
		usedMemory: usedMemory,
	}, true
}
