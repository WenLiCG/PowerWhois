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
10. 支持API模式，你可以把程序作为一个API使用，比如接入你的WHMCS，[详见说明](https://github.com/WenLiCG/PowerWhois/blob/main/API%20documentation.md)

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
- “未找到”来自注册信息查询。DNS 无委派显示“证据不足”，注册局保留会单独显示。购买前仍需由注册商确认。
- 注册确认会生成操作编号。连接中断或上游返回不确定时，可以凭编号向注册商核对，再通过结果页面记录明确的成功或失败。留存记录由 Redis 持久化。
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

原生运行默认监听 `127.0.0.1:8080`，读取与程序同目录的 `config.yaml`。通过 `POWERWHOIS_LISTEN`、`POWERWHOIS_CONFIG` 覆盖，Redis 可用 `REDIS_HOST`、`REDIS_PORT`、`REDIS_DB`、`REDIS_USERNAME`、`REDIS_PASSWORD`。前端使用跨源开发服务器时，设置 `POWERWHOIS_WEB_ORIGIN` 为具体的客户端 Origin。`POWERWHOIS_TRUSTED_PROXIES` 接受以空格分隔的可信代理地址或网段。

## 贡献

欢迎贡献！请随时提交 Pull Request。

## 为爱发电
[爱发电](https://afdian.com/a/Powerwhois)

