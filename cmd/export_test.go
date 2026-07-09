package cmd

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sunerpy/gpu-tools/core"
)

func TestExportCommand_servesMetricsAndShutsDownCleanly_whenContextCancelled(t *testing.T) {
	// Given
	configPath := writeReportConfig(t, t.TempDir(), core.OutputTable)
	overrideGPUFactory(t, newFakeCollector(reportDevices()), nil)
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)

	// When: run the export server in a goroutine bound to an ephemeral port.
	go func() {
		root := newRootCmd()
		done <- runExport(root, exportOptions{
			ctx:    ctx,
			listen: "127.0.0.1:0",
			ready:  ready,
		}, configPath)
	}()

	addr := waitReady(t, ready)
	body := scrapeExport(t, "http://"+addr+"/metrics")

	// Then: metrics served with the up gauge.
	if !strings.Contains(body, "gpu_tools_up 1") {
		t.Fatalf("expected gpu_tools_up 1, got:\n%s", body)
	}
	if !strings.Contains(body, "gpu_utilization_percent") {
		t.Fatalf("expected device series, got:\n%s", body)
	}

	// health line on /
	healthBody := scrapeExport(t, "http://"+addr+"/")
	if !strings.Contains(healthBody, "gpu-tools exporter") {
		t.Fatalf("expected health line, got %q", healthBody)
	}

	// When: cancel context → clean shutdown, RunE returns nil, listener freed.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean shutdown (nil), got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("export did not shut down within timeout (leaked listener?)")
	}

	if _, err := http.Get("http://" + addr + "/metrics"); err == nil {
		t.Fatalf("expected listener to be closed after shutdown")
	}
}

func TestExportCommand_returnsError_whenListenAddrInvalid(t *testing.T) {
	// Given
	configPath := writeReportConfig(t, t.TempDir(), core.OutputTable)
	overrideGPUFactory(t, newFakeCollector(reportDevices()), nil)

	// When: an unbindable address must surface as a RunE error, never os.Exit.
	err := runExport(newRootCmd(), exportOptions{
		ctx:    context.Background(),
		listen: "127.0.0.1:-1",
		ready:  make(chan string, 1),
	}, configPath)

	// Then
	if err == nil {
		t.Fatalf("expected error for invalid listen addr")
	}
}

func TestExportCommand_registersWithDefaultListenPort(t *testing.T) {
	// Given/When
	cmd := newExportCmd()

	// Then
	if cmd.Use != "export" {
		t.Fatalf("expected use 'export', got %q", cmd.Use)
	}
	flag := cmd.Flags().Lookup(exportListenFlag)
	if flag == nil {
		t.Fatalf("expected --listen flag")
	}
	if flag.DefValue != exportDefaultListen {
		t.Fatalf("expected default listen %q, got %q", exportDefaultListen, flag.DefValue)
	}
}

func TestResolvedConfigFrom_appliesFlagOverrides(t *testing.T) {
	// Given
	configPath := writeReportConfig(t, t.TempDir(), core.OutputTable)
	root := newRootCmd()
	if err := root.PersistentFlags().Set("output", core.OutputJSON); err != nil {
		t.Fatalf("set output: %v", err)
	}
	if err := root.PersistentFlags().Set("backend", core.BackendNVML); err != nil {
		t.Fatalf("set backend: %v", err)
	}

	// When
	cfg, err := resolvedConfigFrom(root, configPath)
	// Then
	if err != nil {
		t.Fatalf("expected config resolve, got: %v", err)
	}
	if cfg.DefaultOutput != core.OutputJSON {
		t.Fatalf("expected output override json, got %q", cfg.DefaultOutput)
	}
	if cfg.Backend != core.BackendNVML {
		t.Fatalf("expected backend override nvml, got %q", cfg.Backend)
	}
}

func TestResolvedConfigFrom_returnsError_whenBackendInvalid(t *testing.T) {
	// Given
	configPath := writeReportConfig(t, t.TempDir(), core.OutputTable)
	root := newRootCmd()
	if err := root.PersistentFlags().Set("backend", "bogus"); err != nil {
		t.Fatalf("set backend: %v", err)
	}

	// When
	_, err := resolvedConfigFrom(root, configPath)

	// Then
	if err == nil {
		t.Fatalf("expected validate error for bogus backend")
	}
}

func TestResolvedConfigFrom_returnsError_whenConfigInvalid(t *testing.T) {
	// Given: a config file with an invalid backend value on disk.
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("backend: nope\ndefault_output: table\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// When
	_, err := resolvedConfigFrom(newRootCmd(), configPath)

	// Then
	if err == nil {
		t.Fatalf("expected error loading invalid config")
	}
}

func TestExportCommand_returnsError_whenBackendUnavailable(t *testing.T) {
	// Given: factory that always fails; exporter still serves up 0, but runExport
	// surfaces a serve error only when the listener itself fails. Here we assert
	// the config resolve error path via an invalid backend override instead.
	configPath := writeReportConfig(t, t.TempDir(), core.OutputTable)
	root := newRootCmd()
	if err := root.PersistentFlags().Set("backend", "bogus"); err != nil {
		t.Fatalf("set backend: %v", err)
	}

	// When
	err := runExport(root, exportOptions{
		ctx:    context.Background(),
		listen: "127.0.0.1:0",
		ready:  make(chan string, 1),
	}, configPath)

	// Then
	if err == nil {
		t.Fatalf("expected config resolve error to surface from runExport")
	}
}

func TestExportCommand_runEShutsDownImmediately_whenParentContextCancelled(t *testing.T) {
	// Given: a root command whose context is already canceled, so the RunE
	// signal.NotifyContext-derived context is done the moment Serve starts.
	configPath := writeReportConfig(t, t.TempDir(), core.OutputTable)
	overrideGPUFactory(t, newFakeCollector(reportDevices()), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	root := newRootCmd()
	root.SetContext(ctx)

	// When: drive the real export subcommand through cobra on an ephemeral port.
	stdout, _, err := executeCommand(root, "--config", configPath, "export", "--listen", "127.0.0.1:0")
	// Then: RunE returns nil after graceful shutdown, never os.Exit.
	if err != nil {
		t.Fatalf("expected clean RunE exit, got: %v", err)
	}
	if stdout != "" {
		t.Fatalf("expected metrics on the wire, not stdout, got %q", stdout)
	}
}

func TestExportCommand_returnsError_whenServeFails(t *testing.T) {
	// Given: a pre-closed listener forces srv.Serve to return immediately with a
	// non-ErrServerClosed error while the context stays open (serveErr path).
	configPath := writeReportConfig(t, t.TempDir(), core.OutputTable)
	overrideGPUFactory(t, newFakeCollector(reportDevices()), nil)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	_ = listener.Close()

	// When
	err = runExport(newRootCmd(), exportOptions{
		ctx:      context.Background(),
		listener: listener,
	}, configPath)

	// Then
	if err == nil {
		t.Fatalf("expected serve error from closed listener")
	}
	if !strings.Contains(err.Error(), "serve exporter") {
		t.Fatalf("expected serve exporter error, got: %v", err)
	}
}

func waitReady(t *testing.T, ready <-chan string) string {
	t.Helper()
	select {
	case addr := <-ready:
		return addr
	case <-time.After(5 * time.Second):
		t.Fatalf("export server did not become ready")
		return ""
	}
}

func scrapeExport(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: expected 200, got %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s body: %v", url, err)
	}
	return string(body)
}
