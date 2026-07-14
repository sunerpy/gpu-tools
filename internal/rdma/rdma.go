package rdma

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Port describes a single RDMA device port.
type Port struct {
	Num       int
	State     string // e.g. "PORT_ACTIVE" / "PORT_DOWN" (devinfo) or "Active" (ibstat)
	Rate      string // raw rate number as reported by ibstat, e.g. "200"
	LinkLayer string // "InfiniBand" / "Ethernet"
}

// Device describes a single RDMA host channel adapter.
type Device struct {
	Name      string
	NodeGUID  string
	FWVersion string
	Ports     []Port
}

// Result is the merged RDMA inventory.
type Result struct {
	Devices []Device
}

const (
	toolDevinfo = "ibv_devinfo"
	toolIbstat  = "ibstat"
)

// Collect resolves and runs ibv_devinfo and ibstat, then merges their output.
// If neither tool resolves, it returns ErrToolNotInstalled. A tool that
// resolves but fails to run, or whose output cannot be parsed, surfaces the
// error rather than being silently swallowed.
func Collect(ctx context.Context, runner execRunner) (*Result, error) {
	if runner == nil {
		runner = defaultRunner.(execRunner)
	}

	devinfoPath, devinfoOK := resolve(toolDevinfo)
	ibstatPath, ibstatOK := resolve(toolIbstat)
	if !devinfoOK && !ibstatOK {
		return nil, fmt.Errorf("%w", ErrToolNotInstalled)
	}

	var devinfoDevices, ibstatDevices []Device
	if devinfoOK {
		out, err := runner.Run(ctx, devinfoPath, "-v")
		if err != nil {
			return nil, fmt.Errorf("run %s: %w", toolDevinfo, err)
		}
		devinfoDevices, err = ParseDevinfo(out)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", toolDevinfo, err)
		}
	}
	if ibstatOK {
		out, err := runner.Run(ctx, ibstatPath)
		if err != nil {
			return nil, fmt.Errorf("run %s: %w", toolIbstat, err)
		}
		ibstatDevices, err = ParseIbstat(out)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", toolIbstat, err)
		}
	}

	return &Result{Devices: Merge(devinfoDevices, ibstatDevices)}, nil
}

func resolve(tool string) (string, bool) {
	path, err := lookPath(tool)
	if err != nil {
		return "", false
	}
	return path, true
}

// ParseDevinfo parses `ibv_devinfo -v` output into devices.
func ParseDevinfo(raw []byte) ([]Device, error) {
	var devices []Device
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		switch {
		case hasKey(line, "hca_id"):
			devices = append(devices, Device{Name: value(line, "hca_id")})
		case hasKey(line, "fw_ver"):
			dev, err := lastDevice(devices, line)
			if err != nil {
				return nil, err
			}
			dev.FWVersion = value(line, "fw_ver")
		case hasKey(line, "node_guid"):
			dev, err := lastDevice(devices, line)
			if err != nil {
				return nil, err
			}
			dev.NodeGUID = value(line, "node_guid")
		case hasKey(line, "port"):
			dev, err := lastDevice(devices, line)
			if err != nil {
				return nil, err
			}
			num, err := strconv.Atoi(value(line, "port"))
			if err != nil {
				return nil, fmt.Errorf("parse port number %q: %w", value(line, "port"), err)
			}
			dev.Ports = append(dev.Ports, Port{Num: num})
		case hasKey(line, "state"):
			port, err := lastPort(devices)
			if err != nil {
				return nil, err
			}
			port.State = firstField(value(line, "state"))
		case hasKey(line, "link_layer"):
			port, err := lastPort(devices)
			if err != nil {
				return nil, err
			}
			port.LinkLayer = value(line, "link_layer")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan ibv_devinfo output: %w", err)
	}
	return devices, nil
}

// ParseIbstat parses `ibstat` output into devices.
func ParseIbstat(raw []byte) ([]Device, error) {
	var devices []Device
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "CA '"):
			name := strings.TrimSuffix(strings.TrimPrefix(line, "CA '"), "'")
			devices = append(devices, Device{Name: name})
		case hasKey(line, "Firmware version"):
			dev, err := lastDevice(devices, line)
			if err != nil {
				return nil, err
			}
			dev.FWVersion = value(line, "Firmware version")
		case strings.HasPrefix(line, "Port "):
			if len(devices) == 0 {
				return nil, fmt.Errorf("port field before any device")
			}
			dev := &devices[len(devices)-1]
			field := strings.TrimSuffix(strings.TrimPrefix(line, "Port "), ":")
			num, err := strconv.Atoi(strings.TrimSpace(field))
			if err != nil {
				return nil, fmt.Errorf("parse port number %q: %w", field, err)
			}
			dev.Ports = append(dev.Ports, Port{Num: num})
		case hasKey(line, "State"):
			port, err := lastPort(devices)
			if err != nil {
				return nil, err
			}
			port.State = value(line, "State")
		case hasKey(line, "Rate"):
			port, err := lastPort(devices)
			if err != nil {
				return nil, err
			}
			port.Rate = value(line, "Rate")
		case hasKey(line, "Link layer"):
			port, err := lastPort(devices)
			if err != nil {
				return nil, err
			}
			port.LinkLayer = value(line, "Link layer")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan ibstat output: %w", err)
	}
	return devices, nil
}

// Merge combines devinfo (primary) with ibstat, matching devices by Name and
// ports by Num. devinfo is authoritative; ibstat fills empty fields and
// contributes devices or ports that devinfo lacked.
func Merge(devinfo, ibstat []Device) []Device {
	merged := copyDevices(devinfo)
	index := make(map[string]int, len(merged))
	for i := range merged {
		index[merged[i].Name] = i
	}
	for _, extra := range ibstat {
		i, ok := index[extra.Name]
		if !ok {
			merged = append(merged, copyDevice(extra))
			index[extra.Name] = len(merged) - 1
			continue
		}
		mergeDevice(&merged[i], extra)
	}
	return merged
}

func mergeDevice(dst *Device, src Device) {
	if dst.FWVersion == "" {
		dst.FWVersion = src.FWVersion
	}
	if dst.NodeGUID == "" {
		dst.NodeGUID = src.NodeGUID
	}
	portIndex := make(map[int]int, len(dst.Ports))
	for i := range dst.Ports {
		portIndex[dst.Ports[i].Num] = i
	}
	for _, port := range src.Ports {
		i, ok := portIndex[port.Num]
		if !ok {
			dst.Ports = append(dst.Ports, port)
			portIndex[port.Num] = len(dst.Ports) - 1
			continue
		}
		mergePort(&dst.Ports[i], port)
	}
}

func mergePort(dst *Port, src Port) {
	if dst.State == "" {
		dst.State = src.State
	}
	if dst.Rate == "" {
		dst.Rate = src.Rate
	}
	if dst.LinkLayer == "" {
		dst.LinkLayer = src.LinkLayer
	}
}

func copyDevices(devices []Device) []Device {
	if devices == nil {
		return nil
	}
	out := make([]Device, len(devices))
	for i := range devices {
		out[i] = copyDevice(devices[i])
	}
	return out
}

func copyDevice(device Device) Device {
	device.Ports = append([]Port(nil), device.Ports...)
	return device
}

func lastDevice(devices []Device, line string) (*Device, error) {
	if len(devices) == 0 {
		return nil, fmt.Errorf("field %q before any device", line)
	}
	return &devices[len(devices)-1], nil
}

func lastPort(devices []Device) (*Port, error) {
	if len(devices) == 0 {
		return nil, fmt.Errorf("port field before any device")
	}
	dev := &devices[len(devices)-1]
	if len(dev.Ports) == 0 {
		return nil, fmt.Errorf("port field before any port")
	}
	return &dev.Ports[len(dev.Ports)-1], nil
}

func hasKey(line, key string) bool {
	return strings.HasPrefix(line, key+":")
}

func value(line, key string) string {
	return strings.TrimSpace(strings.TrimPrefix(line, key+":"))
}

func firstField(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
