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
	"github.com/sunerpy/gpu-tools/internal/rdma"
)

// rdmaCollect is the collection seam. Tests replace it to inject a fake RDMA
// inventory without a real ibv_devinfo/ibstat on the host.
var rdmaCollect = func(ctx context.Context) (*rdma.Result, error) {
	return rdma.Collect(ctx, nil)
}

// rdmaPortView is the JSON-tagged projection of rdma.Port. The internal/rdma
// structs carry no json tags, so cmd owns the wire shape here.
type rdmaPortView struct {
	Num       int    `json:"num"`
	State     string `json:"state"`
	Rate      string `json:"rate"`
	LinkLayer string `json:"link_layer"`
}

// rdmaDeviceView is the JSON-tagged projection of rdma.Device.
type rdmaDeviceView struct {
	Name      string         `json:"name"`
	NodeGUID  string         `json:"node_guid"`
	FWVersion string         `json:"fw_version"`
	Ports     []rdmaPortView `json:"ports"`
}

// rdmaResultView is the JSON-tagged projection of rdma.Result.
type rdmaResultView struct {
	Devices []rdmaDeviceView `json:"devices"`
}

func toRDMAResultView(result *rdma.Result) rdmaResultView {
	devices := make([]rdmaDeviceView, 0, len(result.Devices))
	for _, device := range result.Devices {
		ports := make([]rdmaPortView, 0, len(device.Ports))
		for _, port := range device.Ports {
			ports = append(ports, rdmaPortView{
				Num:       port.Num,
				State:     port.State,
				Rate:      port.Rate,
				LinkLayer: port.LinkLayer,
			})
		}
		devices = append(devices, rdmaDeviceView{
			Name:      device.Name,
			NodeGUID:  device.NodeGUID,
			FWVersion: device.FWVersion,
			Ports:     ports,
		})
	}
	return rdmaResultView{Devices: devices}
}

func newRDMACmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rdma",
		Short: "List RDMA devices, ports, rates, and link layer (InfiniBand/RoCE)",
		Long: "List RDMA host channel adapters, their ports, rates, and link layer " +
			"(InfiniBand or RoCE), merging `ibv_devinfo -v` and `ibstat` output. " +
			"Linux only; requires OFED or rdma-core (ibv_devinfo/ibstat).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRDMA(cmd)
		},
	}
}

func runRDMA(cmd *cobra.Command) error {
	if !platformIsLinux() {
		return rdmaUnsupported(cmd)
	}
	cfg, err := resolvedConfig(cmd)
	if err != nil {
		return err
	}
	result, err := rdmaCollect(cmd.Context())
	if err != nil {
		if errors.Is(err, rdma.ErrToolNotInstalled) {
			return &ExitError{Code: 2, Err: fmt.Errorf("ibv_devinfo/ibstat not installed, install OFED or rdma-core and retry: %w", err)}
		}
		return NewExitError(1, err)
	}
	if result == nil {
		return NewExitError(1, fmt.Errorf("rdma returned no result"))
	}
	if err := renderRDMA(cmd.OutOrStdout(), cfg.DefaultOutput, result); err != nil {
		return fmt.Errorf("render rdma result: %w", err)
	}
	return nil
}

// rdmaUnsupported handles the non-Linux path. For JSON output it emits the
// small unsupported-platform object to stdout; for every output mode it returns
// an exit-2 error carrying the clean message.
func rdmaUnsupported(cmd *cobra.Command) error {
	osName := platformOS()
	output, _ := cmd.Flags().GetString(outputFlag)
	if output == core.OutputJSON {
		payload := map[string]any{
			"supported":      false,
			"platform":       osName,
			"reason":         "requires Linux",
			"required_tools": []string{"ibv_devinfo", "ibstat"},
		}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(payload); err != nil {
			return NewExitError(2, fmt.Errorf("encode unsupported-platform payload: %w", err))
		}
	}
	return &ExitError{Code: 2, Err: fmt.Errorf("gpu-tools rdma requires Linux (uses ibv_devinfo/ibstat); current OS: %s", osName)}
}

func renderRDMA(w io.Writer, output string, result *rdma.Result) error {
	switch output {
	case core.OutputTable:
		return renderRDMATable(w, result)
	case core.OutputJSON:
		return renderRDMAJSON(w, result)
	case core.OutputMarkdown:
		return renderRDMAMarkdown(w, result)
	default:
		return fmt.Errorf("unknown rdma output format %q", output)
	}
}

func renderRDMAJSON(w io.Writer, result *rdma.Result) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(toRDMAResultView(result))
}

func renderRDMATable(w io.Writer, result *rdma.Result) error {
	var builder strings.Builder
	builder.WriteString("RDMA Devices\n")
	if len(result.Devices) == 0 {
		builder.WriteString("no RDMA devices found\n")
		_, err := io.WriteString(w, builder.String())
		return err
	}
	for _, device := range result.Devices {
		fmt.Fprintf(&builder, "\nDevice: %s\n", device.Name)
		fmt.Fprintf(&builder, "  NodeGUID:  %s\n", device.NodeGUID)
		fmt.Fprintf(&builder, "  FWVersion: %s\n", device.FWVersion)
		tw := tabwriter.NewWriter(&builder, 0, 0, 2, ' ', 0)
		if _, err := fmt.Fprintln(tw, "  Port\tState\tRate\tLink Layer"); err != nil {
			return err
		}
		for _, port := range device.Ports {
			if _, err := fmt.Fprintf(tw, "  %d\t%s\t%s\t%s\n", port.Num, port.State, port.Rate, port.LinkLayer); err != nil {
				return err
			}
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, builder.String())
	return err
}

func renderRDMAMarkdown(w io.Writer, result *rdma.Result) error {
	var builder strings.Builder
	fmt.Fprintln(&builder, "# RDMA Devices")
	if len(result.Devices) == 0 {
		fmt.Fprintln(&builder)
		fmt.Fprintln(&builder, "No RDMA devices found.")
		_, err := io.WriteString(w, builder.String())
		return err
	}
	for _, device := range result.Devices {
		fmt.Fprintln(&builder)
		fmt.Fprintf(&builder, "## %s\n", device.Name)
		fmt.Fprintln(&builder)
		fmt.Fprintf(&builder, "- NodeGUID: %s\n", device.NodeGUID)
		fmt.Fprintf(&builder, "- FWVersion: %s\n", device.FWVersion)
		fmt.Fprintln(&builder)
		fmt.Fprintln(&builder, "| Port | State | Rate | Link Layer |")
		fmt.Fprintln(&builder, "| --- | --- | --- | --- |")
		for _, port := range device.Ports {
			fmt.Fprintf(&builder, "| %d | %s | %s | %s |\n", port.Num, port.State, port.Rate, port.LinkLayer)
		}
	}
	_, err := io.WriteString(w, builder.String())
	return err
}

func init() {
	registerCommand(newRDMACmd)
}
