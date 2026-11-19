# PowerWhois 停止开发归档清单（安全附件）

整理时间：2026-11-19，America/Los_Angeles（PST，UTC−8）。
整理范围：只复制并敏感检查既有交接/审查正文，不改实现、不跑测试、不访问真实代理、不上传 GitHub、不联系代理、不改 routines、不操作 CloudAgent 或仓库。
方法：software-handoff v1.3。本清单是归档说明，不是新的质量结论。

## 项目与入口
- 项目：PowerWhois
- 仓库：https://github.com/WenLiCG/PowerWhois
- 既有唯一执行协调入口（来源：开发交接原文）：https://cursor.com/agents/bc-2e5599b0-30b6-5ee4-a13c-931846b7b367
- 下一责任人：PM 将本附件交上述既有 Project 只存档。不新建 Project，不合并，不部署，不恢复开发。
- 本包与 VPS Review、TunnelBrowser 严格分开，不得混入同一提交或同一 Project 任务。

## 必须保留的质量边界
- PR8（PW-7d）https://github.com/WenLiCG/PowerWhois/pull/8：head `f83824c3b9e4d374a24826c11c81c64ffbaddf87`，tree `8583233582ee7e0344e4a5abed2aa9a0d9368add`，base `5774f763d1d72ef294c1666329ca4bebab0f34f7`。独立 QA **总体 blocked**。归档时须实时复核远程；若不同，只保留本历史证据，不可沿用通过结论。
- PR9（PW-8）https://github.com/WenLiCG/PowerWhois/pull/9：head `9fba695ff93c5d243e608b484e5473733b2d9019`，tree `f9ff8aa626f8725d49c99bb6a2b17bff337e0636`，base `eb422ab7221ade327a08fd1334c5e813870872e7`。这是**历史独立 pass**，不等于与 PW-7 组合通过，也不是当前远程的实时结论。
- 本地 D1（443 完整进程 / 43 仅 QA harness）与 L1（已执行路径日志扫描）通过，**不能替代**真实 Bright Data 43、443 与认证；真实三项均为 blocked。HTTP CONNECT=200 后 SOCKS greeting 约 20 秒无响应，不能证明认证或访问成功，也不能归因供应商或产品，不能按供应商拒绝 43 的例外放行。
- 作者自测不能代替独立 QA。未运行 PW7+PW8 组合回归。不合并、不部署。

## 敏感检查
对下列归档正文逐份阅读，并扫描 password/token/密钥/user:pass URL/邮箱等模式。
- 未发现密码、令牌、私钥、完整代理 URL、账户日志或私人数据。
- 报告中的 `brd.superproxy.io:22228`、本地 `127.0.0.1:8791`、SOCKS greeting `05 01 02`、以及“凭据未输出/未完成认证”等表述，属于已写在 QA 正式报告中的公开诊断上下文，不含凭据值；为保留 blocked 依据而原文保留。
- 因此**未做脱敏**。归档副本与源文件字节一致。原件未改。

## 收录文件（均已审过）

| 归档路径 | 角色 | 源路径 | SHA256 |
|---|---|---|---|
| DEVELOPER-HANDOFF.md | 开发停止交接 | /workspace/codex/powerwhois/handoff-20261119/DEVELOPER-HANDOFF.md | 39d39c1f7d0f1576d739d51a6979734982bdb82c8057a5bae11fa75e87234a99 |
| QA-HANDOFF-STOP-20261119.md | QA 停止交接 | /workspace/powerwhois/qa-pw7d-85832335/QA-HANDOFF-STOP-20261119.md | f7b3eac14f66ad1a700ada7a9dc600a9019c14d005591423752e5dfc65e2ffa6 |
| PW7D-REVIEW-85832335-20261119.md | PR8 完整独立报告（blocked） | /workspace/powerwhois/qa-pw7d-85832335/PW7D-REVIEW-85832335-20261119.md | 8352a5b20d95df506b40539cb10decf775927e744babac9e75ef80b3e45c770f |
| referenced-safe-evidence/PW8-REVIEW-f9ff8aa6-20261113.md | 交接引用的 PR9 历史独立 pass 报告 | /workspace/powerwhois/qa-pw8-f9ff8aa6/PW8-REVIEW-f9ff8aa6-20261113.md | a1ddacc5ac7c9d961935e9026dbaed58e64d602713b7f16dee462435dc22f43d |
| MANIFEST.md | 本清单 | 本归档生成 | （见包外清单或重新计算） |

PW8 报告不在任务点名的三份主源之内，但开发/QA 交接均引用它作为 PR9 历史 pass 的独立证据，为保留 PR8 blocked 与 PR9 历史 pass 的区别而纳入；已同样敏感检查。

## 明确不收录
未复印原始配置、原始/脱敏日志、密钥、证书、`remaining-20261119/`、作者补丁/mbox，以及仓库源码快照。这些原件仍在共享电脑原路径，未改。作者详细命令日志以交接引用的原记录为准，本包只保留报告正文。

## 需要 PM 注明的局限
- 本包只是可上传的安全附件，整理者未上传、未操作 Cloud Project。
- 远程 head/tree 未在本轮实时核对。
- 真实代理验证仍为 blocked；不得把本归档写成质量通过。
- 无本轮开发停止交接之外的新作者 diff；本地作者提交 SHA 见开发交接（同 tree，SHA 因 commiter 不同而不同，不应重新覆盖推送）。
