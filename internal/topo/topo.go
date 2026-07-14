package topo

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// LinkType classifies the interconnect between two endpoints in the topology
// matrix produced by `nvidia-smi topo -m`.
type LinkType string

const (
	LinkNVLink LinkType = "NVLINK"
	LinkPIX    LinkType = "PIX"
	LinkPXB    LinkType = "PXB"
	LinkPHB    LinkType = "PHB"
	LinkNODE   LinkType = "NODE"
	LinkSYS    LinkType = "SYS"
	LinkSelf   LinkType = "SELF"
	LinkX      LinkType = "X"
)

// Cell is a single entry of the connectivity matrix. Lanes is populated only
// for NVLINK links (e.g. NV12 -> Lanes 12); it is 0 for every other link type.
type Cell struct {
	Type  LinkType
	Lanes int
}

// NICAffinity captures how a single NIC connects to every GPU on the host.
type NICAffinity struct {
	NIC    string
	PerGPU map[string]Cell
}

// Matrix is the structured form of the topology matrix: the GPU-to-GPU cell
// grid plus per-NIC GPU affinity rows.
type Matrix struct {
	GPUs  []string
	Cells map[string]map[string]Cell
	NICs  []NICAffinity
}

// Rating summarizes how favorable a GPU-NIC link is.
type Rating string

const (
	RatingGood Rating = "good" // PIX / PXB
	RatingWarn Rating = "warn" // PHB / NODE
	RatingBad  Rating = "bad"  // SYS
)

// Advice is a single GPU-NIC affinity assessment.
type Advice struct {
	GPU    string
	NIC    string
	Link   LinkType
	Rating Rating
}

// Result bundles the parsed matrix with the derived affinity advice.
type Result struct {
	Matrix Matrix
	Advice []Advice
}

var (
	nvlinkPattern = regexp.MustCompile(`^NV(\d+)$`)
	gpuLabel      = regexp.MustCompile(`^GPU\d+$`)
	nicLabel      = regexp.MustCompile(`^NIC\d+$`)
)

// Collect resolves nvidia-smi, runs `nvidia-smi topo -m`, and parses the
// result into a structured Matrix plus affinity advice. When smiPath is empty
// it resolves nvidia-smi via PATH; a failed lookup wraps ErrToolNotInstalled.
func Collect(ctx context.Context, runner execRunner, smiPath string) (*Result, error) {
	if runner == nil {
		runner = defaultRunner
	}
	if smiPath == "" {
		resolved, err := lookPath("nvidia-smi")
		if err != nil {
			return nil, fmt.Errorf("%w: nvidia-smi", ErrToolNotInstalled)
		}
		smiPath = resolved
	}
	out, err := runner.Run(ctx, smiPath, "topo", "-m")
	if err != nil {
		return nil, fmt.Errorf("run nvidia-smi topo: %w", err)
	}
	matrix, err := Parse(out)
	if err != nil {
		return nil, err
	}
	return &Result{Matrix: *matrix, Advice: Rate(matrix)}, nil
}

type deviceColumn struct {
	label string
	index int
	isGPU bool
}

// Parse converts raw `nvidia-smi topo -m` output into a Matrix. Columns are
// located by their header labels (GPU#/NIC#), not by fixed widths; the
// "CPU Affinity"/"NUMA Affinity" columns and any Legend block are ignored. A
// missing or unparseable matrix returns an error rather than a partial result.
func Parse(raw []byte) (*Matrix, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil, fmt.Errorf("parse topo: empty input")
	}
	lines := strings.Split(text, "\n")

	headerIndex := firstNonEmptyLine(lines)
	if headerIndex < 0 {
		return nil, fmt.Errorf("parse topo: empty input")
	}

	columns := deviceColumns(strings.Fields(lines[headerIndex]))
	if len(columns) == 0 {
		return nil, fmt.Errorf("parse topo: no GPU/NIC columns in header %q", lines[headerIndex])
	}

	matrix := &Matrix{
		GPUs:  gpuLabelsOf(columns),
		Cells: map[string]map[string]Cell{},
	}

	rows := 0
	for _, line := range lines[headerIndex+1:] {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		rowLabel := fields[0]
		isGPURow := gpuLabel.MatchString(rowLabel)
		isNICRow := nicLabel.MatchString(rowLabel)
		if !isGPURow && !isNICRow {
			continue
		}
		rows++

		perGPU, err := rowCells(rowLabel, fields, columns)
		if err != nil {
			return nil, err
		}
		if isGPURow {
			matrix.Cells[rowLabel] = gpuOnly(perGPU, columns)
		} else {
			matrix.NICs = append(matrix.NICs, NICAffinity{NIC: rowLabel, PerGPU: perGPU})
		}
	}
	if rows == 0 {
		return nil, fmt.Errorf("parse topo: no data rows found")
	}
	return matrix, nil
}

// rowCells parses every GPU-column cell of a data row into a label->Cell map.
func rowCells(rowLabel string, fields []string, columns []deviceColumn) (map[string]Cell, error) {
	perGPU := make(map[string]Cell)
	for _, col := range columns {
		if !col.isGPU {
			continue
		}
		cellIndex := col.index + 1 // +1 for the leading row label.
		if cellIndex >= len(fields) {
			return nil, fmt.Errorf("parse topo: row %q missing cell for column %q", rowLabel, col.label)
		}
		cell, err := parseCell(fields[cellIndex])
		if err != nil {
			return nil, fmt.Errorf("parse topo: row %q column %q: %w", rowLabel, col.label, err)
		}
		perGPU[col.label] = cell
	}
	return perGPU, nil
}

// gpuOnly narrows a per-GPU cell map to exactly the GPU columns (it is already
// GPU-only, but this keeps the intent explicit for GPU rows).
func gpuOnly(perGPU map[string]Cell, columns []deviceColumn) map[string]Cell {
	out := make(map[string]Cell, len(perGPU))
	for _, col := range columns {
		if !col.isGPU {
			continue
		}
		if cell, ok := perGPU[col.label]; ok {
			out[col.label] = cell
		}
	}
	return out
}

func firstNonEmptyLine(lines []string) int {
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			return i
		}
	}
	return -1
}

func deviceColumns(headerFields []string) []deviceColumn {
	var columns []deviceColumn
	for i, field := range headerFields {
		switch {
		case gpuLabel.MatchString(field):
			columns = append(columns, deviceColumn{label: field, index: i, isGPU: true})
		case nicLabel.MatchString(field):
			columns = append(columns, deviceColumn{label: field, index: i, isGPU: false})
		}
	}
	return columns
}

func gpuLabelsOf(columns []deviceColumn) []string {
	var gpus []string
	for _, col := range columns {
		if col.isGPU {
			gpus = append(gpus, col.label)
		}
	}
	return gpus
}

func parseCell(token string) (Cell, error) {
	token = strings.TrimSpace(token)
	switch LinkType(token) {
	case LinkX:
		return Cell{Type: LinkSelf}, nil
	case LinkPIX:
		return Cell{Type: LinkPIX}, nil
	case LinkPXB:
		return Cell{Type: LinkPXB}, nil
	case LinkPHB:
		return Cell{Type: LinkPHB}, nil
	case LinkNODE:
		return Cell{Type: LinkNODE}, nil
	case LinkSYS:
		return Cell{Type: LinkSYS}, nil
	}
	if match := nvlinkPattern.FindStringSubmatch(token); match != nil {
		lanes, err := strconv.Atoi(match[1])
		if err != nil {
			return Cell{}, fmt.Errorf("nvlink lane count %q: %w", token, err)
		}
		return Cell{Type: LinkNVLink, Lanes: lanes}, nil
	}
	return Cell{}, fmt.Errorf("unknown link type %q", token)
}

// Rate assesses every GPU-NIC pair, skipping self (X/SELF) and NVLink links.
// PIX/PXB rate good, PHB/NODE rate warn, and SYS rates bad. Results are ordered
// by NIC then GPU for determinism.
func Rate(m *Matrix) []Advice {
	if m == nil {
		return nil
	}
	var advice []Advice
	for _, nic := range m.NICs {
		for _, gpu := range m.GPUs {
			cell, ok := nic.PerGPU[gpu]
			if !ok {
				continue
			}
			rating, ok := ratingFor(cell.Type)
			if !ok {
				continue
			}
			advice = append(advice, Advice{
				GPU:    gpu,
				NIC:    nic.NIC,
				Link:   cell.Type,
				Rating: rating,
			})
		}
	}
	return advice
}

func ratingFor(link LinkType) (Rating, bool) {
	switch link {
	case LinkPIX, LinkPXB:
		return RatingGood, true
	case LinkPHB, LinkNODE:
		return RatingWarn, true
	case LinkSYS:
		return RatingBad, true
	default:
		return "", false
	}
}
