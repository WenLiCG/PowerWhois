# 部署、升级与回滚

本版默认仅本机访问。Compose 从源码构建，CLI/API/MCP 示例见 [USAGE.md](USAGE.md)。历史 systemd、真实查询与重启验收见 [VPS 验收与回滚记录](VPS-ACCEPTANCE-20261103.md)。

本地交付包 `dist/PowerWhois-20261103-r2.zip` 包含源码、Linux amd64 后端/CLI/MCP、Windows amd64 CLI/MCP、网页静态文件、文档、依赖许可及 SHA-256 清单。解压后先读 `DELIVERY.txt`；二进制仍需要 Redis、配置和安全环境变量，不是免配置在线服务。Linux 解压后按清单为 `bin/linux-amd64/` 中的程序补充执行权限；Windows 两个客户端连接后端使用，不包含 Windows 服务端。

## 从源码构建

Go 1.27.4、Node.js 24、npm、Redis 7。Linux/WSL 的 Redis 用于自动化测试，Windows 可运行编译出的客户端。Go 会根据 go.mod 自动下载需要的工具链；锁定依赖在 go.sum/package-lock.json。

```sh
cd src
make setup
make test
make all
```

后端输出到 `dist/backend/`；网页在 `src/frontend/dist/spa/`。Makefile 不覆盖现有部署目录。非 make 环境可以在 `src/backend` 分别运行 `go build`、`go build ./cmd/powerwhois`、`go build ./cmd/powerwhois-mcp`。

启动时配置 `POWERWHOIS_CONFIG` 为 YAML 路径（未设置则读取可执行文件同目录 config.yaml），提供 `POWERWHOIS_ADMIN_USER`、`POWERWHOIS_ADMIN_PASSWORD`、`POWERWHOIS_JWT_SECRET` 和 `POWERWHOIS_QUERY_TOKEN`。密码至少 12 字符，密钥/token 至少 32 字符并使用密码学随机值。不要放入 shell 命令参数、Git、截图或共享日志。默认监听 `127.0.0.1:8080`；`REDIS_HOST/REDIS_PORT/REDIS_DB/REDIS_PASSWORD` 可配置数据库。只读二进制目录部署时，将 `POWERWHOIS_LOG_DIR` 指向服务可写的日志目录；日志下载 ZIP 写在该目录的父目录，两处都应位于受保护的运行数据目录。

环境管理的管理员凭据不能在 Web 修改；配置保存不会把这些秘密落到 YAML。其他设置仍从 Web 修改并原子写入 YAML。配置目录必须可写（需要创建临时文件并替换原文件），仅挂载只读单文件会导致保存失败。自定义注册/API URL 可能包含秘密，配置文件应为 0600，目录限制访问；备份也要同等保护。

开发网页：在 `src/frontend` 运行 `npm run dev`，访问 `http://127.0.0.1:8090`，代理到 8080 后端。开发服务器不用于公网生产服务。

## Docker Compose

需要完整仓库，而不仅是旧 deploy 文件夹。首次新安装：

```sh
cd deploy
cp .env.example .env
chmod 600 .env
# 在编辑器中填写随机凭据；不要将真实值粘贴到命令行。
sudo chown -R 10001:10001 backend
sudo chmod 750 backend
sudo chmod 600 backend/config.yaml
docker compose up -d --build
```

访问 `http://127.0.0.1:8080`。网关仅映射主机回环端口；后端、Redis 不发布主机端口。容器内后端监听 0.0.0.0 是为了网关连接，不等于公网发布。backend 以 UID 10001 运行；首次挂载配置目录必须按上述权限准备。Redis 目录延续 `deploy/redis/redisdata`，AOF everysec、noeviction，不主动清理任务。

部署前可在安全终端执行 `docker compose config --quiet` 检查配置；不要把展开秘密的 `docker compose config` 输出共享。此环境没有 Docker daemon，因此本次未实际执行容器构建/启动，需在目标服务器验收。

## systemd / 原生部署

提供 [powerwhois.service](../deploy/powerwhois.service)。创建专用 `powerwhois` 用户，将二进制放 `/opt/powerwhois/`，工作目录 `/var/lib/powerwhois/`，配置 `/etc/powerwhois/config.yaml`。环境文件 `/etc/powerwhois/powerwhois.env` 由 root 维护（0600），包含上述环境变量；配置和工作目录允许该服务用户写入。安装 unit 后 `systemctl daemon-reload`、`systemctl enable --now powerwhois`。

原生部署可设置 `POWERWHOIS_WEB_ROOT=/opt/powerwhois/current/web`，由后端在同一端口服务构建后的网页、SPA 路由和 WebSocket；目录必须包含 index.html。未设置时保持原来的纯后端行为。也可用 Caddy/Nginx 服务静态网页，将 `/api/*`、`/app/*` 转发到 127.0.0.1:8080；必须保留原 Host 并支持 WebSocket Upgrade。可参考仓库 Caddyfile。

本次 VPS 的三个实际 unit 模板在 [deploy/vps](../deploy/vps/)，采用 DynamicUser、独立 Redis 和只读程序目录；它们依赖验收记录中列出的目录、配置和环境文件，不能单独复制后直接启动。一次只能运行一个拥有同一 Redis 命名空间的后端；不要同时运行旧服务与新服务。worker 使用 15 秒租约、每 3 秒续期，丢失租约会停止服务；supervisor 等待 20 秒再重启。

## 远程访问

个人使用优先 SSH 转发：`ssh -N -L 8080:127.0.0.1:8080 your-server`，浏览器和 CLI 访问本机 8080。MCP HTTP 可另外转发 8081。SSH 连接使用现有密钥/安全配置，不在 URI 或命令参数中嵌入密码。

必须远程访问时，先准备 TLS 反向代理与身份网关：完整 Web 包括原匿名即时查询入口，若不打算公开查询，应保护**整个站点**；只对 API 配 Bearer 不会保护原匿名网页查询。对 HTTP MCP 仅反代 `/mcp` 到回环 8081，保留 Authorization，增加符合客户端环境的 OAuth/网关认证；不关闭 TLS 验证、不取消后端鉴权。不要把静态 token 放在 URL。云端 Agent 通常无法访问用户电脑的 localhost，需要远程可达地址或平台支持的安全隧道。

当前不是多用户 SaaS，没有任务级 ACL、配额/计费隔离或自动清理。数据规模大时监测 Redis 内存、AOF 磁盘、响应时间和上游限流。任务保存完整原始响应，历史数据会持续增长；不应给任务数据库设置会逐出数据的缓存策略。进程重启可恢复，断电持久性仍取决于 Redis/文件系统/备份。

## 第二轮升级补充

从 89a29b6 升级时，同步替换后端和网页；任务键保持兼容。若旧管理页面已在实际环境使用，请在服务端轮换 JWT 签名密钥和可能暴露的管理员/代理/API 凭据，再重新登录；关闭回传不会撤回此前浏览器已取得的秘密。新管理接口不再回传任何秘密，旧管理脚本修改凭据要使用 secretActions；分页客户端必须跟随服务器游标。启用注册时，应用 Redis 用户还需允许读写 `powerwhois:registration:*` 和 `powerwhois:registration-guard:*`，备份包含这些键。先用模拟注册商验收，再根据注册商的幂等能力决定是否启用真实交易。见 [ROUND2.md](ROUND2.md)。

## 从旧版升级

1. 暂停旧批量任务、停止旧后端，确认没有进行中的注册交易。保存旧版本 SHA/二进制/网页、完整 config、环境配置、Redis 版本与目录。
2. 停止 Redis 后复制其**完整数据目录**（含 AOF 多文件目录）；或使用已验证的一致性备份方案。不要只复制一个可能过期的 dump.rdb。备份到独立受保护位置。
3. 备份后再替换部署代码。保留用户配置并补充新协议参数（缺省会使用安全默认值），替换弱密码/JWT。并发须 1–256，重试 0–5，登录有效期 1–30 天；自定义 API 名称须唯一、URL 为 HTTP(S)、正反匹配标识非空且不能相同。不合法配置会拒绝启动，不会静默丢弃。
4. 启动新服务，检查 `/api/web/setting` 和 `/api/openapi.json`，登录检查所有设置，再用少量公开域名测试。
5. 首次 worker 启动将旧单批任务复制到 `powerwhois:v1:*`，包括重查中断时留在 `bulkCheckErrorResult:kept` 的记录。旧键保留，未完成任务导入为 paused，需人工选择后继续。旧 DNS Free 改为 unknown，Reserved 和 Unknown 保留原始标记。导入重复执行不会重复创建。
6. 用 Web 或 CLI 检查迁移任务总数/完成数、原始响应及导出；确认后再恢复生产查询。不自动继续旧注册交易。

## 回滚

停止新后端，先导出需要保留的新任务结果；保存一份当前 Redis/配置备份。恢复升级前匹配的程序、网页、config 和 Redis 完整快照，再启动旧服务。旧数据键虽然保留，但旧版的清空操作会 FLUSHDB，因此不能把“运行旧代码指向新数据库”当成安全回滚。新版本产生的任务不能反向供旧版恢复，需保留新数据库备份或导出文件。

回滚恢复备份会回到备份时刻，不会自动合并之后的数据。保持旧/新服务互斥，防止重复查询或重复注册。

管理员退出记录保存在 Redis 的 `powerwhois:auth:generation`。回滚到旧程序或恢复旧 Redis 快照时，轮换 JWT 签名密钥后再启动服务，避免撤销过的令牌重新生效。
