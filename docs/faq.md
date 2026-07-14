# FAQ

## 没有 NVIDIA GPU 会怎样？

`detect`、`report`、`tune` 会尝试按配置选择 collector。若没有可用的 NVML 或 `nvidia-smi` 后端，
命令会输出包含 `no NVIDIA GPU detected` 的友好错误，并返回退出码 `1`。

这意味着：

- 项目可以在无 GPU 主机上构建和运行 `--help`、`version`、`config` 等命令。
- 请求真实 GPU 数据时需要 NVIDIA Driver 和可用后端。

## 缺少 benchmark 工具会怎样？

`gpu-tools bench` 依赖外部工具：

- `gpu-burn`
- `nvbandwidth`
- `bandwidthTest`

如果指定工具不存在，命令返回退出码 `2`，并输出：

```text
benchmark tool "<tool>" not installed, install it and retry
```

未知工具名会返回普通错误，例如 `unknown benchmark tool "<name>"`。

## 需要安装 NVIDIA Driver 吗？

构建不需要；真实 GPU 数据需要。

- NVML 后端需要主机能动态加载 NVIDIA Management Library。
- nvidia-smi 后端需要可执行的 `nvidia-smi`。
- 无 Driver 的主机仍可构建 `CGO_ENABLED=0` 产物，并运行不依赖 GPU 的命令。
- purego NVML 通过系统动态加载器在运行时 `dlopen` NVML，因此并非完全静态链接；纯 musl（Alpine）不支持 NVML 后端（可回退 `nvidia-smi`）。

## benchmark 需要 root 吗？

`gpu-tools` 本身不强制 root。但外部 benchmark 工具可能要求更高权限、特定驱动设置或独占 GPU。
如果工具本身因权限失败，`gpu-tools bench` 会返回该外部工具执行失败的错误。

## v1 支持 Docker 吗？

不支持 Docker 作为一等交付目标。v1 的主要交付形态是 GitHub Release 中的单一自包含二进制：无 cgo、构建无需 C 工具链、跨 glibc 发行版可移植，但并非完全静态链接。

如果你自行放入容器运行，仍需要把 NVIDIA Driver / runtime 能力正确暴露给容器；这不是 v1 的内置流程。

## 支持 AMD 或 Intel GPU 吗？

NVIDIA 是一等支持（purego NVML + `nvidia-smi`）。AMD 提供**尽力而为**的后端：显式
`--backend amd` 通过 `rocm-smi --json` 读取指标子集——索引、名称、GPU / 显存利用率、显存、
温度、功耗。它**不含**编码 / 解码利用率、PCIe 链路、时钟、功耗上限或按进程数据，这些字段
保持 `0` / 空（见下方「AMD 后端为什么只有部分字段」）。`auto` 不会自动选择 AMD，需显式指定。
Intel GPU 暂不支持。

## `detect --watch` 在无 GPU 主机上会怎样？

`gpu-tools detect --watch <duration>` 在进入刷新循环前会**急切读取一次**。如果后端永久不可用
（无 NVML、无 `nvidia-smi`），这次首读会立即以 `no NVIDIA GPU detected` 报错并返回退出码
`1`——**快速失败，绝不空转重试**。只有首读成功后才会按 tick 持续刷新，直到 Ctrl-C。

## exporter 在无 GPU 主机上会怎样？

`gpu-tools export --listen :9835` 是 headless 的 `/metrics` 端点，**始终返回 HTTP 200**。
在无 GPU 主机上它只输出 `gpu_tools_up 0` 且无任何设备序列（不报错、不刷日志），因此
Prometheus 目标不会仅因某节点没有 GPU 而抖动。裸 `/` 返回 `gpu-tools exporter`。真实读取
错误也归一化为 `up 0`（错误记录到 stderr，不向 promhttp 传播）。

## AMD 后端为什么只有部分字段？

AMD 后端是尽力而为的：`rocm-smi --json` 暴露的字段与 NVML 不对齐，且不同 rocm-smi 版本键名
各异。因此 AMD 后端只映射稳定可得的子集（索引、名称、利用率、显存、温度、功耗），其余 NVIDIA
专有 / NVML 专有字段（Enc/Dec、PCIe、时钟、功耗上限、按进程占用）在 AMD 下保持 `0` / 空，而非
猜测或伪造。

## 能用它杀掉占用 GPU 的进程吗？

不能。`gpu-tools` 是**只读**工具：它采集并展示按进程 GPU 占用（GPU Processes 区块 /
`gpu_process_used_memory_bytes` 指标），但从不 kill 进程、也不修改任何硬件或系统状态。进程
管理请用系统自带工具（`kill`、`nvidia-smi` 等）。这与 `tune` 的只读定位一致。

## 退出码契约是什么？

| 退出码 | 含义                                                                                                    |
| ------ | ------------------------------------------------------------------------------------------------------- |
| `0`    | 成功；`doctor` 默认即使发现问题也返回 `0`。                                                             |
| `1`    | 一般错误 / 解析失败（如 `bench` 缺 `--server`、旧工具 + 新参数校验失败、`doctor --strict` 遇到 FAIL）。 |
| `2`    | 外部工具缺失，或当前平台不支持（`topo`/`doctor`/`rdma`/`prereqs` 在非 Linux 上）。                      |

## 为什么 topo/rdma/doctor 在我的 Mac/Windows 上提示 "requires Linux"？

`gpu-tools topo`、`gpu-tools doctor`、`gpu-tools rdma`、`gpu-tools prereqs` 都依赖
Linux 专有的探测方式（`nvidia-smi topo -m`、`/proc`、`lspci`、`ibv_devinfo`、
`ibstat`），在 macOS / Windows 上没有等价实现。二进制本身仍然跨平台构建和发布，但这
四个命令在非 Linux 上会直接返回退出码 `2` 并提示 "requires Linux"（`-o json` 下还会
输出 `{"supported":false,...}` 对象），而不是尝试运行后报错。

## `gpu-tools doctor` 发现了问题但退出码是 0，这是 bug 吗？

不是 bug，是设计如此：`doctor` 是**只读诊断**，「发现问题」本身不算命令失败，所以
默认总是返回 `0`（前提是命令自身没有出错），方便脚本先看报告再决定怎么处理。如果
你在 CI 里需要「有 FAIL 就让流水线失败」，加上 `--strict`：只有当 `Overall` 是
`FAIL` 时才会返回退出码 `1`；单纯的 `WARN` 永远不会导致非零退出码。

## `bench --tool perftest` 为什么必须传 `--server`？

`perftest`（`ib_write_bw`）是一个客户端/服务端基准：本机作为客户端连接
`--server <ip>` 上已经在跑的 `ib_write_bw` 服务端。没有 `--server` 就无法发起测试，
所以 `gpu-tools bench --tool perftest` 缺少这个参数会在解析阶段直接返回错误、退出码
`1`。`--use-cuda <N>` 是可选的，用于测 GPUDirect RDMA（GDR）路径。

## `bench --tool nccl-tests` 支持多机吗？

当前只支持**单机多卡**（`all_reduce_perf`，`--gpus` 默认 `8`）。它不编排跨节点的
MPI/多主机启动，因此不接受 `--server` 之类的多机参数；`--nccl-debug` 会设置
`NCCL_DEBUG=INFO` 并从输出中检测 GDRDMA 使用情况。

## 为什么没有交互式 TUI？

`gpu-tools` 刻意保持面向脚本 / CI / agent 的非交互形态：一次性快照、`--watch` 的清屏重绘 /
NDJSON 流、以及 headless 的 Prometheus `/metrics`，都能被管道、日志和抓取任务直接消费。没有
全屏交互式 TUI（无按键导航 / 菜单），以保持输出确定性、可 grep、可被 agent 驱动。需要图形化
观测请把 exporter 接到 Grafana。
