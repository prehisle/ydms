package api

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrorCode 定义错误代码，用于前端识别和处理
type ErrorCode string

const (
	// 验证错误
	ErrCodeValidation ErrorCode = "VALIDATION_ERROR"

	// 资源不存在
	ErrCodeNotFound ErrorCode = "NOT_FOUND"

	// 认证错误
	ErrCodeUnauthorized ErrorCode = "UNAUTHORIZED"

	// 权限不足
	ErrCodeForbidden ErrorCode = "FORBIDDEN"

	// 资源冲突
	ErrCodeConflict ErrorCode = "CONFLICT"

	// 上游服务错误
	ErrCodeUpstream ErrorCode = "UPSTREAM_ERROR"

	// 服务器内部错误
	ErrCodeInternal ErrorCode = "INTERNAL_ERROR"
)

// APIError 统一的 API 错误结构
type APIError struct {
	Code       ErrorCode `json:"code"`
	Message    string    `json:"message"`
	Details    string    `json:"details,omitempty"`
	StatusCode int       `json:"-"` // 不序列化到 JSON
}

func (e *APIError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s", e.Message, e.Details)
	}
	return e.Message
}

// NewAPIError 创建新的 API 错误
func NewAPIError(code ErrorCode, statusCode int, message string, details ...string) *APIError {
	err := &APIError{
		Code:       code,
		Message:    message,
		StatusCode: statusCode,
	}
	if len(details) > 0 {
		err.Details = details[0]
	}
	return err
}

// 预定义的常见错误

// 分类相关错误
var (
	ErrCategoryNotFound = NewAPIError(
		ErrCodeNotFound,
		http.StatusNotFound,
		"分类不存在",
	)

	ErrCategoryHasChildren = NewAPIError(
		ErrCodeConflict,
		http.StatusConflict,
		"无法删除包含子分类的分类",
		"请先删除或移动子分类",
	)

	ErrInvalidAdminPassword = NewAPIError(
		ErrCodeUnauthorized,
		http.StatusUnauthorized,
		"管理员密码错误",
		"请提供正确的管理员密码",
	)

	ErrCategoryNameRequired = NewAPIError(
		ErrCodeValidation,
		http.StatusBadRequest,
		"分类名称不能为空",
	)
)

// 文档相关错误
var (
	ErrDocumentNotFound = NewAPIError(
		ErrCodeNotFound,
		http.StatusNotFound,
		"文档不存在",
	)

	ErrDocumentTitleRequired = NewAPIError(
		ErrCodeValidation,
		http.StatusBadRequest,
		"文档标题不能为空",
	)
)

// ErrInvalidDocumentType 创建无效文档类型错误
func ErrInvalidDocumentType(docType string) *APIError {
	return NewAPIError(
		ErrCodeValidation,
		http.StatusBadRequest,
		"无效的文档类型",
		fmt.Sprintf("不支持的文档类型: %s", docType),
	)
}

// ErrInvalidDocumentContent 创建无效文档内容错误
func ErrInvalidDocumentContent(reason string) *APIError {
	return NewAPIError(
		ErrCodeValidation,
		http.StatusBadRequest,
		"文档内容格式错误",
		reason,
	)
}

// ErrInvalidMetadata 创建无效元数据错误
func ErrInvalidMetadata(reason string) *APIError {
	return NewAPIError(
		ErrCodeValidation,
		http.StatusBadRequest,
		"元数据格式错误",
		reason,
	)
}

// 权限相关错误
var (
	ErrProofreaderCannotCreate = NewAPIError(
		ErrCodeForbidden,
		http.StatusForbidden,
		"校对员无法创建内容",
		"只有编辑员和管理员可以创建文档和分类",
	)

	ErrProofreaderCannotEdit = NewAPIError(
		ErrCodeForbidden,
		http.StatusForbidden,
		"校对员无法编辑内容",
		"只有编辑员和管理员可以编辑文档和分类",
	)

	ErrProofreaderCannotDelete = NewAPIError(
		ErrCodeForbidden,
		http.StatusForbidden,
		"校对员无法删除内容",
		"只有编辑员和管理员可以删除文档和分类",
	)

	ErrUserNotFound = NewAPIError(
		ErrCodeUnauthorized,
		http.StatusUnauthorized,
		"用户未登录或会话已过期",
	)
)

// ErrInsufficientPermission 创建权限不足错误
func ErrInsufficientPermission(requiredRoles []string, currentRole string) *APIError {
	return NewAPIError(
		ErrCodeForbidden,
		http.StatusForbidden,
		"权限不足",
		fmt.Sprintf("当前角色 '%s' 无权执行此操作，需要角色: %v", currentRole, requiredRoles),
	)
}

// 上游服务错误
func WrapUpstreamError(err error) *APIError {
	return NewAPIError(
		ErrCodeUpstream,
		http.StatusBadGateway,
		"上游服务错误",
		err.Error(),
	)
}

// 内部错误
var ErrInternal = NewAPIError(
	ErrCodeInternal,
	http.StatusInternalServerError,
	"服务器内部错误",
	"请稍后重试或联系管理员",
)

// respondAPIError 统一的错误响应函数
// 自动识别错误类型并返回相应的 HTTP 状态码
func respondAPIError(w http.ResponseWriter, err error) {
	var apiErr *APIError

	// 如果是 APIError，直接使用
	if errors.As(err, &apiErr) {
		writeJSON(w, apiErr.StatusCode, apiErr)
		return
	}

	// 未知错误，返回通用的内部错误
	writeJSON(w, ErrInternal.StatusCode, ErrInternal)
}
