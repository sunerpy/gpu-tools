package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"

	"github.com/sunerpy/gpu-tools/core"
	"github.com/sunerpy/gpu-tools/internal/exporter"
	"github.com/sunerpy/gpu-tools/internal/gpu"
)

const (
	exportListenFlag    = "listen"
	exportDefaultListen = ":9835"
	exportShutdownGrace = 5 * time.Second
)

// exportOptions carries the injectable seams runExport needs so cmd tests can
// bind an ephemeral port, learn the bound address, stop via context, or force a
// serve error by supplying a pre-closed listener.
type exportOptions struct {
	ctx      context.Context
	listen   string
	ready    chan<- string
	listener net.Listener
}

func newExportCmd() *cobra.Command {
	var listen string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Serve GPU metrics for Prometheus on /metrics",
		Long:  "Run a headless Prometheus exporter that serves GPU metrics on /metrics until interrupted.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			configPath, _ := cmd.Root().PersistentFlags().GetString(configFlag)
			return runExport(cmd, exportOptions{ctx: ctx, listen: listen}, configPath)
		},
	}
	cmd.Flags().StringVar(&listen, exportListenFlag, exportDefaultListen, "address to serve /metrics on")
	return cmd
}

func runExport(cmd *cobra.Command, opts exportOptions, configPath string) error {
	cfg, err := resolvedConfigFrom(cmd, configPath)
	if err != nil {
		return err
	}
	exp := exporter.New(func() (gpu.Collector, error) {
		return gpu.DefaultFactory(*cfg)
	})

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(exp.Registry(), promhttp.HandlerOpts{}))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "gpu-tools exporter\n")
	})

	listener := opts.listener
	if listener == nil {
		var err error
		listener, err = net.Listen("tcp", opts.listen)
		if err != nil {
			return fmt.Errorf("listen on %s: %w", opts.listen, err)
		}
	}
	if opts.ready != nil {
		opts.ready <- listener.Addr().String()
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "gpu-tools exporter listening on %s/metrics\n", listener.Addr())

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(listener)
	}()

	select {
	case <-opts.ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), exportShutdownGrace)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown exporter: %w", err)
		}
		return nil
	case err := <-serveErr:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("serve exporter: %w", err)
		}
		return nil
	}
}

// resolvedConfigFrom mirrors resolvedConfig but takes the config path as an
// argument (the test seam) rather than reading the persistent --config flag.
func resolvedConfigFrom(cmd *cobra.Command, configPath string) (*core.Config, error) {
	cfg, err := core.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	flags := cmd.Root().PersistentFlags()
	if flags.Changed(outputFlag) {
		output, _ := flags.GetString(outputFlag)
		cfg.DefaultOutput = output
	}
	if flags.Changed(backendFlag) {
		backend, _ := flags.GetString(backendFlag)
		cfg.Backend = backend
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func init() {
	registerCommand(newExportCmd)
}
