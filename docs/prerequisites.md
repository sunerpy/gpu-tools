# Prerequisites

`gpu-tools` orchestrates a handful of external tools for its diagnostic and
benchmark commands. It does **not bundle any of them** — most require a host
kernel driver, vendor-licensed userspace components, or a from-source build,
none of which can ship inside a single portable `CGO_ENABLED=0` binary. This
page lists what each feature needs and where to get it.

Run `gpu-tools prereqs` to check the current host live: it detects each tool on
`PATH`, shows the resolved binary path when found, and prints a distro-aware
install hint (Debian/Ubuntu, RHEL/Fedora/Rocky, or a source-build fallback) for
whatever is missing.

```bash
gpu-tools prereqs
gpu-tools prereqs --output json
```

`prereqs` is informational: missing tools are reported, not treated as a
failure, so the command exits `0` on Linux (and `2` on non-Linux, since the
underlying detection is Linux-oriented).

## Catalog

| Tool             | Binary            | Used by                                      | Purpose                             |
| ---------------- | ----------------- | -------------------------------------------- | ----------------------------------- |
| NVIDIA driver    | `nvidia-smi`      | `detect`, `report`, `tune`, `topo`, `doctor` | NVIDIA GPU query/management         |
| rdma-core / OFED | `ibv_devinfo`     | `doctor`, `rdma`                             | RDMA device/port info               |
| rdma-core / OFED | `ibstat`          | `doctor`, `rdma`                             | InfiniBand/RoCE port state          |
| perftest         | `ib_write_bw`     | `bench --tool perftest`                      | RDMA bandwidth/latency benchmark    |
| nccl-tests       | `all_reduce_perf` | `bench --tool nccl-tests`                    | NCCL collective bandwidth (busbw)   |
| DCGM             | `dcgmi`           | (reserved for future `doctor` checks)        | Data Center GPU Manager diagnostics |

### nvidia-smi (NVIDIA driver)

`gpu-tools detect`/`report`/`tune` read GPU data through the purego NVML
backend or through `nvidia-smi`; `gpu-tools topo` parses `nvidia-smi topo -m`
directly, and `gpu-tools doctor` checks for its presence as one of its health
probes.

- Install pointer: NVIDIA's official driver download at
  [nvidia.com/download](https://www.nvidia.com/download/index.aspx).
- Debian/Ubuntu: `apt install nvidia-driver-<ver>`
- RHEL/Fedora/Rocky: `dnf install nvidia-driver`
- In containers, pass `--gpus` (Docker) or the equivalent GPU device request
  instead of installing the driver inside the container image.

### ibv_devinfo (rdma-core / OFED)

Used by `gpu-tools rdma` (device/port inventory) and `gpu-tools doctor` (RDMA
port-state probe). Ships as part of `rdma-core`/`ibverbs-utils` on stock
distros, or as part of NVIDIA Networking's OFED stack on systems that need the
full InfiniBand/RoCE driver set.

- Debian/Ubuntu: `apt install ibverbs-utils rdma-core`
- RHEL/Fedora/Rocky: `dnf install libibverbs-utils rdma-core`
- Source / vendor stack: install MLNX_OFED or DOCA-OFED from
  [NVIDIA Networking](https://network.nvidia.com/).

### ibstat (infiniband-diags)

Used alongside `ibv_devinfo` by `gpu-tools rdma` and `gpu-tools doctor` to read
port state and link layer (InfiniBand vs. Ethernet/RoCE).

- Debian/Ubuntu: `apt install infiniband-diags`
- RHEL/Fedora/Rocky: `dnf install infiniband-diags`
- Source / vendor stack: MLNX_OFED or DOCA-OFED.

### perftest (ib_write_bw)

Used by `gpu-tools bench --tool perftest` to run an RDMA bandwidth/latency
benchmark. Requires a running `ib_write_bw` server on the remote host reachable
via `--server <ip>`; `--use-cuda <N>` exercises the GPUDirect RDMA (GDR) path.

- Debian/Ubuntu: `apt install perftest`
- RHEL/Fedora/Rocky: `dnf install perftest`
- Source: build [github.com/linux-rdma/perftest](https://github.com/linux-rdma/perftest)
  (configure with `--use_cuda` for GDR support).

### nccl-tests (all_reduce_perf)

Used by `gpu-tools bench --tool nccl-tests` to measure NCCL collective
bandwidth (busbw) across the local GPUs (`--gpus`, default `8`). This tool has
no packaged distro build; it must be compiled against a matching NCCL/CUDA
install.

- Source: build [github.com/NVIDIA/nccl-tests](https://github.com/NVIDIA/nccl-tests)
  (`make MPI=1 ...` if you need multi-process launch on a single node).
- `--nccl-debug` sets `NCCL_DEBUG=INFO` and inspects the tool's own output for
  GDRDMA usage; it does not require any extra install beyond nccl-tests itself.

### dcgm (dcgmi)

Not currently consumed by any `gpu-tools` command output, but included in the
`prereqs` catalog as a data-center diagnostics tool that pairs well with
`gpu-tools doctor`.

- Debian/Ubuntu and RHEL/Fedora/Rocky: install the `datacenter-gpu-manager`
  package from NVIDIA's repositories.
- Docs: [docs.nvidia.com/datacenter/dcgm](https://docs.nvidia.com/datacenter/dcgm/).

## What gpu-tools does and does not do

`gpu-tools` shells out to these tools, parses their output, and renders it in
table/JSON/Markdown form. It never installs, updates, or configures any of
them, and it never requires them to build or run the base binary — commands
like `version`, `config`, and `completion` work with none of the above
installed. Only `topo`, `doctor`, `rdma`, `prereqs`, and the `perftest`/
`nccl-tests` modes of `bench` need their respective tool; each fails clearly
with exit code `2` when its tool is missing, rather than guessing or
degrading silently. See the [exit-code contract in the FAQ](faq.md#退出码契约是什么)
for the full table.
