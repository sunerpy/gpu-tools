# Changelog

## [1.1.0](https://github.com/sunerpy/gpu-tools/compare/v1.0.0...v1.1.0) (2026-07-14)


### Features

* **cmd:** add topo/doctor/rdma/prereqs commands and perftest/nccl-tests benchmarks ([#9](https://github.com/sunerpy/gpu-tools/issues/9)) ([52b0277](https://github.com/sunerpy/gpu-tools/commit/52b0277259a7362e402aad7a1c38cc3a8e145007))


### Bug Fixes

* **ci:** gitignore RELEASE_CHANGELOG.md so GoReleaser dirty check passes ([b47aef6](https://github.com/sunerpy/gpu-tools/commit/b47aef62aa0443475dc7a017f93494dfc2cb63f6))
* **ci:** publish the asset-bearing release and prune duplicate empty drafts ([f0f3be0](https://github.com/sunerpy/gpu-tools/commit/f0f3be051792a1711e67e6d3401c59497edfe0e1))
* **ci:** write git-cliff release notes to runner.temp so GoReleaser dirty check passes against the tag tree ([204deb7](https://github.com/sunerpy/gpu-tools/commit/204deb7a906521e15af79afbf2c033f9a712f728))

## 1.0.0 (2026-07-09)


### Features

* **amd:** add rocm-smi backend and wire into cmd and config ([7c96007](https://github.com/sunerpy/gpu-tools/commit/7c96007c5b35f64b53fbfb26d8b3737988d8d4bd))
* **cmd:** add --watch refresh mode to detect ([0fa2179](https://github.com/sunerpy/gpu-tools/commit/0fa217917c0bcd553fac924ee837111fd495f57a))
* **cmd:** add bench subcommand wrapping external tools ([3f0bc9c](https://github.com/sunerpy/gpu-tools/commit/3f0bc9cb4eebb2dd4cd4638a5f905a19ab14101f))
* **cmd:** add cobra root, version, config, completion commands ([b3704d6](https://github.com/sunerpy/gpu-tools/commit/b3704d694eb9a961717a17e9286d1c20439d7fcb))
* **cmd:** add detect subcommand ([88dac77](https://github.com/sunerpy/gpu-tools/commit/88dac775d5a051c5a3e8e0fbcc57845b23db9bf5))
* **cmd:** add Prometheus exporter subcommand ([b22e8ce](https://github.com/sunerpy/gpu-tools/commit/b22e8ce6d81e1f9a69da016068743270941fa0dc))
* **cmd:** add read-only tune recommendations ([691dc61](https://github.com/sunerpy/gpu-tools/commit/691dc61e5a57489f688e4438318fcf9790cf6d62))
* **cmd:** add report subcommand ([daa81f0](https://github.com/sunerpy/gpu-tools/commit/daa81f05dd01f1f51e4816551af059fb16572e9c))
* **core:** add YAML+flag config loader ([08b157b](https://github.com/sunerpy/gpu-tools/commit/08b157bafa16c62f030def380cd40ba530d7f5a3))
* **gpu:** add Collector interface, registry, factory seam, mocks ([8cbb4d4](https://github.com/sunerpy/gpu-tools/commit/8cbb4d48f417531e1201a7e05733a31fd49be28d))
* **gpu:** add nvidia-smi fallback backend ([5aa9148](https://github.com/sunerpy/gpu-tools/commit/5aa9148aaa2ed407c88e53efafb7491fc2748c5d))
* **gpu:** add procinfo package for pid name and user resolution ([021dae0](https://github.com/sunerpy/gpu-tools/commit/021dae05dc4f4dbd53f7af0ac4f252bfb5d8f55b))
* **gpu:** add purego NVML backend (no cgo) ([770c31e](https://github.com/sunerpy/gpu-tools/commit/770c31e6e58a659e3effff9b7665eb8f0dd28638))
* **gpu:** add TTL cache wrapper for collector reads ([0a62054](https://github.com/sunerpy/gpu-tools/commit/0a6205402d9f601efdde920756b8c450d56c5c7e))
* **gpu:** extend Device with process and richer-metric fields ([e0ae51b](https://github.com/sunerpy/gpu-tools/commit/e0ae51b255e330557dfa5c8f958510eb2b941873))
* **nvidia-smi:** add field auto-discovery, compute-apps and encode/pcie metrics ([1b40942](https://github.com/sunerpy/gpu-tools/commit/1b40942eddcbb2f589e0d8636a90f4f0fbc1cc8c))
* **nvml:** add per-process, encoder/decoder and PCIe metrics via purego ([7aa52cf](https://github.com/sunerpy/gpu-tools/commit/7aa52cff5195a0f9cafece82e4c3972a5243b2da))
* **report:** add table/json/markdown renderers ([4a5f583](https://github.com/sunerpy/gpu-tools/commit/4a5f58300abcdfbe585795d1695cb15d0ef1c376))
* **report:** render process table and new device metrics ([72922e1](https://github.com/sunerpy/gpu-tools/commit/72922e1480246f1e0808561eda895cbf31739c0d))
* **version:** add version package and module layout ([47b8efb](https://github.com/sunerpy/gpu-tools/commit/47b8efb5092e23d8600131821804e521bb99bc8e))


### Bug Fixes

* **ci:** resolve govet shadow, misspell, and bump go to 1.26.5 for GO-2026-5856 ([b14f5f5](https://github.com/sunerpy/gpu-tools/commit/b14f5f5418d6a33b4004d422035ac8c17a0f7e53))
* **nvml:** guard purego backend to linux/darwin and add windows stub ([2552f93](https://github.com/sunerpy/gpu-tools/commit/2552f93ee7440fd4c3f5080b2bd6218e2fb14798))
