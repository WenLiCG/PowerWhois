# PowerWhois 停止开发归档（2026-11-19）

开发已经停止。本目录只保存既有交接与已审报告，供以后查阅。它不恢复开发，不授权合并、部署或发布。

## 本文如何产生

- 方法：software-handoff 1.3，software-team-workflow 1.4。方法名只说明整理方式，不表示本仓库安装了这些工具，也不表示本轮做了发布门禁或新的独立测试。
- 材料来源：已审交接附件，以及项目里已有的记录。附件在收入前已核对，zip SHA256 为 `0e0543019e095f0a16b99311341ad6e0f04e792f4f430a735820a167486217aa`，20813 字节，5 个文件。
- 本 README 把附件里的状态、边界和可访问位置整理成一份说明。同目录下的交接正文和审查报告按附件原文字节保存，哈希与附件清单一致，便于核对。没有改写这些报告的结论。
- 归档写入前只读核对了 GitHub 上的 `main`、PR #8 和 PR #9。没有跑测试，没有做组合回归，没有使用付费代理，没有改出口或凭据，没有做 .cn 查询，没有改 ZONEINFO 或生产环境，没有做 RDAP 修复。
- 本提交只有文档。PR #8 与 PR #9 的源码分支没有被这次归档推送或改写。

## 远程核对（只读，2026-11-19）

下列 head 与 tree 与附件中的历史预期一致，因此可以保留附件里已经写明的结论。结论只针对这些确切提交，不自动延伸到其他分支或以后的新提交。

| 引用 | 分支 | head | tree | 本次如何使用既有结论 |
|---|---|---|---|---|
| `main` | `main` | `eb422ab7221ade327a08fd1334c5e813870872e7` | `789901b59f4a8174642463357b4531cad4b6b68b` | 本归档分支从这一提交建出 |
| [PR #8](https://github.com/WenLiCG/PowerWhois/pull/8) PW-7d | `pw-7-whois-proxy-hardening` | `f83824c3b9e4d374a24826c11c81c64ffbaddf87` | `8583233582ee7e0344e4a5abed2aa9a0d9368add` | 远程仍是该 head/tree，总体 **blocked** 继续有效 |
| [PR #9](https://github.com/WenLiCG/PowerWhois/pull/9) PW-8 | `pw-8-cn-parse-panic` | `9fba695ff93c5d243e608b484e5473733b2d9019` | `f9ff8aa626f8725d49c99bb6a2b17bff337e0636` | 远程仍是该 head/tree，历史独立 **pass** 继续有效 |

PR #8 的 base 在原报告中是 `5774f763d1d72ef294c1666329ca4bebab0f34f7`。PR #9 的 base 是上面的 `main`。本次看到两者仍是 open draft，未合并。

另外只读看到、且本次没有推送的 open draft：

- [PR #5](https://github.com/WenLiCG/PowerWhois/pull/5) `pw-5-mask-admin-secrets`，head `c901821f59aa5fec9d778d38adfca5c8b0dbb038`，tree `995f3acffc6f14f7db2f995c447bcbfdf0055566`，base `main`。
- [PR #6](https://github.com/WenLiCG/PowerWhois/pull/6) `pw-6-dates-proxy-hardening`，head `5774f763d1d72ef294c1666329ca4bebab0f34f7`，tree `4f220a886caa123759523ad512454d52ca18c4ca`，base `pw-5-mask-admin-secrets`。

开发交接里记录的本地作者提交与远程 head 不同、tree 相同：PW-7d `abec1a4777e3ee3fa3d937057f1729525c316838`，PW-8 `2572b62d924bd6f2b7286ee92a81f72d71cb10fa`。这是交接原文中的对象记录。不应为了让 SHA 一致而覆盖推送。

## 状态

### Blocked

- 真实 Bright Data 的 443、43，以及错误口令认证，都是 **blocked**。本地 D1 与 L1 不能代替这三项真实验证。
- D1-443 只说明本地完整进程上的错误分类。D1-43 只说明 QA 专用强制路由 harness，不是原生 COM WHOIS，也不是真实 43 验收。
- L1 只覆盖静态检查和已经执行过的进程日志。真实供应商认证成功之后的日志覆盖仍然 blocked。
- 出口 HTTP CONNECT 返回 200 之后，SOCKS greeting 约 20 秒没有响应。这不能证明认证或访问成功，也不能按供应商拒绝 43 的例外放行。一次没有响应的 greeting **不能归因于产品，也不能归因于供应商**。
- 因此 PR #8 总体 **blocked**。不进入 PW-7 与 PW-8 的组合回归，不据此合并或发布。

### Passed（既有证据，范围已写明）

- PR #8 上，本地 D1-443、QA harness 的 D1-43、已执行路径的 L1，以及既有两层 GREEN 与本地补测，在原报告中为 passed。两层 GREEN 第一次因 trimpath 源码路径映射失败并已保留；QA 做路径映射后的再次结果为通过。候选实现没有为了通过而改动。
- PR #9 在上述固定 head/tree 上有历史独立 pass：两层 RED/GREEN、额外离线解析探测、全量 `go test` 与 `go vet`。这不是与 PW-7 的组合通过，也不是合并或部署批准。

### Failed（保留，未改候选）

- PR #8 两层 GREEN 的首次运行失败，原因是 `-trimpath` 之后的相对源码路径，不是产品断言失败。失败记录保留。
- 真实 443 成功预期没有达到。自动结果里的错误类别匹配不能把真实 43 或错口令认证写成通过。正式结论维持 blocked。

### Untested（本归档没有补跑）

- PW-7 与 PW-8 组合回归。
- 真实 .cn 查询、whois3、线上 ZONEINFO、生产部署。
- PR #9：GitHub Checks（当时 check_count 为 0，不等于 CI passed）、Go 1.23、RDAP 同类路径、线上验证。
- 真实认证成功之后的日志保密覆盖，以及其他 TLD、额度、压力和部署构建流程。

作者自测不能代替独立 QA。

## 授权边界

用户已要求停止开发。本次 GitHub 更新只把交接放到仓库里。

不允许从本文件引申出下列操作：恢复编码或测试、组合回归、付费代理、更换出口或凭据、.cn 查询、ZONEINFO 或生产变更、RDAP 修复、合并、部署、发布、强推，以及改写 `pw-7-whois-proxy-hardening`、`pw-8-cn-parse-panic`、`pw-5-mask-admin-secrets`、`pw-6-dates-proxy-hardening`。

以后若要继续，需要用户明确授权，并先重新核对当时的远程 head 与 tree。上面的 blocked 与历史 pass 只在提交仍是表中的 SHA 时沿用。

## 可访问材料

材料在本仓库本目录，不依赖共享电脑上的绝对路径。

| 仓库路径 | 作用 | SHA256 |
|---|---|---|
| `docs/archive/2026-11-19-stop/README.md` | 本说明 | 随本提交 |
| `docs/archive/2026-11-19-stop/DEVELOPER-HANDOFF.md` | 开发停止交接 | `39d39c1f7d0f1576d739d51a6979734982bdb82c8057a5bae11fa75e87234a99` |
| `docs/archive/2026-11-19-stop/QA-HANDOFF-STOP-20261119.md` | QA 停止交接 | `f7b3eac14f66ad1a700ada7a9dc600a9019c14d005591423752e5dfc65e2ffa6` |
| `docs/archive/2026-11-19-stop/PW7D-REVIEW-85832335-20261119.md` | PR #8 独立报告，总体 blocked | `8352a5b20d95df506b40539cb10decf775927e744babac9e75ef80b3e45c770f` |
| `docs/archive/2026-11-19-stop/referenced-safe-evidence/PW8-REVIEW-f9ff8aa6-20261113.md` | PR #9 历史独立 pass | `a1ddacc5ac7c9d961935e9026dbaed58e64d602713b7f16dee462435dc22f43d` |
| `docs/archive/2026-11-19-stop/MANIFEST.md` | 附件整理时的清单原文 | `8404ee342763abbe301a621eaf0554c9c93d71e6be6d4145f4d60d2e77c3d00b` |

清单原文写明，整理附件时尚未实时核对远程。上表的远程核对是本次归档写入前补上的只读事实；head 与 tree 未变，所以没有改写 blocked 与历史 pass。

仓库根目录的 `HANDOFF.md` 指向本目录。该文件前半仍是 2026-11-08 的历史交接，不是本次停止结论。

## 敏感信息

附件在收录前已做过敏感检查：没有密码、令牌、私钥、带口令的完整代理 URL、账户日志或私人数据。报告里的公开诊断上下文（代理主机与端口、本地转发地址、SOCKS greeting 字节，以及“凭据未输出、认证未完成”）不含凭据值，为保留 blocked 依据而按原文保留。

本目录没有配置、账户日志、二进制或密钥。若后续阅读发现凭据，应省略该值，不要把它再写入仓库。
