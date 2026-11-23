# PowerWhois API 文档

当前版本化查询接口以 [OpenAPI 3.0 JSON](src/backend/api/openapi.json) 为准，服务路径 `/api/openapi.json`。

完整请求、分页、错误、状态、CLI/MCP、权限说明和示例见 [docs/USAGE.md](docs/USAGE.md)。

## 保留的 Web HTTP 接口

- `POST /api/login`：管理员登录，返回 JWT。
- `GET /api/web/setting`：公开网页查询参数（不含管理秘密）。
- `GET/PUT /api/admin/setting`：管理配置。
- `GET/DELETE /api/admin/log`：下载/清空日志。
- `POST /api/admin/bulkcheckupload`：旧版批量 TXT 上传。
- `GET /api/admin/bulkcheckresultdownload`：旧版当前批量任务下载。
- `POST /api/admin/tasks/{id}/select`：选择旧版批量兼容接口的当前任务。

所有 `/api/admin/*` 仅接受管理员 Bearer JWT，查询 token 无效。外部 Agent 应使用 `/api/v1`，不复用管理凭据。

## 保留的 WebSocket 协议

同源连接 `/app/ws`，每条消息为 `{ "event": "...", "data": ... }`，最多 4 MiB。token 不放 URL；连接后管理员可发送 `{"event":"adminAuth","data":"JWT"}`，凭据仅放安全的消息帧。

- `ping` → `pong`。
- `webCheck`：`{queryType,domains}`；响应 `webCheckDomains` 和每域名 `webCheckResult`。
- `typoCheck`：`{domain,typoType,ccTlds,queryType}`；需要管理员，响应 `typoResult` 分组和 `webCheckResult`。
- 旧版批量消息 `bulkCheckStart`（`{queryType}`）、`bulkCheckPause`、`bulkCheckResume`、`bulkCheckCancel`、`bulkCheckClear`、`bulkRecheckErrorDomains` 继续保留，需要管理员。clear 只解除当前 Web 批量选择及上传，不再 FLUSHDB。
- `register`：`{registerType,domains,confirmed:true}`；需要管理员及明确确认，只用于人工 Web 操作。没有注册 MCP/CLI 工具。

即时查询断线会停止本连接工作；持久化批量任务通过 `/api/v1/tasks` 操作，不受断线影响。原 `registerStatus` 的 Free 已不再作为可注册承诺，使用新增规范 `status` 字段。旧客户端需要适配认证首帧、确认字段和新增状态；不保证仍依赖旧不安全行为的客户端原样可用。

## 2026-11-03 第二轮变更

管理设置 GET/PUT 不再回传秘密，修改凭据使用 secretActions；任务结果默认摘要并提供单条原文端点，分页与导出必须跟随 nextOffset/X-Next-Offset。完整说明见 [ROUND2](docs/ROUND2.md)。
