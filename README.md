# gpu-tools

> A pure-Go, no-cgo GPU infrastructure **diagnostics + tuning-advice + benchmark + monitoring**
> CLI — single self-contained binary, portable across glibc distributions. More than a
> monitor: it detects inventory, renders reports, prints read-only tuning advice, runs
> benchmarks, watches live, and serves Prometheus metrics.

[![Go 1.26+](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![License MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![CI](https://github.com/sunerpy/gpu-tools/actions/workflows/ci.yml/badge.svg)](https://github.com/sunerpy/gpu-tools/actions/workflows/ci.yml)
[![Codecov](https://codecov.io/gh/sunerpy/gpu-tools/branch/main/graph/badge.svg)](https://codecov.io/gh/sunerpy/gpu-tools)

[简体中文](docs/readme/README.zh.md) · English

## Table of Contents

- [Features](#features)
- [Install](#install)
- [Quickstart](#quickstart)
- [Backends](#backends)
- [Exporter](#exporter)
- [Requirements](#requirements)
- [Using with an LLM or agent](#using-with-an-llm-or-agent)
- [Configuration](#configuration)
- [Development](#development)
- [License](#license)

## Features

- **Detect** — collect a point-in-time NVIDIA GPU inventory with `gpu-tools detect`.
- **Report** — render Markdown, table, or JSON snapshots with `gpu-tools report`.
- **Tune** — print deterministic, read-only tuning recommendations; it never mutates hardware settings.
- **Bench** — run supported external benchmark tools and normalize their parsed throughput.
- **Watch** — refresh live with `gpu-tools detect --watch 2s`: the screen re-renders each
  tick until Ctrl-C, or `-o json --watch` streams NDJSON (one compact object per line).
- **Per-process GPU usage** — `detect` and `report` add a **GPU Processes** section
  (GPU, PID, Type, Process, User, Mem) when compute processes are present.
- **Richer device metrics** — encoder/decoder utilization (`Enc/Dec %`) and PCIe link
  (`genXxwN`) alongside utilization, memory, temperature, power, and clocks.
- **Prometheus exporter** — `gpu-tools export --listen :9835` serves a headless
  `/metrics` endpoint for scraping and Grafana dashboards.
- **AMD backend (best-effort)** — `--backend amd` reads a subset of metrics via
  `rocm-smi`; `auto` still prefers NVIDIA.

See [Architecture](docs/architecture.md) for the collector model and package layout.

## Install

### POSIX one-liner

```bash
curl -fsSL https://raw.githubusercontent.com/sunerpy/gpu-tools/main/scripts/install.sh | sh
```

The installer verifies `checksums.txt` before extracting the release archive. Override
the destination with `GPU_TOOLS_INSTALL_DIR` or `--dir`.

### PowerShell

```powershell
irm https://raw.githubusercontent.com/sunerpy/gpu-tools/main/scripts/install.ps1 | iex
```

The PowerShell installer defaults to
`$env:LOCALAPPDATA\Programs\gpu-tools`; override with
`$env:GPU_TOOLS_INSTALL_DIR` or `-Dir`.

### Go toolchain

```bash
go install github.com/sunerpy/gpu-tools@latest
```

### Prebuilt releases

Download Linux, macOS, or Windows archives from
[GitHub Releases](https://github.com/sunerpy/gpu-tools/releases). Release
artifacts are built with `CGO_ENABLED=0` for `amd64` and `arm64`.

## Quickstart

```bash
# Detect local NVIDIA GPU inventory.
gpu-tools detect

# Print a point-in-time report to stdout.
gpu-tools report --out -

# Show read-only tuning recommendations.
gpu-tools tune

# Run an external benchmark tool and parse the result.
gpu-tools bench --tool gpu-burn
```

Live watch, per-process usage, exporter, and AMD:

```bash
# Refresh the inventory table every 2 seconds until Ctrl-C.
gpu-tools detect --watch 2s

# Stream one compact NDJSON object per tick (machine-readable watch).
gpu-tools --output json detect --watch 2s

# detect/report show a "GPU Processes" section when compute processes exist:
#
#   GPU Processes
#   GPU  PID   Type     Process  User   Mem
#   0    1234  compute  python   alice  512 MiB

# Serve GPU metrics for Prometheus on http://<host>:9835/metrics.
gpu-tools export --listen :9835

# Read AMD GPUs (best-effort subset) via rocm-smi.
gpu-tools --backend amd detect
```

Common global flags:

```bash
gpu-tools --output json detect
gpu-tools --output markdown tune
gpu-tools --backend nvidia-smi report --out -
gpu-tools --config ./config.yaml config show
```

## Backends

`gpu-tools` selects collectors through `--backend auto|nvml|nvidia-smi|amd`:

1. **purego NVML** (`nvml`) — primary NVIDIA backend; loads NVML dynamically without cgo.
2. **nvidia-smi** (`nvidia-smi`) — NVIDIA fallback backend; shells out to `nvidia-smi` and parses CSV.
3. **rocm-smi** (`amd`) — best-effort AMD backend; parses `rocm-smi --json` for a subset of
   metrics (index, name, utilization, memory, temperature, power).
4. **DCGM** — deferred; not implemented in v1.

`auto` prefers NVML, then falls back to `nvidia-smi` (it does **not** auto-select AMD — pass
`--backend amd` explicitly). If no requested backend is available, commands fail gracefully
with `no NVIDIA GPU detected` and exit code `1`. See [FAQ](docs/faq.md) for no-GPU, watch,
exporter, and benchmark exit behavior.

## Exporter

`gpu-tools export --listen :9835` runs a headless Prometheus exporter. Scrape `/metrics`;
a bare `/` returns `gpu-tools exporter`. The endpoint always answers HTTP 200 — on a host
with no GPU it emits `gpu_tools_up 0` and no device series, so a Prometheus target never
flaps just because a node lacks a GPU.

Exposed metrics:

| Metric                            | Labels                        | Meaning                          |
| --------------------------------- | ----------------------------- | -------------------------------- |
| `gpu_tools_up`                    | —                             | 1 if backend available + read ok |
| `gpu_utilization_percent`         | `index,uuid,name`             | GPU utilization %                |
| `gpu_memory_used_bytes`           | `index,uuid,name`             | Used memory (bytes)              |
| `gpu_memory_total_bytes`          | `index,uuid,name`             | Total memory (bytes)             |
| `gpu_temperature_celsius`         | `index,uuid,name`             | Temperature (°C)                 |
| `gpu_power_draw_watts`            | `index,uuid,name`             | Power draw (W)                   |
| `gpu_power_limit_watts`           | `index,uuid,name`             | Power limit (W)                  |
| `gpu_clock_graphics_mhz`          | `index,uuid,name`             | Graphics clock (MHz)             |
| `gpu_clock_mem_mhz`               | `index,uuid,name`             | Memory clock (MHz)               |
| `gpu_encoder_utilization_percent` | `index,uuid,name`             | Encoder utilization %            |
| `gpu_decoder_utilization_percent` | `index,uuid,name`             | Decoder utilization %            |
| `gpu_process_used_memory_bytes`   | `index,pid,process_name,type` | Per-process GPU memory (bytes)   |

> [!NOTE]
> All per-GPU/per-process series use the bare `gpu_` prefix; only `gpu_tools_up` carries the
> `gpu_tools` namespace.

Grafana: point a Prometheus scrape job at `<host>:9835`, then chart `gpu_utilization_percent`
and `gpu_memory_used_bytes` by the `index`/`name` labels; use `gpu_tools_up` for target
health and `gpu_process_used_memory_bytes` for per-process breakdowns.

## Requirements

- A recent Go toolchain is required only when building from source.
- Runtime GPU data requires an installed NVIDIA driver and either NVML or
  `nvidia-smi` on the host.
- The binary itself is pure Go and built with `CGO_ENABLED=0`; no C toolchain is
  needed to build it, and it can start on hosts without NVIDIA GPUs.
- The purego NVML backend loads NVML via the system dynamic loader at runtime,
  so the binary is not fully static and requires the system loader plus an
  NVIDIA driver for real GPU data.
- Benchmarks use external tools (`gpu-burn`, `nvbandwidth`, or `bandwidthTest`);
  some tools may require elevated privileges depending on the environment.

## Using with an LLM or agent

Install via the [Install](#install) section, then drive the CLI with this compact
command contract.

<details>
<summary>Agent command reference</summary>

- `gpu-tools version` — print build/version metadata.
- `gpu-tools config init` — write `~/.gpu-tools/config.yaml`; add `--force` to overwrite.
- `gpu-tools config show` — print resolved YAML after global flag overrides.
- `gpu-tools completion bash|zsh|fish|powershell` — generate shell completions.
- `gpu-tools detect --output json` — emit an inventory snapshot on stdout.
- `gpu-tools detect --watch 2s` — refresh the table each tick; with `-o json` streams NDJSON.
- `gpu-tools report --out - --output markdown` — emit a Markdown report on stdout.
- `gpu-tools tune --output json` — emit read-only advisory recommendations.
- `gpu-tools bench --tool gpu-burn --duration 60s --output json` — run a supported external benchmark.
- `gpu-tools export --listen :9835` — serve a headless Prometheus `/metrics` endpoint.

Global flags: `--output/-o table|json|markdown`, `--backend auto|nvml|nvidia-smi|amd`,
and `--config <path>`.

Exit contract: diagnostics go to stderr, command output goes to stdout, no-GPU
backend selection exits `1` (including `detect --watch` on a GPU-less host, which
fails fast rather than spinning), and missing benchmark tools exit `2`.

</details>

## Configuration

Configuration lives at `~/.gpu-tools/config.yaml` by default and supports:

- `default_output`: `table`, `json`, or `markdown`
- `backend`: `auto`, `nvml`, `nvidia-smi`, or `amd`
- `report_dir`: default directory for report files
- `nvidia_smi_path`: optional `nvidia-smi` binary override

See [Configuration](docs/configuration.md) for field details, flag overrides,
backend selection, and report output rules.

## Development

Common contributor commands:

```bash
make build
make test
make coverage-gate
make fmt
make lint
```

Install local hooks with:

```bash
pre-commit install --hook-type pre-commit --hook-type pre-push
```

See [Development](docs/development.md) for the coverage gate, formatting tools,
Conventional Commits, and release-please flow.

## License

[MIT](LICENSE)
