# VPS 部署与代理操作

当前入口使用 Go 自带 HTTP 服务和 bbolt，**不需要 Redis、Docker 或独立 Web 服务器**。旧 `deploy/backend/typonamer`、`deploy/frontend/`、Docker Compose 和旧购买接口属于历史版本，不能当作当前工作流的发布产物。

## 构建与安装

需要 Go >= 1.26、Node 24/npm。先执行 backend 的 `go test -race ./...`、`go vet ./...`，以及 frontend 的 `npm ci --ignore-scripts`、`npm test`、`npm run lint`、`npm run build`。

在 Linux amd64 上构建：

```sh
cd src/backend
go build -trimpath -o ../../bin/powerwhois .
```

交叉编译时加 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`。将该程序、`src/frontend/dist/spa/` 和 `deploy/systemd/powerwhois.service` 上传到目标机，核对 SHA256。采用以下布局，程序和静态文件归 root 所有，服务数据由 systemd 管理：

```text
/opt/powerwhois/releases/<release>/powerwhois
/opt/powerwhois/releases/<release>/web/index.html
/opt/powerwhois/current -> releases/<release>
/etc/systemd/system/powerwhois.service
/var/lib/powerwhois/jobs.db
```

新装前检查 8080 端口、上述路径和 unit 名称是否被占用，遇到现有安装先备份 unit、旧 current 目标和停止服务后的 DB，不能覆盖其他应用。静态目录中的文件需要可读、目录需要可遍历。安装 unit 后：

```sh
systemd-analyze verify /etc/systemd/system/powerwhois.service
systemctl daemon-reload
systemctl enable --now powerwhois
curl --fail http://127.0.0.1:8080/api/v1/health
```

`Type=simple` 的 active 不等于端口已经就绪，启动或重启后必须以 health 成功作为就绪依据。可将 `/opt/powerwhois/current/powerwhois` 链接到 `/usr/local/bin/powerwhois`。

服务仅绑定 `127.0.0.1:8080`，DynamicUser 运行，3 个 worker，每注册局 host 最短间隔 2 秒，内存硬上限 768 MiB。超过限制时可能被系统终止；重启后在途任务恢复为 paused，需显式 resume。真实批量任务应按注册局政策调整限速，并先确认磁盘容量，响应证据和导出临时文件可能明显大于源工作簿。

## 打开 Web 界面

先核实 VPS SSH 主机指纹；已经记录的指纹改变时停止排查，不跳过验证。在访问端运行并保留终端：

```sh
ssh -N -o StrictHostKeyChecking=yes -o ExitOnForwardFailure=yes -L 127.0.0.1:18080:127.0.0.1:8080 <ssh-user>@<vps-host>
```

浏览器打开 `http://127.0.0.1:18080`。服务会拒绝外部 Host 和跨站 Origin；没有公网登录页，也不需要新建管理账号。SSH 登录保护远程入口，本机其他用户/进程仍处于受信边界内。

页面支持文件导入、无效行原因、候选规则和数量上限、状态对应的操作、扫描进度自动刷新、结果/输入/来源分页、完整原始证据和 XLSX 下载。取消不可恢复，暂停可续跑。

## Dot 使用哪种方式

优先使用现有 CLI，通过 SSH 在 VPS 上执行。它与 Web/API 操作同一服务、同一数据库，输出 JSON，浏览器关闭不影响扫描。不需要为此另建 MCP 服务或公开无认证 API。

```sh
# 先通过 scp 将工作簿上传到有权限读取的目录
powerwhois import --file /path/domains.xlsx --column B --header
# 保存上一步返回的 id，并先预览，避免意外启动大规模查询
powerwhois generate --job <id> --rules omission,repetition,transposition --max-candidates 100
powerwhois results --job <id> --limit 100
powerwhois start --job <id>
powerwhois status --job <id>
powerwhois pause --job <id>
powerwhois resume --job <id>
powerwhois export --job <id> --file /path/new-results.xlsx --denominator all
```

也可让本机 CLI 经 SSH 隧道调用，例如 `powerwhois.exe jobs --url http://127.0.0.1:18080`。HTTP 路由和分页游标见 [工作流说明](LOCAL-WORKFLOW.zh.md)。命令成功只表示响应成功，应检查 `state`、`completed`、`partial`、`truncated` 和 `counts`。

CLI 导出通过同目录临时文件和硬链接原子发布，已有文件或链接都不会被替换。不支持硬链接的文件系统会明确失败，可改用 HTTP/Web 下载。

Dot 的云电脑与当前 Windows 的 localhost 是不同的机器：让 Dot 直接 SSH 到 VPS，或让它在已连接的这台电脑上操作；后者需要本机在线并保持隧道。官方说明见 [Dots 的电脑与应用](https://learn.chatgpt.com/docs/dots)。不要把本机隧道地址当作云端可直接访问的公网地址。

可交给 Dot 的任务描述：

> 通过已授权的 SSH 连接在 VPS 上调用 powerwhois CLI。先列出作业，仅查询指定 id。新建任务时先导入 XLSX，生成最多 100 个候选并报告预览；只在我明确指定扫描范围后 start。保留返回的 id，检查 state 和计数，完成后导出到一个不存在的新文件。not_found 不代表可购买。不要调用购买接口，不要扩大网络访问权限。停止时用 pause；不要用 cancel 代替 pause。

本次已完成 CLI/API 和 ChatGPT 内置浏览器验证；本会话没有可调用的独立 Dot 会话，不能据此声称用户自己的 Dot 已经测试通过。

## 回滚与检查

升级时保留旧 release 和 DB 备份，停止服务、将 current 改回旧 release、启动后检查 health 和作业计数。首次安装的回滚可用 `systemctl disable --now powerwhois` 停止新服务，保留数据供复查。不要在服务运行时复制原始 bbolt 文件作为一致性备份。

```sh
systemctl status powerwhois --no-pager
journalctl -u powerwhois -n 50 --no-pager
ss -lntp '( sport = :8080 )'
powerwhois jobs
df -h /var/lib/powerwhois
```

运行依赖的 npm audit 已清零，构建工具仍有 25 项告警（20 moderate、5 high）。它们没有部署到 VPS；不要将旧 Vite 开发服务暴露到公网。消除它们需要单独迁移 Quasar CLI/ESLint 工具链，不能用 `npm audit fix --force` 盲目升级或降级。
