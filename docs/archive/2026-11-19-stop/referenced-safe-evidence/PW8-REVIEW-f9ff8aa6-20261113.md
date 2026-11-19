# PowerWhois PW-8 独立审查报告

- 结论：**pass**
- 时间：2026-11-13 20:43–20:45（Asia/Shanghai）
- 审查对象：[PR #9](https://github.com/WenLiCG/PowerWhois/pull/9)（Draft，对 main）
- 远程固定 head：`9fba695ff93c5d243e608b484e5473733b2d9019`
- tree：`f9ff8aa626f8725d49c99bb6a2b17bff337e0636`（与作者本地 GREEN `2572b62d` 一致；SHA 不同是 committer 变为 Cursor Agent）
- RED：远程 `ba63998f8d68f7874871176b815f13267e91135d`，tree `45b1f8c857077dd52483edad235427c381eda769`（与本地 `dc2d360` 一致）
- base：`eb422ab7221ade327a08fd1334c5e813870872e7`
- 环境：Go 1.27.3 linux/amd64；`GOEXPERIMENT=nojsonv2`，`GOFLAGS=-ldflags=-checklinkname=0`，`GOPROXY=off`；共享电脑独立 scratch，未改作者分支，未在线上查 .cn
- 证据目录：`/workspace/powerwhois/qa-pw8-f9ff8aa6/`

## 范围与方法

按事先约定的两层方法验证：`go test -c -trimpath` 后用 `env -u GOROOT -u ZONEINFO` 运行测试二进制。层 a 为 `whoislib`（不嵌入 `time/tzdata`，缺 PRC 时日期应留空、其他字段保留、不 panic）；层 b 为 `package main`（依赖真实 `init` 与 `tzdata.go`，缺外部时区源时日期仍须正确）。普通 `go test` 会提供 GOROOT，其通过不能当作 RED 证据。

## 代码审查

| 项 | 结果 | 说明 |
|---|---|---|
| `tzdata.go` 仅 `import _ "time/tzdata"`，未改 `main.go` | pass | 与 base 的 `main.go` 字节一致；package main 的生产改动只有 `tzdata.go` |
| 解析器处理 carbon 带错 | pass | `parseWhoisDate` 在 `c.Error != nil` 时返回空字符串，并有字段级 recover |
| 解析入口 recover | pass | `ParseWhoisResponse` 入口 recover 后返回明确错误，保留已解析字段（`TestPW8ParseEntryRecovers`） |
| 为 cn、中国、中國、tw 补 DateTimeLayout | pass | 布局均为 `2006-01-02 15:04:05` |
| 与 PW-7 无文件重叠 | pass | PW-8 改的是解析器/匹配器和 `tzdata.go`；PW-7d 改的是 `proxydial`、`whois_query.go`、`rdap_query.go` 及相关测试 |

`scheduler/web_check.go` 的 `webCheckHandler` 仍无 recover。本次已知的 `.cn` panic 已由 tzdata 与解析器两层挡住，不判 fail；handler 级 recover 仍是残余建议，不是本任务阻塞项。

## 检查项

| ID | 项 | 状态 | 命令/证据 | 实际 |
|---|---|---|---|---|
| R1 | 远程 head/tree 与声明一致 | pass | `get_pull_request` / `get_commit`，本地 tree 核对 | head `9fba695f`，tree `f9ff8aa6`；RED tree `45b1f8c8` |
| A-RED | 层 a，trimpath 且去掉 GOROOT/ZONEINFO | pass | `go test -c -trimpath` + `env -u GOROOT -u ZONEINFO`，`TestPW8CNParse*` | 3 个子用例均 fail，报 `panic: time: missing Location in call to Date`。`logs/red-whoislib-tzless.log` |
| B-RED | 层 b，同上 | pass | 同上，`TestPW8CNParseMainTimezoneData` | fail，同样 panic。`logs/red-main-tzless.log` |
| CTRL | 普通 `go test` 在 RED 上通过，不能当 RED | pass | `go test -v -count=1`（由 go 提供 GOROOT） | 层 a、层 b 均 PASS。`logs/red-whoislib-gotest.log`、`logs/red-main-gotest.log` |
| A-GREEN | 层 a tzless：不 panic，日期留空 | pass | 同上方法跑 GREEN whoislib `^TestPW8`；另做探测 | 官方测试 PASS。探测：`LoadLocation(PRC)` 为 `unknown time zone PRC`；cn/中国/中國/tw/au 日期均为空，registrar 等字段保留。`logs/green-whoislib-tzless.log`、`logs/probe-green-whoislib-tzless.log` |
| B-GREEN | 层 b tzless：日期正确 | pass | GREEN main `TestPW8CNParseMainTimezoneData` | PASS，断言 `2003-03-17 12:20:05` / `2029-03-17 12:48:36`。探测中 PRC 可加载，cn/中国/中國/tw/au 日期同上。`logs/green-main-tzless.log`、`logs/probe-green-main-tzless.log` |
| NORM | 普通环境与修复前结果一致 | pass | 对同一离线样本，GREEN 与 base `eb422ab` 做普通 `go test` 探测 | cn/中国/中國/tw/au 的 CreationDate/ExpiryDate 均为 `2003-03-17 12:20:05` / `2029-03-17 12:48:36`，两边一致。`logs/probe-green-whoislib-gotest.log`、`logs/probe-base-whoislib-gotest.log` |
| GUARD | carbon 错误路径与入口 recover | pass | `TestPW8ParseCarbonErrorPaths`、`TestPW8ParseEntryRecovers` | GREEN tzless 与普通环境均 PASS |
| FULL | `go test ./...` | pass | `src/backend` 下 `go test ./... -count=1` | typnamer、whoislib ok，vet 无输出，退出码 0。`logs/green-gotest-full.log`、`logs/green-vet.log` |
| CI | GitHub Checks | unrun | `list_check_runs_for_ref` on `9fba695f` | `total_count=0` |
| GO123 | Go 1.23 | unrun | 本机只有 go1.27.3 | 未安装 1.23 |
| PROD | 线上 / 查询 .cn | unrun | 按要求未做 | 未在 whois3 或公网查 .cn |
| RDAP | RDAP 同类路径 | unrun | 范围外 | 未测 |

说明：作者测试只覆盖 `.cn` 样本。中国、中國、tw 以及未改布局的 au 对照，是 QA 在 scratch 里另写的离线探测，不在作者测试套件中。

## 未测 / 残余

- CI 与 Go 1.23 未跑。
- 未在线上验证，也未查询 .cn。
- RDAP 解析不在本任务范围。
- `webCheckHandler` 仍无 recover，建议可在后续任务补，不阻塞本次结论。
- 与 PW-7 的组合回归要等 #8 远程 head 对上后再做。

## 质量建议

PW-8 在固定 head `9fba695f`（tree `f9ff8aa6`）上 **建议通过**。这不是用户批准，也不等于可以合并或部署。
