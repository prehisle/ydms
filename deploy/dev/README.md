# 本地开发环境

本目录包含本地开发所需的 Docker Compose 配置。

## 快速启动

### 1. 启动基础设施

```bash
cd /home/pi/codes/pdms
docker compose -f deploy/dev/docker-compose.infra.yml up -d
```

启动后可用的服务：

| 服务 | 端口 | 访问地址 | 账号密码 |
|------|------|----------|----------|
| PostgreSQL | 5432 | `postgresql://postgres:postgres@localhost:5432` | postgres/postgres |
| Redis | 6379 | `redis://localhost:6379` | 无密码 |
| MinIO API | 9000 | `http://localhost:9000` | minioadmin/minioadmin |
| MinIO 控制台 | 9002 | `http://localhost:9002` | minioadmin/minioadmin |

### 2. 启动 NDR 服务

**方式 A：使用本地源码（推荐，用于开发新功能）**

```bash
cd /home/pi/codes/ndr

# 配置环境变量
cat > .env << 'EOF'
DB_URL=postgresql+psycopg2://postgres:postgres@localhost:5432/ndr
AUTO_APPLY_MIGRATIONS=true
API_KEY_ENABLED=false
LOG_LEVEL=INFO
EOF

# 启动服务
uvicorn app.main:app --reload --host 0.0.0.0 --port 8000
```

**方式 B：使用 Docker 镜像**

```bash
docker compose -f deploy/dev/docker-compose.ndr.yml up -d
```

### 3. 启动 PDMS 后端

```bash
cd /home/pi/codes/pdms/backend

# 配置环境变量
cat > .env << 'EOF'
YDMS_NDR_BASE_URL=http://localhost:8000
YDMS_NDR_API_KEY=secret-123
YDMS_ADMIN_KEY=admin-secret
YDMS_HTTP_PORT=9180
YDMS_DB_HOST=localhost
YDMS_DB_PORT=5432
YDMS_DB_USER=postgres
YDMS_DB_PASSWORD=postgres
YDMS_DB_NAME=ydms
YDMS_DB_SSLMODE=disable
YDMS_JWT_SECRET=dev-secret-key-for-local-testing-min-32-chars
EOF

# 启动（自动重载）
go run ./cmd/server --watch
```

### 4. 启动 PDMS 前端

```bash
cd /home/pi/codes/pdms/frontend
npm install  # 首次运行
npm run dev
```

访问地址：http://localhost:5173

---

## 常用命令

```bash
# 查看服务状态
docker compose -f deploy/dev/docker-compose.infra.yml ps

# 查看日志
docker compose -f deploy/dev/docker-compose.infra.yml logs -f postgres
docker compose -f deploy/dev/docker-compose.infra.yml logs -f redis
docker compose -f deploy/dev/docker-compose.infra.yml logs -f minio

# 停止服务
docker compose -f deploy/dev/docker-compose.infra.yml down

# 清理数据（谨慎！）
docker compose -f deploy/dev/docker-compose.infra.yml down -v
```

## 连接数据库

```bash
# 连接 PostgreSQL
psql -h localhost -U postgres -d ndr
psql -h localhost -U postgres -d ydms

# 连接 Redis
redis-cli
```

## 端口分配

| 端口 | 服务 |
|------|------|
| 5432 | PostgreSQL |
| 6379 | Redis |
| 8000 | NDR API |
| 9000 | MinIO API |
| 9001 | MinIO 控制台 |
| 9180 | PDMS 后端 |
| 5173 | PDMS 前端 (Vite) |
