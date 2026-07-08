package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sunerpy/gpu-tools/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print detailed build and version information for gpu-tools.",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), version.Info())
			return err
		},
	}
}

func init() {
	registerCommand(newVersionCmd)
}
