package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sunerpy/gpu-tools/core"
	"github.com/sunerpy/gpu-tools/internal/health"
)

const doctorStrictFlag = "strict"

// healthRun is the probe-run seam. Tests replace it to inject a fake report
// without running the real /proc, lspci, and ibv_devinfo probes.
var healthRun = func(ctx context.Context) health.Report {
	return health.Run(ctx, health.DefaultProbes(nil))
}

// doctorResultView is a cmd-local JSON projection of health.Result so JSON
// output carries stable field names without editing internal/health.
type doctorResultView struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Hint   string `json:"hint,omitempty"`
}

// doctorReportView is the cmd-local JSON projection of health.Report.
type doctorReportView struct {
	Results []doctorResultView `json:"results"`
	Overall string             `json:"overall"`
}

func newDoctorCmd() *cobra.Command {
	var strict bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run read-only GPU/RDMA environment health checks",
		Long: "Run a set of read-only, non-destructive environment health checks: " +
			"NVIDIA driver presence, nvidia_peermem module, IOMMU=pt on the kernel " +
			"command line, PCIe ACS state, active RDMA ports, and the RDMA link layer. " +
			"Linux only; uses /proc, lspci, and ibv_devinfo. Finding problems is not a " +
			"command failure — the command exits 0 by default even if checks fail. Pass " +
			"--strict to exit non-zero when any check fails (for CI gating).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd, strict)
		},
	}
	cmd.Flags().BoolVar(&strict, doctorStrictFlag, false, "exit non-zero if any check fails")
	return cmd
}

func runDoctor(cmd *cobra.Command, strict bool) error {
	if !platformIsLinux() {
		return doctorUnsupported(cmd)
	}
	cfg, err := resolvedConfig(cmd)
	if err != nil {
		return err
	}
	report := healthRun(cmd.Context())
	if err := renderDoctor(cmd.OutOrStdout(), cfg.DefaultOutput, report); err != nil {
		return fmt.Errorf("render doctor report: %w", err)
	}
	// Exit-code contract: finding problems is not a command failure, so the
	// command exits 0 by default regardless of report.Overall. Only --strict
	// combined with a genuine fail (never warn) exits non-zero, and only after
	// the report has already been rendered to stdout so users always see it.
	if strict && report.Overall == health.StatusFail {
		return NewExitError(1, errors.New("health checks failed (--strict)"))
	}
	return nil
}

// doctorUnsupported handles the non-Linux path. For JSON output it emits the
// small unsupported-platform object to stdout; for every output mode it returns
// an exit-2 error carrying the clean message.
func doctorUnsupported(cmd *cobra.Command) error {
	osName := platformOS()
	output, _ := cmd.Flags().GetString(outputFlag)
	if output == core.OutputJSON {
		payload := map[string]any{
			"supported":      false,
			"platform":       osName,
			"reason":         "requires Linux",
			"required_tools": []string{"nvidia-smi", "lspci", "ibv_devinfo", "ibstat"},
		}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(payload); err != nil {
			return NewExitError(2, fmt.Errorf("encode unsupported-platform payload: %w", err))
		}
	}
	return &ExitError{Code: 2, Err: fmt.Errorf("gpu-tools doctor requires Linux (uses /proc, lspci, ibv_devinfo); current OS: %s", osName)}
}

func renderDoctor(w io.Writer, output string, report health.Report) error {
	switch output {
	case core.OutputTable:
		return renderDoctorTable(w, report)
	case core.OutputJSON:
		return renderDoctorJSON(w, report)
	case core.OutputMarkdown:
		return renderDoctorMarkdown(w, report)
	default:
		return fmt.Errorf("unknown doctor output format %q", output)
	}
}

func doctorReportToView(report health.Report) doctorReportView {
	results := make([]doctorResultView, 0, len(report.Results))
	for _, r := range report.Results {
		results = append(results, doctorResultView{
			Name:   r.Name,
			Status: string(r.Status),
			Detail: r.Detail,
			Hint:   r.Hint,
		})
	}
	return doctorReportView{Results: results, Overall: string(report.Overall)}
}

func renderDoctorJSON(w io.Writer, report health.Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(doctorReportToView(report))
}

func renderDoctorTable(w io.Writer, report health.Report) error {
	var builder strings.Builder
	tw := tabwriter.NewWriter(&builder, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "Check\tStatus\tDetail\tHint"); err != nil {
		return err
	}
	for _, r := range report.Results {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.Name, r.Status, r.Detail, r.Hint); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(&builder, "\nOverall: %s\n", report.Overall)
	_, err := io.WriteString(w, builder.String())
	return err
}

func renderDoctorMarkdown(w io.Writer, report health.Report) error {
	var builder strings.Builder
	fmt.Fprintln(&builder, "## Health Checks")
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, "| Check | Status | Detail | Hint |")
	fmt.Fprintln(&builder, "| --- | --- | --- | --- |")
	for _, r := range report.Results {
		fmt.Fprintf(&builder, "| %s | %s | %s | %s |\n", r.Name, r.Status, r.Detail, r.Hint)
	}
	fmt.Fprintln(&builder)
	fmt.Fprintf(&builder, "Overall: %s\n", report.Overall)
	_, err := io.WriteString(w, builder.String())
	return err
}

func init() {
	registerCommand(newDoctorCmd)
}
