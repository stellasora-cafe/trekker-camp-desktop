# 星塔咖啡厅应用更新服务

统一域名：`https://update.stellasora.cafe`。Cloudflare Worker `stellasora-update-feed` 通过 `RELEASES` 绑定读取私有 R2 桶 `stellasora-app-releases`。服务只开放 GET/HEAD，发布通过 R2 S3 兼容接口完成。

- 版本清单：`/v1/{app}/{channel}/update-{platform}-{arch}.json`
- 不可变安装包：`/packages/{app}/{version}/{filename}`
- 旅人营地应用 ID：`trekker-camp`，正式渠道：`stable`，Windows 平台：`windows-amd64`。

清单沿用 MyGo 的 version、url、size、signature 格式；安装包由每个应用自己的 Ed25519 私钥签名，公钥编入客户端。其他应用可采用相同协议。新版本必须递增版本号，不覆盖已发布安装包。发布时先上传包，再更新清单。清单不缓存；安装包长期缓存，支持 Range 和 HEAD。

构建时通过 `MYGO_UPDATER_PRIVATE_KEY` 提供签名私钥。旅人营地签名密钥位于维护者 Windows 用户配置目录的 `stellasora-cafe/update-keys/trekker-camp/`，不得提交私钥。发布需要通过本地环境变量 `CLOUDFLARE_API_TOKEN` 提供指定桶对象读写权限的账号级 API Token；脚本验证 Token 后按官方规则派生 S3 凭据（Token ID 和 Token 值的 SHA-256），仅在内存使用，无需另存 S3 密钥；不要将令牌写入命令、日志或仓库。

```powershell
node update-service/publish.mjs trekker-camp stable artifacts/signed-release/windows-amd64/update-windows-amd64.json "$env:APPDATA/stellasora-cafe/update-keys/trekker-camp/mygo-update.pub"
```

用拥有 Workers Scripts 和当前区域 Workers Routes 写权限的 Cloudflare 账号运行 `npx wrangler deploy --config update-service/wrangler.jsonc`。Worker 路由使用 `update.stellasora.cafe` 的现有代理记录。部署后访问 `/`，返回的 `runtime` 应为 `worker`。需要回滚时可移除该 Worker 路由，代理记录会恢复指向原 Pages 项目。

桌面前端热更新由营地服务器的 `/_desktop/frontend/` 提供。这里的 MyGo 清单和安装包用于桌面程序更新。

更新服务代码采用 [Apache-2.0](LICENSE)。
