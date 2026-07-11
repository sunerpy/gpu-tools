# 开发指南

本项目是 Go CLI，模块名为 `github.com/sunerpy/gpu-tools`，目标 Go 版本为 `1.26.4`。
发布构建使用 `CGO_ENABLED=0`，交付单一自包含二进制；构建无需 C 工具链，但 purego NVML 会通过系统动态加载器在运行时加载 NVML，因此并非完全静态链接。

## 常用命令

```bash
# 构建本机二进制到 dist/gpu-tools。
make build

# 运行带 race detector 和 atomic coverage 的测试。
make test

# 生成 coverage.html。
make coverage

# 过滤不计入门禁的文件后，要求总覆盖率 >= 95%。
make coverage-gate

# 格式化 Go、YAML、JSON、Markdown。
make fmt

# 检查格式但不写文件。
make fmt-check

# 运行 golangci-lint；缺少时回退到 go vet。
make lint
```

`make coverage-gate` 会过滤以下内容后计算覆盖率：

- `*_test.go`
- `/mocks/`
- `main.go`
- `version/`
- `internal/gpu/nvml/purego_lib.go`

门禁阈值是 **95%**。CI 在上传 Codecov 之前运行该门禁，并通过 `make coverage-parity` 确认
`codecov.yml` 的忽略规则与 Makefile 过滤规则一致。

## 本地 hooks

安装 pre-commit 和 pre-push hooks：

```bash
pre-commit install --hook-type pre-commit --hook-type pre-push
```

当前 `.pre-commit-config.yaml` 映射：

- `pre-commit`：运行 `make fmt`。
- `pre-commit`：运行 `make lint`。
- `pre-push`：运行 `make test`。

## 格式化工具

Go 格式化依赖：

```bash
go install golang.org/x/tools/cmd/goimports@latest
go install mvdan.cc/gofumpt@latest
```

Markdown / YAML / JSON 格式化依赖 `oxfmt`：

```bash
cargo install oxfmt
# 或
npm install -g oxfmt
```

## Conventional Commits

所有提交和 PR 标题应遵循 Conventional Commits：

```text
feat: add gpu inventory command
fix: handle missing nvidia-smi binary
docs: add configuration guide
chore: update release workflow
```

原因：仓库使用 release-please 生成版本和 changelog。Squash merge 必须保留 Conventional PR 标题，
否则 release-please 只能看到 squash commit 的标题，可能无法正确计算版本升级。

## release-please 流程

1. 普通 PR 合并到 `main`。
2. release-please 根据 Conventional Commits 更新或创建 Release PR。
3. Release PR 内同样运行 CI、格式检查、测试和覆盖率门禁。
4. 合并 Release PR 后创建 `v*` tag。
5. GoReleaser 为 Linux、macOS、Windows 的 `amd64` / `arm64` 构建产物。
6. git-cliff 生成 release notes，工作流发布完成的 GitHub Release。

release-please 维护的仓库变更日志按主版本拆分在 `changelog/` 目录下，当前活跃系列为
`changelog/CHANGELOG-v1.x.md`（由 `release-please-config.json` 中的 `changelog-path`
指定）。开启新的主版本系列时，新建 `changelog/CHANGELOG-vN.x.md` 并同步更新
`changelog-path`。这些文件由工具维护，不要手动编辑（见 `changelog/README.md`）。

仓库元数据和 squash-merge 策略的手动命令记录在 [repo-metadata.md](repo-metadata.md)。
