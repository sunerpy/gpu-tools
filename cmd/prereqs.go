package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sunerpy/gpu-tools/core"
	"github.com/sunerpy/gpu-tools/internal/prereq"
)

// prereqChecks is the detection seam. Tests replace it to inject fixed checks
// without depending on the host PATH or /etc/os-release.
var prereqChecks = func() []prereq.CheckResult {
	return prereq.Check()
}

func newPrereqsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prereqs",
		Short: "Check prerequisite external tools and show install guidance",
		Long: "Detect whether each prerequisite external tool (nvidia-smi, ibv_devinfo, " +
			"ibstat, perftest, nccl-tests, dcgm) is installed and print platform-aware " +
			"install guidance for the ones that are missing. Linux only. This is " +
			"informational: missing tools are reported, not treated as a failure, so the " +
			"command exits 0.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPrereqs(cmd)
		},
	}
}

func runPrereqs(cmd *cobra.Command) error {
	if !platformIsLinux() {
		return prereqsUnsupported(cmd)
	}
	cfg, err := resolvedConfig(cmd)
	if err != nil {
		return err
	}
	checks := prereqChecks()
	if err := renderPrereqs(cmd.OutOrStdout(), cfg.DefaultOutput, checks); err != nil {
		return fmt.Errorf("render prereqs result: %w", err)
	}
	return nil
}

// prereqsUnsupported handles the non-Linux path. For JSON output it emits the
// small unsupported-platform object to stdout; for every output mode it returns
// an exit-2 error carrying the clean message.
func prereqsUnsupported(cmd *cobra.Command) error {
	osName := platformOS()
	output, _ := cmd.Flags().GetString(outputFlag)
	if output == core.OutputJSON {
		payload := map[string]any{
			"supported":      false,
			"platform":       osName,
			"reason":         "requires Linux",
			"required_tools": prereqRequiredTools(),
		}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(payload); err != nil {
			return NewExitError(2, fmt.Errorf("encode unsupported-platform payload: %w", err))
		}
	}
	return &ExitError{Code: 2, Err: fmt.Errorf("gpu-tools prereqs requires Linux; current OS: %s", osName)}
}

// prereqRequiredTools lists the binaries this command probes, for the
// unsupported-platform payload.
func prereqRequiredTools() []string {
	tools := make([]string, 0, len(prereq.Tools))
	for _, t := range prereq.Tools {
		tools = append(tools, t.Binary)
	}
	return tools
}

func renderPrereqs(w io.Writer, output string, checks []prereq.CheckResult) error {
	switch output {
	case core.OutputTable:
		return renderPrereqsTable(w, checks)
	case core.OutputJSON:
		return renderPrereqsJSON(w, checks)
	case core.OutputMarkdown:
		return renderPrereqsMarkdown(w, checks)
	default:
		return fmt.Errorf("unknown prereqs output format %q", output)
	}
}

func renderPrereqsJSON(w io.Writer, checks []prereq.CheckResult) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(checks)
}

// installCell shows the install hint only for missing tools; found tools show a
// dash so the guidance column is not noisy.
func installCell(check prereq.CheckResult) string {
	if check.Found {
		return "-"
	}
	if check.Install == "" {
		return "-"
	}
	return check.Install
}

// pathCell renders a dash for tools that were not found.
func pathCell(check prereq.CheckResult) string {
	if check.Path == "" {
		return "-"
	}
	return check.Path
}

func renderPrereqsTable(w io.Writer, checks []prereq.CheckResult) error {
	var builder strings.Builder
	builder.WriteString("Prerequisite Tools\n")
	tw := tabwriter.NewWriter(&builder, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "Tool\tFound\tPath\tPurpose\tInstall"); err != nil {
		return err
	}
	for _, check := range checks {
		if _, err := fmt.Fprintf(tw, "%s\t%t\t%s\t%s\t%s\n",
			check.Tool, check.Found, pathCell(check), check.Purpose, installCell(check)); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := io.WriteString(w, builder.String())
	return err
}

func renderPrereqsMarkdown(w io.Writer, checks []prereq.CheckResult) error {
	var builder strings.Builder
	fmt.Fprintln(&builder, "## Prerequisite Tools")
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, "| Tool | Found | Path | Purpose | Install |")
	fmt.Fprintln(&builder, "| --- | --- | --- | --- | --- |")
	for _, check := range checks {
		fmt.Fprintf(&builder, "| %s | %t | %s | %s | %s |\n",
			check.Tool, check.Found, pathCell(check), check.Purpose, installCell(check))
	}
	_, err := io.WriteString(w, builder.String())
	return err
}

func init() {
	registerCommand(newPrereqsCmd)
}
