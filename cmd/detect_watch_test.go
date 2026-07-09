package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sunerpy/gpu-tools/internal/gpu"
	"github.com/sunerpy/gpu-tools/internal/report"
)

// clearSeq is the ANSI screen-clear sequence emitted once per table/markdown
// watch frame.
const clearSeq = "\033[H\033[2J"

// overrideWatchSeams installs a ticker that fires exactly frames times and then
// cancels the watch context, plus a context whose cancellation the watch loop
// observes. It returns the injected context so the caller can reason about it.
func overrideWatchSeams(t *testing.T, frames int) {
	t.Helper()

	previousTicker := newTicker
	previousCtx := watchContext

	ch := make(chan time.Time, frames)
	ctx, cancel := context.WithCancel(context.Background())

	newTicker = func(time.Duration) (<-chan time.Time, func()) {
		for range frames {
			ch <- time.Now()
		}
		return ch, func() {}
	}
	watchContext = func() (context.Context, context.CancelFunc) {
		// Cancel shortly after the frames drain so the loop exits cleanly.
		go func() {
			deadline := time.After(2 * time.Second)
			for {
				if len(ch) == 0 {
					cancel()
					return
				}
				select {
				case <-deadline:
					cancel()
					return
				case <-time.After(time.Millisecond):
				}
			}
		}()
		return ctx, cancel
	}

	t.Cleanup(func() {
		newTicker = previousTicker
		watchContext = previousCtx
		cancel()
	})
}

func TestDetectWatch_rendersExactlyNTableFrames_whenTickerFiresNTimes(t *testing.T) {
	// Given
	collector := newFakeCollector([]gpu.Device{{Index: 0, UUID: "GPU-0", Name: "RTX 4090"}})
	overrideGPUFactory(t, collector, nil)
	overrideWatchSeams(t, 3)
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, stderr, err := executeCommand(newRootCmd(), "detect", "-o", "table", "--watch", "1s")
	// Then
	if err != nil {
		t.Fatalf("expected watch table to exit cleanly, got %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if got := strings.Count(stdout, clearSeq); got != 3 {
		t.Fatalf("expected exactly 3 screen clears, got %d in:\n%q", got, stdout)
	}
	if got := strings.Count(stdout, "RTX 4090"); got != 3 {
		t.Fatalf("expected device rendered 3 times, got %d", got)
	}
}

func TestDetectWatch_emitsNIndependentNDJSONLines_whenJSONWatchRequested(t *testing.T) {
	// Given
	collector := newFakeCollector([]gpu.Device{{Index: 0, UUID: "GPU-json", Name: "JSON GPU"}})
	overrideGPUFactory(t, collector, nil)
	overrideWatchSeams(t, 4)
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "detect", "-o", "json", "--watch", "1s")
	// Then
	if err != nil {
		t.Fatalf("expected watch json to exit cleanly, got %v", err)
	}
	if strings.Contains(stdout, clearSeq) {
		t.Fatalf("expected NO screen clear in NDJSON mode, got:\n%q", stdout)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected exactly 4 NDJSON lines, got %d:\n%q", len(lines), stdout)
	}
	for i, line := range lines {
		if strings.Contains(line, "  ") {
			t.Fatalf("expected compact JSON on line %d, found indentation: %q", i, line)
		}
		var snapshot report.Snapshot
		if err := json.Unmarshal([]byte(line), &snapshot); err != nil {
			t.Fatalf("expected line %d to be independently unmarshalable, got %v: %q", i, err, line)
		}
		if len(snapshot.Devices) != 1 || snapshot.Devices[0].Name != "JSON GPU" {
			t.Fatalf("expected JSON GPU device on line %d, got %#v", i, snapshot.Devices)
		}
	}
}

func TestDetectWatch_returnsExitErrorOnce_whenBackendPermanentlyUnavailable(t *testing.T) {
	// Given
	overrideGPUFactory(t, nil, gpu.ErrBackendUnavailable)
	// A ticker that would fire many times if the loop erroneously spun.
	overrideWatchSeams(t, 100)
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "detect", "--watch", "1s")

	// Then
	if err == nil {
		t.Fatalf("expected permanent backend error to fail fast")
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

func TestDetectWatch_failsFast_whenFirstReadHitsPermanentBackendError(t *testing.T) {
	// Given: factory succeeds but the collector's first read is permanently unavailable.
	collector := &fakeCollector{countErr: gpu.ErrNoBackend}
	overrideGPUFactory(t, collector, nil)
	overrideWatchSeams(t, 100)
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "detect", "-o", "json", "--watch", "1s")

	// Then
	if err == nil {
		t.Fatalf("expected permanent read error to fail fast")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.Code)
	}
}

func TestDetectWatch_leavesOneShotUnchanged_whenWatchZero(t *testing.T) {
	// Given
	collector := newFakeCollector([]gpu.Device{{Index: 0, UUID: "GPU-0", Name: "RTX 4090"}})
	overrideGPUFactory(t, collector, nil)
	t.Setenv("HOME", t.TempDir())

	// When: explicit --watch 0 must behave identically to no flag (one-shot, no clears).
	stdout, _, err := executeCommand(newRootCmd(), "detect", "-o", "table", "--watch", "0")
	// Then
	if err != nil {
		t.Fatalf("expected one-shot to succeed, got %v", err)
	}
	if strings.Contains(stdout, clearSeq) {
		t.Fatalf("expected NO screen clear in one-shot mode, got:\n%q", stdout)
	}
	if got := strings.Count(stdout, "RTX 4090"); got != 1 {
		t.Fatalf("expected device rendered exactly once, got %d", got)
	}
}

// TestDetectWatch_realTickerSeamProducesChannel guards the production ticker seam
// so its stop function and channel are exercised without a real time delay.
func TestDetectWatch_realTickerSeamProducesChannel(t *testing.T) {
	// Given / When
	ch, stop := newTicker(time.Millisecond)
	defer stop()

	// Then
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("expected production ticker to fire within a second")
	}
}

// TestDetectWatch_realContextSeamCancelsOnStop guards the production context seam
// so signal.NotifyContext is wired and its cancel func releases resources.
func TestDetectWatch_realContextSeamCancelsOnStop(t *testing.T) {
	// Given / When
	ctx, cancel := watchContext()
	cancel()

	// Then
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatalf("expected canceled context to be done")
	}
}

func TestDetectWatch_rendersMarkdownFrames_whenMarkdownWatchRequested(t *testing.T) {
	// Given
	collector := newFakeCollector([]gpu.Device{{Index: 0, UUID: "GPU-md", Name: "MD GPU"}})
	overrideGPUFactory(t, collector, nil)
	overrideWatchSeams(t, 2)
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "detect", "-o", "markdown", "--watch", "1s")
	// Then
	if err != nil {
		t.Fatalf("expected markdown watch to exit cleanly, got %v", err)
	}
	if got := strings.Count(stdout, clearSeq); got != 2 {
		t.Fatalf("expected exactly 2 screen clears, got %d", got)
	}
	if got := strings.Count(stdout, "## GPU Snapshot"); got != 2 {
		t.Fatalf("expected markdown heading rendered 2 times, got %d", got)
	}
}

func TestDetectWatch_clampsTTLToOneSecond_whenWatchExceedsOneSecond(t *testing.T) {
	// Given: watch=5s clamps the cache TTL to 1s; the ticker still drives frames.
	collector := newFakeCollector([]gpu.Device{{Index: 0, Name: "clamp GPU"}})
	overrideGPUFactory(t, collector, nil)
	overrideWatchSeams(t, 1)
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "detect", "-o", "table", "--watch", "5s")
	// Then
	if err != nil {
		t.Fatalf("expected clamped watch to exit cleanly, got %v", err)
	}
	if got := strings.Count(stdout, clearSeq); got != 1 {
		t.Fatalf("expected exactly 1 frame, got %d", got)
	}
}

func TestDetectWatch_returnsWithoutFrames_whenContextCancelsBeforeTick(t *testing.T) {
	// Given: a ticker that never fires and a context already canceled.
	collector := newFakeCollector([]gpu.Device{{Index: 0, Name: "idle GPU"}})
	overrideGPUFactory(t, collector, nil)

	previousTicker := newTicker
	previousCtx := watchContext
	newTicker = func(time.Duration) (<-chan time.Time, func()) {
		return make(chan time.Time), func() {}
	}
	watchContext = func() (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx, cancel
	}
	t.Cleanup(func() {
		newTicker = previousTicker
		watchContext = previousCtx
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "detect", "-o", "table", "--watch", "1s")
	// Then
	if err != nil {
		t.Fatalf("expected canceled watch to exit cleanly, got %v", err)
	}
	if strings.Contains(stdout, clearSeq) {
		t.Fatalf("expected no frames when context cancels first, got:\n%q", stdout)
	}
}

func TestDetectWatch_returnsWithoutFrames_whenTickerChannelCloses(t *testing.T) {
	// Given: a closed ticker channel signals the loop to exit cleanly.
	collector := newFakeCollector([]gpu.Device{{Index: 0, Name: "closed GPU"}})
	overrideGPUFactory(t, collector, nil)

	previousTicker := newTicker
	previousCtx := watchContext
	closed := make(chan time.Time)
	close(closed)
	newTicker = func(time.Duration) (<-chan time.Time, func()) {
		return closed, func() {}
	}
	watchContext = func() (context.Context, context.CancelFunc) {
		return context.WithCancel(context.Background())
	}
	t.Cleanup(func() {
		newTicker = previousTicker
		watchContext = previousCtx
	})
	t.Setenv("HOME", t.TempDir())

	// When
	stdout, _, err := executeCommand(newRootCmd(), "detect", "-o", "table", "--watch", "1s")
	// Then
	if err != nil {
		t.Fatalf("expected closed-ticker watch to exit cleanly, got %v", err)
	}
	if strings.Contains(stdout, clearSeq) {
		t.Fatalf("expected no frames when ticker closes first, got:\n%q", stdout)
	}
}

func TestDetectWatch_returnsWrappedError_whenLaterTickReadFails(t *testing.T) {
	// Given: the eager read succeeds, then a later tick read fails transiently.
	// With a zero cache TTL every collectSnapshot triggers fresh reads: the eager
	// snapshot consumes two count calls (DeviceCount + Device refresh), so failing
	// after two lets the first tick be the failure.
	collector := &countingCollector{devices: []gpu.Device{{Index: 0, Name: "flaky GPU"}}, failAfter: 2}
	overrideGPUFactory(t, collector, nil)
	overrideZeroCacheTTL(t)
	overrideWatchSeams(t, 3)
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "detect", "-o", "table", "--watch", "1s")

	// Then
	if err == nil {
		t.Fatalf("expected a mid-stream read failure to propagate")
	}
	if !strings.Contains(err.Error(), "watch detect snapshot") {
		t.Fatalf("expected watch snapshot error, got %q", err.Error())
	}
}

func TestDetectWatch_returnsWriteError_whenNDJSONEncodeFailsMidStream(t *testing.T) {
	// Given: a factory-backed collector plus a stdout that fails on the tick write.
	collector := newFakeCollector([]gpu.Device{{Index: 0, Name: "json GPU"}})
	overrideGPUFactory(t, collector, nil)
	overrideWatchSeams(t, 1)
	t.Setenv("HOME", t.TempDir())

	root := newRootCmd()
	root.SetOut(failingWriter{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"detect", "-o", "json", "--watch", "1s"})

	// When
	err := root.Execute()

	// Then
	if err == nil || !strings.Contains(err.Error(), "encode ndjson snapshot") {
		t.Fatalf("expected ndjson encode error to propagate, got %v", err)
	}
}

// overrideZeroCacheTTL forces the refresh cache to expire immediately so every
// ticker tick pulls a fresh read from the underlying collector.
func overrideZeroCacheTTL(t *testing.T) {
	t.Helper()
	previous := watchCacheTTL
	watchCacheTTL = func(time.Duration) time.Duration { return 0 }
	t.Cleanup(func() { watchCacheTTL = previous })
}

func TestDetectWatch_returnsWrappedError_whenInitFails(t *testing.T) {
	// Given
	collector := &fakeCollector{initErr: errors.New("init boom")}
	overrideGPUFactory(t, collector, nil)
	overrideWatchSeams(t, 1)
	t.Setenv("HOME", t.TempDir())

	// When
	_, _, err := executeCommand(newRootCmd(), "detect", "--watch", "1s")

	// Then
	if err == nil || !strings.Contains(err.Error(), "initialize GPU collector") {
		t.Fatalf("expected initialize GPU collector error, got %v", err)
	}
}

func TestDetectWatch_returnsWriteError_whenFrameClearFailsMidStream(t *testing.T) {
	// Given: a table watch whose stdout fails on the frame's clear write.
	collector := newFakeCollector([]gpu.Device{{Index: 0, Name: "frame GPU"}})
	overrideGPUFactory(t, collector, nil)
	overrideWatchSeams(t, 1)
	t.Setenv("HOME", t.TempDir())

	root := newRootCmd()
	root.SetOut(failingWriter{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"detect", "-o", "table", "--watch", "1s"})

	// When
	err := root.Execute()

	// Then
	if err == nil || !strings.Contains(err.Error(), "clear watch frame") {
		t.Fatalf("expected clear watch frame error to propagate, got %v", err)
	}
}

func TestRenderFrame_returnsError_whenClearWriteFails(t *testing.T) {
	// Given / When
	err := renderFrame(failingWriter{}, report.TableRenderer{}, &report.Snapshot{})

	// Then
	if err == nil || !strings.Contains(err.Error(), "clear watch frame") {
		t.Fatalf("expected clear watch frame error, got %v", err)
	}
}

func TestRenderFrame_returnsError_whenRenderFails(t *testing.T) {
	// Given: writer accepts the clear sequence, then fails on the render body.
	w := &clearThenFailWriter{}

	// When
	err := renderFrame(w, report.TableRenderer{}, &report.Snapshot{Devices: []gpu.Device{{Index: 0}}})

	// Then
	if err == nil || !strings.Contains(err.Error(), "render detect snapshot") {
		t.Fatalf("expected render detect snapshot error, got %v", err)
	}
}

func TestRenderNDJSON_returnsError_whenEncodeWriteFails(t *testing.T) {
	// Given / When
	err := renderNDJSON(failingWriter{}, &report.Snapshot{})

	// Then
	if err == nil || !strings.Contains(err.Error(), "encode ndjson snapshot") {
		t.Fatalf("expected encode ndjson snapshot error, got %v", err)
	}
}

func TestDetectReadError_returnsRawError_whenNotPermanentBackend(t *testing.T) {
	// Given
	cause := errors.New("transient boom")

	// When
	err := detectReadError(cause)

	// Then
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		t.Fatalf("expected non-permanent error to pass through unwrapped, got ExitError")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("expected original error, got %v", err)
	}
}

// countingCollector serves a fixed device set but starts returning an error on
// its DeviceCount call after failAfter successful reads, simulating a transient
// backend hiccup mid-watch.
type countingCollector struct {
	devices   []gpu.Device
	failAfter int
	reads     int
}

func (c *countingCollector) Init() error     { return nil }
func (c *countingCollector) Shutdown() error { return nil }
func (c *countingCollector) Backend() string { return "counting" }

func (c *countingCollector) DeviceCount() (int, error) {
	c.reads++
	if c.reads > c.failAfter {
		return 0, errors.New("transient count failure")
	}
	return len(c.devices), nil
}

func (c *countingCollector) Device(i int) (*gpu.Device, error) {
	if i < 0 || i >= len(c.devices) {
		return nil, errors.New("device index out of range")
	}
	d := c.devices[i]
	return &d, nil
}

// clearThenFailWriter accepts the first write (the ANSI clear) and fails every
// subsequent write, isolating the render-body error path in renderFrame.
type clearThenFailWriter struct {
	writes int
}

func (w *clearThenFailWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == 1 {
		return len(p), nil
	}
	return 0, errors.New("render body write failed")
}
