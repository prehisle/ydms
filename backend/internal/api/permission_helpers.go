package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/yjxt/ydms/backend/internal/database"
)

// requireNotProofreader 检查当前用户不是校对员角色
// 返回用户对象和可能的错误响应
func (h *Handler) requireNotProofreader(r *http.Request, action string) (*database.User, *httpError) {
	user, err := h.getCurrentUser(r)
	if err != nil {
		return nil, &httpError{
			code:    http.StatusUnauthorized,
			message: errors.New("user not found in context"),
		}
	}

	if user.Role == "proofreader" {
		return nil, &httpError{
			code:    http.StatusForbidden,
			message: fmt.Errorf("proofreaders cannot %s", action),
		}
	}

	return user, nil
}

// requireRole 检查当前用户是否具有指定角色之一
func (h *Handler) requireRole(r *http.Request, allowedRoles ...string) (*database.User, *httpError) {
	user, err := h.getCurrentUser(r)
	if err != nil {
		return nil, &httpError{
			code:    http.StatusUnauthorized,
			message: errors.New("user not found in context"),
		}
	}

	for _, role := range allowedRoles {
		if user.Role == role {
			return user, nil
		}
	}

	return nil, &httpError{
		code: http.StatusForbidden,
		message: fmt.Errorf("role '%s' is not permitted to perform this action (allowed: %v)",
			user.Role, allowedRoles),
	}
}

// getCurrentUser 获取当前用户（已有方法，保持兼容）
// 注意：handler.go 中已经有这个方法，这里仅作为文档说明

// httpError 用于权限检查的错误结构
type httpError struct {
	code    int
	message error
}
