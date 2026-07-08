package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/sunerpy/gpu-tools/core"
)

var marshalYAML = yaml.Marshal

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage gpu-tools configuration",
		Long:  "Manage the gpu-tools YAML configuration file and print resolved settings.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newConfigInitCmd(), newConfigShowCmd())
	return cmd
}

func newConfigInitCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write the default configuration file",
		Long:  "Write the default gpu-tools configuration to ~/.gpu-tools/config.yaml.",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := core.DefaultConfigPath()
			if err != nil {
				return fmt.Errorf("resolve default config path: %w", err)
			}
			if err := writeDefaultConfig(path, force); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Wrote default config to %s\n", path)
			return err
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing config file")
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the resolved configuration",
		Long:  "Load gpu-tools configuration, apply supported flag overrides, and print YAML.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolvedConfig(cmd)
			if err != nil {
				return err
			}
			data, err := marshalYAML(cfg)
			if err != nil {
				return fmt.Errorf("encode config: %w", err)
			}
			_, err = cmd.OutOrStdout().Write(data)
			return err
		},
	}
}

func writeDefaultConfig(path string, force bool) error {
	if _, err := os.Stat(path); err == nil && !force {
		return NewExitError(1, fmt.Errorf("config %s already exists; use --force to overwrite", path))
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat config %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory %s: %w", filepath.Dir(path), err)
	}
	data, err := marshalYAML(defaultConfig())
	if err != nil {
		return fmt.Errorf("encode default config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}

func defaultConfig() *core.Config {
	return &core.Config{
		DefaultOutput: core.OutputTable,
		Backend:       core.BackendAuto,
		ReportDir:     ".",
	}
}

func init() {
	registerCommand(newConfigCmd)
}
