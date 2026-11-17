# 旅人营地桌面客户端 Agent 指南

本仓库维护 Windows 桌面宿主、安装器和包体更新服务。日常开发和 PR 以 `master` 为目标，客户端文案以简体中文为源。网页及供桌面端在线加载的 Misskey 前端在 [服务端仓库](https://github.com/stellasora-cafe/trekker-camp) 维护。

## 开发入口

| 路径 | 职责 |
| --- | --- |
| `main.go`、`internal/webclient/`、`locales/` | 应用入口、窗口、托盘、会话、维护页和客户端文案 |
| `internal/campweb/` | 注入 WebView 的宿主桥接和桌面 API 协议 |
| `internal/webfrontend/`、`internal/webassets/`、`internal/webmedia/` | 前端热更新、运行时素材和磁盘缓存 |
| `installer/`、`mygo.json`、`scripts/` | 安装器、包体版本、资源许可和构建验证 |
| `update-service/` | Cloudflare Worker 更新清单与签名归档发布 |
| `third_party/mygo/` | MyGo 本地依赖。修改前阅读其 [AGENTS.md](third_party/mygo/AGENTS.md) 和 [营地补丁记录](third_party/mygo/CAMP_PATCHES.md) |

使用 `go.mod` 和 `.node-version` 固定的工具链，从仓库根目录运行 `pnpm install --frozen-lockfile`。开发时按 [README](README.md#开发与构建) 启动宿主。登录、安装与卸载验收使用本地服务、测试账号和独立目录，操作前确认测试安装不会覆盖日常使用的客户端。

## 修改与验收

- 保持 Windows 宿主可在 `CGO_ENABLED=0` 下构建。修改 Go 代码后运行 `gofmt`，按职责选对应包的测试。
- 修改 `/api/client/`、`/client-streaming`、WebView 桥接或前端兼容清单时，核对服务端对应实现以及已安装客户端的兼容性。会话、草稿和缓存的归属按各自原有存储规则处理。
- 修改安装器、更新或重启时，验证签名校验、失败恢复、账号与草稿保留。前端热更新按宿主版本隔离，包体更新仅替换安装器拥有的文件。
- 新增源文件按所在目录保留正确的 SPDX 标识。宿主与更新服务采用 Apache-2.0，MyGo 保留 MIT，Misskey 前端保留 AGPL-3.0-only。详见 [LICENSING.md](LICENSING.md)。
- 素材遵循 [发布资源白名单](RELEASING.md#构建安装器)。版本、签名和素材凭据通过环境配置提供，私有素材、密钥、本地账号及 WebView 用户数据留在 Git 之外。公开源码构建使用系统字体回退。

交付前使用 [shipping-desktop-change](.agents/skills/shipping-desktop-change/SKILL.md)，按改动范围执行验证。Windows CI 的完整验证见 [Desktop workflow](.github/workflows/desktop.yml)。

## 提交与发布

提交和推送按用户指定的范围进行。发行标签、发布 Release、切换更新清单和部署 Worker 都需要对应授权。正式发布使用现有签名公钥对应的密钥，保持已安装客户端的更新兼容性。细节见 [RELEASING.md](RELEASING.md) 和 [更新服务说明](update-service/README.md)。
