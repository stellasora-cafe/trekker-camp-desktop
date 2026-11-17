# 第三方软件与素材

此文件随桌面发行包分发。完整源码及依赖版本见对应 Release tag 的 [源码仓库](https://github.com/stellasora-cafe/trekker-camp-desktop)。各依赖随附的许可证与版权声明继续有效。

| 组成部分 | 授权与记录 |
| --- | --- |
| 原创桌面客户端代码 | [Apache-2.0](LICENSE)，适用范围见 [LICENSING.md](LICENSING.md) |
| Misskey 网页前端 | AGPL-3.0-only，发行包内 `LICENSE-AGPL-3.0.txt`，[服务端源码及许可](https://github.com/stellasora-cafe/trekker-camp) |
| MyGo | MIT，源码内 `third_party/mygo/LICENSE`，保留上游与补丁作者版权 |
| Go 及 Go 模块依赖 | 源码内 `go.mod` / `go.sum` 固定版本；各模块保留其随附许可证 |
| MiSans | © 小米科技有限责任公司，[官方字体页面](https://hyperos.mi.com/font/zh/)，按 MiSans 字体知识产权许可协议用于应用界面 |
| 营地 Logo、帐篷双星图标、魔王界面插图 | 独立美术授权，[公开素材来源记录](https://github.com/stellasora-cafe/trekker-camp/blob/desktop-v0.1.2/apps/misskey/packages/frontend/assets/trekker-camp/RIGHTS.md) |
| 维护页插图（运行时下载） | 星塔旅人相关原权利人，保留图中 © Yostar 标识，用于营地维护提示 |

站点素材来自 `stellasora-cafe/trekker-camp-assets`。桌面安装器和包体更新仅随附 MiSans Regular TTF、Regular/Medium/Semibold/Bold 四个 WOFF2 与 `resources/licenses/MiSans-license.pdf`，以及关于页营地图标和组织头像。其他资源运行时从站点下载。源码包和服务端部署包不包含私有素材；原稿、授权凭据、仓库元数据和访问密钥不进入任何客户端包。
