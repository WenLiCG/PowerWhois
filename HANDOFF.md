# PowerWhois 程序员交接文档

## 当前交付 · 2026-11-26 代理链路修复、性能加固与网页体验精修

基线为 GitHub `main@7663175`（PR #16 合并提交）。本地 `main` 原位于 `cfb3026`（落后 `origin/main` 6 个提交，无未提交修改、无本地独有提交或 stash），已快进同步到 `origin/main` 作为工作基线。本轮围绕全库审查、真实 Bright Data SOCKS5 代理实网验证、性能根因修复与真实用户交互精修完成端到端交付，无新增第三方依赖。

| 影响 | 问题与现行行为 |
|---|---|
| 高 · Verisign RDAP 404 | 实网验证发现 Verisign `.com`/`.net` RDAP 服务器在返回合法 TLS 校验通过的 `HTTP/1.1 404 Not Found`（`Content-Type: application/rdap+json`，无 `Content-Length`）后关闭空正文流时触发 `tls: bad record MAC`，导致 `openrdap` 将所有未注册 `.com`/`.net` 域名误判为 `error`（`No RDAP servers responded successfully`）。现在当已认证 RDAP 响应明确返回 `404` 且 `Content-Type` 为 RDAP/JSON 时准确归类为 `not_found`；非 RDAP 的 HTML 404 读错误仍保持 `error`。 |
| 高 · RDAP Bootstrap 缓存失效 | `safeRegistryCache.State()` 透传 `MemoryCache.Good`，但每次 RDAP 查询都新建 `bootstrap.Client`（其内部 `registries` 为空），`openrdap` 仅在收到 `ShouldReload` 时才调用 `Cache.Load()`，导致每次 RDAP 查询都重复从 `data.iana.org/rdap/dns.json` 下载约 200 KB 文件。现在缓存有效时返回 `ShouldReload`，实现跨请求内存复用（24 小时 TTL），实网 RDAP 代理耗时显著降低。 |
| 高 · SOCKS5 代理错误分类与连通性测试 | 原实现将 SOCKS5 认证失败、规则拒绝和误填 HTTP 端口（返回 ASCII `72`/`H`）统一包为 `whois server failed`（且 `openrdap` 在 `NoWorkingServers` 中吞掉底层 `h.Error`），并缺少配置前校验与测试入口。现在统一归类为 `ErrorConnectToProxy` 并附可操作的中文诊断（如识别 HTTP 端口误填、凭据/规则拒绝、超时，且强制脱敏防止代理密码进入日志/结果）；确定性代理认证/协议错误不再无效重试；后端校验并规范化代理主机（自动剥离 `socks5://`/`socks5h://`，拒绝 `http(s)://`）；新增 `POST /api/admin/proxy/test` 固定探测 `rdap.verisign.com:443` 的 TLS 握手连通性（需管理员 JWT，支持测试未保存草稿，不暴露任意目标 SSRF）。 |
| 高 · 协议超时计时边界 | WHOIS、RDAP 与 DNS 此前在 `network.Acquire` / `network.Wait` 排队前就启动单次协议超时，高并发同注册局限速时后排请求可能在等待限流期间误超时。现改为获取并发名额（WHOIS 含速率等待）后再启动单次尝试超时，整体每域名总预算仍由外层 `lookuper.Query` 严格控制；网页即时查询总时限改为 `min(120, max(30, WhoisTimeout, RdapTimeout, DnsTimeout))`。 |
| 中 · 事件驱动并发信号量 | `network.Acquire` 原每 10 ms 轮询一次名额，存在空转 CPU 与定时器分配开销。现改为通道广播唤醒（释放名额与配置变更时即时唤醒），保留匿名/认证名额隔离语义，零空转轮询。 |
| 中 · WHOIS 解析与安全加固 | `ParseWhoisResponse` 增加 `defer recover()` 保护，修复畸形 `Domain Status`（如仅 `http` 或 `-`）与空白 `Name Server` 切片越界 panic，补齐无 NS 的状态域名（如 `.de` `Status: connect`）识别并校验 `carbon` 错误状态；`rdap_parse` 补充 VCard 无 `fn` 时回退 `Handle`；`/api/v1` 查询令牌与 HTTP MCP 令牌改用定长 SHA-256 摘要常量时间比对；`/api/admin/setting` 与 `/api/admin/proxy/test` 统一输出 `Cache-Control: no-store`。 |
| 中 · 前端代理体验与 UI/UX 精修 | 系统设置页新增顶部「保存配置」快捷按钮、代理配置/密码状态徽标、SOCKS5 端口提示与「测试 SOCKS5 代理」按钮（返回毫秒耗时或中文故障诊断）；首次开启代理认证自动切换密码为可填写状态；查询方式选择 `Whois + 代理` 且未配置代理时给出温和提示；新增 `formatError.js` 将常见英文错误翻译为清晰中文（详情弹窗保留原文）；网页与 Typo 查询保留标签页切换草稿并在退出登录时清空；即时结果表去除与状态标签重复的下拉筛选、支持点击域名查看结构化详情（含 `代理` / `经由 SOCKS5 代理` 标识与格式化时间）；批量页新增「搜索本页域名」；修复网页即时输入含空行时的报错行号偏移、优化 1280px 桌面表格列宽与 390px 窄屏批量按钮排版，并修复日志下载缺少 `filename=` 响应头时的异常。 |

**本轮验收**（Windows 11 + WSL2 Ubuntu，Go 1.27.4，Redis 8.0.5，Node v24.14.0，npm 11.9.0）

- **后端**：全部 `.go` 文件按 LF 规范通过 `gofmt` 检查、`go mod verify` 通过、`go vet ./...` 零警告、`go test -race -count=1 ./...` 全部 14 个测试包通过（含新增事件唤醒、代理错误分类/脱敏、Bootstrap 缓存复用、Verisign 404 TLS 容错、WHOIS 防 panic、代理配置校验与 `/api/admin/proxy/test` 回归测试；3 个缺外部原始附件用例按设计跳过）。Linux 三个二进制（`typonamer`、`powerwhois`、`powerwhois-mcp`）与 Windows amd64 交叉构建（`powerwhois.exe`、`powerwhois-mcp.exe`）全部成功。
- **跨入口与压力验收**：`python3 tools/acceptance.py --stress` 通过，覆盖单活租约、API/CLI/stdio MCP/HTTP MCP/管理员 Web 一致性、300 域名 32 并发、暂停/继续/取消、20 次并发幂等重放、重启与 `SIGKILL` 恢复、日志零密钥扫描。
- **漏洞扫描**：`govulncheck ./...` 可达调用 0、导入包级漏洞 0（模块层仍仅列未导入的 `GO-2026-5932`）；前端完整与生产 `npm audit` 均为 0 vulnerabilities。
- **前端**：`npm test` 通过 24/25 项（1 项缺外部大 TXT 附件按设计跳过），`npm run lint`（ESLint）0 错误 0 警告，`npm run build`（Quasar/Vite SPA）成功。
- **真实 Bright Data SOCKS5 代理实网验证**：使用测试账号配置 `brd.superproxy.io:22228`（验证 `socks5h://` 规范化、远程 DNS 与认证），实测 `TestSOCKS5Proxy` 成功（约 2.7–3.0 s）；经代理查询 `example.com`（RDAP `registered`）、`example.org`（RDAP `registered`）、`powerwhois-nonexistent-987654321.com`（Verisign RDAP 404 → `not_found`）、`powerwhois-nonexistent-987654321.org`（PIR RDAP 404 → `not_found`）、`baidu.cn`（WHOIS 43 `registered`）、`powerwhois-nonexistent-987654321.cn`（WHOIS 43 `not_found`）、`denic.de`（WHOIS 43 `registered`）全部准确；验证错误密码与误填 HTTP 端口 `44445` 均返回明确中文代理诊断，日志与结果中零密码泄漏。
- **真实浏览器全链路验收（Chromium）**：使用新构建的 `typonamer` + 生产 SPA + 隔离 Redis 在 `http://127.0.0.1:18080` 完整走通 6 个阶段（桌面 `1280×800` 与移动端 `390×844`）：首页加载与空行行号校正、直连 RDAP（含 `.com` 404 → `未找到`）与结构化详情、管理员登录与未配置代理提示、系统设置错误端口 `44445` 诊断与 `22228` 实网测试通过及保存、代理即时查询与标签页草稿保留、Typo 排除原域名、批量 TXT 上传代理任务执行/本页搜索/详情查看、390px 零横向溢出、退出所有管理会话；全过程控制台报错 0，服务端日志密钥泄漏检查全为 `False`。
- **未验证边界**：本机未安装 Docker，Compose 栈交由 GitHub Actions `compose` 作业验收；外部附件工作簿与 563k 行 TXT 不在本机；未调用真实付费注册商下单；测试网络直连 UDP 53 权威 DNS 存在超时（Typo DNS 用例在 5 项候选上均触发超时/错误反馈正常收口）。

**后续建议**

- 历史任务和原始响应目前不自动过期清理，长期高频使用时建议监控 Redis 内存/磁盘并按需补充保留期清理命令。
- 若后续有仅提供 HTTP CONNECT（无 SOCKS5 端口）的代理供应商需求，可再按需扩展代理协议选项；当前保持仅 SOCKS5 以完整支持端口 43 WHOIS 与远程域名解析。
- 真实付费注册商沙箱下单与具体云端 AI 宿主账号级 MCP 接入仍需在授权现场另行验收。

升级时前后端同步替换即可，无需数据迁移。

## 历史交付 · 2026-11-25 端到端复审与网页体验修复

基线为 GitHub `main@6dc09b7`（PR #15 合并提交）。本机 7 份历史检出逐一核对：PW-5 至 PW-8 的本地分支与远端同名草稿分支树完全一致，11-22 实现分支的两个提交与 main 上的提交补丁等价，旧部署目录只有重新编译的二进制改动，没有本地独有工作，因此以 GitHub main 为准。草稿 PR #5、#6、#8、#9 仍未合并，本轮未改动。PW-8 记录的 `.cn` 日期解析 panic 在当前 main 上已不复现，解析器显式使用 UTC。

| 影响 | 问题与现行行为 |
|---|---|
| 中 · 下载与日志时间 | 发布构建（`-trimpath`）和 Compose 精简镜像都没有时区库，默认时区又是新版 Debian tzdata 已移除的旧别名 `PRC`，导致日志 ZIP 与旧批量 CSV 的文件名缺少时间（`typonamer_logs_.zip`），Compose 的 `TZ=Asia/Shanghai` 不生效，日志时间为 UTC。现已嵌入 Go 时区库（二进制约增加 0.4 MB），并改用 `Asia/Shanghai`。 |
| 中 · 容器日志 | README 让用户用 `docker compose logs typonamer` 排查启动问题，但后端只写容器卷里的文件。恢复单任务版已有、多任务合并时丢失的 `POWERWHOIS_LOG_STDOUT`，Compose 默认开启；文件日志仍供网页下载。 |
| 中 · Typo 候选 | 原域名会作为自身的变体出现：默认选中的 com 后缀，以及互换 `oo` 这类重复字母，都会生成原域名，所以每次默认 Typo 查询都多查一次。现在各分组和查询列表都排除原域名；排除后若没有候选，会给出明确提示。 |
| 中 · 生命周期 | 有 client/server *Prohibited 锁定时，注册局不返回 `ok`（example.com、example.org 实测如此），这类最常见的已注册域名因此显示为 Unknown。现在只含 delete/renew/transfer/update 锁定的显示为 Active；Hold、PendingDelete 等状态仍然优先，autoRenewPeriod 等组合仍为 Unknown。批量页未注册的行显示“—”，详情中附原始状态。 |
| 中 · 网页查询与 Typo | 两页共用一个结果仓库：在一页开始查询会清空另一页的结果，另一页的进度和停止按钮会出现在本页，结果列跟着查询方式选择器变化，而不是跟着结果本身。现在每页只替换自己的结果，重查只影响发起的那一页，行 ID 全局唯一（注册按 ID 选行）；另一页占用时显示说明，结果列按实际产生结果的查询方式显示。 |
| 低 · 批量上传报错 | 文件含空行时，报错行号会偏移（实测第 6 行报成第 3 行），而且提示只有英文。现在服务端错误附带无效的原值，网页再换算成文件里的实际行号，并给出中文说明。 |
| 低 · 资源 | 批量页无论有无进行中的任务，都每 2 秒请求 3 个接口。现在没有活动任务时改为 15 秒一次，有活动任务时仍为 2 秒。同时删除了不再使用的 bulkStore：它每秒更新一次，但没有任何界面读取。 |

**本轮验收**（Linux，Go 1.27.4，Node 24.22.1，Redis 8.0.2）

- 后端：gofmt 无输出，`go mod verify` 和 `go vet` 通过；`go test -race -count=1 ./...` 14 个包全部通过，共 54 个顶层用例，其中 3 个外部附件用例因缺少原始文件按设计跳过。govulncheck 结果：可达调用 0、导入包 0，模块层仍列出未导入的 GO-2026-5932。
- `tools/acceptance.py --stress` 通过，覆盖 API、CLI、stdio 与 HTTP MCP、300 域名并发、暂停/继续/取消、20 次并发幂等、SIGKILL 后恢复，以及日志中无秘密。
- 前端：`npm ci` 后 24 项测试 23 项通过、1 项（外部附件）跳过；ESLint 0 问题；生产构建成功；完整与生产两种 npm audit 均为 0。
- 时区：用 `-trimpath` 分别构建原 main 和本分支，并在屏蔽 `/usr/share/zoneinfo` 的环境下运行。原版下载文件名为 `typonamer_logs_.zip`、日志为 UTC；本分支为 `typonamer_logs_20261125225436.zip`、日志为北京时间。
- 浏览器：Chrome 加载生产 SPA，连接隔离的后端与 Redis，并做真实 RDAP 查询，10 项检查全部通过，包括 example.com 显示 Active、切换查询方式不改变已有结果列、Typo 运行时网页查询结果保留并显示占用说明、Typo 候选不含 google.com、上传错误指向第 6 行、批量生命周期列、空闲 12 秒内 0 次列表请求、390/320px 无横向溢出、无控制台错误。另在该实例上验证了 CLI 同步查询、Typo、无效批量（退出码 2）和 OpenAPI。
- 未验证：本机没有 Docker，Compose 交由 CI 的 compose 作业验证。本沙箱屏蔽 UDP 53 与 TCP 43，DNS/WHOIS 实网查询未测，相关逻辑由单元测试和模拟上游覆盖。`POWERWHOIS_TYPO_REFERENCE` 工作簿不在本机，Typo 参考比对未重跑；若工作簿把原域名列为它自己的变体，这些行现在会按设计报为缺失。

**后续建议**

- `network.Acquire` 每 10 ms 轮询一次上游名额，46 个协程同时等待时约占单核 3.7%。需要省 CPU 时，可改为通知式信号量，同时保留匿名配额。
- 即时查询的错误说明仍直接显示上游原文（如 `context deadline exceeded`）。
- 历史任务没有自动清理。
- 真实代理、注册商和 AI 宿主接入仍需现场验收。

升级时前后端一起替换，无需迁移数据；Compose 用户重建镜像后即可在 `docker compose logs typonamer` 看到应用日志。

## 历史交付 · 2026-11-25 查询证据、批量导出与网页验收

本次以 GitHub `main@cfb3026` 为基线。原工作目录 `4b6d816` 比它落后 26 个提交，没有独有提交；两个未跟踪的历史文档保留在原目录。以下结果来自本轮检查，后文保留各历史版本的交接记录。

| 影响 | 问题与现行行为 |
|---|---|
| 高 · 查询证据 | RDAP 有多个候选端点时，先前的 429 曾遮盖最后的成功或 404。现在按最终响应判断，并保存最终 URL 和响应正文，包括成功时的原始 JSON。新旧记录可并存。自动化应读结构化字段，详情可读原文。Unicode/Punycode 后缀及其路由规则统一匹配；名为 `rdap` 的自定义接口也能保留自己返回的“未找到”。 |
| 高 · 批量资源 | 原实现一次读取至多 100 条完整原文后才裁剪，轮询摘要也可能触发大传输。现在按约 8 MiB 拆分 Redis 回复，单个较大的结果单独处理，保留稀疏输入游标。16 MiB 以上的原文请求报错；老记录的摘要仍可能需要单独载入一整条记录。沿用已有 Redis 键和 ACL 命令。 |
| 中 · 查询与任务操作 | 网页等待服务端确认后再替换上一轮结果。被拒的查询保留既有证据，停止/重查有独立的状态反馈。注册确认固定域名和注册接口，取消任务确认固定任务 ID。批量页显示本页筛选计数，换页或过期请求不会把前一页的数据套到新选择。窄屏管理菜单可访问完整用户名。 |
| 中 · 导出与时间戳 | CLI 的 CSV 标准输出可直接落盘，标准错误提供 `nextOffset=N`。调用方跟随游标，遇 `-1` 结束。任务控制立即返回持久化的更新时间，重启保留未发生状态变化的任务时间。OpenAPI 明确同步查询最多 100 个输入，持久任务最多 100 万。 |
| 中 · Compose 网关 | 缺失的脚本曾得到带 60 天缓存的首页。现在旧资源 URL 返回 404；存在的带哈希构建资源可长期缓存。前端页面也带防嵌入与内容类型保护头，SPA 深链接继续可用。 |

**本轮验收**

- WSL Ubuntu、Go 1.27.4、Redis 8.0.5。Linux 换行源码通过 gofmt、模块校验、vet 和全包 race 测试，49 个顶层用例通过。3 个外部附件用例缺少原始输入。Linux 服务端、CLI、MCP 以及 Windows amd64 CLI/MCP 均完成构建。受限 Redis 输出缓冲下，超过 40 MiB 的结果可分页读取。
- 隔离实进程通过 `tools/acceptance.py --stress`，覆盖 API、CLI、stdio/HTTP MCP、300 域名并发查询、任务暂停/恢复/取消、20 次并发幂等重放、重启与 SIGKILL 后恢复。公开网络只读查询 `example.com` 得到 Verisign 的已注册记录；国际化域名样例命中目标 RDAP 端点后收到上游错误。
- Node 24 通过 `npm ci`、21 个前端测试、ESLint 和生产构建，完整与生产 npm 审计均报告 0 项。外部大 TXT 对应的另一个上传用例缺少原始文件。前端为 JavaScript/Vue，仓库没有单独的类型检查命令。Go 漏洞扫描发现 0 个受影响调用、0 个受影响导入包，模块层仍列出未导入的 OpenPGP 公告 GO-2026-5932。
- Chromium 操作生产 SPA 和隔离后端，完成登录、即时与 Typo 查询、服务端拒绝后保留结果、停止和重查、TXT 上传、101 个域名跨页筛选、详情、CSV 下载、系统设置和退出。1280、390、320 像素宽度检查通过，注册确认使用模拟调用。独立 Caddy 进程已实测深链接、静态资源、缺失资源及响应头。全新三容器安装由仓库的 Compose CI 检查。[工作流](.github/workflows/check.yml)

升级时连同前端一起部署。已有任务无需转换。现场 VPS 部署、商业 SOCKS、付费下单、物理手机及云端 AI 账号本轮均未验收。大量历史查询原文仍占用 Redis 与备份空间。继续使用原有备份和回滚流程。[部署指南](docs/DEPLOYMENT.md)

审查改动已由 PR #14 合入 main（`1e9bb9b`），合并提交的 verify 和 compose 均通过。该次运行提示旧版 checkout/setup Action 使用已弃用的 Node 20，后续工作流已改用声明 Node 24 的官方版本。项目构建仍使用 Go 1.27.4 和 Node 24。[合并记录](https://github.com/WenLiCG/PowerWhois/pull/14) · [主分支验收](https://github.com/WenLiCG/PowerWhois/actions/runs/45631087926)

## 历史交付 · 2026-11-25 审查与体验优化

前序基线为 GitHub `main@4a646fc`（PR #13 合并提交）。本轮在通读全库源码、测试、部署配置与交接历史的基础上完成端到端审查、根因修复、性能与安全加固、Web UI/UX 打磨及全链路实测。架构保持单租户 Go/Fiber + Redis + Vue/Quasar，Web、CLI、HTTP API（`/api/v1`）与 MCP 共用同一查询核心与持久化任务库，无新增第三方依赖。本节以下按日期保留历史版本的交接记录。

| 维度 | 本轮根因修复与优化内容 |
|---|---|
| 高 · 查询正确性与限流 | `dnslib` 修复大小写敏感的 `ns.Hdr.Name` 匹配导致的已注册域名漏判，统一线程安全的 `TldNsCache` 切片克隆读取并将最大委派层级提升至 5；`whoislib` 新增端口 43 限流文本识别（返回 `ErrorRateLimited` 并触发 5s 退避，同时确保含限流声明页脚的正常已注册响应不受影响），支持带时区偏移的 ISO-8601 时间戳统一转 UTC；`network/limits` 为无 `Retry-After` 头的 HTTP 429 补充默认 2s 退避，并将单次 `Retry-After` 上限封顶为 60s，防止异常上游长时间挂起工作协程。 |
| 中 · 性能与资源效率 | `rdaplib` 复用直连 HTTP Transport 连接池并使用带读写锁的内存 Bootstrap 缓存替代逐次新建 `DiskCache`，减少重复 TLS 握手与磁盘/只读目录开销；`jobs.Store.List` 改为 Redis Pipeline 批量读取元数据与计数，`Store.Results` 改为 100 条分块的 `HMGet`（保留原有 8 MiB / 16 MiB 字节预算边界）；`scheduler/bulk_check` 在无管理员 WebSocket 订阅时跳过每秒 Redis 轮询；`database/redis` 支持启动期 `ctx` 取消中断并在连接失败时关闭客户端；`BulkCheck.vue` 在浏览器标签页隐藏时暂停轮询、切回时立即刷新。 |
| 中 · 实用安全加固 | `api/auth` 登录凭据改为固定长度 SHA-256 摘要的 `subtle.ConstantTimeCompare` 常量时间比对；WebSocket `/app/ws` 与 MCP HTTP `/mcp` 收紧 `Origin` 校验（要求合法 `http/https` 且拒绝 `Origin: null` 沙箱内嵌帧）；服务端统一输出 `X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`、`Referrer-Policy: same-origin`；清理 `src/frontend/.env` 遗留局域网地址为 `127.0.0.1:8080`。 |
| 中 · Web 流程与 UI/UX | 重整顶栏、连接状态胶囊（断线支持一键重连）、管理员身份标识与卡片化工作区；网页与 Typo 查询补充前置格式校验（避免误粘 URL 清空已有结果）、一键整理去重（统一小写并去尾点）、`Ctrl+Enter` 快捷键、实时进度展示及中途「停止查询」（新增 WebSocket `webCheckStop` / `webCheckStopped`，未完成项标为未知且支持后续一键续查）；修复跨 Typo 规则生成同域名时的重复行与重复计数问题（合并显示如 `Reverse Letter / ccTLDs`），修复 `isErrorRecheck` 状态残留；结果表支持点击状态标签筛选、一键复制当前筛选域名、导出防公式注入 CSV 及复制原始响应；批量页补充可读模式名、到期日与生命周期列及详情复制；设置页修复环境变量托管 `authUsername` 禁用提示、回车误提交表单、替换词尾空行报错及自定义接口弹窗无表单校验等问题。 |

**本轮实测结果。**
- **后端（WSL Ubuntu, Go 1.27.4, Redis 8.0.5）**：`gofmt -l .` 无输出、`go mod verify` 通过、`go vet ./...` 零警告、`go test -race -count=1 -v ./...` 全部 13 个测试包 **49 项测试 PASS**（0 失败，3 个需外部大附件环境变量的可选回归按设计 SKIP）。编译 Linux（`typonamer`、`powerwhois`、`powerwhois-mcp`）与 Windows amd64（`powerwhois.exe`、`powerwhois-mcp.exe`）二进制均成功。`GOTOOLCHAIN=go1.27.4 go run golang.org/x/vuln/cmd/govulncheck@v1.10.0 ./...` 可达调用与导入包级漏洞均为 **0**（模块层仍仅提示未导入的 `golang.org/x/crypto` OpenPGP 公告）。
- **跨入口与故障恢复验收**：`python3 tools/acceptance.py --stress` 通过，覆盖单活租约互斥、HTTP API / CLI / stdio MCP / HTTP MCP 四通道任务 ID 与 JSON/CSV 导出一致性、300 域名 32 并发吞吐、暂停/继续/取消部分结果保留、20 并发幂等重放、服务重启与 `SIGKILL` 崩溃后租约恢复，以及日志无凭据泄漏扫描。
- **前端（Node v24.14.0, npm 11.9.0）**：`npm test` 通过 **16 项单测**（0 失败，1 项外部附件测试按设计 SKIP），`npm run lint`（ESLint）零错误零警告，`npm run build`（Quasar/Vite SPA）构建成功，`npm audit`（完整依赖与 `--omit=dev` 生产依赖）均为 **0 vulnerabilities**。项目前端为纯 JavaScript/Vue 3，无独立 TypeScript 编译检查脚本。
- **真实浏览器交互验收（Chromium）**：基于生产构建 SPA 与隔离后端/Redis，在桌面端（`1280×800`）与移动端窄屏（`390×844`）完整验证匿名访问、非法 URL 拦截、整理去重、管理员登录（含密码显隐与会话标识）、即时查询、状态筛选、原始证据弹窗与复制、Typo 全选/清空与跨规则去重合并、批量任务进度与详情、系统设置环境变量托管提示与自定义接口弹窗校验、配置保存及退出全部管理会话，全程零控制台报错、窄屏无横向溢出。

**已知边界与后续建议。** 当前架构面向个人或信任的小团队共享使用，无多租户隔离与历史任务自动过期清理；真实商业 SOCKS5 代理、真实付费注册商下单及云端 AI 宿主账号级连接需在现场授权环境另行验收。

## 历史交付（2026-11-25 基线）

基线为 `main@ec0af62`（PR #13 合入后为 `main@4a646fc`）。Go/Fiber 服务提供查询、管理接口和 WebSocket，Redis 保存任务、租约、结果与注册审计。Vue/Quasar 网页、CLI 和 MCP 共用这套后端。服务面向个人或信任的小组，同一批量任务库由所有获授权的查询客户端共享。本节以下按日期保留历史版本的交接记录。

| 影响 | 现行行为与交接事项 |
|---|---|
| 高 · 部署 | 修复了 Compose 样例 YAML 无法加载的问题。全新安装按 README 从模板复制可写配置，配置及 Redis 运行数据均有忽略规则。旧环境未设置 `POWERWHOIS_CONFIG_DIR` 时继续使用原挂载。备份已有配置再升级，勿用空模板覆盖。通过网关请求 `/healthz` 应得到 `ok`。 |
| 高 · 数据迁移 | 首次导入旧单批数据时，遇到重名归并或同域名结果冲突会在发布任务前退出，保留源键。先在备份副本核对。已有迁移标记的任务不会重写。服务无法取得 worker 租约或绑定端口时退出非零。 |
| 高 · 注册 | 等到注册商有并发名额后才开始十分钟防重期限。实际提交前复核会话，断线后网页将未收到回执的域名标为待核实。先按域名和审计信息到注册商查订单，再决定后续操作。 |
| 中 · 查询与导出 | 修复中文后缀 WHOIS 路由和日期解析、Typo 后缀校验。缺少有效 SOCKS 配置会直接显示原因。新任务只规范化一次，默认并发取 15 与服务端上限的较小值，错误重查也服从当前上限。旧版整包 CSV 分页生成，CSV 和日志 ZIP 各自使用私有临时文件，需预留临时磁盘。 |
| 中 · 网页操作 | 设置表单可保存默认并发和空代理，加载失败可以重试，取消编辑会丢弃草稿。登录、会话失效、注册断线分别有明确反馈。重查保留注册结果。 |

**本轮实测。** Linux/WSL 使用 Go 1.27.4 通过 `go vet ./...`、全包 race 测试、模块校验和三个程序的构建。常规套件通过 42 个顶层用例，另在独立 Redis 上补跑全部 3 个外部附件用例。全量 TXT 有 563,807 项，逐项核对 554,867 个有效唯一结果，共 555 页。Typo 命中全部 2,234 条有效参考。Node 24 的 15 项测试（含原始附件）、ESLint、生产构建以及完整和生产 npm audit 均通过。前端为 JavaScript，项目没有单独的 TypeScript 检查脚本。govulncheck 的可达调用与导入包结果均为零，依赖模块层仍报告未导入的 OpenPGP 公告 GO-2026-5932。

实际进程通过 API、CLI、stdio/HTTP MCP、并发幂等、暂停/继续/取消、重启和 SIGKILL 后恢复验收。Chromium 对生产 SPA 和本地后端完成登录、设置、即时查询、Typo、上传、任务控制、详情、下载和退出，覆盖桌面及 390/320px 窄屏。注册测试仅使用本地模拟服务。另用真实网络查询 `example.com`，DNS 与 RDAP 均返回已注册。GitHub 的独立 Linux runner 已从源码构建并启动全新 Compose 栈，经 Caddy 验证网页、设置 API 和 Redis 健康探针。本地还验证了两种配置挂载路径、Caddy 与 Nginx 的配置，以及 Windows CLI/MCP 交叉构建和帮助入口。[Compose 验收记录](https://github.com/WenLiCG/PowerWhois/actions/runs/45561447309/job/139025867592)

现场升级前应验证目标机的备份与恢复，另行验收商业代理、注册商沙箱、实际移动设备以及所用 AI 客户端。本轮未部署到生产 VPS。

## 主分支交接（2026-11-23）

当前部署基线为 `main` 的 Web、CLI、HTTP API 和 MCP 多任务版。通过 [中文 README](README.zh.md) 从源码构建，使用 [部署指南](docs/DEPLOYMENT.md) 备份和迁移旧单批 Redis 数据。已有自动化可以使用 [当前 OpenAPI](src/backend/api/openapi.json) 的 `POST /api/v1/query` 和任务接口。

下方记录对应合并前的单任务版本。其 WebSocket 票据、待核对注册 API 与打包说明对应 [50ef89d 的代码](https://github.com/WenLiCG/PowerWhois/tree/50ef89d30098020da0311aed94aa5e31615ac57d)。迁移前请完成已有待核对注册，保留 Redis 备份和旧程序供核查。

## 独立验证补记（2026-11-22，Cursor，不是补丁作者）

下面「审查修复」一节是作者在本地候选上的自述，原文保留。补丁已用 `git am` 应用到远端 `main` `6662f8ae55447140152119a90b0cf9399159bb21`，作者身份仍是 WenLiCG，提交说明保留 `Co-Authored-By: Claude Opus 5.5`。源提交与本仓库提交不是同一个 SHA：

| 源候选 | 本仓库提交 | tree |
|---|---|---|
| `b0932238c7c358dd9c52d4e4767986aaafbd012f` | `a972b8974d74b92ed852160196d22faa88e2dd94` | `dee5cb71dc63032201f57639a1139a1a73be6dd0` |
| `de0fad6913136724e963e6bf135f1abd42f47ea2` | `ef816e382e5ccdaa881f40bc6f931fe252d2701e` | `b3bfc248b5862a4fdfe85443572e5350021e1470` |

独立验证结论、命令、时间和未测项写在 [docs/handoff/2026-11-22-review-fixes.md](docs/handoff/2026-11-22-review-fixes.md) 第 9 节。验证记录这次提交的父提交是 `ef816e38`；精确 PR head 写在拉取请求正文，不在这里预写。未合并，未部署。

## 审查修复（2026-11-22，待独立验证，未合并）

基于 `main@6662f8a`，分支 `pw-cli-impl-20261122` 只在本地提交：第一个候选 `b093223` 经独立验证为 NO-GO（F1–F4），其后的一个提交修复了这些问题，是新的候选。**新候选尚未独立验证，未推送、未创建拉取请求、未合并。** 本地在隔离 Redis 下：gofmt 无输出，go vet 退出码 0，`go test -race -count=1 -v ./...` 退出码 0（42 个顶层测试通过，0 失败，0 跳过，无数据竞争）；前端 lint、test（4/4）、build 退出码均为 0。Compose 未用 Docker 实际校验和启动，新票据连接流程未在真实浏览器中验证。完整说明、验证结果、行为变化、回滚步骤与合并前检查见 [docs/handoff/2026-11-22-review-fixes.md](docs/handoff/2026-11-22-review-fixes.md)。

验证问题的修复：管理员先通过 HTTPS 申请一次性票据，再用票据连接进入 8 个管理员专用 WebSocket 名额，匿名连接占满 64 个公共名额时管理员仍可连接（F1）；退出登录后，进行中的注册批次不再提交尚未发出的域名，已发出请求的结果照常保存（F2）；每个 WHOIS/RDAP/DNS 上游内，匿名查询最多占用 3 个中的 2 个或 4 个中的 3 个，其余保留给管理员、批量和 API，总并发不变（F3）；待核对注册按“操作编号 + 域名”识别行（F4）。同时修复了原有缺陷：fasthttp 劫持连接上的 `Close()` 不生效，导致关闭连接和停机排空无效。回滚前不要删除 `bulkCheckErrorResult:kept`，并应轮换 JWT 密钥。

要点：后端从右向左解析 `X-Forwarded-For`，只信任可信代理追加的条目，伪造条目无法绕过限流，IPv6 按 /64 限流；Caddy 与后端信任私有代理网段，登录与连接限流按真实客户端计算；匿名访客按公网 IP 限流并使用独立的一半网页并发；查询超时从取得上游名额后开始计时；上游 429 或 WHOIS 限流提示会暂停该上游并重试；批量“重查错误域名”只重查 `Error` 行；批量操作不再阻塞 WebSocket 读循环；新增待核对注册列表、退出登录撤销全部令牌、登录失败改为 401、`/api/v1/check` 集成接口、`/healthz` 与容器标准输出日志。

## 当前交接（2026-11-22）

本期交付 PowerWhois 的域名查询、批量任务、注册及部署修复。交接分支为 `codex/powerwhois-review-fixes`，基于 GitHub `main@6f7dac40cf2c94d24afa492da0baaeccfbc1d006`。已验收的应用与部署包对应提交 `6551edc7f22ead74081721c9293ecaf4f8a4ae93`。交接文档在该版本之后提交。

### 变更和兼容性

- [查询入口](src/backend/lookup/lookuper/lookuper.go)与[域名规范化](src/backend/utils/domain_tools.go)统一网页、Typo 和批量的结果判定。DNS 无委派记录为证据不足，注册局保留单独标记，直连 RDAP 校验证书。批量 CSV 将旧版 DNS 的 `Free` 记录标为证据不足，便于补查。
- [WebSocket](src/backend/api/websocket.go)统一通过 `adminAuth` 消息登录。现有客户端须改用该消息并处理 `adminAuthResult`。管理设置返回脱敏值，代理与供应商凭据留空时由后端保留旧值。匿名流量只使用公开查询模式，服务器限制连接、批次和上游并发。完整事件契约见 [API 文档](API%20documentation.md)。
- [注册](src/backend/register/register.go)要求每次确认带 UUID 格式的 `operationId`。Redis 保存按域名去重的提交与结果。对不确定的结果，先向注册商核对，再通过前端或 `PUT /api/admin/registration/{operationId}` 写回结果。
- [批量任务](src/backend/scheduler/bulk_check.go)沿用 GitHub main 的单任务 Redis 键，并原子写入结果和移除待办。清理只处理本项目的批量键。Web 查询保留原始响应，批量 Redis 记录只保存 CSV 所需字段。
- [部署](README.zh.md#部署与升级)使用 Go 1.27.4 和 Node 22 构建，生成 Linux amd64、arm64 包。源码仓库保留构建方式和部署模板。CLI、MCP 与多任务数据模型属于历史 `codex/review-ai-access@4b6d816` 工作线，见下方版本记录。准备更新现网前先核对实际版本、功能和 Redis 数据格式。

### 验收证据

| 检查 | 结果与覆盖边界 |
|---|---|
| Go | Linux `go test -race -count=1 ./...` 和 `go vet ./...` 通过。使用专用临时 Redis 和本地模拟供应商验证注册防重、结果不确定时的核对、批量暂停与恢复、故障保留待办，以及上传、查询和 CSV 下载。 |
| 浏览器与代理配置 | Node 22.23.5、Yarn 1.22.22 冻结锁文件安装后，前端测试、lint 和构建通过。最终 amd64 包在隔离网络中经 Caddy 完成游客访问、管理员登录、私有查询、断线恢复、模拟注册、退出和 SIGTERM 停机。Compose 配置解析、Caddy 配置校验和 Nginx 语法检查通过。 |
| 批量容量 | 在同机专用 Redis 和模拟查询下完成 100 万行，CSV 有 100 万条数据和一行表头。最终一次从上传到查询完成约 34 秒，CSV 生成与计数约 9 秒。Go 进程峰值 RSS 约 444 MiB，Redis 报告内存峰值约 285 MiB。真实注册局吞吐另行测量。 |
| 依赖与产物 | `govulncheck` 对源码及两份实际交付的 Go 二进制均未报告已知漏洞。两份包通过 SHA-256 校验，内部 `deploy/REVISION` 与二进制 `vcs.revision` 均为 `6551edc7f22ead74081721c9293ecaf4f8a4ae93`。 |

### 部署包与重建

以下校验和对应 2026-11-22 本地候选包。发布流水线会为每个新构建生成自己的 `.sha256` 文件。

| 归档 | SHA-256 |
|---|---|
| `PowerWhois-linux-amd64-6551edc7f22e.tar.gz` | `0335a06295d5725537b15e81c570f9685b5c6ea7392f95365c603a227d90ac8c` |
| `PowerWhois-linux-arm64-6551edc7f22e.tar.gz` | `8375c47316fbc8f70ac01549d3635231be9f22c1ee9b904bb1794a5c4b540dd3` |

从已验收的应用提交重建需使用干净的 Linux 检出，并安装 Go 1.27.4、Node 22、Yarn 1.22.22 和 `make`。Go 集成测试需要 `redis-server`。在新克隆的工作目录执行以下命令。

```bash
git switch --detach 6551edc7f22ead74081721c9293ecaf4f8a4ae93
make -C src setup
(cd src/backend && go vet ./... && go test -race -count=1 ./...)
(cd src/frontend && yarn lint && yarn test)
make -C src bundle
bash src/package.sh amd64
bash src/package.sh arm64
(cd dist && sha256sum -c ./*.sha256)
```

[GitHub Actions](.github/workflows/check.yml)在收到拉取请求或 `main` 推送时构建、测试并生成 `PowerWhois-linux` 产物。

### 接手与上线边界

1. **代码验收**由维护者核对分支差异和 GitHub Actions 结果。关键行为变化集中在管理员 WebSocket 登录、付费供应商授权、注册确认和 Redis 提交语义。
2. **现场验收**由部署负责人核对现网版本并备份配置、前后端文件及 Redis。先用现场数据副本测试完整 Compose、HTTPS/WebSocket 和导出恢复，再一起更新前后端。使用 ARM64 时补做对应实机启动和功能验收。升级步骤见[中文部署说明](README.zh.md#部署与升级)。
3. **上游验收**用现场网络确认真实 WHOIS/RDAP、付费代理认证及连接。注册流程优先使用注册商沙箱。当前记录只有模拟注册结果，之前草稿的验证范围和阻塞见[历史归档](docs/archive/2026-11-19-stop/README.md)。

## 历史归档（2026-11-19 开发停止）

开发已经停止。本节只归档既有记录，不恢复开发，不跑测试，不合并，不部署，不发布。

完整说明、交接正文和已审报告在 [`docs/archive/2026-11-19-stop/`](docs/archive/2026-11-19-stop/README.md)。材料在仓库内，可直接阅读。整理方法是 software-handoff 1.3 与 software-team-workflow 1.4：由既有项目记录和已审交接附件整理，附件 zip SHA256 为 `0e0543019e095f0a16b99311341ad6e0f04e792f4f430a735820a167486217aa`。没有新的测试结论。

写入本节前只读核对远程，head 与 tree 仍与历史预期一致，因此保留原结论，不把旧结论套用到别的提交：

- `main`：`eb422ab7221ade327a08fd1334c5e813870872e7`，tree `789901b59f4a8174642463357b4531cad4b6b68b`。
- [PR #8](https://github.com/WenLiCG/PowerWhois/pull/8) `pw-7-whois-proxy-hardening`：head `f83824c3b9e4d374a24826c11c81c64ffbaddf87`，tree `8583233582ee7e0344e4a5abed2aa9a0d9368add`。总体 **blocked**。
- [PR #9](https://github.com/WenLiCG/PowerWhois/pull/9) `pw-8-cn-parse-panic`：head `9fba695ff93c5d243e608b484e5473733b2d9019`，tree `f9ff8aa626f8725d49c99bb6a2b17bff337e0636`。这是历史独立 pass，不是与 PR #8 的组合通过，也不是合并批准。

必须保留的边界：真实 Bright Data 的 443、43 与认证仍然 blocked。本地 D1 与 L1 不能代替真实验证。SOCKS greeting 没有响应时，不能归因于产品，也不能归因于供应商。PW-7 与 PW-8 组合回归、付费代理、出口或凭据变更、.cn 查询、ZONEINFO 或生产变更、RDAP 修复都未做，也不在本次授权内。PR #5 与 PR #6 仍是 open draft，本次没有推送它们，也没有推送 PR #8 与 PR #9 的源码分支。

以下第 1–10 节是 2026-11-08 的历史交接，不是本次停止结论，也不授权继续实现。

## 历史交接（2026-11-08）

核查日期：2026 年 11 月 8 日（UTC）
交接对象：接手开发、测试和部署的程序员

公开版保留版本、功能和验收记录。现场地址、个人路径和备份位置由项目所有者单独交接。

## 1 接手结论与工作边界

先保全本地及 VPS 的较新工作版本，再决定开发分支。GitHub main 的代码基线为恢复版 bf1258b4。11 月 4 日报告中的较新版本仍需从本地开发分支取得。用恢复版覆盖现有项目或 VPS，可能丢失较新的功能和修复。

发布前核对了 GitHub 基线、旧 PR 状态和本地最新提交。新版 VPS 服务和数据库的最后可用证据来自 2026 年 11 月 4 日的测试报告，11 月 8 日未做 VPS 复验。历史测试结果按对应版本和日期记录。

本次 GitHub 提交仅发布本文。

### 用户明确要求

- 审查并修复代码问题，找出合理优化；除非必要，不要简化现有功能。
- 保留 Web，增加适合 AI 助理使用的 CLI 等入口。Web 供用户操作，CLI 供助理操作，两者共存。
- 优化 Web 的操作流程和视觉，不以新工作台替换、隐藏或删除原版能力。
- 使用用户提供的数据测试 Web 与 AI 助理两个通道，发现问题后修复并回归。
- DNS 和 WHOIS 默认线程数为 15。早期报告中的默认 100 属于旧阶段，不能作为最终需求。
- 后续工作放在 PowerWhois 项目中统一管理；接入验证优先使用 SSH 隧道。

“接口测试通过”“代码存在”“网页入口能打开”“真实 AI 宿主已接入”是四种不同结论，不可混写。

## 2 版本基线与分歧

### 文档发布前核实的 GitHub 基线

- 仓库：WenLiCG/PowerWhois，默认分支 main。
- 用户指定的原版提交：a0757083bc0628011e8cf725dc3cfdb11a623933，提交时间 2026-02-27 06:30:45 UTC，提交说明 Update README.zh.md。
- 文档发布前的 main：bf1258b4dc7e067038cc6728f6868d4561fbfabb，提交时间 2026-11-03 08:07:33 UTC。
- 两者的 Git tree 完全一致：e6a5fd52051d97b3946fe165a6eae733390dd36b，共 154 个文件。bf1258b4 是保留后续提交历史的文件恢复提交，不是强制改写 main 历史。
- 该文件树没有 .github/workflows。不能把“没有失败的 CI”表述成“恢复版 CI 通过”。
- 原版日期不等于 2026 年 10 月 1 日的现场部署日期；未找到足以证明当日本机或 VPS 实际版本的证据。

### 需要分别保留的开发轨迹

1. a0757083：原版产品和功能对照基线。
2. bf1258b4：文档发布前的 main，文件与原版一致，提交历史不同。
3. 2ca81f4：11 月 3 日原始开发任务报告的阶段版本，是后续修复所依据的较早版本。
4. 031ab1917aee：11 月 4 日较早测试报告中的本地和 VPS 版本。
5. 4c635c907e09：随后 Web 修复部署到 VPS 的阶段版本，报告当时尚未回写原项目目录。
6. 4b6d816bab8c6e2ef1bb9c9b560e06f134ffd285：11 月 4 日最终验收记录的较新工作版本，已同步原 PowerWhois 项目与 VPS，未推 GitHub。下文简称 4b6d816bab8c。

发布前已确认本地 codex/review-ai-access 指向 4b6d816bab8c，且比 GitHub 代码基线多 9 个提交。GitHub 查询无法解析该提交。线上构建是否一致仍需核实。

### PR 3 不作为新版入口

[PR 3 修复导出与服务生命周期](https://github.com/WenLiCG/PowerWhois/pull/3) 当前仍为 open、draft，mergeable=false，mergeable_state=dirty。head 为 b9244efbe37b281867b552fd047546cf9428b6fb，分支 codex/review-improve，创建于 11 月 2 日。它是旧阶段草稿，不能代表 11 月 4 日最新版本。不要通过直接合并它来“同步最新修复”；应先整理真实的最新源码及分支关系，再形成可审查的差异。

## 3 必须保留的功能与源码导航

下列功能来自原版 README 和本次检查的源码。它们是回归清单，不是新版已完整保留的声明。原版 README 关于“所有后缀”和“100 万域名”的描述也不是本次性能或正确性验证结果。

### 原版功能清单

- 网页域名查询、结果列表、原始 WHOIS 信息查看。
- 批量文件导入、查询、结果下载及进度反馈。
- DNS、WHOIS、RDAP 查询及后缀相关策略；Mix 模式按后缀选择直连、代理或 DNS。
- SOCKS 代理及按后缀的代理规则。
- Typo 生成、选项与默认配置、排序和统计。
- 自定义 WHOIS 查询 API、自定义注册 API 和域名注册流程。
- 管理员登录、配置读取和修改、线程与重试设置、日志下载和清空。
- 原 API 与 WebSocket 事件契约。

注册功能保留不意味着可以真实下单测试。自动测试应使用模拟接口或注册商沙箱；真实购买必须另行明确批准域名、注册商、费用和期限。

### 原版目录和入口

- src/backend/main.go：Fiber 服务入口，固定监听 :8080；/app/ws 为 WebSocket 入口，/api 为 HTTP 路由前缀。
- src/backend/api/routes.go：登录、Web 设置、管理员设置、日志及批量上传下载路由。
- src/backend/api/websocket.go：WebSocket 处理；接手时对照前端事件逐项检查兼容性。
- src/backend/scheduler/：Web 查询、批量查询和注册调度。
- src/backend/lookup/：DNS、WHOIS、RDAP、自定义查询及结果结构。
- src/backend/typo/、utils/、register/：Typo、域名规范化和后缀、注册能力。
- src/backend/config/、database/、log/：配置、Redis 和日志。
- src/frontend/src/components/：AdminLogin、AdminSetting、WebCheck、BulkCheck、TypoCheck 等原界面。
- src/frontend/src/stores/、utils/websocketHandler.js：前端状态及实时事件。
- deploy/：编译产物与部署配置；src/Makefile 会覆盖 deploy 下的构建输出，不能直接在唯一现场副本上执行。

11 月 3 日曾明确报告，新工作台没有完整接通管理员登录和设置、日志操作、SOCKS／后缀代理、自定义 WHOIS／注册 API、原 Mix 路由及部分 Typo 细节。11 月 4 日虽报告 Web、CLI、API、MCP 共存，但现有证据不足以确认上述缺口已全部消除。接手程序员应逐项记录“已实现且验证”“有代码但不可达”“缺失”“需要产品确认”，不能仅用构建通过替代功能验收。

## 4 架构与部署

### 原版架构 本次源码核实

前端为 Vue 3、Quasar 2 和 Pinia；后端为 Go、Fiber 2，模块名 typonamer。go.mod 声明 Go 1.23.0。原 README 建议 Node.js 20+、Yarn 1.22+，package.json 允许 Node 18／20／22 和 Yarn 1.21.1+。复现时应固定实际使用版本并记录，不把声明范围当成已验证的构建组合。

原版使用 Redis 保存批量查询状态和结果，WebSocket 推送查询事件。原部署目录含 Caddy、后端、Redis 7 的 docker-compose.yml，并另有 Nginx 配置。compose 把宿主机 8080 映射到 Caddy 8080，开启 APP_TUNING=true；这不等于现场实际使用该配置。

后端 Redis 默认 localhost:6379、DB 0，源码使用空密码。原版精确运行应使用专用隔离 Redis；不要接入其他业务共用数据库。原版与新版数据库模型不同，不能假定可以互读或接续同一批任务。

### 新版架构 历史报告

Web、CLI、HTTP API 与 MCP 共用任务；较后记录描述 CLI 和 MCP 通过同一 HTTP API 操作，为单租户、单活 worker。新增导入、Typo 生成、候选去重、来源追踪、暂停、恢复、取消、错误重查、分页、原始证据和导出。支持 XLSX、CSV、TXT，并报告有四表导出。HTTP 和本地 stdio MCP 各有 11 个工具。

新版任务存储为 Redis：4b6d816 对应测试记录中保存的 jobs/jobs.go 源码输出显示 Store.Redis 为 *redis.Client，任务 meta、input、results、counts、queued、idem、worker 均使用 Redis。这是从历史源码输出确认的实现，今日部署仍待现场复核。

bbolt 对应 11 月 2 日另一条开发轨迹的 jobs.db。旧库仅备份保留。新版另有旧 Redis 单批任务迁移能力，仍需逐任务验证。接手时分别保留旧库和当前 Redis 数据。

查询调度增加注册局限速、Retry-After 冷却、独立工作池及每主机限制。单域名总预算历史默认 60 秒，报告包含暂停和离线时间；这个行为需重新确认，并在界面和文档解释清楚。配置上限和线程数不是实际吞吐承诺，不要宣称“支持 512”就等于实测同时执行 512 个请求。

本地 4b6d816bab8c 的 src/backend/go.mod 声明 Go 1.27.3。复现时以对应版本的依赖文件和构建说明为准。

### 部署与访问

- 历史部署使用 Debian 12 x86_64 和 systemd，Web／API、MCP 与 Redis 由独立服务管理。接手时核对实际服务配置和数据位置。
- 11 月 4 日切换前保存了备份。最终同步记录显示 41 个文件校验通过。
- 历史验证采用 SSH 隧道。操作前核对 SSH 主机指纹、主机身份、监听范围和目标服务。
- 公网 HTTPS 方案待验收。

部署接手时先确认监听地址、反向代理、服务管理方式、构建 SHA、启动参数、数据目录、备份位置和正在运行的作业。本文不提供未核实的启动命令、凭据路径或现成可复制的生产覆盖命令。

## 5 已做优化和验证边界

### 历史报告的优化

- 持久作业和多入口共用状态，新增 Excel 导入和结构化导出、来源记录及无效行原因。
- 将已注册、未找到、未知和错误分开，保存原始响应；DNS 没有 NS 不再直接等同于可注册。
- 修复上传元数据、XLSX 工作表范围、导出覆盖和悬空符号链接风险；增加 CSV 公式处理。
- 服务 SIGTERM 退出和数据库空闲写入优化；注册局等待期间释放 worker，让其他注册局继续运行。
- Web 增加任务列表、结果详情、错误重查、实时状态，以及心跳、重连和轮询后备。
- 11 月 4 日补充修复大任务部分 CSV 导出缓慢，以及 WHOIS 服务器解析失败被误标为超时。

旧模块中的 TLS 校验、密码日志和 Redis 清理曾有修复报告，但部分旧模块曾未接入新运行入口。必须逐项确认修复在当前运行路径生效，不能据“代码改过”判断已上线修好。

### 11 月 4 日验证记录

- 554,867 个有效去重域名：完成全量模拟数据测试，覆盖分页、暂停、重启和恢复；VPS 报告完成全量导入与任务控制。TXT 原始 563,807 行，其中 1,117 无效，7,823 重复或规范化合并。不是全部域名真实外部查询。
- 386 个域名：报告完成真实 DNS、WHOIS、WebSocket 查询，并核对同一任务的 Web 后端、API、CLI、MCP 导出一致。
- Typo 表：2,234 条有效预期匹配，4 条公式未执行，2 条无效；无效输入提示、缺少来源和 .ac.uk 判定得到修复，暂不支持的查询标为 unknown。
- 真实浏览器：文件选择、小批量 CSV、暂停、恢复和重查通过。大文件任务处理 853 条后暂停并取消，结果保留；最终修复后这 853 条通过真实网页约 1.2 秒下载，内容与 API 一致。
- HTTP MCP 和本地 stdio MCP：各 11 个工具的协议／接口测试通过。
- MCP 权限范围：历史报告可读取／导出共享任务，并创建、暂停、恢复、取消任务，不是只读，也未按用户隔离；不能通过这些工具注册域名或修改管理员配置。接入前须明确这个权限边界。
- 历史健康检查记录三项服务均在运行，并验证了已有任务数据。当前状态须现场复核。
- 最终记录的 DNS、WHOIS、Web 并发为 15，RDAP 为 4。不要恢复早期默认 100；限速与并发是不同参数，具体速率单位和配置须读取最新版说明。
- 较早阶段报告后端 race、vet、构建及前端检查通过，但本次未重跑，也不能据此证明 4b6d816bab8c 的每一项最终检查都已留有完整记录。

### 尚未完成或不能由现有证据证明

- 真实 dot、Grok 或其他 AI 宿主经项目级 MCP 连接完成端到端操作。最后记录中连接尚未安装，持续访问授权仍待明确批准。
- 55.5 万域名全部真实查询、百万行新版验收、长期运行和上游持续限速下的行为。
- 原版所有功能逐项回归和旧 API／WebSocket 的完整兼容性。
- SOCKS 代理、Docker Compose 部署和公网 HTTPS 的实际验收。
- 今日本地、GitHub、VPS 的三方一致性、现场备份可恢复性及数据库完整性。
- 真实注册商付款或域名注册。不得把 mock／沙箱测试称为已验证真实交易。

DNS 无记录、WHOIS／RDAP 未找到、网络超时都不能单独证明域名可购买。最终可注册状态、价格及保留策略由注册局和注册商决定。

## 6 风险与接手优先级

### P0 先保全和核实

1. 核对原项目与测试副本的分支、提交和工作区差异，保存 Git bundle、源码快照和必要构建产物。
2. 在 VPS 上记录实际运行版本、服务配置、数据目录、活动作业和最近备份。先确认备份一致性方案，再做任何停服或数据变更。
3. 比较 GitHub 原版、最新源码、线上构建三者；确认哪些修复仅在本地，建立一条可复现的接手分支。不要 reset 覆盖唯一新版，不要强推 main，不直接合并旧 PR 3。
4. 核查凭证、日志脱敏、权限和监听范围。只交接必要访问权限，避免把 config.yaml、令牌或数据库导出放入公开仓库。

### P1 恢复完整能力并完成验收

1. 以第 3 节功能清单逐项评估缺口，决定在原版基线上移植修复还是继续新版共存路线。两条路线均需保留原功能，未经用户认可不删功能。
2. 验证 DNS／WHOIS 默认 15 在 Web、CLI、API、MCP 的创建参数、服务默认值和实际调度中一致。
3. 对照旧 API 和 WebSocket 写兼容测试；覆盖登录／设置、代理、Mix、自定义查询、Typo 细节和注册 mock 流程。
4. 执行真实 AI 宿主验收：先取得项目级持续访问的明确授权；通过 SSH 隧道连接，只在新建测试任务上测试暂停、取消、恢复和导出，保留每次调用确认。
5. 在最终提交上重新跑后端测试、race、vet、前端 lint／构建和浏览器回归；保存失败案例、环境、命令、提交 SHA 与测试证据。

### P2 性能和运维完善

- 分阶段扩大数据量，分别记录导入耗时、内存、数据库体积、查询吞吐、错误率、冷却队列及导出耗时；区分模拟与实网。
- 补充崩溃／重启、断网、长时间 429、多个注册局并行、WebSocket 断开、重复点击和取消后数据保留测试。
- 定义备份保留和恢复演练、日志轮换、错误分类、任务停滞检测及部署回滚流程。
- 评估超时预算是否应包含暂停和离线时间；验证 .au 等历史冷却状态，不能凭旧报告清除上游限速。

### 原版静态检查发现

- RDAP 请求源码含 InsecureSkipVerify: true，必须在准备重新启用该路径时整改并验证可信证书链。
- Web 调度代码使用 WebCheckDomainLimit 计算并发，与“独立线程设置”的意图不符，需核查配置定义并建立回归测试。
- 原版 DNS 分支把空 NS／未找到 NS 映射为 free，存在语义误判风险；应区分注册状态与解析状态。
- Redis 使用空密码；历史记录提示旧逻辑曾包含全库清理。即便尚未重新审计所有调用，也不得复用其他业务 Redis。
- compose 发布 8080 且开启调优入口；公网安全性还取决于宿主机和代理设置，不能直接运行原配置后宣称安全。
- 前端 test 脚本仅输出 No test specified 并退出成功，不代表已有前端自动化测试。

## 7 接手步骤与验收产物

### 第一阶段 只读盘点

取得用户允许的项目和 VPS 访问；确认目标机器与仓库。记录 Git 状态、完整 SHA、远端、依赖文件、服务和数据位置。先检查现场备份是否存在及可读，再保存当前最新数据，不能用旧备份覆盖新任务。

11 月 3 日曾保存本地修改版副本、Git bundle 和 VPS 备份，11 月 4 日切换前也留有备份。恢复前逐一核验时间点、完整性和对应版本。

建议交付版本清单：每个副本的完整 SHA、文件树／构建哈希、工作区差异、构建时间、工具链、数据库版本和快照时间。对不一致项写明保留与合并方案。

### 现有报告与证据

交接时向项目所有者取得对应版本的验收报告和结构化证据，核对日期、内容和提交。

- 11 月 4 日验收报告、部署与健康记录、浏览器回归记录及大任务网页导出样本。
- 本地开发分支的 docs/DEPLOYMENT.md 和 docs/VPS-ACCEPTANCE-20261103.md 提供部署与历史验收说明。
- 同一分支的 docs/REVIEW.md、docs/USAGE.md、docs/TEST-REPORT.md 和 docs/ROUND2.md 记录各阶段的审查、用法和测试结果。

### 第二阶段 隔离复现

分别在隔离环境运行原版及新版，不共享数据库，不接生产注册付款接口。原版使用独立 Redis；新版按确认后的实际存储组件，从对应备份副本验证恢复和迁移。核对配置示例是否含默认凭证并替换为专用测试值，禁止使用公开仓库中的样例密钥。

先做小样本功能回归，再使用用户数据副本测试导入、无效行、去重、来源、Typo、查询、暂停、恢复、取消与导出。记录数据来源和校验哈希，不把包含凭据或不必要个人信息的原始日志提交仓库。

### 第三阶段 逐项验收

每个功能记录：测试用例、入口、输入规模、期望、实际结果、提交 SHA、日期、证据位置、通过／失败／未测试。至少覆盖：

- 同一任务跨 Web、CLI、API、MCP 的 ID、进度、结果数量和导出一致。
- 已注册、未找到、unknown、超时、解析失败、限速、保留域名的分类和证据。
- Web 文件选择和下载的真实浏览器交互，不能只调用后端替代。
- 进程重启后任务状态可解释，取消后结果保留，旧任务不受新测试影响。
- 权限校验、同源／回环限制、导出防覆盖、CSV 公式、TLS 校验、日志脱敏。
- 原版功能清单全部有状态；每项缺口都有处理结论或用户认可的变更。

### 第四阶段 发布和回滚

只有完成代码审查、确认发布目标和获得部署许可后才切换。发布前保存当前程序、配置和一致数据备份，记录回滚版本及恢复条件。代码回滚与数据回滚分别判断；不能假定旧版自动继续新版任务，也不能把历史 jobs.db 备份或 Redis 单批迁移能力当成已完成全量数据迁移。

发布后复核健康状态、监听范围、实际构建 SHA、小任务及导出，并确认原任务状态。推送、合并、部署是不同步骤，每步单独记录结果。最终交付源码、可复现构建说明、脱敏配置说明、测试矩阵、部署回滚说明和待办清单。

## 8 安全与凭证交接

本文件不包含密码、JWT 密钥、SSH 私钥、API key、访问令牌或完整生产配置。

- 由项目所有者确认接手程序员身份及必要权限；通过认可的密码管理器、受限凭据系统或平台邀请交接，不在聊天、PR、普通附件中复制密钥。
- 分开管理 SSH、管理员登录、JWT、自定义查询 API、注册商 API 和 MCP 访问凭证。查询权限不应隐含注册／付款权限。
- 为新人员建立可撤销的最小权限访问。新增持续 MCP 访问、创建或配置长期凭证前，必须取得明确授权。
- 证书、SSH 主机指纹和目标地址应独立核验。自签名证书问题不应通过全局禁用 TLS 验证处理。
- 如发现凭证曾进入日志或 Git 历史，先评估影响并取得授权后轮换；删除当前文件不足以从历史中撤销泄漏。
- 向程序员交接数据前确认内容及受众。备份含生产任务、原始响应或配置时，不默认上传公开仓库或公开文件链接。

## 9 仍需现场确认的问题

1. 本地与线上构建是否一致，其他测试副本是否还有未合入的修改？
2. VPS 今天运行什么构建，监听哪些地址，数据与备份目录在哪里，是否有运行中的作业？
3. 原版管理员、代理、Mix、自定义 API、日志和 Typo 细节已接通到什么程度？
4. 最新新版的数据库格式、配置契约、Go 工具链和完整测试记录是什么？
5. 是否已有真实 AI 宿主连接的后续授权与验收证据？现有记录不能证明已经完成。
6. 用户最终选择在原版上逐项移植，还是继续新版共存架构？先盘点差异，不代替用户重新选择路线。

## 10 核查来源

以下 GitHub 链接在 2026 年 11 月 8 日读取；源码链接固定到用户指定原版，避免 main 后续变化造成歧义。

[项目仓库](https://github.com/WenLiCG/PowerWhois)  |  [原版提交 a0757083](https://github.com/WenLiCG/PowerWhois/commit/a0757083bc0628011e8cf725dc3cfdb11a623933)  |  [恢复提交 bf1258b4](https://github.com/WenLiCG/PowerWhois/commit/bf1258b4dc7e067038cc6728f6868d4561fbfabb)

[原版功能与部署说明](https://github.com/WenLiCG/PowerWhois/blob/a0757083bc0628011e8cf725dc3cfdb11a623933/README.zh.md)  |  [后端入口](https://github.com/WenLiCG/PowerWhois/blob/a0757083bc0628011e8cf725dc3cfdb11a623933/src/backend/main.go)  |  [API 路由](https://github.com/WenLiCG/PowerWhois/blob/a0757083bc0628011e8cf725dc3cfdb11a623933/src/backend/api/routes.go)

[后端依赖](https://github.com/WenLiCG/PowerWhois/blob/a0757083bc0628011e8cf725dc3cfdb11a623933/src/backend/go.mod)  |  [前端依赖与脚本](https://github.com/WenLiCG/PowerWhois/blob/a0757083bc0628011e8cf725dc3cfdb11a623933/src/frontend/package.json)  |  [原版 Compose 配置](https://github.com/WenLiCG/PowerWhois/blob/a0757083bc0628011e8cf725dc3cfdb11a623933/deploy/docker-compose.yml)

[Redis 连接实现](https://github.com/WenLiCG/PowerWhois/blob/a0757083bc0628011e8cf725dc3cfdb11a623933/src/backend/database/redis.go)  |  [RDAP TLS 配置](https://github.com/WenLiCG/PowerWhois/blob/a0757083bc0628011e8cf725dc3cfdb11a623933/src/backend/lookup/rdaplib/rdap_query.go)  |  [Web 查询调度](https://github.com/WenLiCG/PowerWhois/blob/a0757083bc0628011e8cf725dc3cfdb11a623933/src/backend/scheduler/web_check.go)

[批量查询和 DNS 结果分类](https://github.com/WenLiCG/PowerWhois/blob/a0757083bc0628011e8cf725dc3cfdb11a623933/src/backend/scheduler/bulk_check.go)  |  [构建 Makefile](https://github.com/WenLiCG/PowerWhois/blob/a0757083bc0628011e8cf725dc3cfdb11a623933/src/Makefile)  |  [旧草稿 PR 3](https://github.com/WenLiCG/PowerWhois/pull/3)

需求与历史依据：用户 2026 年 11 月 2 日关于保留功能和 Web／CLI 共存的要求；11 月 3 日原版恢复与交接记录；11 月 4 日数据测试、SSH 隧道、默认线程 15 及最终修复报告。上述历史记录未代替本次现场复验。
