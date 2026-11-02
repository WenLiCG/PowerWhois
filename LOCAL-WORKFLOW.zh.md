# PowerWhois 本地自动化版

本分支将默认入口改为共享的持久化作业服务。CLI、HTTP API、Web 页面操作同一数据库；扫描不依赖浏览器或 WebSocket。无需 Redis，也没有域名购买入口。原 `deploy/` 预编译文件和旧接口文档未重新发布，不适用于这个入口。

## 启动和一次提交 Excel

在项目根目录打开 PowerShell。已构建的程序是 `bin/powerwhois.exe`：

```powershell
.\bin\powerwhois.exe serve --listen 127.0.0.1:8080 --data data/powerwhois.db --web src/frontend/dist/spa
```

不传 `--web` 会使用内置轻量页面。服务只允许 loopback，拒绝外部 Host 和跨来源请求。首次查询读取 IANA RDAP 引导表；无支持的 HTTPS RDAP 服务时返回 `unknown`，不会改用 DNS 推断可注册。

**默认认证边界：当前本地 API/CLI 不使用账户、JWT 或 bearer token，也不自动初始化或保存任何凭据。** 它把能连接本机 loopback 的进程视为可信；Host/Origin 校验用于阻止浏览器跨站和 DNS rebinding，不能隔离同机其他系统用户/进程。不能用它作为共享或公网管理服务。旧 JWT 登录实现没有被当前 main 注册，配置中的示例密码/密钥已清空；未安全配置时旧函数也拒绝登录。

另一个终端运行：

```powershell
$job = .\bin\powerwhois.exe import --file C:\path\domains.xlsx --sheet Sheet1 --column B --header | ConvertFrom-Json
.\bin\powerwhois.exe generate --job $job.id --rules omission,repetition,transposition --max-candidates 500000
.\bin\powerwhois.exe results --job $job.id --limit 100
.\bin\powerwhois.exe start --job $job.id
.\bin\powerwhois.exe status --job $job.id
.\bin\powerwhois.exe export --job $job.id --file C:\path\results.xlsx --denominator all
```

也可用 `tools/scan-excel.ps1` 一次完成导入、生成、提交。它返回 job JSON，不等待整个联网扫描完成。没有标题时省略 `--header`；可用 `--url http://127.0.0.1:18080` 连接其他本地端口。导出目标已存在时 CLI 拒绝覆盖。

```powershell
.\tools\scan-excel.ps1 -File C:\path\domains.xlsx -Sheet Sheet1 -Column B -Header
.\bin\powerwhois.exe pause --job $job.id
.\bin\powerwhois.exe resume --job $job.id
.\bin\powerwhois.exe cancel --job $job.id
.\bin\powerwhois.exe seeds --job $job.id --limit 100
.\bin\powerwhois.exe origins --job $job.id --limit 100
.\bin\powerwhois.exe results --job $job.id --limit 1000 --after '上页 next'
```

stdout 是 JSON，错误写入 stderr。退出码：0 成功取得响应，1 网络/服务/文件操作失败，2 参数错误。`status` 返回成功不代表作业已完成，应检查 `state`、`completed`、`counts`、`partial`、`truncated`。

## 数据与结果口径

- 真正读取 XLSX。可选 sheet、列及标题行；保存原始值、工作表、行号、规范 DNS 名称和每个候选的 seed/规则关联。公式单元格明确无效，包括没有缓存结果的公式；不会将前导连字符、下划线或 URL 清洗成另一个域名。
- 导入通过 ZIP/XML 流式读取所选列，不调用 Excelize 整表加载接口。仅保留使用到的共享字符串，其他列和无关 SST 内容不展开。解析 SpreadsheetML 字符串转义；共享公式 follower 没有存储表达式时保留 `Raw="="` 并判为无效，不尝试求值或改名。普通外部超链接仅忽略，不访问目标。
- DNS 名称只做大小写、外部空白及 IDNA 的等价规范化。seed 必须是注册局边界下的完整域名，拒绝子域名，保留其无效原因。PSL 的私有托管后缀不当作注册局后缀。
- 支持 8 类：`omission`（删除）、`repetition`（重复）、`transposition`（相邻换位）、`insertion`（插入 a-z/0-9）、`horizontal` / `vertical`（键盘替换）、`www`、`tld`。输出顺序、去重及关联确定；默认全部 8 类，建议先显式选择规则并预览。
- 默认唯一候选上限 500,000，可设 1..2,000,000。达到上限明确标记 `truncated`，计数只代表已处理的生成前缀，不声称穷尽所有规则。`rawCandidates` 含重复/无效/身份不变的尝试，`validCandidates` 为通过校验且不是不变原名的尝试数（显式 includeSeeds 除外），`candidates` 为唯一扫描数，`origins` 为唯一候选-来源行-规则关联数。
- 状态为 `registered` / `not_found` / `unknown` / `error`。`registered` 需要匹配名称的 RDAP domain 对象；缺少注册商、日期或 NS 不改判。`not_found` 来自 IANA 指定服务的 404，允许 RFC 7480 规定的空正文；非空畸形正文仍为错误。**not_found 不保证可购买**。
- 保存 UTC 检查时间、provider URL、HTTP 状态、错误、每次尝试与最多 128 KiB 响应证据。证据过大时返回错误，不伪造结果。XLSX 证据显示受 Excel 单元格文本上限约束，完整记录可通过结果 API 读取。
- XLSX 含 `Summary`、`Results`、`Sources`、`InvalidInputs`。未检查的记录为 `pending`，部分结果明确标记；文本写为字符串并防公式注入。`--denominator all` 用全部唯一扫描域名，含待查/未知/错误；`known` 只用 registered + not_found。分母为 0 显示未定义，不产生除零公式。

## 持久化与限速

bbolt 事务分别保存 job 元数据、输入、候选、来源、待查队列、在途 lease 和结果。生成约每 1,000 次候选尝试提交一个有限批次，结果与队列移除原子提交。保存失败终止调度并将健康状态置为失败，修好磁盘后重启；不会先删除待查项。相同 lease 的结果仅提交一次。

进程中断后，在途项回到待查队列，扫描中的作业变为 `paused`，需显式 `resume`。处于 `generating` 的作业使用相同生成参数再次 `generate` 即从事务检查点继续。外部查询可能在崩溃前已经发生，因此恢复保证结果幂等和不丢队列，不承诺外部查询恰好一次。

生成入场状态校验和 `generating` 转换在同一写事务内完成，不能覆盖已经提交的取消。同一已生成作业重复相同参数为幂等；改变规则、上限等参数返回 409，需重新导入为新作业。生成期间暂停/取消不作为扫描生命周期动作接受，会明确返回状态冲突。

全服务最多 10 个固定 worker；多个作业共享该上限。另有每个 provider host 的间隔、单次超时、最多 3 次尝试、指数退避和共享 429 Retry-After 冷却。默认每 provider 1 秒一请求，本次实网测试使用 2 秒。`--provider-interval` 最小 100ms 是参数边界，不代表 provider 允许该速率；大任务应按其政策配置。暂停、取消会撤销 lease 并取消在途请求，旧响应不能覆盖续跑的新 lease。

## API

所有路由以 `/api/v1` 开头。列表是 `{items,next}`，`after` 使用返回的 next，禁止把整批结果通过 WebSocket 推送。

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | `/health` | 健康状态，持久化故障为 503 |
| GET | `/jobs?limit=100&after=...` | 作业列表 |
| POST | `/jobs/import` | multipart file/sheet/column/header/source |
| GET | `/jobs/{id}` | 元数据和计数 |
| POST | `/jobs/{id}/generate` | JSON rules/tlds/maxCandidates/includeSeeds |
| POST | `/jobs/{id}/start`、`pause`、`resume`、`cancel` | 生命周期 |
| GET | `/jobs/{id}/items`、`seeds`、`origins` | 分页记录，limit 最大 1,000 |
| GET | `/jobs/{id}/export?denominator=all` | 流式 XLSX 导出 |

导入限制为 32 MiB 上传、256 MiB 总解压、100,000 行（标题另计）、所选列原始文本合计 32 MiB、单格 32,767 字符；还限制 ZIP/relationship 数量及 XML token/嵌套，避免稀疏行列和压缩输入导致内存展开。当前需求的 10,000 行已实测。

导出逐行写入临时 XML，并将 ZIP 直接写到目标，不在堆中积累整个最终压缩文件。每张 XLSX 最多 1,048,576 行，单成员 XML 小于 2 GiB、压缩文件最多 512 MiB；超限明确失败，不交付成功文件，使用分页 API 获取更大规模证据。数据库由一个服务进程独占；CLI 连接该服务，不能同时启动另一个进程打开相同 DB。

## 构建和检查

需要 Go >= 1.25（本机实际 Go 1.27.3）、Node/npm。前端提交了 npm lockfile。

```powershell
cd src/backend
go test ./...
go vet ./...
go build -o ../../bin/powerwhois.exe .
# Windows race 检查还需要 C 编译器，设置 CGO_ENABLED=1 和 CC 后：
go test -race ./...
cd ../frontend
npm ci --ignore-scripts
npm run lint
npm run build
```

`tools/check.ps1` 在同一机器上自动定位本任务的便携 Go（若 PATH 没有 Go），执行测试、vet、构建、lint/build。原 Redis、注册 API 和 WebSocket 包保留作参考，但默认 main 不导入它们。旧代码同时补了密码日志删除、弱 JWT 配置拒绝、TLS 验证、无元数据误判修复、DNS 不推断 Free、保存失败保留队列及删除指定键替代 FlushDB。

## 验收证据

最终真实规模、恢复、CLI/API 一致性、实网样本和复现命令见 `VALIDATION.zh.md`。公开数据来源为 [WebsiteLaunches/top-100-million-domains](https://github.com/WebsiteLaunches/top-100-million-domains)，commit、文件大小、SHA256 和抽样方法固定在 `testdata/real/source.json`，附 MIT 许可。仓库不提交下载的 CSV、测试 DB、结果 XLSX 或二进制；运行 `./tools/fetch-testdata.ps1` 按需取得约 1.4 MB 的固定源文件并验证哈希，不执行来源仓库脚本。

RDAP 语义依据 [RFC 7480 §5.3](https://www.rfc-editor.org/rfc/rfc7480.html#section-5.3)、[RFC 9083](https://www.rfc-editor.org/rfc/rfc9083.html)，服务发现依据 [IANA RDAP DNS bootstrap](https://www.iana.org/assignments/rdap-dns)。XLSX 使用 BSD-3-Clause 的 [Excelize](https://github.com/xuri/excelize)，事务存储使用 MIT 的 [bbolt](https://github.com/etcd-io/bbolt)；依赖版本固定在 go.mod/go.sum。
