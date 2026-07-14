# 配置说明

`gpu-tools` 默认读取 `~/.gpu-tools/config.yaml`。如果文件不存在，程序使用内置默认值：

```yaml
default_output: table
backend: auto
report_dir: .
nvidia_smi_path: ""
```

## 字段

| 字段              | 允许值 / 示例                          | 说明                                                 |
| ----------------- | -------------------------------------- | ---------------------------------------------------- |
| `default_output`  | `table`、`json`、`markdown`            | 默认输出格式；可被全局 `--output/-o` 覆盖。          |
| `backend`         | `auto`、`nvml`、`nvidia-smi`、`amd`    | GPU 采集后端；可被全局 `--backend` 覆盖。            |
| `report_dir`      | `.`、`./reports`、`/var/log/gpu-tools` | `gpu-tools report` 未指定 `--out` 时写入报告的目录。 |
| `nvidia_smi_path` | `/usr/bin/nvidia-smi`                  | 可选的 `nvidia-smi` 路径覆盖；用于 nvidia-smi 后端。 |

## 命令和参数

初始化默认配置：

```bash
gpu-tools config init
gpu-tools config init --force
```

查看解析后的配置：

```bash
gpu-tools config show
gpu-tools --output json --backend nvml config show
gpu-tools --config ./config.yaml config show
```

全局参数：

- `--output/-o table|json|markdown`：覆盖 `default_output`。
- `--backend auto|nvml|nvidia-smi|amd`：覆盖 `backend`。
- `--config <path>`：指定要加载的 YAML 配置文件路径。

报告参数：

- `gpu-tools report --out <path>`：写入指定文件。
- `gpu-tools report --out -`：写入 stdout。
- 未指定 `--out` 时，生成 `gpu-report-YYYYMMDD-HHMMSS.md` 并写入 `report_dir`。

## 实时刷新（`detect --watch`）

`gpu-tools detect` 支持 `--watch <duration>` 实时刷新模式（`0` 为默认，即关闭 watch）：

```bash
# 每 2 秒清屏并重绘一次清单表格，直到 Ctrl-C。
gpu-tools detect --watch 2s

# 搭配 -o json：每个 tick 流式输出一行紧凑 NDJSON 对象（不清屏）。
gpu-tools --output json detect --watch 2s
```

- 表格 / markdown 输出会在每帧前清屏并整表重绘；JSON 输出则每个 tick 追加一行 NDJSON。
- watch 会**急切读取一次**：在无 GPU 主机上直接以退出码 `1` 快速失败，绝不空转重试。
- watch 模式下由 TTL 缓存（钳制到至多 1 秒）驱动，保证刷新顺滑且数据足够新。

## Prometheus exporter（`export --listen`）

`gpu-tools export` 运行一个无界面的 Prometheus exporter：

```bash
# 在 :9835 暴露 /metrics（默认地址即 :9835）。
gpu-tools export --listen :9835

# 仅监听本地回环的自定义端口。
gpu-tools export --listen 127.0.0.1:19835
```

- `--listen <addr>`：`/metrics` 服务监听地址，默认 `:9835`。
- 抓取 `/metrics` 获取指标；裸 `/` 返回 `gpu-tools exporter`。
- 无 GPU 主机上返回 `gpu_tools_up 0` 且无设备序列，HTTP 仍为 200。
- 收到 SIGINT / SIGTERM 后优雅关闭（5s 宽限）。
- 指标名与标签详见 [README 的 Exporter 一节](readme/README.zh.md#exporter)。

## AMD 后端（`--backend amd`）

```bash
gpu-tools --backend amd detect
gpu-tools --backend amd report --out -
```

- 通过 `rocm-smi --json` 采集，尽力而为，仅覆盖指标子集：索引、名称、利用率、显存、温度、功耗。
- 不含编码 / 解码利用率、PCIe 链路、时钟、功耗上限或按进程数据。
- 找不到 `rocm-smi` 时以 `no NVIDIA GPU detected` 报错并返回退出码 `1`（后端不可用的统一路径）。
- `auto` **不会**自动选择 AMD（NVML `10` → nvidia-smi `20` → amd `30`），需显式 `--backend amd`。

## 新命令与配置的关系（`topo` / `doctor` / `rdma` / `prereqs` / `bench`）

这些命令没有引入任何新配置字段：

- `gpu-tools topo` 复用现有的 `nvidia_smi_path`（与 nvidia-smi 后端共用同一路径覆盖）。
- `topo`、`doctor`、`rdma`、`prereqs`、`bench` 都遵循全局的 `--output/-o` 和 `--backend`
  参数（`--backend` 目前只影响 GPU Collector 相关命令，这些新命令本身不选择 Collector
  后端）。
- `doctor`、`rdma`、`prereqs` 不读取 `nvidia_smi_path`（它们不调用 nvidia-smi）。

## 后端选择

`backend: auto` 按注册优先级选择后端：

1. `nvml`：purego NVML 主后端，优先级 `10`。
2. `nvidia-smi`：CSV 解析回退后端，优先级 `20`。
3. `amd`：尽力而为的 rocm-smi 后端，优先级 `30`（`auto` 不会自动选它）。

如果显式设置 `--backend nvml`、`--backend nvidia-smi` 或 `--backend amd`，程序只尝试指定后端。
未知后端会返回 `unknown gpu backend "<name>"`。

> [!NOTE]
> 当前 nvidia-smi 后端的二进制路径解析会读取默认配置位置
> `~/.gpu-tools/config.yaml` 中的 `nvidia_smi_path`；常规根命令配置仍可通过
> `--config <path>` 指定。

## Driver 要求

真实 GPU 数据依赖主机 NVIDIA Driver：

- NVML 后端需要可动态加载的 NVML 库。
- nvidia-smi 后端需要可执行的 `nvidia-smi`。
- 无可用后端时，`detect`、`report`、`tune` 会输出 `no NVIDIA GPU detected` 并返回退出码 `1`。

构建本项目不需要 NVIDIA Driver；发布二进制使用 `CGO_ENABLED=0`，构建无需 C 工具链。NVML
后端通过系统动态加载器在运行时 `dlopen` NVML，因此并非完全静态链接；纯 musl（Alpine）不支持
NVML 后端（可回退 `nvidia-smi`）。
