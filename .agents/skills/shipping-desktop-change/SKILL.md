---
name: shipping-desktop-change
description: 交付旅人营地桌面宿主、安装器、前端缓存或更新服务改动时，按范围执行验证并确认发布边界。
---

# 交付旅人营地桌面端改动

在桌面仓库根目录执行。按 [AGENTS.md](../../../AGENTS.md) 定位负责模块，使用 `go.mod` 和 `.node-version` 固定的工具链。需要 Node 依赖时运行 `pnpm install --frozen-lockfile`。

## 按改动范围验证

| 改动 | 验证 |
| --- | --- |
| Go 宿主、会话、窗口与缓存 | 对改动文件运行 `gofmt`，运行受影响包的 `go test` 和 `go vet`。跨包修改用 `go test ./... -timeout 3m` 和 `go vet ./...`。 |
| 宿主桥接、桌面 API 或前端兼容 | 运行 `go test ./internal/campweb ./internal/webclient ./internal/webfrontend`，核对服务端协议。需要界面验收时使用本地服务和独立 `--data-dir`。 |
| 安装器、包体更新与发布脚本 | 运行 `pnpm test`、`go tool mygo vet .` 和 `go tool mygo build -platform windows/amd64`。在 Windows 上使用 `CGO_ENABLED=0`。安装与卸载验收参考 [Desktop CI](../../../.github/workflows/desktop.yml)，在隔离环境执行。 |
| 签名更新或本地数据保留 | 运行本页的 MyGo 更新回归命令，并运行 `go test ./internal/webclient`。有签名归档时按 [发布指南](../../../RELEASING.md#本地验证) 调用 `verify-package.mjs`。 |
| 更新 Worker | 运行 `node --test update-service/worker.test.mjs`，检查 GET、HEAD、Range、清单缓存行为。涉及发布脚本时核对归档先于清单上传。 |
| 文档或 Agent 配置 | 检查文档链接、入口路径与 `git diff --check`。 |

MyGo 更新回归从桌面仓库根目录运行：

```powershell
go -C third_party/mygo test . -run '^(TestUpdater|TestUpdaterSource|TestUpdaterWaitsForOpenFiles|TestCampPackageUpdatePreservesLocalStores)$' -timeout 2m
```

## 交付

检查改动文件与 Git 索引，确认本地账号、私有素材、凭据和构建产物保持在忽略范围内。说明验证结果及环境限制。涉及界面时说明是否完成实际界面验收。需要正式发布时，按授权执行 [发布流程](../../../RELEASING.md)，使用与客户端内置公钥匹配的签名密钥。
