package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/yjxt/ydms/backend/internal/database"
)

// contextKey 类型用于 context key
type contextKey string

const (
	// UserContextKey context 中存储用户信息的 key
	UserContextKey contextKey = "user"
	// ClaimsContextKey context 中存储 JWT claims 的 key
	ClaimsContextKey contextKey = "claims"
)

// AuthMiddleware JWT 认证中间件
func AuthMiddleware(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 从 Authorization header 获取 token
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				respondError(w, http.StatusUnauthorized, errors.New("missing authorization header"))
				return
			}

			// 解析 Bearer token
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				respondError(w, http.StatusUnauthorized, errors.New("invalid authorization header format"))
				return
			}

			tokenString := parts[1]

			// 验证 token
			claims, err := ValidateToken(tokenString, jwtSecret)
			if err != nil {
				respondError(w, http.StatusUnauthorized, errors.New("invalid token: "+err.Error()))
				return
			}

			// 将 claims 存入 context
			ctx := context.WithValue(r.Context(), ClaimsContextKey, claims)

			// 构造简化的 User 对象存入 context（避免每次都查询数据库）
			user := &database.User{
				ID:       claims.UserID,
				Username: claims.Username,
				Role:     claims.Role,
			}
			ctx = context.WithValue(ctx, UserContextKey, user)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole 角色检查中间件
func RequireRole(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := r.Context().Value(UserContextKey).(*database.User)
			if !ok {
				respondError(w, http.StatusUnauthorized, errors.New("user not found in context"))
				return
			}

			// 检查用户角色是否在允许列表中
			allowed := false
			for _, role := range allowedRoles {
				if user.Role == role {
					allowed = true
					break
				}
			}

			if !allowed {
				respondError(w, http.StatusForbidden, errors.New("insufficient permissions"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// respondError 返回 JSON 错误响应
func respondError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + err.Error() + `"}`))
}
