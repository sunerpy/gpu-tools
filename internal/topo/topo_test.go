package topo

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// matrixInput joins the given lines with tabs already embedded and appends a
// trailing newline, mimicking the exact shape of `nvidia-smi topo -m`.
func matrixInput(lines ...string) []byte {
	return []byte(strings.Join(lines, "\n") + "\n")
}

// twoGPUNVLink models a 2-GPU NVLink machine with one NIC row plus CPU/NUMA
// affinity columns.
func twoGPUNVLink() []byte {
	return matrixInput(
		"\tGPU0\tGPU1\tNIC0\tCPU Affinity\tNUMA Affinity",
		"GPU0\t X \tNV12\tPXB\t0-23\t0",
		"GPU1\tNV12\t X \tSYS\t24-47\t1",
		"NIC0\tPXB\tSYS\t X ",
		"",
		"Legend:",
		"",
		"  X    = Self",
		"  SYS  = Connection traversing PCIe as well as the SMP interconnect",
		"  NV#  = Connection traversing a bonded set of # NVLinks",
	)
}

// pureP CIe models a machine with no NVLink; GPU-GPU distances are PCIe
// classifications (PIX/PHB/SYS) and there are two NIC rows.
func purePCIe() []byte {
	return matrixInput(
		"\tGPU0\tGPU1\tNIC0\tNIC1\tCPU Affinity\tNUMA Affinity",
		"GPU0\t X \tPHB\tPIX\tSYS\t0-15\t0",
		"GPU1\tPHB\t X \tSYS\tPXB\t16-31\t1",
		"NIC0\tPIX\tSYS\t X \tPHB",
		"NIC1\tSYS\tPXB\tPHB\t X ",
		"Legend:",
		"  X = Self",
	)
}

// singleGPU models a machine with exactly one GPU and affinity columns but no
// NIC rows.
func singleGPU() []byte {
	return matrixInput(
		"\tGPU0\tCPU Affinity\tNUMA Affinity",
		"GPU0\t X \t0-23\t0",
	)
}

// nvlinkNoAffinity models an older format with NVLink and no affinity columns
// and no NICs.
func nvlinkNoAffinity() []byte {
	return matrixInput(
		"\tGPU0\tGPU1",
		"GPU0\t X \tNV12",
		"GPU1\tNV12\t X ",
	)
}

func TestParse(t *testing.T) {
	t.Run("parses 2-GPU NVLink machine with NIC and affinity columns", func(t *testing.T) {
		// When
		m, err := Parse(twoGPUNVLink())
		// Then
		if err != nil {
			t.Fatalf("Parse returned error: %v", err)
		}
		if got := m.GPUs; len(got) != 2 || got[0] != "GPU0" || got[1] != "GPU1" {
			t.Fatalf("GPUs = %v, want [GPU0 GPU1]", got)
		}
		self := m.Cells["GPU0"]["GPU0"]
		if self.Type != LinkSelf {
			t.Fatalf("GPU0->GPU0 type = %q, want %q", self.Type, LinkSelf)
		}
		link := m.Cells["GPU0"]["GPU1"]
		if link.Type != LinkNVLink || link.Lanes != 12 {
			t.Fatalf("GPU0->GPU1 = %+v, want NVLINK lanes 12", link)
		}
		if link := m.Cells["GPU1"]["GPU0"]; link.Type != LinkNVLink || link.Lanes != 12 {
			t.Fatalf("GPU1->GPU0 = %+v, want NVLINK lanes 12", link)
		}
		if len(m.NICs) != 1 || m.NICs[0].NIC != "NIC0" {
			t.Fatalf("NICs = %+v, want single NIC0", m.NICs)
		}
		if c := m.NICs[0].PerGPU["GPU0"]; c.Type != LinkPXB {
			t.Fatalf("NIC0->GPU0 = %q, want PXB", c.Type)
		}
		if c := m.NICs[0].PerGPU["GPU1"]; c.Type != LinkSYS {
			t.Fatalf("NIC0->GPU1 = %q, want SYS", c.Type)
		}
	})

	t.Run("parses pure-PCIe machine with two NICs and no NVLink", func(t *testing.T) {
		// When
		m, err := Parse(purePCIe())
		// Then
		if err != nil {
			t.Fatalf("Parse returned error: %v", err)
		}
		if len(m.GPUs) != 2 {
			t.Fatalf("GPUs = %v, want 2", m.GPUs)
		}
		if c := m.Cells["GPU0"]["GPU1"]; c.Type != LinkPHB || c.Lanes != 0 {
			t.Fatalf("GPU0->GPU1 = %+v, want PHB lanes 0", c)
		}
		if len(m.NICs) != 2 {
			t.Fatalf("NICs = %+v, want 2", m.NICs)
		}
		if c := m.NICs[0].PerGPU["GPU0"]; c.Type != LinkPIX {
			t.Fatalf("NIC0->GPU0 = %q, want PIX", c.Type)
		}
		if c := m.NICs[1].PerGPU["GPU1"]; c.Type != LinkPXB {
			t.Fatalf("NIC1->GPU1 = %q, want PXB", c.Type)
		}
	})

	t.Run("parses single-GPU machine with affinity columns and no NICs", func(t *testing.T) {
		// When
		m, err := Parse(singleGPU())
		// Then
		if err != nil {
			t.Fatalf("Parse returned error: %v", err)
		}
		if len(m.GPUs) != 1 || m.GPUs[0] != "GPU0" {
			t.Fatalf("GPUs = %v, want [GPU0]", m.GPUs)
		}
		if c := m.Cells["GPU0"]["GPU0"]; c.Type != LinkSelf {
			t.Fatalf("GPU0->GPU0 = %q, want SELF", c.Type)
		}
		if len(m.NICs) != 0 {
			t.Fatalf("NICs = %+v, want none", m.NICs)
		}
	})

	t.Run("parses NVLink machine with no affinity columns and no NICs", func(t *testing.T) {
		// When
		m, err := Parse(nvlinkNoAffinity())
		// Then
		if err != nil {
			t.Fatalf("Parse returned error: %v", err)
		}
		if len(m.GPUs) != 2 {
			t.Fatalf("GPUs = %v, want 2", m.GPUs)
		}
		if c := m.Cells["GPU1"]["GPU0"]; c.Type != LinkNVLink || c.Lanes != 12 {
			t.Fatalf("GPU1->GPU0 = %+v, want NVLINK lanes 12", c)
		}
		if len(m.NICs) != 0 {
			t.Fatalf("NICs = %+v, want none", m.NICs)
		}
	})
}

func TestParse_errors(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
	}{
		{name: "empty input", raw: []byte("")},
		{name: "whitespace only", raw: []byte("   \n\t\n")},
		{name: "garbage with no device columns", raw: matrixInput("some random text", "no matrix here")},
		{name: "header only without data rows", raw: matrixInput("\tGPU0\tGPU1")},
		{name: "data row too short", raw: matrixInput("\tGPU0\tGPU1", "GPU0\t X ")},
		{name: "unknown link token", raw: matrixInput("\tGPU0\tGPU1", "GPU0\t X \tWAT", "GPU1\tWAT\t X ")},
		{name: "malformed nvlink lane count", raw: matrixInput("\tGPU0\tGPU1", "GPU0\t X \tNV", "GPU1\tNV\t X ")},
		{name: "only affinity columns no gpu", raw: matrixInput("\tCPU Affinity\tNUMA Affinity", "GPU0\t0-23\t0")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// When
			_, err := Parse(tc.raw)
			// Then
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want error", tc.name)
			}
		})
	}
}

func TestRate(t *testing.T) {
	// Given
	m, err := Parse(purePCIe())
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	// When
	advice := Rate(m)

	// Then
	byKey := make(map[string]Advice, len(advice))
	for _, a := range advice {
		byKey[a.NIC+"/"+a.GPU] = a
	}
	want := map[string]Rating{
		"NIC0/GPU0": RatingGood, // PIX
		"NIC0/GPU1": RatingBad,  // SYS
		"NIC1/GPU0": RatingBad,  // SYS
		"NIC1/GPU1": RatingGood, // PXB
	}
	if len(advice) != len(want) {
		t.Fatalf("advice count = %d, want %d (%+v)", len(advice), len(want), advice)
	}
	for key, wantRating := range want {
		got, ok := byKey[key]
		if !ok {
			t.Fatalf("advice missing key %q; got %+v", key, advice)
		}
		if got.Rating != wantRating {
			t.Fatalf("advice[%q].Rating = %q, want %q", key, got.Rating, wantRating)
		}
	}
}

func TestRate_classifications(t *testing.T) {
	// Given a matrix that exercises every rating bucket plus skipped links.
	m := &Matrix{
		GPUs: []string{"GPU0"},
		Cells: map[string]map[string]Cell{
			"GPU0": {"GPU0": {Type: LinkSelf}},
		},
		NICs: []NICAffinity{
			{NIC: "NICgood1", PerGPU: map[string]Cell{"GPU0": {Type: LinkPIX}}},
			{NIC: "NICgood2", PerGPU: map[string]Cell{"GPU0": {Type: LinkPXB}}},
			{NIC: "NICwarn1", PerGPU: map[string]Cell{"GPU0": {Type: LinkPHB}}},
			{NIC: "NICwarn2", PerGPU: map[string]Cell{"GPU0": {Type: LinkNODE}}},
			{NIC: "NICbad", PerGPU: map[string]Cell{"GPU0": {Type: LinkSYS}}},
			{NIC: "NICself", PerGPU: map[string]Cell{"GPU0": {Type: LinkSelf}}},
			{NIC: "NICx", PerGPU: map[string]Cell{"GPU0": {Type: LinkX}}},
			{NIC: "NICnv", PerGPU: map[string]Cell{"GPU0": {Type: LinkNVLink, Lanes: 12}}},
			{NIC: "NICmissing", PerGPU: map[string]Cell{}},
		},
	}

	// When
	advice := Rate(m)

	// Then only the five rateable pairs should be present.
	got := make(map[string]Rating, len(advice))
	for _, a := range advice {
		got[a.NIC] = a.Rating
	}
	want := map[string]Rating{
		"NICgood1": RatingGood,
		"NICgood2": RatingGood,
		"NICwarn1": RatingWarn,
		"NICwarn2": RatingWarn,
		"NICbad":   RatingBad,
	}
	if len(got) != len(want) {
		t.Fatalf("rated pairs = %+v, want %+v", got, want)
	}
	for nic, wantRating := range want {
		if got[nic] != wantRating {
			t.Fatalf("advice[%q] = %q, want %q", nic, got[nic], wantRating)
		}
	}
}

func TestRate_nilMatrix(t *testing.T) {
	if advice := Rate(nil); advice != nil {
		t.Fatalf("Rate(nil) = %+v, want nil", advice)
	}
}

type fakeRunner struct {
	output []byte
	err    error
	name   string
	args   []string
	called bool
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.called = true
	r.name = name
	r.args = append([]string(nil), args...)
	if r.err != nil {
		return nil, r.err
	}
	return r.output, nil
}

func overrideLookPath(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	previous := lookPath
	lookPath = fn
	t.Cleanup(func() { lookPath = previous })
}

func TestCollect(t *testing.T) {
	t.Run("parses topo output using an explicit smiPath", func(t *testing.T) {
		// Given
		runner := &fakeRunner{output: twoGPUNVLink()}

		// When
		result, err := Collect(context.Background(), runner, "/usr/bin/nvidia-smi")
		// Then
		if err != nil {
			t.Fatalf("Collect returned error: %v", err)
		}
		if !runner.called {
			t.Fatalf("runner was not called")
		}
		if runner.name != "/usr/bin/nvidia-smi" {
			t.Fatalf("runner name = %q, want /usr/bin/nvidia-smi", runner.name)
		}
		if len(runner.args) != 2 || runner.args[0] != "topo" || runner.args[1] != "-m" {
			t.Fatalf("runner args = %v, want [topo -m]", runner.args)
		}
		if len(result.Matrix.GPUs) != 2 {
			t.Fatalf("GPUs = %v, want 2", result.Matrix.GPUs)
		}
		if len(result.Advice) != 2 {
			t.Fatalf("advice = %+v, want 2 entries", result.Advice)
		}
	})

	t.Run("resolves nvidia-smi via lookPath when smiPath is empty", func(t *testing.T) {
		// Given
		overrideLookPath(t, func(name string) (string, error) {
			if name != "nvidia-smi" {
				t.Fatalf("lookPath name = %q, want nvidia-smi", name)
			}
			return "/opt/bin/nvidia-smi", nil
		})
		runner := &fakeRunner{output: singleGPU()}

		// When
		result, err := Collect(context.Background(), runner, "")
		// Then
		if err != nil {
			t.Fatalf("Collect returned error: %v", err)
		}
		if runner.name != "/opt/bin/nvidia-smi" {
			t.Fatalf("runner name = %q, want /opt/bin/nvidia-smi", runner.name)
		}
		if len(result.Matrix.GPUs) != 1 {
			t.Fatalf("GPUs = %v, want 1", result.Matrix.GPUs)
		}
	})

	t.Run("wraps ErrToolNotInstalled when nvidia-smi is missing", func(t *testing.T) {
		// Given
		overrideLookPath(t, func(string) (string, error) {
			return "", errors.New("not found")
		})
		runner := &fakeRunner{output: twoGPUNVLink()}

		// When
		_, err := Collect(context.Background(), runner, "")

		// Then
		if !errors.Is(err, ErrToolNotInstalled) {
			t.Fatalf("err = %v, want ErrToolNotInstalled", err)
		}
		if runner.called {
			t.Fatalf("runner should not be called when nvidia-smi is missing")
		}
	})

	t.Run("propagates runner errors", func(t *testing.T) {
		// Given
		runner := &fakeRunner{err: errors.New("boom")}

		// When
		_, err := Collect(context.Background(), runner, "/usr/bin/nvidia-smi")

		// Then
		if err == nil {
			t.Fatalf("Collect returned nil error, want runner error")
		}
	})

	t.Run("propagates parse errors on garbage output", func(t *testing.T) {
		// Given
		runner := &fakeRunner{output: []byte("garbage without a matrix")}

		// When
		_, err := Collect(context.Background(), runner, "/usr/bin/nvidia-smi")

		// Then
		if err == nil {
			t.Fatalf("Collect returned nil error, want parse error")
		}
	})

	t.Run("falls back to the default runner when runner is nil", func(t *testing.T) {
		// Given
		overrideLookPath(t, func(string) (string, error) {
			return "", errors.New("not found")
		})

		// When: nil runner still reaches lookPath and fails cleanly.
		_, err := Collect(context.Background(), nil, "")

		// Then
		if !errors.Is(err, ErrToolNotInstalled) {
			t.Fatalf("err = %v, want ErrToolNotInstalled", err)
		}
	})
}

func TestOSExecRunner_Run(t *testing.T) {
	t.Run("returns command output on success", func(t *testing.T) {
		// When
		out, err := osExecRunner{}.Run(context.Background(), "echo", "hello")
		// Then
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
		if strings.TrimSpace(string(out)) != "hello" {
			t.Fatalf("Run output = %q, want hello", out)
		}
	})

	t.Run("returns an error for a missing binary", func(t *testing.T) {
		// When
		_, err := osExecRunner{}.Run(context.Background(), "definitely-not-a-real-binary-xyz")

		// Then
		if err == nil {
			t.Fatalf("Run returned nil error, want error for missing binary")
		}
	})
}

func TestDefaultLookPath(t *testing.T) {
	// The package-level lookPath resolves real binaries and errors on missing
	// ones. Exercise both paths against the default implementation.
	if _, err := lookPath("definitely-not-a-real-binary-xyz"); err == nil {
		t.Fatalf("lookPath returned nil error for a missing binary")
	}
}
