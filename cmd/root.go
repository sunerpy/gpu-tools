package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/sunerpy/gpu-tools/core"
	_ "github.com/sunerpy/gpu-tools/internal/gpu/nvidiasmi"
	_ "github.com/sunerpy/gpu-tools/internal/gpu/nvml"
)

const (
	outputFlag  = "output"
	configFlag  = "config"
	backendFlag = "backend"
)

type commandConstructor func() *cobra.Command

var commandConstructors []commandConstructor

var rootCmd = newRootCmd()

func newRootCmd() *cobra.Command {
	defaults := defaultRootOptions()
	cmd := &cobra.Command{
		Use:   "gpu-tools",
		Short: "GPU diagnostics and tuning utilities",
		Long: `gpu-tools provides command line utilities for inspecting GPU environments,
rendering reports, and preparing future detection, tuning, and benchmark workflows.`,
		Example: `  gpu-tools --help
  gpu-tools version
  gpu-tools config init
  gpu-tools config show --output json`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			_, err := resolvedConfig(cmd)
			return err
		},
	}
	cmd.PersistentFlags().StringP(outputFlag, "o", defaults.DefaultOutput, "output format: table, json, or markdown")
	cmd.PersistentFlags().String(configFlag, defaults.ConfigPath, "path to config file")
	cmd.PersistentFlags().String(backendFlag, defaults.Backend, "GPU backend: auto, nvml, or nvidia-smi")
	for _, constructor := range commandConstructors {
		cmd.AddCommand(constructor())
	}
	return cmd
}

func Execute() {
	if code := execute(rootCmd); code != 0 {
		os.Exit(code)
	}
}

func execute(cmd *cobra.Command) int {
	if err := cmd.Execute(); err != nil {
		return handleExecuteError(cmd.ErrOrStderr(), err)
	}
	return 0
}

func handleExecuteError(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintln(stderr, err)
	if exitErr, ok := errors.AsType[*ExitError](err); ok {
		if exitErr.Code != 0 {
			return exitErr.Code
		}
	}
	return 1
}

func registerCommand(constructor commandConstructor) {
	commandConstructors = append(commandConstructors, constructor)
	rootCmd.AddCommand(constructor())
}

type rootOptions struct {
	ConfigPath    string
	DefaultOutput string
	Backend       string
}

func defaultRootOptions() rootOptions {
	configPath, err := core.DefaultConfigPath()
	if err != nil {
		return rootOptions{DefaultOutput: core.OutputTable, Backend: core.BackendAuto}
	}
	cfg, err := core.Load(configPath)
	if err != nil {
		return rootOptions{ConfigPath: configPath, DefaultOutput: core.OutputTable, Backend: core.BackendAuto}
	}
	return rootOptions{ConfigPath: configPath, DefaultOutput: cfg.DefaultOutput, Backend: cfg.Backend}
}

func resolvedConfig(cmd *cobra.Command) (*core.Config, error) {
	root := cmd.Root()
	flags := root.PersistentFlags()
	configPath, err := flags.GetString(configFlag)
	if err != nil {
		return nil, fmt.Errorf("read --config: %w", err)
	}
	cfg, err := core.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if flags.Changed(outputFlag) {
		output, err := flags.GetString(outputFlag)
		if err != nil {
			return nil, fmt.Errorf("read --output: %w", err)
		}
		cfg.DefaultOutput = output
	}
	if flags.Changed(backendFlag) {
		backend, err := flags.GetString(backendFlag)
		if err != nil {
			return nil, fmt.Errorf("read --backend: %w", err)
		}
		cfg.Backend = backend
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}
