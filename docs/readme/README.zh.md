# gpu-tools

> 纯 Go NVIDIA GPU 基础设施 CLI，覆盖检测、报告、调优建议和基准测试；单一自包含二进制、无 cgo、跨 glibc 发行版可移植。

[![Go 1.26+](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![License MIT](https://img.shields.io/badge/License-MIT-blue.svg)](../../LICENSE)
[![CI](https://github.com/sunerpy/gpu-tools/actions/workflows/ci.yml/badge.svg)](https://github.com/sunerpy/gpu-tools/actions/workflows/ci.yml)
[![Codecov](https://codecov.io/gh/sunerpy/gpu-tools/branch/main/graph/badge.svg)](https://codecov.io/gh/sunerpy/gpu-tools)

简体中文 · [English](../../README.md)

## 目录

- [功能](#功能)
- [安装](#安装)
- [快速开始](#快速开始)
- [后端](#后端)
- [运行要求](#运行要求)
- [LLM / Agent 使用](#llm--agent-使用)
- [配置](#配置)
- [开发](#开发)
- [许可证](#许可证)

## 功能

- **Detect**：通过 `gpu-tools detect` 采集当前 NVIDIA GPU 清单。
- **Report**：通过 `gpu-tools report` 输出 Markdown、表格或 JSON 快照。
- **Tune**：输出确定性的只读调优建议，不会修改硬件设置。
- **Bench**：运行受支持的外部基准工具，并归一化解析吞吐结果。

Collector 模型和包布局见[架构说明](../architecture.md)。

## 安装

### POSIX 一行安装

```bash
curl -fsSL https://raw.githubusercontent.com/sunerpy/gpu-tools/main/scripts/install.sh | sh
```

安装脚本会先校验 `checksums.txt`，再解压 Release 归档。可通过
`GPU_TOOLS_INSTALL_DIR` 或 `--dir` 覆盖安装目录。

### PowerShell

```powershell
irm https://raw.githubusercontent.com/sunerpy/gpu-tools/main/scripts/install.ps1 | iex
```

PowerShell 安装脚本默认写入 `$env:LOCALAPPDATA\Programs\gpu-tools`；可通过
`$env:GPU_TOOLS_INSTALL_DIR` 或 `-Dir` 覆盖。

### Go 工具链

```bash
go install github.com/sunerpy/gpu-tools@latest
```

### 预构建 Release

可从 [GitHub Releases](https://github.com/sunerpy/gpu-tools/releases) 下载
Linux、macOS 或 Windows 归档。发布产物使用 `CGO_ENABLED=0` 构建，覆盖
`amd64` 和 `arm64`。

## 快速开始

```bash
# 检测本机 NVIDIA GPU 清单。
gpu-tools detect

# 将一次性报告打印到 stdout。
gpu-tools report --out -

# 展示只读调优建议。
gpu-tools tune

# 运行外部基准工具并解析结果。
gpu-tools bench --tool gpu-burn
```

常用全局参数：

```bash
gpu-tools --output json detect
gpu-tools --output markdown tune
gpu-tools --backend nvidia-smi report --out -
gpu-tools --config ./config.yaml config show
```

## 后端

`gpu-tools` v1 仅支持 NVIDIA，并通过 `--backend auto|nvml|nvidia-smi` 选择采集器：

1. **purego NVML**（`nvml`）：主后端；无需 cgo，通过系统动态加载器在运行时 `dlopen` NVML。
2. **nvidia-smi**（`nvidia-smi`）：回退后端；调用 `nvidia-smi` 并解析 CSV。
3. **DCGM**：延期实现；v1 未包含。

`auto` 优先使用 NVML，再回退到 `nvidia-smi`。如果没有可用后端，命令会以
`no NVIDIA GPU detected` 友好报错并返回退出码 `1`。无 GPU 和基准测试退出行为见
[FAQ](../faq.md)。

## 运行要求

- 只有从源码构建时才需要 Go 工具链。
- 真实 GPU 数据需要主机安装 NVIDIA Driver，并可访问 NVML 或 `nvidia-smi`。
- 二进制本身是纯 Go，并以 `CGO_ENABLED=0` 构建；构建无需 C 工具链，在无 NVIDIA GPU 的主机上也能启动。
- purego NVML 后端通过系统动态加载器在运行时 `dlopen` NVML，因此并非完全静态链接；真实 GPU 数据仍需要系统加载器和 NVIDIA Driver，纯 musl（Alpine）不支持 NVML 后端（可回退 `nvidia-smi`）。
- 基准测试依赖外部工具（`gpu-burn`、`nvbandwidth` 或 `bandwidthTest`）；部分工具可能需要更高权限。

## LLM / Agent 使用

先按[安装](#安装)章节安装，再用以下精简命令契约驱动 CLI。

<details>
<summary>Agent 命令参考</summary>

- `gpu-tools version`：输出构建和版本元数据。
- `gpu-tools config init`：写入 `~/.gpu-tools/config.yaml`；加 `--force` 可覆盖。
- `gpu-tools config show`：在应用全局参数覆盖后输出 YAML。
- `gpu-tools completion bash|zsh|fish|powershell`：生成 shell completion。
- `gpu-tools detect --output json`：在 stdout 输出 GPU 清单快照。
- `gpu-tools report --out - --output markdown`：在 stdout 输出 Markdown 报告。
- `gpu-tools tune --output json`：输出只读建议。
- `gpu-tools bench --tool gpu-burn --duration 60s --output json`：运行受支持的外部基准工具。

全局参数：`--output/-o table|json|markdown`、`--backend auto|nvml|nvidia-smi`、
`--config <path>`。

退出契约：诊断信息写入 stderr，命令输出写入 stdout；无 GPU 后端返回 `1`，缺少基准工具返回 `2`。

</details>

## 配置

默认配置文件位于 `~/.gpu-tools/config.yaml`，支持：

- `default_output`：`table`、`json` 或 `markdown`
- `backend`：`auto`、`nvml` 或 `nvidia-smi`
- `report_dir`：报告文件默认目录
- `nvidia_smi_path`：可选的 `nvidia-smi` 二进制路径覆盖

字段、参数覆盖、后端选择和报告输出规则见[配置说明](../configuration.md)。

## 开发

常用贡献者命令：

```bash
make build
make test
make coverage-gate
make fmt
make lint
```

安装本地 hooks：

```bash
pre-commit install --hook-type pre-commit --hook-type pre-push
```

覆盖率门禁、格式化工具、Conventional Commits 和 release-please 流程见[开发说明](../development.md)。

## 许可证

[MIT](../../LICENSE)
