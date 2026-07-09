package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/sunerpy/gpu-tools/core"
	"github.com/sunerpy/gpu-tools/internal/gpu"
	"github.com/sunerpy/gpu-tools/internal/gpu/cache"
	"github.com/sunerpy/gpu-tools/internal/report"
)

// watchFlag is the detect-local flag enabling refresh mode. A zero duration
// (the default) keeps the one-shot behavior unchanged.
const watchFlag = "watch"

// clearScreen is the ANSI sequence written before each table/markdown watch
// frame to move the cursor home and erase the screen.
const clearScreen = "\033[H\033[2J"

// newTicker is the ticker seam. Tests replace it to drive an exact number of
// frames without waiting on wall-clock time.
var newTicker = func(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTicker(d)
	return t.C, t.Stop
}

// watchContext is the cancellation seam. Production wires it to SIGINT via
// signal.NotifyContext; tests replace it to cancel deterministically after the
// injected ticker drains.
var watchContext = func() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

func newDetectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "detect",
		Short: "Detect local NVIDIA GPU inventory",
		Long:  "Detect local NVIDIA GPUs and render a point-in-time inventory snapshot.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDetect(cmd)
		},
	}
	cmd.Flags().Duration(watchFlag, 0, "continuously refresh every <duration> (e.g. 2s); 0 disables watch mode")
	return cmd
}

func runDetect(cmd *cobra.Command) error {
	cfg, err := resolvedConfig(cmd)
	if err != nil {
		return err
	}
	watch, err := cmd.Flags().GetDuration(watchFlag)
	if err != nil {
		return fmt.Errorf("read --watch: %w", err)
	}
	if watch > 0 {
		return runDetectWatch(cmd, cfg, watch)
	}
	return runDetectOnce(cmd, cfg)
}

// runDetectOnce performs the unchanged one-shot detection flow.
func runDetectOnce(cmd *cobra.Command, cfg *core.Config) (err error) {
	collector, err := gpu.DefaultFactory(*cfg)
	if err != nil {
		return detectFactoryError(err)
	}
	if err := collector.Init(); err != nil {
		return fmt.Errorf("initialize GPU collector: %w", err)
	}
	defer func() {
		err = errors.Join(err, collector.Shutdown())
	}()

	renderer, err := report.RendererFor(cfg.DefaultOutput)
	if err != nil {
		return fmt.Errorf("select detect renderer: %w", err)
	}
	snapshot, err := collectSnapshot(collector)
	if err != nil {
		return err
	}
	if err := renderer.Render(cmd.OutOrStdout(), snapshot); err != nil {
		return fmt.Errorf("render detect snapshot: %w", err)
	}
	return nil
}

// runDetectWatch wraps the collector in a TTL cache and re-renders one frame per
// ticker tick until SIGINT (or the injected context) cancels. The first read is
// taken eagerly so a permanently-unavailable backend fails fast with exit 1
// instead of spinning.
func runDetectWatch(cmd *cobra.Command, cfg *core.Config, watch time.Duration) (err error) {
	inner, err := gpu.DefaultFactory(*cfg)
	if err != nil {
		return detectFactoryError(err)
	}
	if err := inner.Init(); err != nil {
		return fmt.Errorf("initialize GPU collector: %w", err)
	}
	defer func() {
		err = errors.Join(err, inner.Shutdown())
	}()

	ndjson := cfg.DefaultOutput == core.OutputJSON
	var renderer report.Renderer
	if !ndjson {
		renderer, err = report.RendererFor(cfg.DefaultOutput)
		if err != nil {
			return fmt.Errorf("select detect renderer: %w", err)
		}
	}

	collector := cache.New(inner, watchCacheTTL(watch))

	// Eager first read: fail fast on a permanent backend error, never loop.
	// The snapshot is discarded here — rendering happens once per ticker tick so
	// exactly one frame is emitted per tick.
	if _, err := collectSnapshot(collector); err != nil {
		return detectReadError(err)
	}

	out := cmd.OutOrStdout()
	ctx, cancel := watchContext()
	defer cancel()
	ticks, stop := newTicker(watch)
	defer stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-ticks:
			if !ok {
				return nil
			}
			snapshot, serr := collectSnapshot(collector)
			if serr != nil {
				return fmt.Errorf("watch detect snapshot: %w", serr)
			}
			if ndjson {
				if werr := renderNDJSON(out, snapshot); werr != nil {
					return werr
				}
				continue
			}
			if werr := renderFrame(out, renderer, snapshot); werr != nil {
				return werr
			}
		}
	}
}

// watchCacheTTL clamps the refresh cache TTL to at most one second so a watch
// interval longer than a second still reflects fresh data each frame. It is a
// package var so tests can force per-tick refresh.
var watchCacheTTL = func(watch time.Duration) time.Duration {
	if watch > time.Second {
		return time.Second
	}
	return watch
}

// renderFrame clears the screen then renders one full table/markdown snapshot.
func renderFrame(w io.Writer, renderer report.Renderer, snapshot *report.Snapshot) error {
	if _, err := io.WriteString(w, clearScreen); err != nil {
		return fmt.Errorf("clear watch frame: %w", err)
	}
	if err := renderer.Render(w, snapshot); err != nil {
		return fmt.Errorf("render detect snapshot: %w", err)
	}
	return nil
}

// renderNDJSON writes one compact, newline-terminated JSON object with no screen
// clear, per plan R3.
func renderNDJSON(w io.Writer, snapshot *report.Snapshot) error {
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(snapshot); err != nil {
		return fmt.Errorf("encode ndjson snapshot: %w", err)
	}
	return nil
}

// collectSnapshot reads a consistent device snapshot from the collector.
func collectSnapshot(collector gpu.Collector) (*report.Snapshot, error) {
	count, err := collector.DeviceCount()
	if err != nil {
		return nil, fmt.Errorf("count GPU devices: %w", err)
	}
	devices := make([]gpu.Device, 0, count)
	for i := range count {
		device, err := collector.Device(i)
		if err != nil {
			return nil, fmt.Errorf("read GPU device %d: %w", i, err)
		}
		if device == nil {
			return nil, fmt.Errorf("read GPU device %d: nil device", i)
		}
		devices = append(devices, *device)
	}
	host, _ := os.Hostname()
	return &report.Snapshot{
		Host:      host,
		Timestamp: time.Now(),
		Backend:   collector.Backend(),
		Devices:   devices,
	}, nil
}

func detectFactoryError(err error) error {
	if errors.Is(err, gpu.ErrBackendUnavailable) || errors.Is(err, gpu.ErrNoBackend) {
		return NewExitError(1, fmt.Errorf("no NVIDIA GPU detected: %w", err))
	}
	return NewExitError(1, fmt.Errorf("select GPU backend: %w", err))
}

// detectReadError maps a permanent backend failure surfaced on the first watch
// read to a single exit-1 error instead of a retry loop.
func detectReadError(err error) error {
	if errors.Is(err, gpu.ErrBackendUnavailable) || errors.Is(err, gpu.ErrNoBackend) {
		return NewExitError(1, fmt.Errorf("no NVIDIA GPU detected: %w", err))
	}
	return err
}

func init() {
	registerCommand(newDetectCmd)
}
