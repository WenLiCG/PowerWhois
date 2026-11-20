# PowerWhois 开发暂停交接（2026-11-19，PST）

## 状态与责任
用户于 2026-11-19 11:20 PST 要求停止开发，完成 handoff 后更新 GitHub。本文件仅归档，不授权续写、真实链路重试、组合回归、合并或部署。下一责任人是 PM（归档协调）；未来开发写者待用户恢复后由 PM 明确，不自动改派。
唯一仓库：https://github.com/WenLiCG/PowerWhois 。唯一执行协调入口：https://cursor.com/agents/bc-2e5599b0-30b6-5ee4-a13c-931846b7b367 。只经既有入口归档，不新建重叠实现。

## 固定候选与本地现场
- PW-7d，https://github.com/WenLiCG/PowerWhois/pull/8 ：QA 于今日核实远程 head f83824c3b9e4d374a24826c11c81c64ffbaddf87，tree 8583233582ee7e0344e4a5abed2aa9a0d9368add，base 5774f763d1d72ef294c1666329ca4bebab0f34f7。归档时应实时复核，若不同保留历史证据且不可沿用通过结论。
- PW-7 本地 /workspace/codex/powerwhois/PW-7/repo，分支 pw-7-d1-fix，HEAD abec1a4777e3ee3fa3d937057f1729525c316838；2026-11-19 本次只读核对 status 无未提交项，tree 与上述候选相同。本地与远程 SHA 因补丁应用提交者不同而不同，不应重新覆盖推送。
- PW-8，https://github.com/WenLiCG/PowerWhois/pull/9 ：独立报告固定远程 head 9fba695ff93c5d243e608b484e5473733b2d9019，tree f9ff8aa626f8725d49c99bb6a2b17bff337e0636，base eb422ab7221ade327a08fd1334c5e813870872e7；这是历史固定版本，归档时须实时复核。
- PW-8 本地 /workspace/codex/powerwhois/PW-8/repo，分支 pw-8-cn-parse-panic，HEAD 2572b62d924bd6f2b7286ee92a81f72d71cb10fa；本次 status 无未提交项，tree 与上述候选相同。
- 生成方式：由 Codex gpt-6.1-sol 生成、Codex 程序员复核。没有让 sol 独立审查自身实现。此次停止后未改源码、未重跑测试。

## 原任务与实现
PW-7d：SOCKS CONNECT REP 1–8 在 WHOIS/43 与 RDAP/443 统一报告“代理连接失败”，固定日志 rep=N，不回退直连，不记录原始敏感错误；REP0 后的目标超时仍保留超时语义。共享 proxydial 与调用方/相关测试已提交。
任务原文 /workspace/codex/powerwhois/PW-7/specs/7d-red.md 与 7d-green.md；早期 D1/L1 约束在 specs/7c.md 和 7c-green.md。实现报告 logs/7d-codex-report-green.md；交付补丁 delivery/pw7d.mbox 和 delivery/SHA256SUMS-pw7d.txt 均在 PW-7 根目录下。
PW-8：离线修复 .cn carbon 解析 panic，字段与入口防护、匹配布局、main 包引入 time/tzdata；未编辑 main.go，不扩大 RDAP 解析修复。
任务原文 /workspace/codex/powerwhois/PW-8/specs/pw8-red.md 与 pw8-green.md；作者详报 /workspace/codex/powerwhois/PW-8/delivery/REPORT.md（历史“未推送”字样描述当时本地交付，后续已有 PR9，不代表现在仍未同步）。

## 作者验证（不代替独立 QA）
Go 1.27.3 linux；GOEXPERIMENT=nojsonv2，GOFLAGS=-ldflags=-checklinkname=0，离线 GOPROXY=off/GOSUMDB=off。
PW-7d 作者 RED 的 REP 用例先失败，GREEN 后作者全量、vet 与 race 已通过；详细命令与日志需连同原记录保留，不能只引用摘要。Codex 沙箱最初不能监听 socket，仅编译与内存测试通过；随后作者本地完整验证与独立 QA 是另行证据，不得把沙箱未跑项写成沙箱通过。
PW-8 有效 RED 是 go test -c -trimpath 后 env -u GOROOT -u ZONEINFO 运行两层二进制；首次均因 missing Location 失败。普通 go test 的 RED 反而通过，不能作为有效失败证据。GREEN 两层 tz-less 与正常环境通过；go test ./... -count=1 -v -timeout 300s 与 go vet ./... exit0。具体命令、日志和残余见 delivery/REPORT.md。

## 独立质量结论与失败/重试
PW-7d 总体 blocked，以 QA 原报告为准：
/workspace/powerwhois/qa-pw7d-85832335/PW7D-REVIEW-85832335-20261119.md
SHA256 8352a5b20d95df506b40539cb10decf775927e744babac9e75ef80b3e45c770f。
- D1-443 本地原生完整进程 passed；D1-43 仅 QA-only 强制路由 harness passed，非原生 COM WHOIS 或真实43验收。
- L1 静态与已执行进程日志扫描 passed，凭据命中0；真实供应商认证成功后的日志覆盖 blocked。
- 真实 Bright Data 443、43、错口令认证均 blocked：出口 HTTP CONNECT=200 后 SOCKS greeting 等待20秒无响应；不能推断供应商拒绝43，也不能把错误分类符合预期当真实认证通过。
- 原两层 GREEN 首次失败涉及 -trimpath 后 runtime.Caller 的相对源码路径；QA 临时路径映射后 r2 于07:08 PST exit0，07:34 PST 本地补测 exit0。首次失败保留，未改候选，不再无故重复已通过项。
- QA 已停自建进程、清理临时凭据配置；未启动组合回归。
PW-8 独立 pass 仅对应固定 head：
/workspace/powerwhois/qa-pw8-f9ff8aa6/PW8-REVIEW-f9ff8aa6-20261113.md 。报告时间换算为 2026-11-13 04:43–04:45 PST。RED/GREEN 两层、额外解析探测、全量与 vet 已验证；CI check_count=0 不等于 CI passed；Go1.23、RDAP同类路径及线上验证未运行。

## 边界、未完成与恢复判据
当前 PW-7d blocked，不进入 PW-8 组合，不合并、不部署、不改线上 ZONEINFO，不访问 whois3，不执行真实 .cn 查询；不自行更换出口、凭据或扩大操作。旧真实代理工具带 .cn 查询，不能原样运行。秘密只经平台密钥，不上传任何原始凭据配置、原始敏感日志或环境文件。
恢复必须有用户明确授权和 PM 续作票；先核对远程 base/head/tree、目录/分支/diff 与现有证据。若获授权，先恢复可验证真实代理链路并独立判定443、43、认证拒绝和成功日志覆盖，PW-7d 正式通过后再执行既定组合回归；此处是未来判据，不是现在的执行指令。
建议方法：software-handoff v1.3；software-team-workflow v1.4；独立验证使用 software-release-gate，故障使用 systematic-debugging。共享聊天技能不等于远端已安装。

## GitHub 归档要求
本交接及两份独立报告的安全正文应进入现有项目正式记录或 PR 可访问文档；不能仅留下共享电脑绝对路径。远端记录应标明 paused/blocked、固定候选、未测与禁止操作，保留 QA 原判，不追加代码修复。由 PM 协调唯一归档写入者，避免多人重复更新。GitHub 完成链接与归档提交/评论标识需回报后才算更新完成。
