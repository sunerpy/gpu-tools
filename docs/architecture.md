# 架构说明

`gpu-tools` 是面向 NVIDIA GPU 基础设施的纯 Go CLI。核心设计目标是：优先拿到真实 GPU 数据，
但在缺少驱动、库或 GPU 的主机上也能静态构建、启动并友好失败。

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

`auto` 后端按照优先级遍历已注册后端：NVML 可用时使用 NVML，否则尝试 `nvidia-smi`。

## 为什么 purego + `CGO_ENABLED=0`

- **可移植性**：发布产物是单个静态二进制，覆盖 Linux、macOS、Windows 的 `amd64` / `arm64`。
- **部署简单**：不要求用户在构建阶段安装 CUDA 或 NVML 头文件。
- **优雅降级**：运行期再动态检查 NVML / `nvidia-smi`；没有 GPU 或驱动时返回清晰错误，而不是构建失败。
- **CI 友好**：CI 可以在普通 runner 上构建和测试大部分逻辑；真实 GPU 数据只在有 NVIDIA Driver 的主机上可用。

## 包布局

| 路径                     | 职责                                                                                |
| ------------------------ | ----------------------------------------------------------------------------------- |
| `cmd`                    | Cobra CLI：`version`、`config`、`completion`、`detect`、`report`、`tune`、`bench`。 |
| `core`                   | 配置模型、默认路径、输出格式和后端常量。                                            |
| `internal/gpu`           | GPU 设备模型、`Collector` 接口、后端注册表和 `DefaultFactory`。                     |
| `internal/gpu/nvml`      | purego NVML collector。                                                             |
| `internal/gpu/nvidiasmi` | `nvidia-smi` collector 和 CSV 字段转换。                                            |
| `internal/report`        | 表格、JSON、Markdown snapshot renderer。                                            |
| `internal/tune`          | 只读调优规则：温度、节流、ECC、功耗余量等。                                         |
| `internal/bench`         | 外部 benchmark runner、工具枚举、输出解析。                                         |
| `version`                | ldflags 注入的版本、构建时间、commit、目标 OS / Arch。                              |

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
