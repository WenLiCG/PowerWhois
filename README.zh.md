[English](README.md) | 中文

# PowerWhois

保留原有网页域名查询、批量任务、Typo、代理、自定义接口和管理功能，并增加不绑定 AI 厂商的 CLI、OpenAPI HTTP API 与可选 MCP。

- DNS、WHOIS/RDAP、Mixed、代理、自定义查询；结果区分已注册、未找到、未知、超时、错误。
- 批量任务有稳定 ID，支持暂停、继续、取消、重查、分页和部分导出；关闭浏览器或 CLI 后仍在服务端执行。
- Web、CLI、API、MCP 共用查询核心和批量任务库。
- AI 查询凭据不能注册、删数据或修改管理配置；原手动注册保留在管理员 Web，并要求确认。
- 默认只监听本机，远程使用 SSH 转发或受保护的 HTTPS 网关。

**未找到不等于可注册，DNS 无记录不等于未注册。**

## 使用与交付

- [完整审查、修复和功能对照](docs/REVIEW.md)
- [CLI / API / MCP 示例、权限与 AI 环境接入条件](docs/USAGE.md)
- [部署、升级、迁移与回滚](docs/DEPLOYMENT.md)
- [测试报告及未验证事项](docs/TEST-REPORT.md)
- [OpenAPI 3.0 文档](src/backend/api/openapi.json)；运行后 `/api/openapi.json`

本版从完整源码构建，需要 Go 1.27.3、Node.js 24 和 Redis。安装依赖后在 `src` 执行 `make test all`。Docker 部署见部署文档。

**仓库原 `deploy/backend/typonamer` 与 `deploy/frontend` 是历史预构建文件，不代表本版源码。** 新 Compose 使用根目录 Dockerfile 重新构建。首次运行前设置独立随机凭据，不内置通用密码。

![批量任务界面](docs/screenshots/batch-desktop.jpg)

沿用原项目 MIT 许可，见 [LICENSE](LICENSE)。
