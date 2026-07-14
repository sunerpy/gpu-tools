# 架构说明

`gpu-tools` 是面向 NVIDIA GPU 基础设施的纯 Go CLI。核心设计目标是：优先拿到真实 GPU 数据，
但在缺少驱动、库或 GPU 的主机上也能用 `CGO_ENABLED=0` 构建、启动并友好失败。

## 三层 Collector 路线

1. **purego NVML（当前主后端）**
   - 包：`internal/gpu/nvml`
   - 注册名：`nvml`
   - 优先级：`10`
   - 通过 purego 动态加载 NVML，不引入 cgo。

2. **nvidia-smi fallback（当前回退后端）**
   - 包：`internal/gpu/nvidiasmi`
   - 注册名：`nvidia-smi`
   - 优先级：`20`
   - 执行 `nvidia-smi --query-gpu=... --format=csv,noheader,nounits` 并解析 CSV。

3. **DCGM（延期）**
   - v1 未实现。
   - 未来可作为更完整的数据中心监控后端接入同一 `Collector` seam。

此外还有一个**尽力而为的 AMD 后端**（`internal/gpu/amd`，注册名 `amd`，优先级 `30`），
通过 `rocm-smi --json` 读取指标子集。它优先级最低，因此 `auto` 仍优先 NVIDIA
（NVML `10` → nvidia-smi `20` → amd `30`）；只有显式 `--backend amd` 才会主动选它。
详见下方「AMD 后端」一节。

`auto` 后端按照优先级遍历已注册后端：NVML 可用时使用 NVML，否则尝试 `nvidia-smi`。

## 为什么 purego + `CGO_ENABLED=0`

- **可移植性**：发布产物是单一自包含二进制、无 cgo，覆盖 Linux、macOS、Windows 的 `amd64` / `arm64`，并可跨 glibc 发行版运行。
- **部署简单**：构建阶段不需要 C 工具链、CUDA 或 NVML 头文件。
- **真实边界**：purego NVML 通过系统动态加载器在运行时 `dlopen` `libnvidia-ml.so.1`，因此并非完全静态链接；`readelf -d` 可见 NEEDED `libdl` / `libc` / `libpthread`，真实 GPU 数据仍依赖系统加载器和 NVIDIA Driver。
- **musl 取舍**：纯 musl（Alpine）不支持 NVML 后端；如环境提供 `nvidia-smi`，可回退到 nvidia-smi 后端。
- **优雅降级**：运行期再动态检查 NVML / `nvidia-smi`；没有 GPU 或驱动时返回清晰错误，而不是构建失败。
- **CI 友好**：CI 可以在普通 runner 上构建和测试大部分逻辑；真实 GPU 数据只在有 NVIDIA Driver 的主机上可用。

## 包布局

| 路径                     | 职责                                                                                          |
| ------------------------ | --------------------------------------------------------------------------------------------- |
| `cmd`                    | Cobra CLI：`version`、`config`、`completion`、`detect`、`report`、`tune`、`bench`、`export`。 |
| `core`                   | 配置模型、默认路径、输出格式和后端常量。                                                      |
| `internal/gpu`           | GPU 设备模型、`Collector` 接口、后端注册表和 `DefaultFactory`。                               |
| `internal/gpu/nvml`      | purego NVML collector（含按进程采集）。                                                       |
| `internal/gpu/nvidiasmi` | `nvidia-smi` collector、字段自动发现、`--query-compute-apps` 进程采集。                       |
| `internal/gpu/amd`       | 尽力而为的 `rocm-smi` collector（指标子集）。                                                 |
| `internal/gpu/cache`     | watch 模式下的 TTL 缓存 collector 包装器。                                                    |
| `internal/exporter`      | Prometheus `prometheus.Collector`，拥有自己的 registry。                                      |
| `internal/report`        | 表格、JSON、Markdown snapshot renderer（含 GPU Processes 区块）。                             |
| `internal/tune`          | 只读调优规则：温度、节流、ECC、功耗余量等。                                                   |
| `internal/bench`         | 外部 benchmark runner、工具枚举、输出解析；含 perftest / nccl-tests。                         |
| `internal/platform`      | 运行时 OS 检测 seam（`IsLinux`/`CurrentOS`），供新命令的 Linux-only 门禁复用。                |
| `internal/topo`          | 解析 `nvidia-smi topo -m`，生成 GPU/NIC 连接矩阵和亲和性建议。                                |
| `internal/health`        | `doctor` 的只读探针框架（driver、`nvidia_peermem`、IOMMU、PCIe ACS、RDMA 端口/link layer）。  |
| `internal/rdma`          | 合并 `ibv_devinfo -v` 和 `ibstat` 输出，得到 RDMA 设备 / 端口 / 速率 / link layer 清单。      |
| `internal/prereq`        | 前置工具检测目录 + 按发行版的安装指引；被 `prereqs` 命令和 `doctor` 提示复用。                |
| `version`                | ldflags 注入的版本、构建时间、commit、目标 OS / Arch。                                        |

## `Collector` 接口

所有 GPU 后端实现同一接口：

```go
type Collector interface {
    Init() error
    Shutdown() error
    DeviceCount() (int, error)
    Device(i int) (*Device, error)
    Backend() string
}
```

`Device` 统一使用 Go 类型表达内存、功耗、温度、时钟、利用率、节流原因、ECC、MIG、Driver 和 CUDA
版本等字段。上层命令不直接知道 NVML 或 `nvidia-smi` 的细节。

## 注册表和 `DefaultFactory` seam

后端通过注册表接入：

```go
gpu.Register("nvml", 10, newCollector)
gpu.Register("nvidia-smi", 20, newCollector)
```

`internal/gpu.Select("auto")` 按优先级选择第一个可用 collector；显式后端则只按名称查找。

命令层通过 `gpu.DefaultFactory(core.Config)` 创建 collector：

```go
var DefaultFactory = func(cfg core.Config) (Collector, error) {
    return Select(cfg.Backend)
}
```

这个 seam 让命令测试可以替换 collector factory，而无需真实 NVIDIA GPU。

## 字段自动发现（nvidia-smi）

nvidia-smi 后端不硬编码固定的 `--query-gpu` 字段集，而是先探测驱动支持哪些字段：

- 运行 `nvidia-smi --help-query-gpu`，用 `parseSupportedFields` 匹配任意 `"field" - desc`
  行，得到当前驱动支持的字段名集合 `supportedFields`。
- `wantedFields` 声明期望字段（含 T4 的基础字段与 v1.1 新增的
  `utilization.encoder`、`utilization.decoder`、`pcie.link.gen.current`、
  `pcie.link.width.current`）。新增字段是**非强制**的：当旧驱动的 `--help-query-gpu`
  未列出它们时，`supportedFields` 守卫会将其丢弃，对应字段保持 `0`；字段名→列索引的解析
  本就容忍缺失。
- 最终仅用「期望 ∩ 支持」的字段发起真实 `--query-gpu` 查询，parser 再按字段名映射回列，
  因此不同驱动版本的列顺序/缺失都能安全处理。

## 按进程采集（GPU Processes）

`detect` / `report` 在存在计算进程时追加 **GPU Processes** 区块，进程数据有两条独立来源：

- **NVML（purego）+ `/proc`**：NVML collector 直接查询每个设备上的计算进程，再用
  `procinfo.Resolve` 在 Linux `/proc` 上解析 PID → 进程名 / 用户。
- **nvidia-smi `--query-compute-apps` 回退**：这是一次**独立于设备查询**的第二次查询——
  `nvidia-smi --query-compute-apps=gpu_uuid,pid,process_name,used_memory --format=csv,noheader,nounits`
  （参数切片，无 shell）。`used_memory` 从 MiB 换算为字节；`Type` 硬编码为 `compute`
  （graphics 进程仅 NVML 可见）。是否发起该查询同样经 `--help-query-compute-apps` 的
  `gpu_uuid` 支持探测（复用字段自动发现的通用逻辑）。

**R2 精确 UUID 归属**：由已解析设备构建 `map[uuid]deviceIndex`，每个 compute app 按
`byUUID[app.uuid]` 查找——命中则挂到该设备的 `Processes`；未命中（未知 / ghost uuid）**静默丢弃**，
**绝不按索引猜测归属**。两条共享同一 uuid 的记录会挂到同一设备。

所有降级路径均**非致命**（返回带空 `Processes` 的设备，不报错）：`--help-query-compute-apps`
本身失败、help 成功但缺 `gpu_uuid`、或 compute-apps 查询自身出错，都会跳过进程采集。畸形行
（列数不符、空 uuid、非数字 pid、坏 used_memory）逐行丢弃，同一输出中的合法兄弟行仍保留。

renderer 侧：`collectProcesses` 将所有设备的进程扁平化后按 `(GPU index 升序, PID 升序)`
稳定排序，作为 table / markdown 的唯一顺序保证；JSON 则保留每设备的原始进程顺序（Device 契约）。

## TTL 缓存（watch 模式）

`gpu-tools detect --watch <d>` 用 `internal/gpu/cache` 的 TTL 缓存 collector 包装真实
collector：`cache.New(inner, watchCacheTTL(watch))`。TTL 被 `watchCacheTTL` 钳制到
**至多 1 秒**，因此即使刷新间隔更长，每帧仍反映足够新的数据，同时在高频刷新下避免对后端的
重复读取，让刷新保持顺滑。watch 会先**急切读取一次**：若后端永久不可用则以退出码 `1`
快速失败，绝不进入重试循环。

## Exporter 设计

`internal/exporter.Exporter` 实现 `prometheus.Collector`（Describe + Collect），构造时接收
一个 `Factory func() (gpu.Collector, error)`（绑定 `gpu.DefaultFactory(cfg)`）。内部 collector
在**首次抓取**时惰性构建（factory + `Init()`）并缓存，后续抓取复用。

- **自有 registry**：exporter 通过 `prometheus.NewRegistry()` + `reg.MustRegister(e)` 拥有
  自己的 `*prometheus.Registry`，**绝不**用全局默认 registry；因此连续构建两个 exporter 也不会 panic。
  `cmd/export.go` 用 `promhttp.HandlerFor(exp.Registry(), ...)` 挂载 `/metrics`。
- **无 GPU → `up 0`**：后端不可用（`ErrBackendUnavailable` / `ErrNoBackend`）时，仅发出
  `gpu_tools_up 0`、无任何设备序列，HTTP 200，安静不报错；真实读取错误则记录到一个 `io.Writer`
  seam（默认 stderr）并同样发出 `up 0`。`Collect` 从不向 promhttp 传播错误。
- **并发安全**：`Collect` 全程持 `sync.Mutex`，重叠抓取不会竞争内部 collector。
- **命名约定**：只有 `gpu_tools_up` 带 `gpu_tools` 命名空间；所有 per-GPU / per-process 序列用
  裸 `gpu_` 前缀（有意为之，代码内有注释防止后人「修正」）。功耗从 NVML 的毫瓦换算为瓦特。

`cmd/export.go` 用 `signal.NotifyContext(SIGINT, SIGTERM)` 监听中断，`ctx.Done()` 后优雅
`srv.Shutdown(5s)`；RunE 返回 error，从不 `os.Exit`。这是一个 headless `/metrics` 端点，
**没有** Web UI（无 HTML / 模板 / 会话 / 数据库）。

## AMD 后端（rocm-smi）

`internal/gpu/amd` 是尽力而为的 AMD collector，注册名 `amd`、优先级 `30`（最低，故 `auto`
仍优先 NVIDIA，需显式 `--backend amd`）。它运行
`rocm-smi --showid --showproductname --showuse --showmemuse --showtemp --showpower --json`
并解析 JSON。

- **指标子集**：仅覆盖 index、name、GPU / 显存利用率、显存总量 / 已用、温度、功耗。
  **不含** 编码 / 解码利用率、PCIe 链路、时钟、功耗上限或按进程数据（这些保持 `0` / 空）。
- 每个字段用**候选键列表**读取（不同 rocm-smi 版本键名不同，如温度有多种 `Temperature (...)`
  写法），提升跨版本兼容性。找不到 `rocm-smi` 时返回 `gpu.ErrBackendUnavailable`。

## 新命令：外部命令编排（`topo` / `doctor` / `rdma` / `prereqs`）

`topo`、`doctor`、`rdma`、`prereqs` 是**四个新命令**，不是新的 `Collector` 后端——
后端注册表未变（NVML `10` / nvidia-smi `20` / amd `30`）。它们遵循同一套「外部命令
编排」模型：

1. **Linux 门禁**：命令入口先调用 `platformIsLinux()`（`internal/platform`）。非
   Linux 时返回退出码 `2`；`-o json` 下额外向 stdout 输出
   `{"supported":false,"platform":...,"reason":"requires Linux","required_tools":[...]}`。
2. **execRunner seam**：每个包通过一个可替换的 exec 函数变量调用外部工具（`nvidia-smi
topo -m`、`lspci`、`ibv_devinfo -v`、`ibstat`），测试注入假输出，不依赖真实硬件。
3. **解析为领域模型**：各包把原始文本解析成结构化结果（`topo.Result`、
   `health.Report`、`rdma.Result`、`prereq.CheckResult`），cmd 层只负责渲染
   table/json/markdown。
4. **降级为退出码 2**：外部工具缺失时（`errors.Is(err, xxx.ErrToolNotInstalled)`）
   命令返回退出码 `2`，附带安装提示，而不是崩溃或返回 `1`。

`internal/prereq` 额外提供一个独立的检测目录（工具名 → 二进制 → 用途 → 按发行版的
安装指引），被 `prereqs` 命令直接使用，也可被 `doctor` 的 `Hint` 字段复用同一套指引
（`prereq.HintFor`）。`gpu-tools` 本身**不打包**任何这些工具；它们都需要单独安装
（内核驱动、OFED/rdma-core、编译产物等），详见[前置依赖说明](prerequisites.md)。
