package rdma

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

const devinfoSingleActive = `hca_id: mlx5_0
    transport:          InfiniBand (0)
    fw_ver:             20.31.1014
    node_guid:          0c42:a103:0089:1234
        port:   1
                state:          PORT_ACTIVE (4)
                max_mtu:        4096 (5)
                active_mtu:     4096 (5)
                sm_lid:         1
                link_layer:     InfiniBand
`

const devinfoPortDown = `hca_id: mlx5_1
    transport:          InfiniBand (0)
    fw_ver:             20.31.1014
    node_guid:          0c42:a103:0089:5678
        port:   1
                state:          PORT_DOWN (1)
                max_mtu:        4096 (5)
                active_mtu:     4096 (5)
                link_layer:     InfiniBand
`

const devinfoNoLinkLayer = `hca_id: mlx5_0
    transport:          InfiniBand (0)
    fw_ver:             20.31.1014
    node_guid:          0c42:a103:0089:1234
        port:   1
                state:          PORT_ACTIVE (4)
`

const devinfoMultiDevice = `hca_id: mlx5_0
    transport:          InfiniBand (0)
    fw_ver:             20.31.1014
    node_guid:          0c42:a103:0089:1234
        port:   1
                state:          PORT_ACTIVE (4)
                link_layer:     InfiniBand
hca_id: mlx5_1
    transport:          InfiniBand (0)
    fw_ver:             20.31.2000
    node_guid:          0c42:a103:0089:9999
        port:   1
                state:          PORT_DOWN (1)
                link_layer:     InfiniBand
        port:   2
                state:          PORT_ACTIVE (4)
                link_layer:     Ethernet
`

const ibstatInfiniBand = `CA 'mlx5_0'
    CA type: MT4123
    Number of ports: 1
    Firmware version: 20.31.1014
    Port 1:
        State: Active
        Physical state: LinkUp
        Rate: 200
        Link layer: InfiniBand
`

const ibstatEthernetRoCE = `CA 'mlx5_0'
    CA type: MT4123
    Number of ports: 1
    Firmware version: 20.31.1014
    Port 1:
        State: Active
        Physical state: LinkUp
        Rate: 100
        Link layer: Ethernet
`

func TestParseDevinfo_parsesSingleActiveDevice(t *testing.T) {
	// Given / When
	devices, err := ParseDevinfo([]byte(devinfoSingleActive))
	// Then
	if err != nil {
		t.Fatalf("expected devinfo parse to succeed: %v", err)
	}
	want := []Device{{
		Name:      "mlx5_0",
		NodeGUID:  "0c42:a103:0089:1234",
		FWVersion: "20.31.1014",
		Ports: []Port{{
			Num:       1,
			State:     "PORT_ACTIVE",
			LinkLayer: "InfiniBand",
		}},
	}}
	if !reflect.DeepEqual(devices, want) {
		t.Fatalf("unexpected devices:\n got %#v\nwant %#v", devices, want)
	}
}

func TestParseDevinfo_parsesPortDown(t *testing.T) {
	// Given / When
	devices, err := ParseDevinfo([]byte(devinfoPortDown))
	// Then
	if err != nil {
		t.Fatalf("expected devinfo parse to succeed: %v", err)
	}
	if len(devices) != 1 || len(devices[0].Ports) != 1 {
		t.Fatalf("expected one device with one port, got %#v", devices)
	}
	if devices[0].Ports[0].State != "PORT_DOWN" {
		t.Fatalf("expected PORT_DOWN, got %q", devices[0].Ports[0].State)
	}
	if devices[0].Name != "mlx5_1" {
		t.Fatalf("expected mlx5_1, got %q", devices[0].Name)
	}
}

func TestParseDevinfo_parsesMultipleDevicesAndPorts(t *testing.T) {
	// Given / When
	devices, err := ParseDevinfo([]byte(devinfoMultiDevice))
	// Then
	if err != nil {
		t.Fatalf("expected devinfo parse to succeed: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected two devices, got %d", len(devices))
	}
	if devices[1].Name != "mlx5_1" || len(devices[1].Ports) != 2 {
		t.Fatalf("expected mlx5_1 with two ports, got %#v", devices[1])
	}
	if devices[1].Ports[1].Num != 2 || devices[1].Ports[1].LinkLayer != "Ethernet" {
		t.Fatalf("expected second port Ethernet, got %#v", devices[1].Ports[1])
	}
}

func TestParseDevinfo_emptyInputReturnsNoDevices(t *testing.T) {
	// Given / When
	devices, err := ParseDevinfo([]byte("   \n\t\n"))
	// Then
	if err != nil {
		t.Fatalf("expected empty input to succeed: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("expected no devices, got %#v", devices)
	}
}

func TestParseDevinfo_portBeforeDeviceReturnsError(t *testing.T) {
	// Given
	raw := "        port:   1\n                state:          PORT_ACTIVE (4)\n"
	// When
	_, err := ParseDevinfo([]byte(raw))
	// Then
	if err == nil || !strings.Contains(err.Error(), "port") {
		t.Fatalf("expected orphan-port error, got %v", err)
	}
}

func TestParseDevinfo_malformedPortNumberReturnsError(t *testing.T) {
	// Given
	raw := "hca_id: mlx5_0\n        port:   abc\n"
	// When
	_, err := ParseDevinfo([]byte(raw))
	// Then
	if err == nil || !strings.Contains(err.Error(), "port number") {
		t.Fatalf("expected malformed port number error, got %v", err)
	}
}

func TestParseDevinfo_orphanFieldsBeforeDeviceReturnError(t *testing.T) {
	// Given
	tests := []struct {
		name string
		raw  string
	}{
		{name: "fw_ver", raw: "fw_ver:             20.31.1014\n"},
		{name: "node_guid", raw: "node_guid:          0c42:a103:0089:1234\n"},
		{name: "state before device", raw: "state:          PORT_ACTIVE (4)\n"},
		{name: "link_layer before device", raw: "link_layer:     InfiniBand\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			_, err := ParseDevinfo([]byte(tt.raw))
			// Then
			if err == nil {
				t.Fatalf("expected error for orphan field %q", tt.name)
			}
		})
	}
}

func TestParseDevinfo_stateBeforePortReturnsError(t *testing.T) {
	// Given
	raw := "hca_id: mlx5_0\n        state:          PORT_ACTIVE (4)\n"
	// When
	_, err := ParseDevinfo([]byte(raw))
	// Then
	if err == nil || !strings.Contains(err.Error(), "before any port") {
		t.Fatalf("expected before-any-port error, got %v", err)
	}
}

func TestParseDevinfo_linkLayerBeforePortReturnsError(t *testing.T) {
	// Given
	raw := "hca_id: mlx5_0\n        link_layer:     InfiniBand\n"
	// When
	_, err := ParseDevinfo([]byte(raw))
	// Then
	if err == nil || !strings.Contains(err.Error(), "before any port") {
		t.Fatalf("expected before-any-port error, got %v", err)
	}
}

func TestParseDevinfo_emptyStateValueLeavesStateEmpty(t *testing.T) {
	// Given
	raw := "hca_id: mlx5_0\n        port:   1\n                state:\n"
	// When
	devices, err := ParseDevinfo([]byte(raw))
	// Then
	if err != nil {
		t.Fatalf("expected empty state value to parse: %v", err)
	}
	if devices[0].Ports[0].State != "" {
		t.Fatalf("expected empty state, got %q", devices[0].Ports[0].State)
	}
}

func TestParseIbstat_orphanFieldsBeforeDeviceReturnError(t *testing.T) {
	// Given
	tests := []struct {
		name string
		raw  string
	}{
		{name: "firmware", raw: "Firmware version: 20.31.1014\n"},
		{name: "state", raw: "State: Active\n"},
		{name: "rate", raw: "Rate: 200\n"},
		{name: "link layer", raw: "Link layer: InfiniBand\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			_, err := ParseIbstat([]byte(tt.raw))
			// Then
			if err == nil {
				t.Fatalf("expected error for orphan field %q", tt.name)
			}
		})
	}
}

func TestParseIbstat_fieldBeforePortReturnsError(t *testing.T) {
	// Given
	tests := []struct {
		name string
		raw  string
	}{
		{name: "state", raw: "CA 'mlx5_0'\n        State: Active\n"},
		{name: "rate", raw: "CA 'mlx5_0'\n        Rate: 200\n"},
		{name: "link layer", raw: "CA 'mlx5_0'\n        Link layer: InfiniBand\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			_, err := ParseIbstat([]byte(tt.raw))
			// Then
			if err == nil || !strings.Contains(err.Error(), "before any port") {
				t.Fatalf("expected before-any-port error for %q, got %v", tt.name, err)
			}
		})
	}
}

func TestMerge_fillsStateWhenDevinfoPortStateEmpty(t *testing.T) {
	// Given
	devinfo := []Device{{Name: "mlx5_0", Ports: []Port{{Num: 1, LinkLayer: "InfiniBand"}}}}
	ibstat := []Device{{Name: "mlx5_0", Ports: []Port{{Num: 1, State: "Active", Rate: "200"}}}}
	// When
	merged := Merge(devinfo, ibstat)
	// Then
	p := merged[0].Ports[0]
	if p.State != "Active" {
		t.Fatalf("expected ibstat state to fill empty devinfo state, got %q", p.State)
	}
	if p.LinkLayer != "InfiniBand" {
		t.Fatalf("expected devinfo link layer preserved, got %q", p.LinkLayer)
	}
}

func TestMerge_fillsNodeGUIDWhenDevinfoEmpty(t *testing.T) {
	// Given
	devinfo := []Device{{Name: "mlx5_0", Ports: []Port{{Num: 1, State: "PORT_ACTIVE"}}}}
	ibstat := []Device{{Name: "mlx5_0", NodeGUID: "0c42:a103:0089:1234"}}
	// When
	merged := Merge(devinfo, ibstat)
	// Then
	if merged[0].NodeGUID != "0c42:a103:0089:1234" {
		t.Fatalf("expected ibstat node guid to fill, got %q", merged[0].NodeGUID)
	}
}

func TestParseIbstat_parsesInfiniBand(t *testing.T) {
	// Given / When
	devices, err := ParseIbstat([]byte(ibstatInfiniBand))
	// Then
	if err != nil {
		t.Fatalf("expected ibstat parse to succeed: %v", err)
	}
	want := []Device{{
		Name:      "mlx5_0",
		FWVersion: "20.31.1014",
		Ports: []Port{{
			Num:       1,
			State:     "Active",
			Rate:      "200",
			LinkLayer: "InfiniBand",
		}},
	}}
	if !reflect.DeepEqual(devices, want) {
		t.Fatalf("unexpected devices:\n got %#v\nwant %#v", devices, want)
	}
}

func TestParseIbstat_parsesEthernetRoCE(t *testing.T) {
	// Given / When
	devices, err := ParseIbstat([]byte(ibstatEthernetRoCE))
	// Then
	if err != nil {
		t.Fatalf("expected ibstat parse to succeed: %v", err)
	}
	if len(devices) != 1 || len(devices[0].Ports) != 1 {
		t.Fatalf("expected one device with one port, got %#v", devices)
	}
	if devices[0].Ports[0].LinkLayer != "Ethernet" {
		t.Fatalf("expected Ethernet link layer, got %q", devices[0].Ports[0].LinkLayer)
	}
	if devices[0].Ports[0].Rate != "100" {
		t.Fatalf("expected rate 100, got %q", devices[0].Ports[0].Rate)
	}
}

func TestParseIbstat_emptyInputReturnsNoDevices(t *testing.T) {
	// Given / When
	devices, err := ParseIbstat([]byte("\n  \n"))
	// Then
	if err != nil {
		t.Fatalf("expected empty input to succeed: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("expected no devices, got %#v", devices)
	}
}

func TestParseIbstat_portBeforeDeviceReturnsError(t *testing.T) {
	// Given
	raw := "    Port 1:\n        State: Active\n"
	// When
	_, err := ParseIbstat([]byte(raw))
	// Then
	if err == nil || !strings.Contains(err.Error(), "port") {
		t.Fatalf("expected orphan-port error, got %v", err)
	}
}

func TestParseIbstat_malformedPortNumberReturnsError(t *testing.T) {
	// Given
	raw := "CA 'mlx5_0'\n    Port x:\n        State: Active\n"
	// When
	_, err := ParseIbstat([]byte(raw))
	// Then
	if err == nil || !strings.Contains(err.Error(), "port number") {
		t.Fatalf("expected malformed port number error, got %v", err)
	}
}

func TestMerge_devinfoOnlyKeepsDevinfoFields(t *testing.T) {
	// Given
	devinfo := []Device{{
		Name:      "mlx5_0",
		NodeGUID:  "0c42:a103:0089:1234",
		FWVersion: "20.31.1014",
		Ports:     []Port{{Num: 1, State: "PORT_ACTIVE", LinkLayer: "InfiniBand"}},
	}}
	// When
	merged := Merge(devinfo, nil)
	// Then
	if !reflect.DeepEqual(merged, devinfo) {
		t.Fatalf("expected devinfo-only merge to be unchanged:\n got %#v\nwant %#v", merged, devinfo)
	}
}

func TestMerge_ibstatFillsRateAndLinkLayerWhenEmpty(t *testing.T) {
	// Given
	devinfo := []Device{{
		Name:      "mlx5_0",
		NodeGUID:  "0c42:a103:0089:1234",
		FWVersion: "20.31.1014",
		Ports:     []Port{{Num: 1, State: "PORT_ACTIVE"}}, // no link layer, no rate
	}}
	ibstat := []Device{{
		Name:  "mlx5_0",
		Ports: []Port{{Num: 1, State: "Active", Rate: "200", LinkLayer: "InfiniBand"}},
	}}
	// When
	merged := Merge(devinfo, ibstat)
	// Then
	if len(merged) != 1 || len(merged[0].Ports) != 1 {
		t.Fatalf("expected one merged device with one port, got %#v", merged)
	}
	p := merged[0].Ports[0]
	if p.State != "PORT_ACTIVE" {
		t.Fatalf("expected devinfo State to win, got %q", p.State)
	}
	if p.Rate != "200" {
		t.Fatalf("expected ibstat Rate to fill, got %q", p.Rate)
	}
	if p.LinkLayer != "InfiniBand" {
		t.Fatalf("expected ibstat LinkLayer to fill, got %q", p.LinkLayer)
	}
}

func TestMerge_doesNotOverrideExistingDevinfoFields(t *testing.T) {
	// Given
	devinfo := []Device{{
		Name:  "mlx5_0",
		Ports: []Port{{Num: 1, State: "PORT_ACTIVE", Rate: "400", LinkLayer: "InfiniBand"}},
	}}
	ibstat := []Device{{
		Name:  "mlx5_0",
		Ports: []Port{{Num: 1, State: "Active", Rate: "200", LinkLayer: "Ethernet"}},
	}}
	// When
	merged := Merge(devinfo, ibstat)
	// Then
	p := merged[0].Ports[0]
	if p.Rate != "400" || p.LinkLayer != "InfiniBand" {
		t.Fatalf("expected devinfo fields preserved, got %#v", p)
	}
}

func TestMerge_appendsIbstatOnlyDevices(t *testing.T) {
	// Given
	ibstat := []Device{{
		Name:      "mlx5_9",
		FWVersion: "1.0",
		Ports:     []Port{{Num: 1, State: "Active", Rate: "100", LinkLayer: "Ethernet"}},
	}}
	// When
	merged := Merge(nil, ibstat)
	// Then
	if len(merged) != 1 || merged[0].Name != "mlx5_9" {
		t.Fatalf("expected ibstat-only device appended, got %#v", merged)
	}
	if merged[0].Ports[0].LinkLayer != "Ethernet" {
		t.Fatalf("expected ibstat link layer, got %#v", merged[0].Ports[0])
	}
}

func TestMerge_appendsIbstatOnlyPortsToKnownDevice(t *testing.T) {
	// Given
	devinfo := []Device{{
		Name:  "mlx5_0",
		Ports: []Port{{Num: 1, State: "PORT_ACTIVE", LinkLayer: "InfiniBand"}},
	}}
	ibstat := []Device{{
		Name:  "mlx5_0",
		Ports: []Port{{Num: 2, State: "Active", Rate: "200", LinkLayer: "InfiniBand"}},
	}}
	// When
	merged := Merge(devinfo, ibstat)
	// Then
	if len(merged[0].Ports) != 2 {
		t.Fatalf("expected two ports after merge, got %#v", merged[0].Ports)
	}
	if merged[0].Ports[1].Num != 2 || merged[0].Ports[1].Rate != "200" {
		t.Fatalf("expected ibstat-only port appended, got %#v", merged[0].Ports[1])
	}
}

func TestMerge_fillsFWVersionWhenDevinfoEmpty(t *testing.T) {
	// Given
	devinfo := []Device{{Name: "mlx5_0", Ports: []Port{{Num: 1, State: "PORT_ACTIVE"}}}}
	ibstat := []Device{{Name: "mlx5_0", FWVersion: "20.31.1014", Ports: []Port{{Num: 1, Rate: "200"}}}}
	// When
	merged := Merge(devinfo, ibstat)
	// Then
	if merged[0].FWVersion != "20.31.1014" {
		t.Fatalf("expected ibstat FW version to fill, got %q", merged[0].FWVersion)
	}
}

func TestCollect_returnsDevices_whenOnlyDevinfoResolves(t *testing.T) {
	// Given
	runner := &fakeRunner{outputs: map[string][]byte{"ibv_devinfo": []byte(devinfoNoLinkLayer)}}
	overrideLookPath(t, func(name string) (string, error) {
		if strings.HasSuffix(name, "ibstat") {
			return "", exec.ErrNotFound
		}
		return "/usr/bin/" + name, nil
	})
	// When
	result, err := Collect(context.Background(), runner)
	// Then
	if err != nil {
		t.Fatalf("expected collect to succeed with devinfo only: %v", err)
	}
	if len(result.Devices) != 1 || result.Devices[0].Name != "mlx5_0" {
		t.Fatalf("expected one device from devinfo, got %#v", result.Devices)
	}
	if runner.calls["ibstat"] {
		t.Fatalf("expected ibstat not to be run when it is missing")
	}
}

func TestCollect_mergesBothTools_whenBothResolve(t *testing.T) {
	// Given
	runner := &fakeRunner{outputs: map[string][]byte{
		"ibv_devinfo": []byte(devinfoNoLinkLayer),
		"ibstat":      []byte(ibstatInfiniBand),
	}}
	overrideLookPath(t, func(name string) (string, error) { return "/usr/bin/" + name, nil })
	// When
	result, err := Collect(context.Background(), runner)
	// Then
	if err != nil {
		t.Fatalf("expected collect to succeed: %v", err)
	}
	if len(result.Devices) != 1 {
		t.Fatalf("expected one merged device, got %#v", result.Devices)
	}
	p := result.Devices[0].Ports[0]
	if p.State != "PORT_ACTIVE" {
		t.Fatalf("expected devinfo State, got %q", p.State)
	}
	if p.Rate != "200" || p.LinkLayer != "InfiniBand" {
		t.Fatalf("expected ibstat rate/link layer filled, got %#v", p)
	}
}

func TestCollect_returnsErrToolNotInstalled_whenBothMissing(t *testing.T) {
	// Given
	runner := &fakeRunner{}
	overrideLookPath(t, func(string) (string, error) { return "", exec.ErrNotFound })
	// When
	result, err := Collect(context.Background(), runner)
	// Then
	if result != nil {
		t.Fatalf("expected no result when both tools missing, got %#v", result)
	}
	if !errors.Is(err, ErrToolNotInstalled) {
		t.Fatalf("expected ErrToolNotInstalled, got %v", err)
	}
	if runner.anyCalled() {
		t.Fatalf("expected runner never called when both tools missing")
	}
}

func TestCollect_usesDefaultRunner_whenRunnerIsNilAndBothMissing(t *testing.T) {
	// Given
	overrideLookPath(t, func(string) (string, error) { return "", exec.ErrNotFound })
	// When
	_, err := Collect(context.Background(), nil)
	// Then
	if !errors.Is(err, ErrToolNotInstalled) {
		t.Fatalf("expected ErrToolNotInstalled, got %v", err)
	}
}

func TestCollect_returnsRunnerError_whenPresentToolFails(t *testing.T) {
	// Given
	runErr := errors.New("boom")
	runner := &fakeRunner{errs: map[string]error{"ibv_devinfo": runErr}}
	overrideLookPath(t, func(name string) (string, error) {
		if strings.HasSuffix(name, "ibstat") {
			return "", exec.ErrNotFound
		}
		return "/usr/bin/" + name, nil
	})
	// When
	_, err := Collect(context.Background(), runner)
	// Then
	if !errors.Is(err, runErr) {
		t.Fatalf("expected wrapped runner error, got %v", err)
	}
	if !strings.Contains(err.Error(), "ibv_devinfo") {
		t.Fatalf("expected ibv_devinfo context, got %v", err)
	}
}

func TestCollect_returnsParseError_whenPresentToolOutputMalformed(t *testing.T) {
	// Given
	runner := &fakeRunner{outputs: map[string][]byte{
		"ibv_devinfo": []byte("hca_id: mlx5_0\n        port:   abc\n"),
	}}
	overrideLookPath(t, func(name string) (string, error) {
		if strings.HasSuffix(name, "ibstat") {
			return "", exec.ErrNotFound
		}
		return "/usr/bin/" + name, nil
	})
	// When
	_, err := Collect(context.Background(), runner)
	// Then
	if err == nil || !strings.Contains(err.Error(), "port number") {
		t.Fatalf("expected parse error surfaced, got %v", err)
	}
}

func TestCollect_returnsIbstatRunnerError_whenOnlyIbstatResolvesAndFails(t *testing.T) {
	// Given
	runErr := errors.New("ibstat boom")
	runner := &fakeRunner{errs: map[string]error{"ibstat": runErr}}
	overrideLookPath(t, func(name string) (string, error) {
		if strings.HasSuffix(name, "ibv_devinfo") {
			return "", exec.ErrNotFound
		}
		return "/usr/bin/" + name, nil
	})
	// When
	_, err := Collect(context.Background(), runner)
	// Then
	if !errors.Is(err, runErr) {
		t.Fatalf("expected wrapped ibstat runner error, got %v", err)
	}
	if !strings.Contains(err.Error(), "ibstat") {
		t.Fatalf("expected ibstat context, got %v", err)
	}
}

func TestOsExecRunnerRun_returnsCommandStdout_whenCommandSucceeds(t *testing.T) {
	// Given
	runner := osExecRunner{}
	// When
	out, err := runner.Run(context.Background(), "echo", "rdma-ok")
	// Then
	if err != nil {
		t.Fatalf("expected echo to succeed: %v", err)
	}
	if strings.TrimSpace(string(out)) != "rdma-ok" {
		t.Fatalf("expected echo output rdma-ok, got %q", string(out))
	}
}

func TestOsExecRunnerRun_returnsError_whenCommandFails(t *testing.T) {
	// Given
	runner := osExecRunner{}
	missingBinary := "/nonexistent/gpu-tools-rdma-missing-binary"
	// When
	out, err := runner.Run(context.Background(), missingBinary)
	// Then
	if out != nil {
		t.Fatalf("expected no stdout when command fails, got %q", string(out))
	}
	if err == nil || !strings.Contains(err.Error(), "run "+missingBinary) {
		t.Fatalf("expected wrapped exec error, got %v", err)
	}
}

func TestLookPath_returnsWrappedResultForDefaultImplementation(t *testing.T) {
	// Given
	missingBinary := "/nonexistent/gpu-tools-rdma-missing-binary"
	// When
	path, err := lookPath(os.Args[0])
	_, missingErr := lookPath(missingBinary)
	// Then
	if err != nil {
		t.Fatalf("expected current test binary lookup to succeed: %v", err)
	}
	if path == "" {
		t.Fatalf("expected non-empty path for current test binary")
	}
	if missingErr == nil || !strings.Contains(missingErr.Error(), "look path "+missingBinary) {
		t.Fatalf("expected wrapped missing binary error, got %v", missingErr)
	}
}

type fakeRunner struct {
	outputs map[string][]byte
	errs    map[string]error
	calls   map[string]bool
}

func (r *fakeRunner) tool(name string) string {
	switch {
	case strings.HasSuffix(name, "ibv_devinfo"):
		return "ibv_devinfo"
	case strings.HasSuffix(name, "ibstat"):
		return "ibstat"
	default:
		return name
	}
}

func (r *fakeRunner) Run(_ context.Context, name string, _ ...string) ([]byte, error) {
	tool := r.tool(name)
	if r.calls == nil {
		r.calls = map[string]bool{}
	}
	r.calls[tool] = true
	if r.errs != nil {
		if err, ok := r.errs[tool]; ok {
			return nil, err
		}
	}
	if r.outputs != nil {
		if out, ok := r.outputs[tool]; ok {
			return out, nil
		}
	}
	return nil, nil
}

func (r *fakeRunner) anyCalled() bool {
	for _, called := range r.calls {
		if called {
			return true
		}
	}
	return false
}

func overrideLookPath(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	previous := lookPath
	lookPath = fn
	t.Cleanup(func() { lookPath = previous })
}
