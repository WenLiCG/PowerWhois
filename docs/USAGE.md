# Web、CLI、HTTP API 与 MCP

本版是一套查询核心、一个任务库、多个入口。CLI 和 MCP 都调用 HTTP API；Web 批量查询也调用同一 API。原网页即时查询和 Typo 查询仍通过 WebSocket 调用同一个查询核心，但属于临时会话，**没有持久化任务 ID**。需要断线续跑时使用“批量任务”。

## 结果语义

| status | 含义 |
|---|---|
| registered | 查询得到注册记录，或 DNS 存在域名委派 |
| not_found | 注册数据源明确返回未找到；仍可能被保留、限制或无法注册 |
| unknown | DNS 没有委派、不支持该后缀，或注册数据无法可靠解析 |
| timeout | 本域名在规定时限内没有完成；批量任务继续处理其他域名 |
| error | 上游错误、限流、连接失败等；不能据此判断可注册 |

每个结果包含 `domain/status/source/queriedAt/queryError/rawResponse` 及原有注册日期、NS 等字段。`queriedAt` 是 UTC；旧数据导入时没有可靠查询时间，留空并标记 `source=legacy-import`。原 Web 的 `registerStatus` 字段仍保留为兼容别名。

`domainStatus` 是 `rawDomainStatus` 的生命周期摘要，仅对已注册结果有意义：Hold、Inactive、PendingDelete、RedemptionPeriod 优先；`ok/active` 或只有 client/server 的 delete、renew、transfer、update prohibited 锁定时为 Active（注册局在有锁定时不返回 `ok`，如 example.com）；其他组合（例如 autoRenewPeriod）为 Unknown。网页批量结果对未注册行显示“—”。

Typo 生成结果不包含原域名本身。批量或同步查询中有无效域名时整批拒绝，错误为 `invalid domain at line N: "原值"`，其中 N 是提交数组中的位置（Web 与 CLI 会先去掉空行）；网页上传会换算成文件实际行号。

未支持后缀等本地校验结果标记 `source=local-validation`，不伪装成注册局响应。`.ac.uk` 由 [Jisc](https://www.jisc.ac.uk/domain-registry) 独立管理，当前没有专门适配，因此 WHOIS/Mixed 返回 unknown 和原因；不能把 Nominet 的 `.uk` RDAP 404 当作 `.ac.uk` 未注册证据。DNS 仍可独立查询委派。

`whois` 模式沿用原项目的 RDAP 优先路由；`dns`、`mixed`、`proxy` 也保留。`proxy` 模式使用系统设置中的 **SOCKS5 代理**（由代理端完成远程域名解析，同时支持 HTTPS RDAP 443 与传统 WHOIS 43；配置时填写主机名与 SOCKS5 端口如 `22228`，勿填写 HTTP 代理端口，可在管理设置中点击「测试 SOCKS5 代理」即时验证）。自定义 WHOIS API 使用配置中的名称。原长名称 `whoisQuery/dnsQuery/mixedQuery/whoisQueryWithProxy` 继续有效。输入须为纯域名，支持 IDN，转换为小写 ASCII；子域名按原项目规则归并到注册域名，自动去重。后缀使用仓库维护的列表，不保证所有未来新后缀立即可用。

新 RDAP 查询的 `rawResponse` 保存最终上游响应正文（成功的 RDAP 域名记录为 JSON，错误正文可能为空或使用其他格式），`source` 记录最终请求 URL。旧任务保留当时存储的原文格式。自动化读取日期、状态或 NS 时应使用相应的结构化字段。配置代理和 Mixed 路由时，Unicode 后缀与其 Punycode 写法等效。

## 权限与凭据

- Web 管理员登录后可使用原有设置、代理、自定义 API、日志、手动注册。注册必须在 Web 明确确认；已发给注册商的请求无法撤回，失败或取消后先查注册商订单再重试。
- `POWERWHOIS_QUERY_TOKEN`：至少 32 字符的随机凭据，只能查询、生成 Typo、创建/读取/暂停/恢复/取消/重查查询任务、导出。不能注册域名、删除数据、修改代理、读取管理配置或日志。
- `POWERWHOIS_MCP_TOKEN`：可选 HTTP MCP 的独立入口凭据，不应复用管理员 JWT。MCP 进程向后端使用查询凭据。
- CLI/MCP 客户端从 `POWERWHOIS_QUERY_TOKEN` 或 `POWERWHOIS_TOKEN_FILE` 读取；文件配置优先，文件应仅当前用户可读。示例不会把凭据放在命令参数里。
- 单租户：同一服务的所有查询凭据和管理员共享任务历史。没有用户级隔离，查询凭据只适合分发给信任的用户。轮换查询 token 需要重启后端。
- 管理菜单中的「退出所有管理会话」通过 `POST /api/admin/logout` 撤销本实例发出的管理员 JWT，独立查询和 MCP 凭据继续有效。已发往注册商的请求会继续收集结果，未通过提交检查的域名会停止提交。

## CLI

先安装构建出的 `powerwhois`（Windows 为 `powerwhois.exe`），并由安全的进程环境或权限受限的 token 文件提供查询凭据。`POWERWHOIS_URL` 默认 `http://127.0.0.1:8080`；非回环 HTTP 被拒绝。

```sh
powerwhois --help
powerwhois query --mode dns example.com example.org
powerwhois query --mode whois --timeout 30 --retries 1 example.com
powerwhois typo --domain example.com --types skipLetter,doubleLetter --tlds net,org
powerwhois tasks create --file domains.txt --mode mixed --concurrency 16 --timeout 30 --retries 1 --key inventory-20261103
powerwhois tasks create --file - --mode whois < domains.txt
powerwhois tasks list --limit 100
powerwhois tasks get --id TASK_ID
powerwhois tasks pause --id TASK_ID
powerwhois tasks resume --id TASK_ID
powerwhois tasks cancel --id TASK_ID
powerwhois tasks retry --id TASK_ID
powerwhois tasks results --id TASK_ID --offset 0 --limit 1000
powerwhois tasks export --id TASK_ID --format csv --offset 0 --limit 1000 > page-0.csv
```

参数必须在位置域名之前。文件每行一个域名，允许 UTF-8 BOM；空行忽略。标准输出默认 JSON，CSV 导出除外；错误只写标准错误。退出码：0 请求成功；2 参数错误；3 认证失败；4 不存在；5 状态或幂等冲突；1 网络/服务失败。成功响应中仍可能有单个域名的 `error/timeout`，需检查结果状态。

Web 批量、CLI、API 和 MCP 省略并发数时取 15 与服务端批量上限中的较小值；服务端 DNS 和 WHOIS 默认最大并发也为 15，网页即时查询默认并发为 15。RDAP 保留独立限流配置；WHOIS 模式优先使用 RDAP 时，仍受 RDAP 上限约束。并发数不等于每秒请求数。已有部署需更新对应配置字段，升级程序不会覆盖现有配置。TXT 含无效域名时整批拒绝，不静默跳过；按提示修正或将无效项单独保存后重新提交。Excel 不是原生上传格式，应先按工作表含义提取域名列为 TXT，勿执行公式或统一跳过首行。

批量创建立即返回 ID，CLI 退出不会停止任务。`--timeout` 是包括重试、协议限流等待在内的**每域名总时限**；`--retries` 为最多 5 次额外瞬时错误重试。后端的各协议超时也会生效。单批最多 100 万输入，HTTP body 最多 100 MiB，Web TXT 最多 50 MiB；这些是输入上限，不是吞吐性能承诺。

## HTTP API

机器可读文档：运行服务的 `GET /api/openapi.json`，源码在 [openapi.json](../src/backend/api/openapi.json)。所有 `/api/v1/*` 请求使用 `Authorization: Bearer …`，管理员 JWT 也可调用。没有 query-string token。

| 方法 / 路径（前缀 `/api/v1`） | 操作 |
|---|---|
| POST `/query` | 同步查询，最多 100 个输入；整个 HTTP 查询最长 120 秒 |
| POST `/typo` | 生成候选，返回去重列表及按类型分组 |
| POST `/tasks` | 持久化任务，首次 202；同键同参数重放 200 |
| GET `/tasks?offset=0&limit=100` | 最近任务列表 |
| GET `/tasks/{id}` | 状态、完成数和各结果计数 |
| POST `/tasks/{id}/{pause,resume,cancel,retry}` | 控制任务；retry 新建任务，不改原结果 |
| GET `/tasks/{id}/results?offset=0&limit=100` | 已有结果分页 |
| GET `/tasks/{id}/export?format=json&offset=0&limit=1000` | JSON / CSV 分页导出 |

Python 标准库示例，无额外依赖：

```python
import json, os, urllib.request
url = os.getenv("POWERWHOIS_URL", "http://127.0.0.1:8080")
body = {"domains": ["example.com", "example.org"], "mode": "mixed",
        "concurrency": 8, "timeout": 30, "retries": 1}
request = urllib.request.Request(url + "/api/v1/tasks",
    data=json.dumps(body).encode(), headers={
        "Content-Type": "application/json",
        "Authorization": "Bearer " + os.environ["POWERWHOIS_QUERY_TOKEN"],
        "Idempotency-Key": "inventory-20261103"})
with urllib.request.urlopen(request, timeout=60) as response:
    print(json.load(response)["id"])
```

错误结构为 `{"error":{"code":"invalid_request","message":"..."}}`：400 输入不合法，401 未认证，404 不存在，409 状态/幂等冲突，429 请求过快（读取 `Retry-After`），503 存储暂不可用。服务限制每来源 IP 每分钟 30 次同步查询、60 次创建任务、1200 次 v1 请求；登录 5 次。反向代理默认不信任任意来源转发的 IP，多个远程客户端可能共用代理侧计数。

结果列表默认不返回原文（`includeRaw=false`），需要原文时使用 `GET /api/v1/tasks/{id}/results/{index}`，其中 index 是结果的 order；也可显式 `includeRaw=true`。同步 `/query` 原文预览上限为每域名 64 KiB，截断时 `rawResponseTruncated=true`；需要完整证据请先创建持久任务。同步总结果超过 8 MiB 返回 413，请改为任务分页。

CLI 导出 CSV 时，标准输出只包含 CSV。标准错误中的 `nextOffset=N` 可作为下一次导出的 `--offset N`，值为 `-1` 时结束。缺失或无效的游标会使该次导出失败，调用脚本应检查退出码。

### 任务与分页规则

任务状态：`queued → running → completed`，也可 `paused/canceled/failed`。暂停后可继续，取消后只保留已有结果，不能继续；存储类失败可修复后 resume。错误域名本身不使任务失败，仍会计入 completed。任务按 FIFO 排队，一次执行一批，批内并发受服务端上限限制。

创建时请设置稳定的 `Idempotency-Key`（最多 128 字节）：相同键和规范化参数返回原 ID；不同参数返回 409。键与任务一起长期保留，**没有自动过期**。未提供键时，每次创建是独立任务。pause/resume/cancel 对已处于目标状态的任务是幂等操作；`retry` 每调用一次都新建带 parentTaskId 的任务，丢失响应时先查任务列表，不要盲重试。

`offset` 是规范化后**输入数组的位置**，不是已完成结果数量；`limit` 为 1–1000，但结果和导出还有约 8 MiB 的编码预算，单条最多 16 MiB，因此一页可能少于 limit 条，必须使用服务端返回的游标，不能自行加 limit。运行中的页面可能为空，但仍必须按 `nextOffset` 前进；`-1` 表示末页。任务完成后从 0 重新读取可获得完整结果。导出附带 `X-Task-State` 和 `X-Export-Consistency`（complete/partial）；只有 completed 为 complete。CSV 用 `X-Next-Offset` 响应头，后续页合并时只保留首份表头。Web 导出自动分页；CLI/MCP 一次导出一页，避免大结果塞入模型上下文。活动任务的跨页导出不提供同一时刻快照，要完整稳定导出应等待完成或暂停。

重启自动恢复之前 running 的任务，跳过已提交结果。单个结果和计数原子提交，输入不会被消费删除；中断时正在查询但未提交的域名可能重新向上游查询。不能承诺网络请求 exactly-once。Redis AOF everysec 遇到主机断电仍可能丢失最后约一秒数据，需要备份。历史不自动清理，请监控 Redis 内存和磁盘。

## MCP

使用官方 Go SDK；11 个工具：`query_domains`、`generate_typos`、`create_task`、`list_tasks`、`get_task`、`pause_task`、`resume_task`、`cancel_task`、`retry_task`、`results_task`、`export_task`。输入有类型/schema，成功结果在 `structuredContent` 中。示例：

```json
{"name":"create_task","arguments":{"domains":["example.com","example.org"],"mode":"mixed","timeout":30,"concurrency":8,"idempotencyKey":"inventory-20261103"}}
```

默认 stdio：`powerwhois-mcp`。不要手工向它输入 CLI 命令，由 MCP 客户端启动并交换协议消息。输出只用于 MCP 协议。

可选 Streamable HTTP：安全设置 `POWERWHOIS_MCP_TOKEN` 后运行 `powerwhois-mcp --http 127.0.0.1:8081`，端点 `/mcp`。只允许回环监听；远程通过 HTTPS 反向代理或受信隧道。当前提供静态 Bearer 认证，**没有 OAuth 授权服务器，也没有旧 SSE 专用端点**。

### 各 AI 环境的接入边界（2026-11-03）

| 环境 | 可用路径与条件 | 本次验证程度 |
|---|---|---|
| 本机/服务器上的 Codex 或其他终端 Agent | CLI；或 stdio MCP，进程需能访问后端并读取查询凭据 | 真实 CLI/stdio 进程和官方 SDK 客户端通过；本机 `codex mcp add --help` 确认支持两种传输 |
| Codex Streamable HTTP | MCP URL + `bearer_token_env_var`，本地可走 SSH 转发 | 协议握手和 Bearer 拒绝/接受已测试；未修改用户全局 Codex 配置 |
| ChatGPT dot / ChatGPT 应用 | 先确认该具体产品提供本地终端、OpenAPI 集成或远程 MCP App 配置；能用浏览器也可直接操作 Web | 没有在用户账号中建立连接，不能从“ChatGPT”名称推断支持本地 stdio |
| ChatGPT 远程 MCP App | 需要账号/工作区允许、可达的受保护端点及其支持的认证；不接受静态 Header 的环境需要 OAuth 网关或受支持的安全隧道 | 官方能力与认证说明核对；无账号级验收 |
| xAI/Grok Agent API | 官方支持 Remote MCP 的 SDK/Responses 环境，使用可达 HTTPS `/mcp`、认证 Header、`allowed_tools` | 官方文档核对；未调用付费 xAI API，也不承诺普通 Grok 聊天客户端支持 |
| 其他 Agent | 依其支持选择 CLI、OpenAPI HTTP 或 MCP | 标准协议层已测试 |

Codex 的配置示例（只保存路径和环境变量名，不保存秘密）：

```toml
[mcp_servers.powerwhois]
command = "/absolute/path/powerwhois-mcp"
env_vars = ["POWERWHOIS_URL", "POWERWHOIS_QUERY_TOKEN", "POWERWHOIS_TOKEN_FILE"]
tool_timeout_sec = 130
```

HTTP 方式替代上述条目：

```toml
[mcp_servers.powerwhois]
url = "http://127.0.0.1:8081/mcp"
bearer_token_env_var = "POWERWHOIS_MCP_TOKEN"
tool_timeout_sec = 130
```

Windows 的 command 用 `C:/absolute/path/powerwhois-mcp.exe`。环境变量必须在启动客户端时可见；配置文件存在不代表连接成功，应实际列出工具并调用 `list_tasks` 验证。

xAI 支持 Remote MCP 的客户端可使用下列工具配置，凭据由调用方运行时注入：

```python
tool = {
    "type": "mcp", "server_label": "powerwhois",
    "server_url": os.environ["POWERWHOIS_REMOTE_MCP_URL"],
    "headers": {"Authorization": "Bearer " + os.environ["POWERWHOIS_MCP_TOKEN"]},
    "allowed_tools": ["query_domains", "generate_typos", "create_task", "get_task", "export_task"]
}
```

官方依据：[Codex MCP](https://learn.chatgpt.com/docs/extend/mcp?surface=cli)、[ChatGPT MCP App 与本地网络接入限制](https://help.openai.com/en/articles/12584461-developer-mode-and-full-mcp-connectors-in-chatgpt)、[认证要求](https://developers.openai.com/plugins/build/auth)、[xAI Remote MCP](https://docs.x.ai/developers/tools/remote-mcp)。产品能力会变化，以实际账号配置界面与握手结果为准。

本轮设置秘密修改、注册审计和兼容变更见 [ROUND2.md](ROUND2.md)。
