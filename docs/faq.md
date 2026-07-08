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

暂不支持。v1 是 NVIDIA-only：后端为 purego NVML 和 `nvidia-smi`。AMD / Intel 后端延期。
