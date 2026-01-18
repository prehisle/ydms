# CLAUDE.md

本文件为 Claude Code (claude.ai/code) 在此仓库中工作时提供指导。

## 语言要求
- 总是使用简体中文，场景包括与用户交互、生成的文档、代码注释
- 代码提交及推送需用户每次明确要求才进行

## 架构概览

YDMS（资料管理系统）是一个全栈应用，采用 Go 后端和 React 前端：

**技术栈**：
- **后端**：Go 1.24+、GORM、PostgreSQL、JWT 认证
- **前端**：React 18、TypeScript、Vite、Ant Design、TanStack Query、Monaco Editor
- **数据库**：PostgreSQL（通过 GORM ORM）
- **测试**：Go testing、Playwright E2E、Vitest 单元测试
- **部署**：Docker、Docker Compose

- **后端**：Go 1.22+ 服务，充当上游 NDR（节点-文档-关系）服务的代理/门面
  - 入口：`backend/cmd/server/main.go`
  - HTTP 路由：`backend/internal/api/router.go` 和 `handler.go`
  - 领域服务：`backend/internal/service/`（category.go、documents.go、document_types.go）
  - NDR 客户端：`backend/internal/ndrclient/client.go` - 封装所有上游 NDR API 调用
  - 工具库：`backend/internal/config/`、`backend/internal/cache/`（cache.Provider 目前为空实现）
  - 代码生成工具：`backend/cmd/docgen` - 基于配置生成文档类型相关代码

- **前端**：Vite + React + Ant Design + TanStack Query
  - 入口：`frontend/src/main.tsx`
  - 按领域组织的功能：`frontend/src/features/categories/`、`frontend/src/features/documents/`
  - 文档类型插件：`frontend/src/features/documents/typePlugins/` - 各文档类型的预览和编辑组件
  - 生成的代码：`frontend/src/generated/` - docgen 工具生成的类型定义
  - API 客户端：`frontend/src/api/`（http.ts、categories.ts、documents.ts）
  - 输出：`frontend/dist/`（gitignore 排除）

- **文档类型配置**：`doc-types/config.yaml` - 集中配置所有文档类型，驱动代码生成

- **文档**：`docs/` 包含设计笔记和 `docs/backend/openapi.json`（NDR OpenAPI 规范）

## 开发命令

### 快速开始（使用 Makefile）
```bash
# 查看所有可用命令
make help

# 快速重置数据库（推荐）- 清空数据但保留表结构
make quick-reset

# 完整重置并初始化（重建数据库）
make reset-init

# 启动后端开发服务器
make dev-backend

# 启动前端开发服务器
make dev-frontend

# 运行后端测试
make test-backend

# 运行 E2E 测试
make test-e2e

# 运行 E2E 测试（UI 模式）
make test-e2e-ui

# 基于 doc-types 配置生成前后端代码
make generate-doc-types

# 安装所有依赖
make install

# 清理临时文件
make clean
```

### 前端
```bash
cd frontend

# 安装依赖（在修改 lockfile 后）
npm install

# 在 http://localhost:9001 启动开发服务器
npm run dev

# 启用拖拽调试（在浏览器控制台输出 [drag-debug] 日志）
VITE_DEBUG_DRAG=1 npm run dev

# 构建生产版本
npm run build

# 预览生产版本构建
npm run preview

# E2E 测试
npm run test:e2e              # 无头模式运行测试
npm run test:e2e:ui           # UI 模式运行测试
npm run test:e2e:headed       # 有头模式运行测试
npm run test:e2e:debug        # 调试模式运行测试

# 运行前端单元测试
npm test                      # Vitest 单元测试
```

### 后端
```bash
cd backend

# 启动服务器（加载 .env 或 YDMS_* 环境变量）
go run ./cmd/server

# 启动并在 .go/.env/go.mod/go.sum 变更时自动重新加载
# 二进制文件编译到 backend/tmp/server-dev
go run ./cmd/server --watch

# 运行所有测试并显示覆盖率
go test ./... -cover

# 运行特定测试
go test ./internal/service -run TestCreateDocument

# 运行 NDR 集成测试（需要运行中的 NDR 服务）
go test ./internal/ndrclient -run TestRealNDRIntegration \
  -ndr.url=http://localhost:9001 \
  -ndr.apikey=your-key \
  -ndr.user=test-user

# 格式化代码
gofmt -w .
go vet ./...

# 整理依赖
go mod tidy
```

### 前端
```bash
cd frontend

# 安装依赖（在修改 lockfile 后）
npm install

# 在 http://localhost:9001 启动开发服务器
npm run dev

# 启用拖拽调试（在浏览器控制台输出 [drag-debug] 日志）
VITE_DEBUG_DRAG=1 npm run dev

# 构建生产版本
npm run build

# 预览生产版本构建
npm run preview

# E2E 测试
npm run test:e2e              # 无头模式运行测试
npm run test:e2e:ui           # UI 模式运行测试
npm run test:e2e:headed       # 有头模式运行测试
npm run test:e2e:debug        # 调试模式运行测试
```

### 数据库管理
```bash
# 快速重置（日常使用，推荐）
make quick-reset

# 完整重置（遇到问题时）
make reset-init

# 使用 Go 工具重置
cd backend && go run ./cmd/reset-db

# 手动连接数据库（需配置 .env）
PGPASSWORD=admin psql -h 192.168.1.4 -p 5432 -U admin -d ydms
```

**默认管理员账号**（重置后自动创建）：
- 用户名：`super_admin`
- 密码：`admin123456`

详细的数据库重置指南请参阅 `docs/MAINTENANCE_GUIDE.md`。

## 关键领域概念

### 文档类型系统与插件架构

YDMS 实现了基于 YAML 配置驱动的文档类型系统，支持灵活扩展新的文档类型。

#### 配置驱动的类型定义
文档类型在 `doc-types/config.yaml` 中集中配置，包括：
- 类型 ID 和显示标签
- 内容格式（html 或 yaml）
- 模板文件路径
- 前端插件钩子导入路径
- 主题配置（针对 HTML 类型）

当前支持的文档类型：
1. **markdown_v1** - Markdown 格式，基础文档
2. **comprehensive_choice_v1** - YAML 格式，综合选择题
3. **case_analysis_v1** - YAML 格式，案例分析题
4. **essay_v1** - YAML 格式，论文题
5. **dictation_v1** - YAML 格式，默写练习
6. **knowledge_overview_v1** - HTML 格式，知识点概览（支持多主题）
   - 内置主题：经典蓝、暖色晨曦、夜间沉浸、玻璃拟态、竹林墨韵

#### 代码生成工具
使用 `backend/cmd/docgen` 工具基于配置自动生成代码：
```bash
make generate-doc-types
# 或
cd backend && go run ./cmd/docgen --config ../doc-types/config.yaml \
  --repo-root .. --frontend-dir ../frontend --backend-dir .
```

生成的代码：
- 后端：文档类型常量、验证逻辑
- 前端：`frontend/src/generated/` 目录下的类型定义和元数据

#### 前端插件系统
- **注册机制**：`frontend/src/features/documents/previewRegistry.tsx` 提供插件注册接口
- **类型插件**：位于 `frontend/src/features/documents/typePlugins/` 目录
- 每个类型插件实现：
  - `types.ts` - TypeScript 类型定义
  - `register.tsx` - 预览渲染组件和注册逻辑
- 插件通过 `registerYamlPreview()` 注册自定义渲染器

每个文档包含：
- `type`：配置中定义的类型 ID
- `content`：结构为 `{"format": "html"|"yaml", "data": "<内容字符串>"}`
- `metadata`：灵活的 JSON 对象（如 `{"difficulty": 1-5, "tags": [...]}`）

文档验证在服务层通过 `ValidateDocumentContent()` 和 `ValidateDocumentMetadata()` 进行。

### 分类树与 NDR 集成
- 分类是存储在 NDR 服务中的层级节点
- `service/category.go` 中的后端方法聚合分页的 NDR 响应（GetTree、ListDeleted）
- 拖拽排序使用 `/api/v1/categories/{id}/reposition` 端点，该端点在一个原子操作中组合移动和重排
- `reposition` 端点需要 `new_parent_id` 和 `ordered_ids`（必须包含被移动的节点）

### 关系
- 文档通过关系绑定到分类节点（node_id ↔ document_id）
- 软删除：文档/节点用 `deleted_at` 标记；使用 `/nodes/{id}/restore` 或 `/documents/{id}/restore` 恢复
- 清除：`/nodes/{id}/purge` 或 `/documents/{id}/purge` 永久删除

### 用户认证和权限系统

#### 角色定义
YDMS 实现了基于角色的访问控制（RBAC），定义了三种用户角色：

1. **super_admin（超级管理员）**
   - 完全访问权限
   - 可以创建/编辑/删除所有用户（包括其他 super_admin）
   - 可以管理所有分类和文档
   - 可以为 course_admin 分配课程权限

2. **course_admin（课程管理员）**
   - 可以创建/编辑/删除校对员用户
   - 可以管理分配给自己的课程（根节点）及其子节点
   - 可以创建/编辑/删除分类和文档
   - 不能管理其他管理员

3. **proofreader（校对员）**
   - **只读**查看分类树
   - **可以编辑**文档内容
   - **可以查看和恢复**文档历史版本
   - **不能**创建/删除文档
   - **不能**创建/编辑/删除分类
   - **不能**访问用户管理
   - **可以**修改自己的密码

#### 数据库模型

**users 表**:
```sql
id            SERIAL PRIMARY KEY
username      VARCHAR(100) UNIQUE NOT NULL
password_hash VARCHAR(255) NOT NULL
role          VARCHAR(50) NOT NULL  -- super_admin, course_admin, proofreader
display_name  VARCHAR(100)
created_by_id INT REFERENCES users(id)
created_at    TIMESTAMP
updated_at    TIMESTAMP
deleted_at    TIMESTAMP  -- 软删除
```

**course_permissions 表**:
```sql
id           SERIAL PRIMARY KEY
user_id      INT REFERENCES users(id)
root_node_id INT NOT NULL  -- NDR 中的根节点 ID
created_at   TIMESTAMP
```

#### 认证流程

1. **默认管理员**: 数据库迁移会自动创建 `super_admin / admin123456`，可通过 `YDMS_DEFAULT_ADMIN_*` 环境变量覆盖，部署后务必强制修改密码。

2. **登录**: 用户使用用户名和密码登录
   - `POST /api/v1/auth/login`
   - 返回 JWT token 和用户信息

3. **认证**: 使用 JWT token 进行 API 请求
   - Header: `Authorization: Bearer <token>`
   - Middleware 验证 token 并提取用户信息

4. **其他操作**:
   - 获取当前用户: `GET /api/v1/auth/me`
   - 修改密码: `POST /api/v1/auth/change-password`
   - 登出: `POST /api/v1/auth/logout` (前端删除 token)

#### 用户管理 API

- `GET /api/v1/users` - 获取用户列表
- `POST /api/v1/users` - 创建用户
- `GET /api/v1/users/:id` - 获取用户详情
- `DELETE /api/v1/users/:id` - 删除用户
- `GET /api/v1/users/:id/courses` - 获取用户的课程权限
- `POST /api/v1/users/:id/courses` - 授予课程权限
- `DELETE /api/v1/users/:id/courses/:rootNodeId` - 撤销课程权限

#### 前端权限控制

**分类树 (CategoryTreePanel)**:
- `canManageCategories` prop 控制是否显示管理功能
- 工具栏中的"新建根目录"按钮根据权限显示/隐藏
- 右键菜单根据角色动态生成：
  - proofreader 只能看到"复制所选"（只读操作）
  - 管理员可以看到所有操作（新建、编辑、删除、剪切、粘贴）

**用户管理 (UserManagementPage)**:
- 仅 super_admin 和 course_admin 可见
- super_admin 可以创建所有角色用户
- course_admin 只能创建 proofreader
- 不能删除自己
- course_admin 不能删除其他管理员

**路由保护**:
- 所有业务路由需要认证 (JWT middleware)
- 前端使用 `PrivateRoute` 组件保护路由
- `AuthContext` 提供全局用户状态

#### 测试账号

**默认管理员**（数据库重置后自动创建）：
```
用户名: super_admin
密码:   admin123456
```

**开发/测试环境的其他账号**：
```
超级管理员:
  Username: testadmin
  Password: newpass456

课程管理员:
  Username: course_admin1
  Password: testpass123

校对员:
  创建后使用设置的密码
```

**安全提示**:
- 首次登录后请立即修改密码
- 生产环境必须修改所有默认密码

### AI 文档处理系统

YDMS 集成了 IDPP（Intelligent Document Processing Pipeline）用于 AI 辅助文档处理。

#### 架构

```
┌─────────────┐      ┌─────────────┐      ┌─────────────┐
│ PDMS 前端   │─────>│ PDMS 后端   │─────>│ Prefect API │
│ (React)     │      │ (Go:9002)   │      │ (:4200)     │
└─────────────┘      └─────────────┘      └─────────────┘
       │                   │                     │
       │ 轮询状态           │                     ▼
       │<──────────────────┤              ┌─────────────┐
       │                   │<─────────────│ IDPP Worker │
       │                   │   回调        │ (Prefect)   │
       │                   │              └─────────────┘
```

#### 可用流水线

1. **generate_knowledge_overview** - 知识点概览生成
   - 基于文档引用关系，读取源文档内容，调用 LLM 生成 HTML 格式的知识点学习资料
   - 支持预览模式和执行模式

2. **polish_document** - 文档润色
   - 使用 LLM 优化文档语言表达和格式
   - 自动修正语法错误

#### API 端点

- `GET /api/v1/processing/pipelines` - 获取可用流水线列表
- `POST /api/v1/processing` - 触发流水线
- `GET /api/v1/processing/jobs?document_id=X` - 获取文档处理历史
- `GET /api/v1/processing/jobs/{id}` - 获取任务详情
- `POST /api/v1/processing/callback/{id}` - 回调端点（内部使用）

#### 前端使用

在文档编辑器中，点击工具栏的 "AI 处理" 按钮：
1. 选择流水线（如"知识点概览生成"）
2. 选择模式：
   - **预览模式**：仅生成结果，不保存到文档
   - **执行模式**：生成结果并保存到文档
3. 查看处理进度和结果

#### 环境配置

后端需要配置 Prefect 相关环境变量：
```
# Prefect 集成（不设置则禁用）
YDMS_PREFECT_BASE_URL=http://localhost:4200  # Prefect Server API 地址
YDMS_PREFECT_WEBHOOK_SECRET=your-secret      # Webhook 回调验证密钥
YDMS_PREFECT_TIMEOUT=300                     # API 请求超时（秒）
YDMS_PUBLIC_BASE_URL=http://your-host:9002   # 回调公开地址（用于 IDPP 回调 PDMS）
```

IDPP Worker 需要配置：
```
PDMS_WEBHOOK_SECRET=your-secret  # 与 YDMS_PREFECT_WEBHOOK_SECRET 相同
```

#### 部署流水线

在 IDPP 项目中部署 Prefect Deployments：
```bash
cd /home/pi/codes/IDPP
prefect deploy --all
```

### API Key 认证系统

YDMS 支持 API Key 认证，用于外部程序批量管理课程。

#### 特性
- **长期有效**：支持永不过期或设置过期时间
- **权限继承**：API Key 关联到用户账号，自动继承用户的角色和课程权限
- **灵活认证**：支持 `Authorization: Bearer <api-key>` 和 `X-API-Key: <api-key>` 两种方式
- **安全存储**：API Key 以 SHA256 哈希存储，完整密钥仅在创建时返回一次

#### API Key 格式
```
ydms_<environment>_<random-string>
例如: ydms_prod_abc123defghijk456lmnopqrstuv789wxyz
```

#### 管理端点（需要认证）
- `POST /api/v1/api-keys` - 创建 API Key（仅超级管理员）
- `GET /api/v1/api-keys` - 列出 API Keys
- `GET /api/v1/api-keys/{id}` - 获取详情
- `PATCH /api/v1/api-keys/{id}` - 更新信息
- `POST /api/v1/api-keys/{id}/revoke` - 撤销
- `DELETE /api/v1/api-keys/{id}` - 永久删除（仅超级管理员）
- `GET /api/v1/api-keys/stats` - 统计信息

#### 快速开始
```bash
# 1. 登录获取 JWT token
TOKEN=$(curl -X POST http://localhost:9002/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"super_admin","password":"admin123456"}' | jq -r '.token')

# 2. 创建 API Key
API_KEY=$(curl -X POST http://localhost:9002/api/v1/api-keys \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"批量管理工具","user_id":2,"environment":"prod"}' \
  | jq -r '.api_key')

# 3. 使用 API Key 访问 API
curl -H "X-API-Key: $API_KEY" http://localhost:9002/api/v1/categories
```

**文档链接**：
- [API Keys 指南（权威）](docs/guides/api-keys.md)
- 历史与实现文档（归档）：`docs/archive/`

#### 前端 UI 访问

**仅超级管理员可见**，通过以下步骤访问：
1. 登录 YDMS 系统
2. 点击右上角用户头像
3. 选择 "API Key 管理"

**主要功能**：
- 查看 API Key 列表和统计信息
- 创建新 API Key（完整密钥仅显示一次）
- 编辑 API Key 名称
- 撤销/删除 API Key
- 实时状态显示（活跃/过期/已撤销）

## 环境配置

后端从 `backend/` 或环境变量读取 `.env`：
```
YDMS_NDR_BASE_URL=http://localhost:9001
YDMS_NDR_API_KEY=your-ndr-key
YDMS_HTTP_PORT=9180
YDMS_DEFAULT_USER_ID=system
YDMS_ADMIN_KEY=your-ndr-admin-key
YDMS_DEBUG_TRAFFIC=1  # 记录向 NDR 的 HTTP 请求和响应
YDMS_JWT_SECRET=your-jwt-secret  # JWT 签名密钥
```

前端读取 `frontend/.env`：
```
VITE_API_BASE_URL=http://localhost:9002  # API 基础 URL（可选，默认使用 Vite 代理）
VITE_DEBUG_DRAG=1  # 启用拖拽调试日志
VITE_DEBUG_MENU=1  # 启用菜单调试模式
```

**绝不提交秘钥**，如 `YDMS_NDR_API_KEY` 或 `YDMS_ADMIN_KEY`。

## 测试策略

### 后端测试
- 单元测试与代码共地：`*_test.go`
- 优先使用表驱动测试
- 夹具在 `backend/internal/service/testdata/`
- NDR 集成测试需要运行中的服务（见上面的命令）
- 所有测试已更新为使用新的文档类型系统（markdown_v1, dictation_v1, comprehensive_choice_v1, case_analysis_v1, essay_v1, knowledge_overview_v1）

### 前端测试
- **Playwright E2E 测试**: `frontend/e2e/` 目录包含端到端测试
  - 认证流程测试: `auth.spec.ts`
  - 用户管理测试: `user-management.spec.ts`
  - 权限控制测试: `permissions.spec.ts`
  - 测试辅助工具: `fixtures/auth.ts`
  - 测试结果: `TEST_RESULTS.md`
- 运行测试: `npm run test:e2e`
- UI 模式调试: `npm run test:e2e:ui`
- **单元测试**: 使用 Vitest + React Testing Library
  - 运行: `npm test`
  - 配置: 已配置 React Testing Library 和 Jest DOM 匹配器

## 代码风格

### Go
- 使用 `gofmt`/`goimports` 格式化
- 包名小写，导出函数 PascalCase
- 共享错误遵循 `ErrXYZ` 约定
- 服务和处理器使用表驱动测试
- 小接口，镜像现有的缓存/配置抽象

### TypeScript/React
- 严格 TypeScript：显式返回类型
- 组件在 PascalCase 文件中，hooks 为 `useCamelCase`
- 每个组件共置资源
- 共享查询键在 `src/hooks/` 下（如适用）
- 描述性的 prop 名称
- 前端构建使用 Vite，配置了代码分割优化（Monaco Editor、YAML parser 等分离）

## 提交约定
遵循常规提交：
- `feat:` - 新功能
- `fix:` - 修复 bug
- `docs:` - 文档变更
- `refactor:` - 代码重构
- `test:` - 测试增加/变更

使用简洁的命令式摘要。分组相关变更。跨越后端和前端时提及领域。

## Pull Request 指南
PR 应包括：
- 行为变更摘要
- 运行的测试命令
- 链接的问题（如适用）
- 面向用户的变更需要 UI 截图或 curl 示例
- 工作流或 API 变更时更新 `docs/`、`AGENTS.md` 或此文件

## 常见调试

### 后端
- 启用 `YDMS_DEBUG_TRAFFIC=1` 记录所有 NDR HTTP 请求和响应
- 检查 `backend/server.log`（gitignore 排除）
- watch 模式写入 `backend/tmp/` - 重置时清理

### 前端
- 使用 `VITE_DEBUG_DRAG=1` 进行拖拽诊断，浏览器控制台显示 `[drag-debug]` 日志
- 使用 `VITE_DEBUG_MENU=1` 启用菜单调试模式
- 使用 `VITE_API_BASE_URL` 自定义 API 端点（开发时默认通过 Vite 代理到 localhost:9002）
- React Query DevTools 在开发时可用

### Flex 布局高度溢出问题

**问题现象**：内容被顶出屏幕、滚动条无效、固定元素（如状态栏）不可见

**根本原因**：Flex 布局中高度约束链断裂。当使用 `height: 100%` 而父元素没有明确高度，或 flex 子元素没有 `minHeight: 0` 时，内容会溢出。

**调试方法**：
1. 使用 Playwright MCP 的 `browser_evaluate` 检查元素实际尺寸和样式
2. 检查布局链中每个元素的 `height`、`overflow`、`flex`、`minHeight` 属性
3. 对比 `scrollHeight` 和 `clientHeight` 判断是否可滚动

**修复原则**：
```css
/* 外层容器 - 固定高度并隐藏溢出 */
.outer-container {
  height: 100vh;      /* 或明确的高度值 */
  overflow: hidden;   /* 防止内容溢出 */
}

/* Flex 容器 */
.flex-container {
  display: flex;
  flex-direction: column;
  min-height: 0;      /* 关键！允许 flex 子元素收缩 */
  overflow: hidden;
}

/* Flex 子元素 - 可增长区域 */
.flex-child {
  flex: 1;
  min-height: 0;      /* 关键！允许内容收缩 */
  overflow: hidden;   /* 或 auto，取决于是否需要滚动 */
}

/* 滚动容器 */
.scroll-container {
  flex: 1;
  min-height: 0;
  overflow: auto;     /* 启用滚动 */
}

/* 固定高度元素 */
.fixed-element {
  flex-shrink: 0;     /* 防止被压缩 */
}
```

**Ant Design 特殊处理**：
- `ant-layout-sider-children` 默认 `overflow: visible`，需要 CSS 覆盖
- 使用 className 选择器覆盖内部样式：
  ```css
  .my-sider > .ant-layout-sider-children {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
  }
  ```

**检查清单**：
- [ ] 根容器有明确高度（`100vh` 或固定值）
- [ ] 所有 flex 容器设置 `minHeight: 0`
- [ ] 滚动区域父元素设置 `overflow: hidden`
- [ ] 滚动容器设置 `overflow: auto`
- [ ] 固定高度元素设置 `flexShrink: 0`
- [ ] Ant Design 组件检查默认 overflow 行为

### Docker Compose 问题

**错误：`KeyError: 'ContainerConfig'`**
- **原因**：使用旧版 `docker-compose` V1 与镜像元数据不兼容
- **解决**：
  ```bash
  # 使用 Docker Compose V2（空格而非连字符）
  docker compose down
  docker compose up -d

  # 如果命令不存在，安装 Docker Compose V2
  sudo apt-get install docker-compose-plugin

  # 完全重建（如果仍失败）
  docker compose down
  docker rm -f ydms-postgres ydms-app ydms-frontend
  docker pull postgres:16-alpine
  docker compose up -d --force-recreate
  ```

**端口冲突错误：`port is already allocated`**
- **检查端口占用**：
  ```bash
  sudo lsof -i :9001   # 前端 HTTP 端口
  sudo lsof -i :9002   # 后端 API 端口
  sudo lsof -i :5432   # PostgreSQL（如果暴露）
  ```
- **解决方案**：
  - 方案1：停止占用端口的服务（如 `sudo systemctl stop postgresql`）
  - 方案2：修改 `.env` 文件使用其他端口，然后 `docker compose restart`

**服务无法启动或健康检查失败**
- **检查日志**：
  ```bash
  docker compose logs postgres   # 数据库日志
  docker compose logs ydms-app   # 后端日志
  docker compose logs frontend   # 前端日志
  ```
- **检查网络连接**：
  ```bash
  docker compose exec ydms-app ping postgres  # 测试内部网络
  ```
- **验证环境配置**：
  ```bash
  # 确保 .env 文件包含所有必需变量
  grep -E "POSTGRES_PASSWORD|YDMS_NDR_API_KEY|YDMS_JWT_SECRET" .env
  ```

## 生成的文件

### 应保持未跟踪的文件
以下生成的输出应保持在 `.gitignore` 中：
- `backend/tmp/` - watch 模式编译的二进制文件
- `backend/.gocache/` - Go 构建缓存
- `backend/server.log` - 服务器运行日志
- `frontend/dist/` - 前端构建产物
- `frontend/node_modules/` - npm 依赖

### 代码生成的文件（应提交）
`make generate-doc-types` 命令会生成以下文件，这些**应该提交**到版本控制：
- `frontend/src/generated/` - 自动生成的 TypeScript 类型定义和元数据
  - 包含基于 `doc-types/config.yaml` 生成的文档类型常量和接口

**重要**：修改 `doc-types/config.yaml` 后，务必运行 `make generate-doc-types` 并提交生成的代码。

## 生产部署

### Docker 部署
项目支持前后端分离的 Docker 部署方式：

```bash
# 构建后端镜像
docker build -t ydms-backend:latest -f Dockerfile .

# 构建前端镜像
docker build -t ydms-frontend:latest -f Dockerfile.frontend ./frontend

# 使用 Docker Compose 部署
cd deploy/production
cp .env.example .env
nano .env  # 配置环境变量
docker compose up -d
```

### 一键部署脚本
```bash
# 使用本地配置文件部署（推荐）
./scripts/deploy_prod.sh --env-file deploy/production/.env.1.31

# 查看部署选项
./scripts/deploy_prod.sh --help
```

### 生产环境检查
```bash
# 查看服务状态
docker compose ps

# 查看日志
docker compose logs ydms-app

# 重启服务
docker compose restart ydms-app
```

详细的生产部署指南请参阅 [deploy/production/README.md](deploy/production/README.md)。

### 关键配置
生产环境必须配置以下环境变量：
- `POSTGRES_PASSWORD` - 数据库密码
- `YDMS_NDR_BASE_URL` - NDR 服务地址
- `YDMS_NDR_API_KEY` - NDR API 密钥
- `YDMS_ADMIN_KEY` - NDR 管理员密钥
- `YDMS_JWT_SECRET` - JWT 签名密钥

**安全提示**：
- 生产环境必须修改所有默认密码
- 使用强密码和随机生成的密钥
- 定期备份 PostgreSQL 数据库

## 常见工作流

### 日常开发流程
```bash
# 1. 启动前重置数据库到干净状态
make quick-reset

# 2. 在不同终端启动后端和前端
make dev-backend   # 终端1：后端服务
make dev-frontend  # 终端2：前端服务

# 3. 开发中...修改代码，自动重载

# 4. 运行测试验证变更
make test-backend  # 后端测试
make test-e2e      # E2E 测试

# 5. 提交前检查
cd backend && go vet ./...    # 检查代码
cd frontend && npm run build  # 验证构建
```

### 添加新文档类型（使用插件系统）
1. **配置文档类型**：在 `doc-types/config.yaml` 中添加新类型定义
2. **创建模板文件**：在 `doc-types/<type-id>/template.yaml` 或 `template.html` 创建模板
3. **运行代码生成**：`make generate-doc-types` 自动生成后端和前端代码
4. **实现前端插件**：
   - 在 `frontend/src/features/documents/typePlugins/<type-id>/` 创建目录
   - 添加 `types.ts` 定义 TypeScript 类型
   - 添加 `register.tsx` 实现预览渲染逻辑
   - 在组件中调用 `registerYamlPreview()` 注册插件
5. **添加测试数据**：在 `backend/internal/service/testdata/` 添加测试数据
6. **更新测试用例**：确保后端和前端测试覆盖新类型

**旧方式（手动）**：
如果不使用代码生成，需要手动：
1. 在 `backend/internal/service/document_types.go` 中添加类型常量
2. 更新 `ValidateDocumentContent()` 和 `ValidateDocumentMetadata()` 函数
3. 手动创建前端类型定义和预览组件

### 调试 NDR 集成问题
```bash
# 1. 启用调试模式
export YDMS_DEBUG_TRAFFIC=1

# 2. 启动服务并查看日志
make dev-backend

# 3. 日志会显示所有 HTTP 请求和响应
# 检查 backend/server.log 文件

# 4. 运行集成测试
cd backend
go test ./internal/ndrclient -run TestRealNDRIntegration \
  -ndr.url=http://localhost:9001 \
  -ndr.apikey=your-key \
  -ndr.user=test-user
```

### 修复 E2E 测试失败
```bash
# 1. 重置数据库到已知状态
make quick-reset

# 2. 以 UI 模式运行失败的测试
cd frontend
npx playwright test --ui

# 3. 或以有头模式运行查看浏览器行为
npx playwright test --headed

# 4. 调试特定测试
npx playwright test --debug auth.spec.ts

# 5. 查看测试结果报告
npx playwright show-report
```

### 添加新的用户权限功能
1. 更新数据库模型（`backend/internal/models/`）
2. 修改认证 middleware（`backend/internal/api/middleware.go`）
3. 更新用户服务层（`backend/internal/service/users.go`）
4. 在前端更新 `AuthContext` 和权限检查
5. 添加 E2E 测试验证权限控制（`frontend/e2e/permissions.spec.ts`）

### 处理端口冲突
```bash
# 检查端口占用
lsof -i :9002  # YDMS 后端
lsof -i :9001  # Vite 前端
lsof -i :9000  # NDR 服务

# 终止占用端口的进程
kill -9 <PID>

# 或使用 fuser
fuser -k 9002/tcp
```

### 使用 API Key 进行批量管理
```bash
# 1. 创建 API Key（需要超级管理员身份）
TOKEN=$(curl -s -X POST http://localhost:9002/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"super_admin","password":"admin123456"}' | jq -r '.token')

# 创建课程管理员账号
USER_ID=$(curl -s -X POST http://localhost:9002/api/v1/users \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "username":"batch_admin",
    "password":"secure_pass",
    "role":"course_admin",
    "display_name":"批量管理账号"
  }' | jq -r '.id')

# 创建 API Key
API_KEY=$(curl -s -X POST http://localhost:9002/api/v1/api-keys \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"批量导入工具\",\"user_id\":$USER_ID,\"environment\":\"prod\"}" \
  | jq -r '.api_key')

echo "API Key: $API_KEY"
# 保存此密钥！它只会显示一次

# 2. 使用 API Key 批量创建分类
curl -X POST http://localhost:9002/api/v1/categories \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"label": "新课程", "parent_id": null}'

# 3. 批量创建文档
for i in {1..10}; do
  curl -X POST http://localhost:9002/api/v1/documents \
    -H "X-API-Key: $API_KEY" \
    -H "Content-Type: application/json" \
    -d "{
      \"title\": \"文档 $i\",
      \"type\": \"markdown_v1\",
      \"content\": {\"format\": \"markdown\", \"data\": \"# 内容 $i\"}
    }"
done

# 4. 查看 API Key 统计
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:9002/api/v1/api-keys/stats
```

详细的 Python 批量导入示例请参阅 `docs/guides/api-keys.md`。
