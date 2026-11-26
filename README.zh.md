[English](README.md) | 中文

# PowerWhois

保留原有网页域名查询、批量任务、Typo、代理、自定义接口和管理功能，并增加不绑定 AI 厂商的 CLI、OpenAPI HTTP API 与可选 MCP。

- DNS、WHOIS/RDAP、Mixed、SOCKS5 代理和自定义查询均区分已注册、未找到、未知、超时与错误。域名支持 Unicode、Punycode、中文或全宽句点，以及单个根尾点。RDAP 保留最终来源与原文，网关 HTML 404 显示为查询错误，注册局的 RDAP 否定应答显示为「未找到」。
- 网页即时查询与 Typo 查询支持标签页草稿保留、输入整理去重、中途停止与续查、点击状态筛选、代理标识与友好中文错误提示、一键复制域名及防公式注入 CSV 导出；两页各自保留结果，跨规则重复候选自动合并，Typo 候选不含原域名。
- 批量任务支持暂停、继续、取消、重查、本页域名搜索、分页和部分导出，关闭浏览器或 CLI 后仍在服务端执行。CLI 导出 CSV 时向标准错误输出下一页输入位置，可直接将标准输出重定向到文件。
- 管理设置提供 SOCKS5 代理连通性一键测试（`POST /api/admin/proxy/test`）与清晰故障诊断（如区分 SOCKS5 端口与 HTTP 代理端口），并自动保护代理与注册商密钥不回显。
- Web、CLI、API、MCP 共用查询核心和批量任务库。
- AI 查询凭据不能注册、删数据或修改管理配置；原手动注册保留在管理员 Web，并要求确认。
- 默认只监听本机，远程使用 SSH 转发或受保护的 HTTPS 网关。

**未找到不等于可注册，DNS 无记录不等于未注册。**

点击域名可查看并复制上游原文。批量详情读取失败时，可在弹窗内重试。恢复备份后，若输入条数或结果计数不一致，该批任务会保留证据、标记失败，后续批次可继续运行。请对照一致的 Redis 备份核查异常任务后再继续。

## Docker Compose 部署

在 Linux 主机安装 Docker Engine 和 Compose v2 插件。首次部署时克隆仓库并创建私有环境文件。

```sh
git clone https://github.com/WenLiCG/PowerWhois.git
cd PowerWhois/deploy
test -e .env || install -m 600 .env.example .env
${EDITOR:-vi} .env
```

新安装保留 `POWERWHOIS_CONFIG_DIR=./runtime/backend`，并填写四项凭据。设置管理员用户名和至少 12 字符的密码，为 `POWERWHOIS_JWT_SECRET` 和 `POWERWHOIS_QUERY_TOKEN` 分别生成至少 32 字符的随机值。确保 `.env` 不进入 Git。

```sh
sudo install -d -o 10001 -g 10001 -m 750 runtime/backend
sudo test ! -e runtime/backend/config.yaml && sudo install -o 10001 -g 10001 -m 600 backend/config.yaml runtime/backend/config.yaml
docker compose config --quiet
docker compose up -d --build
docker compose ps
curl -fsS --retry 12 --retry-connrefused --retry-delay 2 http://127.0.0.1:8080/healthz
```

Compose 从源码构建后端和网页。后端容器使用 UID 10001，保存设置时需要写入挂载的配置目录。Redis 任务数据保存在 `deploy/redis/redisdata`。升级时保留原 `.env` 和配置目录。未设置 `POWERWHOIS_CONFIG_DIR` 的已有实例继续读取 `deploy/backend`。新安装需要代理或注册商时，在管理设置中填写自己的供应商。

在主机打开 `http://127.0.0.1:8080`，使用配置的管理员账号登录。端口绑定在主机回环地址。部署到远程主机时，在自己的电脑运行 `ssh -N -L 8080:127.0.0.1:8080 user@server`，再打开同一地址。可用 `docker compose logs --tail 100 typonamer caddy redis` 查看启动日志。

## Linux 原生部署

安装 Go 1.27.4、Node.js 24、npm、make 和 Redis。在仓库根目录依次执行 `make -C src setup` 和 `make -C src all`。后端、CLI 与 MCP 二进制在 `dist/backend/`，网页在 `src/frontend/dist/spa/`。部署指南包含 Redis、私有环境文件、`POWERWHOIS_WEB_ROOT`、systemd 服务，以及已有实例的备份和迁移步骤。 [原生部署、升级与回滚](docs/DEPLOYMENT.md)

## 使用文档

- [本轮审查、验收与升级注意事项](HANDOFF.md)
- [历史审查、修复和功能对照](docs/REVIEW.md)
- [CLI / API / MCP 示例、权限与 AI 环境接入条件](docs/USAGE.md)
- [测试报告及未验证事项](docs/TEST-REPORT.md)
- [OpenAPI 3.0 文档](src/backend/api/openapi.json)；运行后 `/api/openapi.json`

![已完成的批量任务、本页筛选和 CSV 导出](docs/screenshots/batch-workspace.jpg)

沿用原项目 MIT 许可，见 [LICENSE](LICENSE)。

第二轮安全与正确性修复、接口兼容变化和新增测试见 [ROUND2.md](docs/ROUND2.md)。
