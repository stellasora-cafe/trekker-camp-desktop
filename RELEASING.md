# 桌面客户端发布

桌面客户端使用 `desktop-vX.Y.Z`。桌面标签的版本必须与 `mygo.json` 一致。工作流先创建 GitHub Release 草稿，审查后发布该 Release 才激活 `update.stellasora.cafe` 上的正式更新清单。

## 构建安装器

从仓库根目录执行 `go tool mygo build -platform windows/amd64`，默认生成中文 Windows 安装器。安装位置默认是当前用户的 `%LOCALAPPDATA%/Programs/旅人营地`，可选择其他可写目录；安装器提供开始菜单入口、可选桌面快捷方式及系统应用列表中的卸载入口。普通源码构建不需要私有素材。

安装器和包体更新内置关于营地页面、营地图标、组织头像，以及 MiSans Regular TTF 和 Regular/Medium/Semibold/Bold 四个 WOFF2。字体放在 `resources/camp/fonts/misans-v1/`，图像放在 `resources/about/`，原字体许可放在 `resources/licenses/MiSans-license.pdf`。前端页面按需联网加载并使用浏览器缓存，页面显示后后台下载完整前端包到安装目录；维护插图和其他私有资源运行时下载到独立缓存。构建每次重新生成忽略的 `.release-resources/`，按明确白名单复制，不递归打包素材库。

本地含字体构建优先使用 `CAMP_DESKTOP_ASSETS_DIR`，其次使用 `CAMP_ASSETS_DIR`，再自动查找已有 `private-assets/public` 中的完整字体；显式路径指向素材库的 `public` 目录。原许可从相邻 `originals/fonts/misans-v1/MiSans-license.pdf` 读取。可选组织头像源为忽略的 `.release-source/about/organization.png`。普通无素材源码构建使用系统字体回退。

设置 `MYGO_UPDATER_PRIVATE_KEY` 后，同一次构建还生成签名的完整应用归档与更新清单。归档包含客户端 EXE、`resources/` 和许可文件；前端热更新 ZIP 使用独立协议。私钥应通过环境变量或本机受保护文件提供，不提交到仓库。Ed25519 用于更新归档验证；Windows 安装器的 Authenticode 使用单独的代码签名证书，当前本地安装器未配置该证书。

## GitHub 发布环境

在桌面仓库的 `desktop-release` 环境配置以下变量和密钥。已安装的客户端继续信任现有签名公钥，必须使用对应的原密钥。

- Secret `CAMP_ASSETS_READ_TOKEN`：仅限私有素材仓库内容读取的凭据。
- Variable `CAMP_DESKTOP_ASSETS_REVISION`：经审查的素材提交号（完整 40 位 SHA）；正式构建校验该版本和干净工作副本，仅检出字体与许可所在目录。
- Secret `MYGO_UPDATER_PRIVATE_KEY`：与客户端内置公钥匹配的更新签名密钥。
- Secret `CLOUDFLARE_UPDATE_PUBLISH_TOKEN`：更新服务 R2 发布凭据，仅供 Release 发布工作流使用。

环境可设置发布审批。推送 `desktop-vX.Y.Z` 标签后，`desktop-release.yml` 校验版本、构建宿主、执行测试、验证包内 EXE、五个字体、关于页图像、法律声明及签名，然后生成 Release 草稿。发行说明链接该标签的完整对应源码与构建脚本；安装包的 `resources/release.json` 记录代码标签与对应源码地址。

审查安装器与更新归档后，发布草稿。`desktop-publish.yml` 仅响应正式桌面 Release，重新校验归档签名与内容，再使用公共更新服务的发布脚本先上传应用包、最后切换清单。创建草稿不会激活客户端更新。整个流程不部署网站。

## 客户端更新行为

客户端在线页面就绪 10 秒后、重新连接和每 6 小时检查包体版本。有新版本时显示营地风格的原生更新窗口，用户选择下载安装。下载、大小和签名校验通过后，MyGo 在安装目录替换 EXE 与随附 `resources/`；失败保留当前可运行版本。

安装后可选择重启或稍后启动。重启前保存可恢复的本地发帖草稿；上传、提交、特殊发帖流程和未接入草稿保存的编辑器会阻止重启。账号及加密凭证保留在用户数据目录；`frontend-cache/` 与 `resource-cache/` 独立于安装包拥有的 `resources/`，包体更新保留这两个目录。不同宿主版本的前端缓存隔离，新宿主先按需加载匹配的前端页面，再后台暂存完整前端包；EXE 和安装器均不携带前端 ZIP。服务器负责发布前端构建，桌面安装器构建准备宿主、白名单字体与关于页图像和法律声明。

启动时先本地读取当前站点的加密会话；已登录直接进入营地并复用缓存，更新后台检查。未登录才显示原生欢迎页，不主动检查连接，登录和注册按钮保持可用。点击后仅检查前端 manifest 兼容性，等待期间禁用按钮并显示转圈；失败显示红字并允许重试。登录与注册 WebView 直接加载 `/desktop-auth`，页面显示后后台下载其余前端资源，不等待完整 ZIP。

原生欢迎页、网页登录界面和登录后的 Logo 菜单提供“检查更新”，主动检查前端与包体版本；网页与 PWA 不显示此桌面专属入口。

## 本地验证

```powershell
node --test scripts/prepare-release.test.mjs scripts/package-files.test.mjs scripts/third-party-licenses.test.mjs scripts/package-restart.test.mjs
go test ./internal/webclient ./internal/webfrontend ./internal/webassets
go -C third_party/mygo test . -run 'TestUpdater$|TestUpdaterSource|TestUpdaterWaitsForOpenFiles|TestCampPackageUpdate'
node scripts/verify-package.mjs build/windows-amd64
```

`verify-package.mjs` 对正式签名包检查完整客户端、字体与关于页图像白名单、对应许可和 Ed25519 签名；正式发布使用 `--require-fonts`，拒绝缺少字体、前端 ZIP、维护插图及其他未批准文件。安装/卸载验收使用独立测试目录，先确认不存在同标识的用户安装，保留正常客户端和用户数据。
