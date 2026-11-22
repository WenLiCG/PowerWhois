[English](./README.md) | 中文

# PowerWhois

## 网页版: [link](https://whois.1ka.net)

## screenshot
![screenshot1](./img/screen1.jpg)
![screenshot2](./img/screen2.jpg)
![screenshot3](./img/screen3.jpg)
![screenshot4](./img/screen4.jpg)
![screenshot5](./img/screen5.jpg)


## 功能介绍
1. 支持网页查询和批量文件检查，批量任务可暂停、恢复与导出 CSV。
2. 网页查询的结果点击后会弹出原始whois信息
3. 支持 RDAP、WHOIS 和 DNS。覆盖范围取决于注册局，DNS 仅提供委派证据。
4. 支持设置代理，并可以针对不同的后缀设置代理
5. 新增Mix模式，此模式下针对不同的后缀可以自由设置直连，代理，或者DNS扫描
6. 支持typo生成
7. 支持自定义whois的API，可以把自己常用的注册商的API自己加进去查询域名的可用性
8. 支持域名注册，并且自定义register的API，可以把自己常用的注册商的API自己加进去，方便注册域名
9. 后台设置可以控制线程，失败重试等各种设置
10. 支持 API 模式。设置 `POWERWHOIS_API_KEYS` 后，可用 `GET /api/v1/check?domain=example.com` 单域名查询，便于接入 WHMCS 等系统。网页使用的 WebSocket 协议也有文档。[详见说明](https://github.com/WenLiCG/PowerWhois/blob/main/API%20documentation.md)

## 目录结构

```
.
├── deploy  (部署相关)
│   ├── backend (后端程序)
│   ├── docker-compose.yml
│   ├── frontend    (前端程序)
│   ├── nginx       (Nginx配置文件)
│   └── redis       (Redis相关)
├── README.md   (说明文档)
└── src (源码目录)
    ├── backend     (后端源码)
    ├── frontend    (前端源码)
    └── Makefile
```

# 部署与升级

1. 从对应提交的 GitHub Actions 下载 `PowerWhois-linux`，或用下方命令打包。分别提供 Linux `amd64` 与 `arm64`。用 `sha256sum -c PowerWhois-linux-<架构>-<提交>.tar.gz.sha256` 校验下载，再解压。
2. 新安装在解压得到的 `deploy` 中设置 `POWERWHOIS_ADMIN_PASSWORD` 和 `POWERWHOIS_JWT_SECRET`。可导出环境变量，或使用仅部署用户可读的 Compose `.env`。默认会话有效期一天，更换 JWT 密钥会使旧令牌失效。
3. 默认应用以 UID/GID 1000 运行。使该用户可读写 `deploy/backend` 和 `config.yaml`，也可设置 `POWERWHOIS_UID`、`POWERWHOIS_GID` 使用现有部署用户。服务端配置更新通过在同目录安全替换文件完成。
4. 在宿主机 Nginx 的 `http` 配置中引入 `deploy/nginx/typonamer.conf`，替换域名和证书路径，执行 `nginx -t`。Caddy 的发布端口绑定在宿主机 `127.0.0.1:8080`，由 Nginx 对外终止 TLS。
5. 进入 `deploy` 执行 `docker compose up -d --build`。通过配置的 HTTPS 域名访问。

升级前先备份当前二进制、前端、配置和 Redis 数据，记录现网正在运行的提交。替换二进制及静态目录，按需合并代理配置；沿用原安装的配置文件与 Redis 数据目录。此分支兼容 GitHub main 原单任务 Redis 键。使用过本地多任务版或其他数据格式时，应先在隔离实例核对迁移。应用服务适合一个进程独占一套批量任务键。

## 操作语义

- 网页允许匿名直连 WHOIS、RDAP、DNS 和 Mix。付费代理和自定义接口需要管理员身份。网页连接每次处理一批，默认查询并发 15，网页每批条数由服务端配置控制。
- 匿名访客按公网 IP 限流：默认每分钟 500 个域名（设置项“匿名访客每分钟域名数”），每个 IP 最多 16 个 WebSocket 连接，并且只使用网页并发的一半。本机和内网地址不受这些限制。后端从右向左读取 `X-Forwarded-For`，在第一个非可信代理地址处停止，客户端伪造的条目不会被采用。IPv6 按 /64 限流。前面有 CDN 时，需在 Nginx 启用 `real_ip`，使 `$remote_addr` 为访客地址。不要把 Caddy 端口发布到本机回环以外：能从内网地址访问 Caddy 或后端的程序可以指定客户端地址。
- 上游返回 429 或 WHOIS 限流提示时，该上游会暂停（遵守 `Retry-After`，最长 2 分钟），开启“超时重试”时会在暂停后重试。查询超时只计算与上游通信的时间，不包括排队时间。
- 批量“重查错误域名”只重查查询失败的行，DNS 证据不足和注册局保留的结果保持不变。
- 在每个 WHOIS、RDAP 或 DNS 上游内部，匿名查询最多占用共享名额的一部分（3 个中的 2 个，或 4 个中的 3 个），管理员、批量任务和 API 始终有余量，对注册局的总请求数不增加。
- 匿名访客共用 64 个 WebSocket 连接。已登录管理员的浏览器先通过 HTTPS 申请一次性票据，再用票据连接，进入 8 个管理员专用连接，匿名连接占满时管理员仍能连接。
- “退出登录”会使所有浏览器中已签发的管理员令牌失效，关闭管理员专用连接，并停止进行中的注册批次提交尚未发出的域名；已经发出的请求仍会收集并保存结果。
- “未找到”来自注册信息查询。DNS 无委派显示“证据不足”，注册局保留会单独显示。购买前仍需由注册商确认。
- 注册确认会生成操作编号。连接中断或上游返回不确定时，可以凭编号向注册商核对，再通过结果页面或“管理 → 待核对注册”记录明确的成功或失败。刷新页面后仍可在“待核对注册”中找到这些操作。留存记录由 Redis 持久化。
- 批量上传支持 UTF-8 文本，每行一个域名或 HTTP(S) URL。上限 100 MiB、100 万行。暂停和取消在 worker 结束后显示终态。CSV 按首次出现顺序导出已保存行。

# 开发与打包

需要 Go 1.27.4、Node 22、Yarn 1.22.22 和 Linux 工具链。

```bash
make -C src setup
(cd src/backend && go test -race ./... && go vet ./...)
(cd src/frontend && yarn lint && yarn test)
make -C src package GOARCH=amd64
```

打包在已提交的源码检出中运行，产物写到 `dist/`，包含版本、校验文件和完整的 `deploy` 目录。CI 同时构建 arm64 包。`make -C src all` 可将本地构建写入 `deploy` 目录，用于开发验证。

原生运行默认监听 `127.0.0.1:8080`，读取与程序同目录的 `config.yaml`。通过 `POWERWHOIS_LISTEN`、`POWERWHOIS_CONFIG` 覆盖，Redis 可用 `REDIS_HOST`、`REDIS_PORT`、`REDIS_DB`、`REDIS_USERNAME`、`REDIS_PASSWORD`。前端使用跨源开发服务器时，设置 `POWERWHOIS_WEB_ORIGIN` 为具体的客户端 Origin。`POWERWHOIS_TRUSTED_PROXIES` 接受以空格或逗号分隔的可信代理地址或网段，格式错误时拒绝启动；Compose 默认信任私有网段，未设置时只信任本机回环，适合同机 Nginx 之后的原生部署。`POWERWHOIS_API_KEYS` 设置集成 API 密钥（每个至少 24 个字符，逗号分隔，未设置时接口返回 404）。`POWERWHOIS_LOG_STDOUT=true` 让日志同时输出到标准输出（Compose 已开启）。健康检查地址为后端的 `/healthz`，不经 Caddy 对外暴露。

## 贡献

欢迎贡献！请随时提交 Pull Request。

## 为爱发电
[爱发电](https://afdian.com/a/Powerwhois)

