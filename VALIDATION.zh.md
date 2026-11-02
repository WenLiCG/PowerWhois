# 本地验收记录

本记录使用仓库根目录的相对路径，测试基于 `a0757083bc0628011e8cf725dc3cfdb11a623933` 之后的本地优化版本。完整测试输出、数据库和工作簿保留在被 Git 忽略的 `validation/` 中；它们不是仓库内可直接下载的附件。公开提交包含实现、回归测试、固定数据源元数据、MIT 许可证和复现脚本。

## 本地验收产物（不纳入 Git）

- `bin/powerwhois.exe`：可运行的本地服务和 JSON CLI。
- `LOCAL-WORKFLOW.zh.md`：启动、单次 Excel 提交、分页、生命周期、导出和 API 说明。
- `tools/scan-excel.ps1`：一条命令导入、生成、提交扫描，实际验证过。
- `validation/final-real-10k/seeds-10000.xlsx`：真实 10,000 个唯一原始域名。
- `validation/deliverables/real-10k-results-reviewed.xlsx`：**大作业联网检查计数为 0**，221,949 条导出记录均为 pending，Summary 标为部分结果；含 223,345 条来源关联。另取其中 20 个候选建立独立小作业实测，没有启动大作业。
- `validation/deliverables/final-corpus-stratified-live.xlsx`：最终候选集的 20 个分层真实在线结果，5 registered、15 not_found；含 `SelectionProvenance` 原始 seed/规则/行号/候选集成员证明。
- `validation/deliverables/live-sample-results-reviewed.xlsx`：较早的独立协议样本，1 registered、19 not_found，与最终候选集交集为 0；仅作补充证据保留。
- `validation/deliverables/reviewed-manifest.json`、`validation/review/service/verification.json`：最终二进制/文件哈希与完整回读核对。

其他 `validation/real-10k`、`service`、`online`、`final-service`、`final-online` 是早期调试中间结果，**不是最终验收数据**。最终复核以 `validation/review/`、上述交付列表为准；`verified-service/jobs.db` 保存 CLI/API 作业，`verified-online` 保留原协议样本，`final-real-10k` 保留规范输入和来源过滤记录。

## 公开真实数据

来源：[WebsiteLaunches/top-100-million-domains](https://github.com/WebsiteLaunches/top-100-million-domains)，commit `df5c729c3984e4ed7cf0e3bbd9102927564af9c9`。MIT 许可保存在 `testdata/real/SOURCE-LICENSE`。

只下载仓库的 10k 和 100k 小文件，没有下载 100 million 全量数据，没有运行来源脚本。10k 文件含重复、无效或非注册局根域名，不能提供 10,000 个有效唯一 seed，故从 100k 文件按原始顺序过滤。不是随机填充，也没有要求凑到固定 typo 数。

| 项目 | 实测 |
|---|---:|
| 读取来源行 | 10,309 |
| 拒绝的来源值 | 29 |
| 重复规范域名 | 280 |
| 写入 XLSX / 导入有效唯一 seed | 10,000 |
| 原始候选尝试 | 229,724 |
| 合法且非身份不变的候选尝试 | 227,506 |
| 合法唯一候选 / 唯一扫描域名 | 221,949 |
| 唯一候选-来源行-规则关联 | 223,345 |
| 数量上限是否触发 | 否（上限 500,000） |
| 此大任务实际联网检查数 | **0** |

规则固定为 omission / repetition / transposition。支持其他 5 类规则，但本次规模数据不混入它们。未只筛选 .com；输入涵盖多个后缀。在线验证样本是 .com，不能据此声称所有 TLD 的 provider 都已实测。

`top-100k-domains.csv`（1,431,810 字节）SHA256：
`c0306291ab5ca312d1651de96c4704e84ede8fac327b9e2ce12eb99673aef2b2`

规范 XLSX SHA256：
`8fd9dd5e2187135df5227eac2a9f43aa95d4ed596f31131d177386e383ffb820`

排序后唯一候选的逐行 SHA256：
`4e5213feed33a035dbe212e586fa6116631441280e14221c4d34188f5a4dfa07`

来源过滤、哈希及 XLSX 回读记录：`validation/final-real-10k/sampling.json`、`validation-report.json`。

## 中断恢复与 CLI/API 一致性

真实 10k 生成在持久化候选数 50,619 时调用进程 `os.Exit(75)` 强制退出，DB 没有正常 Close。退出后的检查点和重新打开的检查点均为 50,619，随后用原参数继续到 221,949；未丢失已提交候选、计数或来源关联。

最新复核证据：`validation/review/real-10k/interrupted-checkpoint.json`、`recovered-checkpoint.json`、`validation-report.json`。恢复完成后重新打开 DB，并将结果和来源全部流式导出、读回；最终候选哈希与历史验证相同。

随后用真实 CLI → HTTP API 独立导入同一 10k XLSX、重新生成并导出。通过最多 1,000 条一页读取全部候选，严格验证注册局域名边界和最终哈希；与中断恢复后的作业完全一致。最终交付 XLSX 也实际打开并逐行核对：Results 221,949 行、Sources 223,345 行、InvalidInputs 0 行（均不含标题）。

CLI/API 复核大作业位于本地 `validation/verified-service/jobs.db`。它保持 ready 状态，未被提交联网扫描。

## 性能观测

本机实际读取的规格：Intel Core i5-11300H @ 3.10GHz、8 个逻辑处理器、Windows 10.0.26200 x64。工具链 Go 1.27.3、Node 24.14.0；Windows race 使用校验过 SHA256 的便携 llvm-mingw。见 `validation/machine.json`。

修复后的完整 CLI 路径观测：导入 0.395 秒、生成 15.443 秒、导出 3.355 秒；服务进程峰值 Working Set 226,484,224 字节，约 216.0 MiB。历史修复前的 33.025 秒/279.8 MiB 记录仍保留，不将两次不同负载的测量解释为性能提升倍数。

这些是本机本轮绝对测量，期间还有其他检查执行，不作为跨机器吞吐保证，也没有用未运行的旧版基准声称提升倍数。生成已从逐规则 fsync 改为约 1,000 次候选尝试一批的事务，并保留原子检查点；候选总数不决定内存中列表大小。复核后的真实恢复验证进程采样 Go 堆峰值为 103,460,864 字节，与进程 Working Set 口径不同；该次恢复生成与另一个磁盘 fixture 并行，耗时 61.634 秒，不能和空闲运行直接比较。另有 10,000 条保留 .test 名称、每条 8,192 原始字节（base64 后 10,924 字符）证据的纯合成 fixture：导出 89,653,824 字节、采样导出堆峰值 59,962,504 字节，导出加回读 4.954 秒，网络请求数为 0。它只验证带证据导出的内存路径，不是实网扫描结果。

## 真实联网验证

最终候选抽样于 2026-11-01 17:42:29–17:43:09 UTC 实测完成：20/20，registered=5、not_found=15、unknown=0、error=0，20 次 HTTPS RDAP 尝试，最小发送间隔 **2,000.1144ms**。没有调用购买 API，not_found 不保证可购买。

`tools/sample-final.go` 离线遍历最终作业的候选/来源，校验整个候选集 SHA256 为 `4e5213feed33a035dbe212e586fa6116631441280e14221c4d34188f5a4dfa07`；按固定 SHA256 评分确定性选择 omission 7、repetition 7、transposition 6，覆盖 20 个不同 seed，20/20 确认为最终 221,949 候选的成员。`sample.json` 记录 seed、原始行号、规则、评分、源作业及算法；`tools/verify-final-sample.go` 离线核对实际 API 结果与抽样集合、发送间隔和导出，并添加来源证明页。大作业本身始终 ready、completed=0。

为精确查询已选候选，小作业导入这些名称，再使用 `--rules tld --tlds com --include-seeds --max-candidates 20`；同后缀变换不增加候选，断言候选恰为 20、未截断。小作业自身的 Sources 规则为 seed，原始 typo 规则和 10k 来源映射在 `SelectionProvenance` 页，不将二者混淆。

该设计是 **仅 .com、规则配额均衡、seed 不重复的确定性抽样**，不是全候选或全 TLD 的均匀随机样本，不能据 15/20 估计整体 not_found 比例或可购买率。证据在 `validation/review/stratified-online/{sample.json,status.json,results.json,report.json,large-job-unscanned.json}`，最终 XLSX SHA256 为 `a32f77e6a5dd87cb4af921dca13d981c7ecaf65ade553b5f67b1f13a14deb123`。

以下是保留的较早协议测试，不能替代上面的最终候选抽样：

实际 seed 从指定来源取出：indyweek.com（来源 CSV 第 10,404 行）、google.com（第 1 行）、itnews.com.au（第 10,401 行）；详见 `validation/verified-online/provenance.json`。为限制实网规模，生成上限为 20，实际扫描的是 indyweek.com 及其 19 个真实 typo，保留原始域名与来源关联；没有扫描整个 seed 表。

较早协议测试：20/20 完成，registered=1、not_found=19、unknown=0、error=0，共 20 次 HTTP 尝试。每 provider 配置间隔 2 秒，记录的最小请求发送间隔 2,000ms。证据是实时取得的 authoritative HTTPS RDAP 响应，不是 mock，也未调用购买 API。

**独立核对后的抽样限制：这 20 个样本取自 PSL 边界修正前的较早 seed 工作簿。indyweek.com 和 itnews.com.au 位于最终 CSV 10,309 行截点之外，不在最终 10,000 seed 中；20 个被查名称与最终 221,949 候选的交集为 0。** 它们是单个 .com seed 的前缀候选、针对空正文 404 的定向协议测试，不是最终大作业的抽样、不是无偏样本，也不能估计全部候选的注册比例。大作业联网数仍为 0。见 `validation/review/sampling-audit.json`。

初版实测发现 Verisign 的负回答为空正文 404。根据 [RFC 7480 §5.3](https://www.rfc-editor.org/rfc/rfc7480.html#section-5.3)，正文可以省略；修复后重新实测得到以上最终分类。非空畸形正文仍归 error。**not_found 只表示注册局查询没有记录，不能保证可购买。**

另外用一条 `tools/scan-excel.ps1` 提交 1 个候选，实际完成并返回 not_found。这个候选与上述样本重合，是单独的脚本验证，不计入 20 个样本的批次计数。完整记录位于 `validation/verified-online/report.json`、`results.json`、`one-command-final-status.json`。

## 实际通过的检查

- 全 backend `go test -race ./... -count=1`、`go vet ./...`、`go build ./...` 和可执行入口 build。
- `npm ci --ignore-scripts` 的干净安装、`npm run lint`、`npm run build`。仅 npm lockfile 作为依赖锁定文件；移除了会在构建时被重写的旧 Yarn lockfile。
- XLSX valid / duplicate / invalid / 无缓存公式输入；确定性生成和所有 8 类规则输出；来源关联；数量上限；分母选择；公式注入防护；超长证据显式截断，API 保留完整原始证据。
- fake HTTPS RDAP：registered、JSON 404、空正文 404、429 + Retry-After、timeout、malformed、名称不匹配、无支持 provider、稀疏元数据、未信任 TLS。
- 组合端到端：XLSX 导入 → typo → fake RDAP → 在途作业停止/DB 重开恢复 → 继续 → XLSX；全局 worker 上限、重复启动、暂停/续跑/取消、旧 lease 拒绝、结果幂等；持久化故障事务回滚和不丢队列。
- 实际 CLI/API 同一作业、JSON 与退出码、拒绝重定向、分页、导出；221,949 条实规模回读和哈希核对。
- CLI server 在打开 DB 前拒绝 `0.0.0.0`、`[::]`、外部 IP 和域名监听；跨站 Origin、null Origin、伪造 Host、Host/Origin 不匹配的写请求均为 403，作业保持 ready。
- Web 实际打开 CLI 创建的大作业，显示正确计数、pending 与部分结果，完成下一页操作；控制台 error 为空。核对的是浏览器页面，未启动大任务扫描。
- `git diff --check` 无空白错误。仍有原 frontend 构建工具的 Node deprecation 和 Browserslist 数据过旧提示，构建结果成功。

最新全量 race 输出为 `validation/review/final-race.log`（workflow 11.263 秒），vet 完成为 `final-vet.txt`；之前综合输出在 `validation/checks.log`、`e2e-check.log`。XLSX 回读使用 Excelize，没有声称在 Microsoft Excel GUI 中逐张检查了二十多万行。

源码发布前另做完整 backend race/vet/build（workflow 15.417 秒）、frontend `npm ci --ignore-scripts` / lint / build，均通过；锁文件改用官方 npm registry，依赖版本及完整性哈希保持不变。新增数据下载脚本在独立输出目录实测通过，下载的大小和 SHA256 匹配固定元数据。该轮输出在本地 `validation/publication-preparation/`。

## 独立收尾审查

两条独立审查轴的原始发现与修复说明保存在 `FINAL-REVIEW.zh.md`。审查确实找出原测试遗漏：整表加载、生成/取消入场竞态、ready 作业忽略新参数、重定向及大/损坏 429 正文绕过冷却、最终 ZIP 在堆中累积。已逐项修复并补专门回归；不是仅以原测试通过为依据。

实际源文件和许可证的 Git blob 与 GitHub 固定 commit 的 tree 元数据一致；该 commit 日期为 **2025-11-20T18:55:34Z**，不是声称 2026 年实时排名。使用的是固定 commit 下的 raw `data/top-100k-domains.csv`，没有把 README 的 v1.0 release 下载 URL 当作实际来源。独立逐行核对 seed 工作簿与 CSV 前 10,000 个有效唯一名称的顺序完全一致。见 `validation/review/source-audit.json`、`sampling-audit.json`。

认证初始化无需口令或 token：新入口没有账户/JWT/CLI bearer token，明确使用单机 loopback 信任边界；拒绝非 loopback 监听并校验 Host/Origin，防浏览器跨站写和 DNS rebinding。同机其他用户/进程不受身份隔离。旧 Web/购买/Redis 接口未注册到新 main，不能通过当前 API 绕过共享服务。**当前只验收本地 CLI/API；远端或 VPS 的安全部署未验收，不应直接作为公网管理服务使用。** 更详细边界及导入/导出资源上限见调用说明。

## 可复现的离线容量检查

先安装 Go >= 1.25，并从仓库根目录运行：

```powershell
./tools/fetch-testdata.ps1
cd src/backend
go build -o ../../bin/powerwhois-scale.exe ./cmd/scale
# 预计退出码 75，模拟没有 Close 数据库的进程中断。
../../bin/powerwhois-scale.exe --source ../../testdata/real/top-100k-domains.csv --output ../../validation/reproduce --interrupt-at 50000
../../bin/powerwhois-scale.exe --source ../../testdata/real/top-100k-domains.csv --output ../../validation/reproduce --resume
```

该程序不进行 DNS、WHOIS、RDAP 请求，不执行来源仓库脚本。下载仅为固定 commit 下的 100k CSV（约 1.4 MB），不下载 100 million 数据集；文件大小和 SHA256 不匹配时拒绝使用。恢复后的 `validation-report.json` 应记录 10,000 seed、221,949 唯一候选、223,345 来源关联，候选行哈希与上文一致。

`go run ./cmd/evidence-scale --output ../../validation/evidence-reproduce` 是独立的 `.test` 合成证据容量测试，不是实网结果。完整离线回归可运行 `./tools/check.ps1 -Race`；Windows race 另需 C 编译器。GitHub Actions 仅执行 backend race/vet/build 与 frontend lint/build，不启动真实在线扫描。

## 未执行范围

没有进行 221,949 个候选的全量联网扫描，也没有验收远端安全部署或第三方历史工作簿的旧格式还原。本地完整验证资料仍保留，公开发布排除数据库、结果 XLSX、二进制、node_modules 和下载的 CSV。

原本地验收使用 `go build -a -trimpath`，重复构建哈希一致，二进制 SHA256 为 `5c5cd585ebbb53b723068551ff2bc3a0dae75a82900e42f2f867cc26ced2aa5a`。该二进制不随源码发布；不同 Git 提交或工具链重新构建可产生不同哈希。
