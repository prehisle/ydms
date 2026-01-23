# Repository Guidelines

## 项目结构与模块组织
仓库分为 `backend/` 与 `frontend/` 两大端。Go 服务入口位于 `backend/cmd/server`，HTTP 路由集中在 `backend/internal/api`，业务逻辑放在 `backend/internal/service`，外部 NDR 客户端在 `backend/internal/ndrclient`。React 管理端代码位于 `frontend/src/`，构建产物输出到 `frontend/dist/`，设计文档与 OpenAPI 规范位于 `docs/`。临时构建文件（例如 `backend/tmp/`、`.gocache/`、`server.log`）应保持未跟踪状态。

## 构建、测试与开发命令
- `cd backend && go run ./cmd/server`：按当前 `.env` 启动 API 服务。
- `cd backend && go run ./cmd/server --watch`：监视 Go 源与环境变量变化并热重载，可在 `backend/tmp/server-dev` 找到二进制。
- `cd backend && go test ./... -cover`：执行后端单元测试并输出覆盖率。
- `cd frontend && npm install`：同步前端依赖；锁文件变更后必跑。
- `cd frontend && npm run dev`：本地启动 Vite，默认端口 5173。
- `cd frontend && npm run build` 或 `npm run preview`：验证生产构建。

## 编码风格与命名规范
Go 代码使用 `goimports`、`gofmt` 格式化，包名全小写，导出符号 PascalCase，共享错误统一 `ErrXxx`。React 组件采用 PascalCase 文件名与导出，Hook 以 `useCamelCase` 命名，TypeScript 保持显式返回类型与语义化 Prop 名称。必要时添加精炼注释解释复杂逻辑，避免琐碎注释。

## 测试指引
Go 单测文件紧邻被测代码，命名为 `*_test.go`，可复用 `backend/internal/service/testdata` 下的夹具。集成测试需外部 NDR 服务，仅在具备凭据时运行（`go test ./internal/ndrclient -run TestRealNDRIntegration -ndr.url=...`）。提交前至少跑后端单测；前端尚未建立自动化测试，若需新增工具先在 PR 中提议。

## 提交与合并请求规范
提交消息遵循 `feat: ...`、`fix: ...` 等惯例前缀，语句保持祈使语并聚焦单一变更。PR 需说明行为变更、影响范围、已执行测试命令以及相关 Issue，UI 变动附示例截图或 cURL 片段。多域修改应在描述中点明涉及的服务或前端模块。

## 安全与配置提示
敏感配置通过 `.env` 管理，禁止提交实际的 `YDMS_NDR_API_KEY`、`YDMS_ADMIN_KEY` 或外部端点。启用 Watch 模式会在 `backend/tmp/` 写入临时二进制，定期清理并确保日志文件不入库。必要时使用环境变量前缀 `YDMS_` 进行覆盖，保持与文档一致。

### API Key 认证架构

PDMS 使用两套独立的 API Key 体系：

| 类型 | 格式 | 用途 |
|------|------|------|
| **YDMS API Key** | `ydms_*` | 外部服务（如 IDPP）调用 PDMS API |
| **NDR API Key** | `ndr_*` | PDMS 内部调用 NDR 微服务 |

**重要设计原则**：
- PDMS 接收到的 `X-API-Key` header（YDMS API Key）仅用于认证调用方身份
- PDMS 调用 NDR 时**始终使用配置的 `YDMS_NDR_API_KEY`**，不透传请求中的 API Key
- 这两套 Key 完全独立，不能混用

**为外部服务创建 API Key**：
1. 使用超级管理员登录 PDMS
2. 进入 "API Key 管理" 页面创建新 Key
3. 将生成的 `ydms_*` 格式 Key 提供给外部服务使用

## 沟通约定
所有协作与反馈请统一使用中文表述，确保讨论语境一致、记录易于追溯。
