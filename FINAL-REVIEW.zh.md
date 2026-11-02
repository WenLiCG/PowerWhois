# 独立收尾审查

比较基点为 `a0757083bc0628011e8cf725dc3cfdb11a623933`。修改尚未提交，因此审查使用 `git diff HEAD` 加所有未跟踪新源码/测试/说明；不能用空的 `HEAD...HEAD` 冒充完整 diff。两条子代理独立完成审查，下面保留各自轴，不混为一个评分。

## Standards

独立报告的硬问题（行号指修复前快照）：

1. **P1，excel.go:127**：`GetCellFormula` 进入 Excelize 的整表加载和稀疏行列扩展路径，破坏了流式和内存边界。已改成 ZIP/XML 所选列解析、磁盘 spool 和仅需共享字符串；补 XFD 稀疏列、高行号、压缩/文本/token 预算测试。
2. **P2，excel.go:192/219**：生成在事务外读 draft，可能覆盖先提交的取消。已把校验和 generating 转换放进同一写事务；补持有未提交取消的确定性入场回归，以及并发重复 pause/resume/cancel 队列守恒测试。
3. **P2，provider.go:202**：重定向后将 B 的 429 冷却施加给 A；大/损坏正文也能跳过 Retry-After。已在正文读取前处理状态/头，使用最终 provider host，规范 host/default port，并保存实际 provider URL。补跨作业 A→B / direct B、本体中断/过大、取消和 HTTPS 降级拒绝测试。

没有额外可操作的风格 heuristic 问题。子代理确认新 main 不注册旧购买/Redis/WebSocket 路径，loopback Host/Origin 防护未发现额外可复现缺口。

## Spec

独立报告的需求缺口：

1. **P1，“安全取消”**：相同生成入场竞态，修复及测试同上。
2. **P1，“429 共享退避 / provider 尝试证据”**：相同最终 host 和正文分支问题，修复及测试同上。
3. **P2，“预览/数量上限”**：ready 作业传入新规则/上限时静默成功但不改变候选。现相同参数幂等，不同参数返回 409，要求新建导入作业。
4. **P2，“大规模 XLSX 有限内存”**：导入整表问题，且 Excelize 最终 WriteToBuffer 仍累积完整 ZIP。导入已修；导出通过受支持的 ZipWriter 工厂将压缩字节直写目标，并设 512 MiB 文件和小于 2 GiB 单 XML 成员边界。补 ZIP 回读、输出/成员限额和 checked-evidence 容量 fixture。

该轴未将“无 token”误列成用户未要求的 token 功能失败，但要求准确说明认证边界。当前入口不初始化账户/JWT/CLI token，信任本机 loopback；同机其他用户/进程不隔离。禁止公网 bind，旧 JWT 示例值清空且旧入口不可达。

## 数据和证据边界

独立源审计确认 raw 100k CSV 与固定 commit 的 Git blob 一致、MIT 许可相符；commit 日期为 2025-11-20。最终 10k 工作簿逐行与按原顺序选取的前 10,000 有效唯一名称一致，来源截点 10,309 行。没有将 README release URL 误记为实际下载来源。

同时确认原在线 20 是较早工作簿中单个 indyweek.com 的候选前缀，不在最终 10k 中；与最终 221,949 个候选交集为 0。它是有明显 seed/TLD/选择偏差的真实协议测试，不能代表最终批量任务，更不能据其 19/20 估计可注册率。说明和验收文档已明确更正，旧证据保留。

随后在最终候选集中另抽 20 个真实在线样本：20 个不同 seed，omission 7 / repetition 7 / transposition 6，确定性 SHA256 评分，仅 .com。全候选哈希和 20/20 成员关系已核实。实际结果为 5 registered、15 not_found，20 次尝试，最小发送间隔 2,000.1144ms；不是无偏全 TLD 样本，不能估计整体可购买率。样本列表、规则/来源关联及结果保存在本地忽略的 `validation/review/stratified-online/` 和 `validation/deliverables/final-corpus-stratified-live.xlsx`。只启动独立 20 条小作业，大作业仍 ready、completed=0。

## 检查与未声称事项

完整复测、真实 10k 恢复重跑、checked-evidence fixture、构建命令/哈希分别写入 `validation/review/`。checked-evidence fixture 使用保留 `.test` 名称和 fake provider，网络请求数为 0，与真实域名生成、原协议 20 和最终候选集分层 20 的记录严格区分。

最终额外回归确认：非 loopback 监听在 DB 打开前被拒绝；错误 Host、跨站/null Origin、Host/Origin 不匹配的写请求均被拒绝，不能启动 ready 作业。新入口不初始化任何 token，不持久化凭据；该本地进程信任模型不隔离其他本机用户。远端/VPS 的安全部署没有验收。最新完整 `go test -race ./... -count=1` 与 `go vet ./...` 通过。

未启动 22 万全量联网扫描，未验收远端部署。测试数据库、工作簿和完整响应留在本地，不随代码发布；固定数据源与复现方法见 `VALIDATION.zh.md`。未因现有测试通过而宣称所有可能故障都已排除。

轴内统计：Standards 3 项、最严重 P1；Spec 4 项、最严重 P1。上列问题均已实施修复，最终运行结果以各日志和构建/导出 manifest 为准。
