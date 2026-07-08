# gpu-tools

> Pure-Go NVIDIA GPU infrastructure CLI for detect, report, tuning advice, and
> benchmark workflows — single self-contained binary, no cgo, portable across glibc distributions.

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

Common global flags:

```bash
gpu-tools --output json detect
gpu-tools --output markdown tune
gpu-tools --backend nvidia-smi report --out -
gpu-tools --config ./config.yaml config show
```

## Backends

`gpu-tools` is NVIDIA-only in v1 and selects collectors through
`--backend auto|nvml|nvidia-smi`:

1. **purego NVML** (`nvml`) — primary backend; loads NVML dynamically without cgo.
2. **nvidia-smi** (`nvidia-smi`) — fallback backend; shells out to `nvidia-smi` and parses CSV.
3. **DCGM** — deferred; not implemented in v1.

`auto` prefers NVML, then falls back to `nvidia-smi`. If neither backend is
available, commands fail gracefully with `no NVIDIA GPU detected` and exit code
`1`. See [FAQ](docs/faq.md) for no-GPU and benchmark exit behavior.

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
- `gpu-tools report --out - --output markdown` — emit a Markdown report on stdout.
- `gpu-tools tune --output json` — emit read-only advisory recommendations.
- `gpu-tools bench --tool gpu-burn --duration 60s --output json` — run a supported external benchmark.

Global flags: `--output/-o table|json|markdown`, `--backend auto|nvml|nvidia-smi`,
and `--config <path>`.

Exit contract: diagnostics go to stderr, command output goes to stdout, no-GPU
backend selection exits `1`, and missing benchmark tools exit `2`.

</details>

## Configuration

Configuration lives at `~/.gpu-tools/config.yaml` by default and supports:

- `default_output`: `table`, `json`, or `markdown`
- `backend`: `auto`, `nvml`, or `nvidia-smi`
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
