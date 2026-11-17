# 旅人营地桌面客户端 Copilot 指南

本仓库维护 Windows WebView2 宿主、安装器和签名包体更新。日常开发分支为 `master`，文案源语言为简体中文。Misskey 网页由服务端仓库维护。修改桥接、设备会话或前端兼容清单时核对服务端协议，优先用独立的测试目录和本地服务验收。

Go 宿主须支持 `CGO_ENABLED=0`。修改 `third_party/mygo/` 前阅读其 [AGENTS.md](../third_party/mygo/AGENTS.md)，保留上游 MIT 声明。宿主和更新服务采用 Apache-2.0，运行时加载的 Misskey 前端保留 AGPL-3.0-only。打包只取 [发布资源白名单](../RELEASING.md#构建安装器) 允许的文件。凭据、私有素材和本地账号留在 Git 之外。

审查包体更新和重启时检查签名、失败恢复、账号与草稿保留，以及前端和素材缓存的隔离。普通源码构建支持无私有素材的环境。涉及发布、标签、稳定更新清单或部署 Worker 的操作按用户授权执行。

按 [桌面交付技能](../.agents/skills/shipping-desktop-change/SKILL.md) 选择验证。Go 改动运行受影响包测试和 `go vet`，构建或 Worker 改动运行 `pnpm test` 及对应验证。完整工程约定见 [AGENTS.md](../AGENTS.md)。
