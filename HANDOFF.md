# PowerWhois 程序员交接文档

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
