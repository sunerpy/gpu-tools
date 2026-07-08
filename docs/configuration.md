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
| `backend`         | `auto`、`nvml`、`nvidia-smi`           | GPU 采集后端；可被全局 `--backend` 覆盖。            |
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
- `--backend auto|nvml|nvidia-smi`：覆盖 `backend`。
- `--config <path>`：指定要加载的 YAML 配置文件路径。

报告参数：

- `gpu-tools report --out <path>`：写入指定文件。
- `gpu-tools report --out -`：写入 stdout。
- 未指定 `--out` 时，生成 `gpu-report-YYYYMMDD-HHMMSS.md` 并写入 `report_dir`。

## 后端选择

`backend: auto` 按注册优先级选择后端：

1. `nvml`：purego NVML 主后端，优先级 `10`。
2. `nvidia-smi`：CSV 解析回退后端，优先级 `20`。

如果显式设置 `--backend nvml` 或 `--backend nvidia-smi`，程序只尝试指定后端。未知后端会返回
`unknown gpu backend "<name>"`。

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
