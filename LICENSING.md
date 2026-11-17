# 桌面客户端许可边界

旅人营地由[星塔咖啡厅（stellasora-cafe）](https://github.com/stellasora-cafe)开发与维护，是《星塔旅人》的非官方社区。

## 原创桌面客户端代码：Apache-2.0

本目录的 [Apache 2.0 许可证](LICENSE) 适用于标记 `SPDX-License-Identifier: Apache-2.0` 的原创桌面客户端代码：正式入口 `main.go`，以及 `campweb`、`webclient`、`webassets`、`webfrontend`、`webmedia` 中相应源文件。它们负责系统窗口、原生欢迎界面、设备协议、资源交付与更新。原创更新服务也采用 [Apache-2.0](update-service/LICENSE)。

正式入口使用 MyGo 自带按钮。MyGo 保留其 MIT 许可证与版权声明。源码中的具体 SPDX 声明与第三方许可优先于目录级说明。

## Misskey 前端：保留原许可

首次联网下载及后续热更新的 Misskey 前端继续采用 AGPL-3.0-only，前端中的独立第三方依赖继续适用各自许可。可从[营地源码仓库](https://github.com/stellasora-cafe/trekker-camp)取得对应版本源码、构建脚本和根目录 AGPL 许可证。每次发布须提供与实际二进制及前端包对应的源码版本。


应用运行时使用多种许可证的组成部分，发布说明分别列出宿主、下载的前端及资源的许可。

## 独立工具模块

未来星塔工具箱可分发团队独立开发、单独许可的闭源模块，并在模块入口和发行说明标明其许可。此类模块通过独立的宿主能力或服务接口交互，不复制、链接或改写 AGPL 前端后作为闭源代码发布。现有 AGPL 部分的对应源码和构建文件继续公开。

## 字体、美术与品牌

字体、图像、音频和品牌素材按各自的素材授权使用。桌面安装器与包体更新随附关于页图像、MiSans Regular TTF、Regular/Medium/Semibold/Bold 四个 WOFF2 和原字体许可；其他独立资源运行时下载。公开代码、服务端 Release 部署包及前端热更新 ZIP 不携带私有素材。关于营地与法律声明随宿主提供。

MiSans 用于应用界面，保留原字体文件与官方许可要求；关于页展示“本应用使用 MiSans 字体”并链接官方页面。素材清单与来源见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)，完整团队记录见私有素材仓库的 `RIGHTS.md`。
