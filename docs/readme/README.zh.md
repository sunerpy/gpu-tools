# gpu-tools

> 纯 Go、无 cgo 的 GPU 基础设施 **诊断 + 调优建议 + 基准测试 + 监控** CLI——单一自包含二进制、跨 glibc 发行版可移植。不只是监控器：检测清单、生成报告、输出只读调优建议、运行基准、实时刷新，并对外暴露 Prometheus 指标。

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
- [Exporter](#exporter)
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
- **Watch**：`gpu-tools detect --watch 2s` 实时刷新，每个 tick 重绘表格直到 Ctrl-C；
  搭配 `-o json --watch` 则流式输出 NDJSON（每行一个紧凑 JSON 对象）。
- **按进程 GPU 占用**：当存在计算进程时，`detect` 和 `report` 会追加一段 **GPU Processes**
  区块（GPU、PID、Type、Process、User、Mem）。
- **更丰富的设备指标**：在利用率、显存、温度、功耗、时钟之外，新增编码/解码利用率
  （`Enc/Dec %`）和 PCIe 链路（`genXxwN`）。
- **Prometheus exporter**：`gpu-tools export --listen :9835` 暴露一个无界面的 `/metrics`
  端点，供抓取和 Grafana 仪表盘使用。
- **AMD 后端（尽力而为）**：`--backend amd` 通过 `rocm-smi` 读取指标子集；`auto` 仍优先 NVIDIA。

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

实时刷新、按进程占用、exporter 与 AMD：

```bash
# 每 2 秒刷新一次清单表格，直到 Ctrl-C。
gpu-tools detect --watch 2s

# 每个 tick 流式输出一个紧凑 NDJSON 对象（机器可读的 watch）。
gpu-tools --output json detect --watch 2s

# 存在计算进程时，detect/report 会展示 "GPU Processes" 区块：
#
#   GPU Processes
#   GPU  PID   Type     Process  User   Mem
#   0    1234  compute  python   alice  512 MiB

# 在 http://<host>:9835/metrics 暴露 Prometheus 指标。
gpu-tools export --listen :9835

# 通过 rocm-smi 读取 AMD GPU（尽力而为的指标子集）。
gpu-tools --backend amd detect
```

常用全局参数：

```bash
gpu-tools --output json detect
gpu-tools --output markdown tune
gpu-tools --backend nvidia-smi report --out -
gpu-tools --config ./config.yaml config show
```

## 后端

`gpu-tools` 通过 `--backend auto|nvml|nvidia-smi|amd` 选择采集器：

1. **purego NVML**（`nvml`）：NVIDIA 主后端；无需 cgo，通过系统动态加载器在运行时 `dlopen` NVML。
2. **nvidia-smi**（`nvidia-smi`）：NVIDIA 回退后端；调用 `nvidia-smi` 并解析 CSV。
3. **rocm-smi**（`amd`）：尽力而为的 AMD 后端；解析 `rocm-smi --json`，仅覆盖指标子集
   （索引、名称、利用率、显存、温度、功耗）。
4. **DCGM**：延期实现；v1 未包含。

`auto` 优先使用 NVML，再回退到 `nvidia-smi`（**不会**自动选择 AMD——需显式传
`--backend amd`）。如果所请求的后端不可用，命令会以 `no NVIDIA GPU detected` 友好报错并
返回退出码 `1`。无 GPU、watch、exporter 和基准测试退出行为见 [FAQ](../faq.md)。

## Exporter

`gpu-tools export --listen :9835` 运行一个无界面的 Prometheus exporter。抓取 `/metrics`；
裸 `/` 返回 `gpu-tools exporter`。该端点始终返回 HTTP 200——在无 GPU 主机上会输出
`gpu_tools_up 0` 且无任何设备序列，因此 Prometheus 目标不会仅因某节点没有 GPU 而抖动。

暴露的指标：

| 指标                              | 标签                          | 含义                        |
| --------------------------------- | ----------------------------- | --------------------------- |
| `gpu_tools_up`                    | —                             | 后端可用且读取成功时为 1    |
| `gpu_utilization_percent`         | `index,uuid,name`             | GPU 利用率 %                |
| `gpu_memory_used_bytes`           | `index,uuid,name`             | 已用显存（字节）            |
| `gpu_memory_total_bytes`          | `index,uuid,name`             | 总显存（字节）              |
| `gpu_temperature_celsius`         | `index,uuid,name`             | 温度（°C）                  |
| `gpu_power_draw_watts`            | `index,uuid,name`             | 功耗（W）                   |
| `gpu_power_limit_watts`           | `index,uuid,name`             | 功耗上限（W）               |
| `gpu_clock_graphics_mhz`          | `index,uuid,name`             | 图形时钟（MHz）             |
| `gpu_clock_mem_mhz`               | `index,uuid,name`             | 显存时钟（MHz）             |
| `gpu_encoder_utilization_percent` | `index,uuid,name`             | 编码器利用率 %              |
| `gpu_decoder_utilization_percent` | `index,uuid,name`             | 解码器利用率 %              |
| `gpu_process_used_memory_bytes`   | `index,pid,process_name,type` | 单进程 GPU 显存占用（字节） |

> [!NOTE]
> 所有 per-GPU / per-process 序列均使用裸 `gpu_` 前缀；只有 `gpu_tools_up` 带 `gpu_tools`
> 命名空间。

Grafana：将 Prometheus 抓取任务指向 `<host>:9835`，再按 `index`/`name` 标签绘制
`gpu_utilization_percent` 和 `gpu_memory_used_bytes`；用 `gpu_tools_up` 表示目标健康度，
用 `gpu_process_used_memory_bytes` 展示按进程的占用拆解。

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
- `gpu-tools detect --watch 2s`：每个 tick 刷新表格；搭配 `-o json` 则流式输出 NDJSON。
- `gpu-tools report --out - --output markdown`：在 stdout 输出 Markdown 报告。
- `gpu-tools tune --output json`：输出只读建议。
- `gpu-tools bench --tool gpu-burn --duration 60s --output json`：运行受支持的外部基准工具。
- `gpu-tools export --listen :9835`：暴露一个无界面的 Prometheus `/metrics` 端点。

全局参数：`--output/-o table|json|markdown`、`--backend auto|nvml|nvidia-smi|amd`、
`--config <path>`。

退出契约：诊断信息写入 stderr，命令输出写入 stdout；无 GPU 后端返回 `1`（包括
`detect --watch` 在无 GPU 主机上会快速失败而非空转），缺少基准工具返回 `2`。

</details>

## 配置

默认配置文件位于 `~/.gpu-tools/config.yaml`，支持：

- `default_output`：`table`、`json` 或 `markdown`
- `backend`：`auto`、`nvml`、`nvidia-smi` 或 `amd`
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
