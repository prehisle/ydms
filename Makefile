.PHONY: help quick-reset reset-db reset-init test-e2e dev clean docs-check docs-lint docs-openapi infra-up infra-down infra-logs infra-ps

help: ## 显示帮助信息
	@echo "YDMS 项目命令"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ==================== 基础设施 ====================

infra-up: ## 启动本地基础设施 (PostgreSQL, Redis, MinIO)
	@docker compose -f deploy/dev/docker-compose.infra.yml up -d
	@echo ""
	@echo "基础设施已启动:"
	@echo "  PostgreSQL: localhost:5432 (postgres/postgres)"
	@echo "  Redis:      localhost:6379"
	@echo "  MinIO:      localhost:9000 (minioadmin/minioadmin)"
	@echo "  MinIO 控制台: http://localhost:9002"

infra-down: ## 停止本地基础设施
	@docker compose -f deploy/dev/docker-compose.infra.yml down

infra-logs: ## 查看基础设施日志
	@docker compose -f deploy/dev/docker-compose.infra.yml logs -f

infra-ps: ## 查看基础设施状态
	@docker compose -f deploy/dev/docker-compose.infra.yml ps

infra-clean: ## 清理基础设施数据（谨慎！）
	@docker compose -f deploy/dev/docker-compose.infra.yml down -v
	@echo "基础设施数据已清理"

# ==================== 数据库 ====================

quick-reset: ## 快速重置（仅清空数据，推荐）
	@./scripts/quick-reset.sh

reset-db: ## 重置数据库（删除所有数据）
	@./scripts/reset-db.sh

reset-init: ## 完整重置并初始化（重建数据库）
	@./scripts/reset-and-init.sh

dev-backend: ## 启动后端开发服务器
	@cd backend && go run ./cmd/server --watch

dev-frontend: ## 启动前端开发服务器
	@cd frontend && npm run dev

test-backend: ## 运行后端测试
	@cd backend && go test ./... -cover

generate-doc-types: ## 基于 doc-types 配置生成前后端代码
	@cd backend && go run ./cmd/docgen --config ../doc-types/config.yaml --repo-root .. --frontend-dir ../frontend --backend-dir .

test-e2e: ## 运行 E2E 测试
	@cd frontend && npx playwright test --reporter=list

test-e2e-ui: ## 运行 E2E 测试（UI 模式）
	@cd frontend && npx playwright test --ui

clean: ## 清理临时文件
	@rm -rf backend/tmp backend/server.log
	@rm -rf frontend/dist frontend/node_modules/.vite
	@echo "清理完成"

install-frontend: ## 安装前端依赖
	@cd frontend && npm install

install-backend: ## 安装后端依赖
	@cd backend && go mod tidy

install: install-backend install-frontend ## 安装所有依赖

hooks: ## 安装本仓库 git hooks（阻止未通过的提交/推送）
	@git config core.hooksPath .githooks
	@chmod +x .githooks/pre-commit .githooks/pre-push .githooks/commit-msg
	@echo "已启用 hooks: core.hooksPath=.githooks"

docs-check: ## 检查 Markdown 内部链接有效性
	@bash scripts/check-docs.sh

docs-lint: ## 运行 markdownlint（若已安装）
	@if command -v markdownlint >/dev/null 2>&1; then \
	  markdownlint "**/*.md"; \
	else \
	  echo "markdownlint 未安装，跳过（可用: npm i -g markdownlint-cli）"; \
	fi

docs-openapi: ## 打开 OpenAPI 静态预览页面
	@echo "打开 docs/api/index.html"
	@if command -v xdg-open >/dev/null 2>&1; then xdg-open docs/api/index.html; \
	elif command -v open >/dev/null 2>&1; then open docs/api/index.html; \
	else echo "请手动在浏览器打开 docs/api/index.html"; fi
