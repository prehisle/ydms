package api

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/yjxt/ydms/backend/internal/auth"
)

// RouterConfig 路由器配置
type RouterConfig struct {
	Handler           *Handler
	AuthHandler       *AuthHandler
	UserHandler       *UserHandler
	CourseHandler     *CourseHandler
	APIKeyHandler     *APIKeyHandler
	AssetsHandler     *AssetsHandler
	ProcessingHandler *ProcessingHandler
	JWTSecret         string
	DB                *gorm.DB // 用于 API Key 验证
}

// NewRouter creates the HTTP router and wires handler endpoints.
func NewRouter(h *Handler) http.Handler {
	// 为了向后兼容，保留原有签名
	// 实际应用中应该使用 NewRouterWithConfig
	mux := http.NewServeMux()

	wrap := h.applyMiddleware

	mux.Handle("/healthz", wrap(http.HandlerFunc(h.Health)))
	mux.Handle("/api/v1/healthz", wrap(http.HandlerFunc(h.Health)))
	mux.Handle("/api/v1/ping", wrap(http.HandlerFunc(h.Ping)))
	mux.Handle("/api/v1/categories", wrap(http.HandlerFunc(h.Categories)))
	mux.Handle("/api/v1/categories/", wrap(http.HandlerFunc(h.CategoryRoutes)))
	mux.Handle("/api/v1/documents", wrap(http.HandlerFunc(h.Documents)))
	mux.Handle("/api/v1/documents/", wrap(http.HandlerFunc(h.DocumentRoutes)))
	mux.Handle("/api/v1/nodes/", wrap(http.HandlerFunc(h.NodeRoutes)))

	return mux
}

// NewRouterWithConfig 创建带认证功能的路由器
func NewRouterWithConfig(cfg RouterConfig) http.Handler {
	mux := http.NewServeMux()

	wrap := cfg.Handler.applyMiddleware
	authWrap := cfg.Handler.applyAuthMiddleware(cfg.JWTSecret, cfg.DB)

	// 健康检查端点（公开）
	mux.Handle("/health", wrap(http.HandlerFunc(cfg.Handler.Health)))
	mux.Handle("/healthz", wrap(http.HandlerFunc(cfg.Handler.Health)))
	mux.Handle("/api/v1/healthz", wrap(http.HandlerFunc(cfg.Handler.Health)))
	mux.Handle("/api/v1/ping", wrap(http.HandlerFunc(cfg.Handler.Ping)))

	// 认证端点
	mux.Handle("/api/v1/auth/login", wrap(http.HandlerFunc(cfg.AuthHandler.Login)))
	mux.Handle("/api/v1/auth/logout", authWrap(http.HandlerFunc(cfg.AuthHandler.Logout)))
	mux.Handle("/api/v1/auth/me", authWrap(http.HandlerFunc(cfg.AuthHandler.Me)))
	mux.Handle("/api/v1/auth/change-password", authWrap(http.HandlerFunc(cfg.AuthHandler.ChangePassword)))

	// 用户管理端点（需要认证）
	if cfg.UserHandler != nil {
		mux.Handle("/api/v1/users", authWrap(http.HandlerFunc(handleUsersRoot(cfg.UserHandler))))
		mux.Handle("/api/v1/users/", authWrap(http.HandlerFunc(handleUserRoutes(cfg.UserHandler))))
	}

	// 课程管理端点（需要认证）
	if cfg.CourseHandler != nil {
		mux.Handle("/api/v1/courses", authWrap(http.HandlerFunc(cfg.CourseHandler.ListCourses)))
		mux.Handle("/api/v1/courses/", authWrap(http.HandlerFunc(handleCourseRoutes(cfg.CourseHandler))))
	}

	// API Key 管理端点（需要认证，仅限管理员）
	if cfg.APIKeyHandler != nil {
		mux.Handle("/api/v1/api-keys", authWrap(http.HandlerFunc(cfg.APIKeyHandler.APIKeys)))
		mux.Handle("/api/v1/api-keys/", authWrap(http.HandlerFunc(cfg.APIKeyHandler.APIKeyRoutes)))
	}

	// 业务端点（需要认证）
	mux.Handle("/api/v1/categories", authWrap(http.HandlerFunc(cfg.Handler.Categories)))
	mux.Handle("/api/v1/categories/", authWrap(http.HandlerFunc(cfg.Handler.CategoryRoutes)))
	mux.Handle("/api/v1/documents", authWrap(http.HandlerFunc(cfg.Handler.Documents)))
	mux.Handle("/api/v1/documents/", authWrap(http.HandlerFunc(cfg.Handler.DocumentRoutes)))
	mux.Handle("/api/v1/nodes/", authWrap(http.HandlerFunc(cfg.Handler.NodeRoutes)))

	// 路径解析端点（需要认证）
	mux.Handle("/api/v1/resolve/", authWrap(http.HandlerFunc(cfg.Handler.ResolveRoutes)))

	// Assets 端点（需要认证）
	if cfg.AssetsHandler != nil {
		mux.Handle("/api/v1/assets", authWrap(http.HandlerFunc(cfg.AssetsHandler.Assets)))
		mux.Handle("/api/v1/assets/", authWrap(http.HandlerFunc(cfg.AssetsHandler.AssetRoutes)))
	}

	// Processing 端点（AI 处理）
	if cfg.ProcessingHandler != nil {
		// 触发和查询端点（需要认证）
		mux.Handle("/api/v1/processing", authWrap(http.HandlerFunc(cfg.ProcessingHandler.Processing)))
		mux.Handle("/api/v1/processing/pipelines", authWrap(http.HandlerFunc(cfg.ProcessingHandler.ProcessingRoutes)))
		mux.Handle("/api/v1/processing/jobs", authWrap(http.HandlerFunc(cfg.ProcessingHandler.ProcessingRoutes)))
		mux.Handle("/api/v1/processing/jobs/", authWrap(http.HandlerFunc(cfg.ProcessingHandler.ProcessingRoutes)))
		// Callback 端点（不需要 JWT 认证，由 Webhook Secret 验证）
		mux.Handle("/api/v1/processing/callback/", wrap(http.HandlerFunc(cfg.ProcessingHandler.ProcessingRoutes)))
	}

	return mux
}

// handleUsersRoot 处理 /api/v1/users 路由（GET 和 POST）
func handleUsersRoot(h *UserHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.ListUsers(w, r)
		case http.MethodPost:
			h.CreateUser(w, r)
		default:
			respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
	}
}

// handleUserRoutes 处理用户相关子路由
func handleUserRoutes(h *UserHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// GET /api/v1/users/:id
		// DELETE /api/v1/users/:id
		if len(path) > len("/api/v1/users/") {
			// 检查是否是课程权限相关路由
			if strings.Contains(path, "/courses") {
				if strings.HasSuffix(path, "/courses") && r.Method == http.MethodGet {
					h.GetUserCourses(w, r)
					return
				}
				if strings.HasSuffix(path, "/courses") && r.Method == http.MethodPost {
					h.GrantCoursePermission(w, r)
					return
				}
				if r.Method == http.MethodDelete {
					h.RevokeCoursePermission(w, r)
					return
				}
			} else {
				// 用户基本操作
				switch r.Method {
				case http.MethodGet:
					h.GetUser(w, r)
				case http.MethodDelete:
					h.DeleteUser(w, r)
				default:
					respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
				}
				return
			}
		}

		respondError(w, http.StatusNotFound, errors.New("not found"))
	}
}

// handleCourseRoutes 处理课程相关子路由
func handleCourseRoutes(h *CourseHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// POST /api/v1/courses (创建课程)
		if path == "/api/v1/courses" && r.Method == http.MethodPost {
			h.CreateCourse(w, r)
			return
		}

		// DELETE /api/v1/courses/:id
		if len(path) > len("/api/v1/courses/") && r.Method == http.MethodDelete {
			h.DeleteCourse(w, r)
			return
		}

		respondError(w, http.StatusNotFound, errors.New("not found"))
	}
}

func (h *Handler) applyMiddleware(next http.Handler) http.Handler {
	handler := next
	handler = requestContextMiddleware(h.defaults.UserID)(handler)
	handler = corsMiddleware(handler)
	handler = loggingMiddleware(handler)
	return handler
}

func (h *Handler) applyAuthMiddleware(jwtSecret string, db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		handler := next
		// 先应用认证中间件（支持 JWT 和 API Key）
		handler = authMiddlewareWrapper(jwtSecret, db)(handler)
		// 再应用其他中间件
		handler = corsMiddleware(handler)
		handler = loggingMiddleware(handler)
		return handler
	}
}

// authMiddlewareWrapper 认证中间件包装器（支持 JWT 和 API Key）
func authMiddlewareWrapper(jwtSecret string, db *gorm.DB) func(http.Handler) http.Handler {
	if db != nil {
		// 使用灵活的认证中间件（支持 JWT 和 API Key）
		return auth.FlexibleAuthMiddleware(db, jwtSecret)
	}
	// 降级为仅支持 JWT
	return auth.AuthMiddleware(jwtSecret)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(lrw, r)

		duration := time.Since(start)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, lrw.status, duration)
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (lrw *loggingResponseWriter) WriteHeader(status int) {
	lrw.status = status
	lrw.ResponseWriter.WriteHeader(status)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// For development we allow all origins; adjust as needed for production.
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, x-api-key, x-user-id, x-request-id, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func requestContextMiddleware(defaultUserID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("x-user-id") == "" && defaultUserID != "" {
				r.Header.Set("x-user-id", defaultUserID)
			}
			if r.Header.Get("x-request-id") == "" {
				r.Header.Set("x-request-id", uuid.NewString())
			}
			next.ServeHTTP(w, r)
		})
	}
}
