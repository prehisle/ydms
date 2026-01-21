package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/yjxt/ydms/backend/internal/auth"
	"github.com/yjxt/ydms/backend/internal/database"
	"github.com/yjxt/ydms/backend/internal/service"
)

// BatchHandler 处理批量操作的 HTTP 请求
type BatchHandler struct {
	batchWorkflowService *service.BatchWorkflowService
	batchSyncService     *service.BatchSyncService
}

// NewBatchHandler 创建 BatchHandler
func NewBatchHandler(batchWorkflowSvc *service.BatchWorkflowService, batchSyncSvc *service.BatchSyncService) *BatchHandler {
	return &BatchHandler{
		batchWorkflowService: batchWorkflowSvc,
		batchSyncService:     batchSyncSvc,
	}
}

// BatchWorkflowRoutes 处理批量工作流路由
// /api/v1/nodes/{nodeId}/workflows/batch/preview - POST 预览
// /api/v1/nodes/{nodeId}/workflows/batch/execute - POST 执行
// /api/v1/workflows/batches/{batchId} - GET 查询状态
// /api/v1/workflows/batches - GET 列表
func (h *BatchHandler) BatchWorkflowRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// 处理批次状态查询和列表
	if strings.HasPrefix(path, "/api/v1/workflows/batches") {
		relPath := strings.TrimPrefix(path, "/api/v1/workflows/batches")
		relPath = strings.TrimPrefix(relPath, "/")

		if relPath == "" {
			// /api/v1/workflows/batches - 列表
			if r.Method == http.MethodGet {
				h.listBatchWorkflows(w, r)
				return
			}
			respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}

		// /api/v1/workflows/batches/{batchId} - 查询状态
		if r.Method == http.MethodGet {
			h.getBatchWorkflowStatus(w, r, relPath)
			return
		}
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}

	// 处理节点批量工作流 /api/v1/nodes/{nodeId}/workflows/batch/*
	if strings.HasPrefix(path, "/api/v1/nodes/") {
		h.handleNodeBatchWorkflow(w, r)
		return
	}

	respondError(w, http.StatusNotFound, errors.New("not found"))
}

// handleNodeBatchWorkflow 处理节点级别的批量工作流请求
func (h *BatchHandler) handleNodeBatchWorkflow(w http.ResponseWriter, r *http.Request) {
	// 解析路径: /api/v1/nodes/{nodeId}/workflows/batch/(preview|execute)
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/nodes/")
	parts := strings.Split(path, "/")

	if len(parts) < 4 || parts[1] != "workflows" || parts[2] != "batch" {
		respondError(w, http.StatusNotFound, errors.New("invalid batch workflow path"))
		return
	}

	nodeID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid node ID"))
		return
	}

	action := parts[3]
	meta := metaFromRequestContext(r)

	switch action {
	case "preview":
		if r.Method != http.MethodPost {
			respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		h.previewBatchWorkflow(w, r, meta, nodeID)
	case "execute":
		if r.Method != http.MethodPost {
			respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		h.executeBatchWorkflow(w, r, meta, nodeID)
	default:
		respondError(w, http.StatusNotFound, errors.New("unknown action"))
	}
}

// previewBatchWorkflow 预览批量工作流
func (h *BatchHandler) previewBatchWorkflow(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, nodeID int64) {
	var req service.BatchWorkflowPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}

	if req.WorkflowKey == "" {
		respondError(w, http.StatusBadRequest, errors.New("workflow_key is required"))
		return
	}

	result, err := h.batchWorkflowService.PreviewBatchWorkflow(r.Context(), meta, nodeID, req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// executeBatchWorkflow 执行批量工作流
func (h *BatchHandler) executeBatchWorkflow(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, nodeID int64) {
	var req service.BatchWorkflowExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}

	if req.WorkflowKey == "" {
		respondError(w, http.StatusBadRequest, errors.New("workflow_key is required"))
		return
	}

	result, err := h.batchWorkflowService.ExecuteBatchWorkflow(r.Context(), meta, nodeID, req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusAccepted, result)
}

// getBatchWorkflowStatus 获取批量工作流状态
func (h *BatchHandler) getBatchWorkflowStatus(w http.ResponseWriter, r *http.Request, batchID string) {
	result, err := h.batchWorkflowService.GetBatchWorkflowStatus(r.Context(), batchID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, err)
			return
		}
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// listBatchWorkflows 列出批量工作流
func (h *BatchHandler) listBatchWorkflows(w http.ResponseWriter, r *http.Request) {
	meta := metaFromRequestContext(r)

	limit := 20
	offset := 0

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}

	results, total, err := h.batchWorkflowService.ListBatchWorkflows(r.Context(), meta, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":    results,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": int64(offset+len(results)) < total,
	})
}

// BatchSyncRoutes 处理批量同步路由
// /api/v1/nodes/{nodeId}/sync/batch/preview - POST 预览
// /api/v1/nodes/{nodeId}/sync/batch/execute - POST 执行
// /api/v1/sync/batches/{batchId} - GET 查询状态
// /api/v1/sync/batches - GET 列表
func (h *BatchHandler) BatchSyncRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// 处理批次状态查询和列表
	if strings.HasPrefix(path, "/api/v1/sync/batches") {
		relPath := strings.TrimPrefix(path, "/api/v1/sync/batches")
		relPath = strings.TrimPrefix(relPath, "/")

		if relPath == "" {
			// /api/v1/sync/batches - 列表
			if r.Method == http.MethodGet {
				h.listBatchSyncs(w, r)
				return
			}
			respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}

		// /api/v1/sync/batches/{batchId} - 查询状态
		if r.Method == http.MethodGet {
			h.getBatchSyncStatus(w, r, relPath)
			return
		}
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}

	// 处理节点批量同步 /api/v1/nodes/{nodeId}/sync/batch/*
	if strings.HasPrefix(path, "/api/v1/nodes/") {
		h.handleNodeBatchSync(w, r)
		return
	}

	respondError(w, http.StatusNotFound, errors.New("not found"))
}

// handleNodeBatchSync 处理节点级别的批量同步请求
func (h *BatchHandler) handleNodeBatchSync(w http.ResponseWriter, r *http.Request) {
	// 解析路径: /api/v1/nodes/{nodeId}/sync/batch/(preview|execute)
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/nodes/")
	parts := strings.Split(path, "/")

	if len(parts) < 4 || parts[1] != "sync" || parts[2] != "batch" {
		respondError(w, http.StatusNotFound, errors.New("invalid batch sync path"))
		return
	}

	nodeID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid node ID"))
		return
	}

	action := parts[3]
	meta := metaFromRequestContext(r)

	switch action {
	case "preview":
		if r.Method != http.MethodPost {
			respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		h.previewBatchSync(w, r, meta, nodeID)
	case "execute":
		if r.Method != http.MethodPost {
			respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		h.executeBatchSync(w, r, meta, nodeID)
	default:
		respondError(w, http.StatusNotFound, errors.New("unknown action"))
	}
}

// previewBatchSync 预览批量同步
func (h *BatchHandler) previewBatchSync(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, nodeID int64) {
	var req service.BatchSyncPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// 允许空请求体（使用默认值）
		req = service.BatchSyncPreviewRequest{}
	}

	result, err := h.batchSyncService.PreviewBatchSync(r.Context(), meta, nodeID, req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// executeBatchSync 执行批量同步
func (h *BatchHandler) executeBatchSync(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, nodeID int64) {
	var req service.BatchSyncExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// 允许空请求体（使用默认值）
		req = service.BatchSyncExecuteRequest{}
	}

	result, err := h.batchSyncService.ExecuteBatchSync(r.Context(), meta, nodeID, req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusAccepted, result)
}

// getBatchSyncStatus 获取批量同步状态
func (h *BatchHandler) getBatchSyncStatus(w http.ResponseWriter, r *http.Request, batchID string) {
	result, err := h.batchSyncService.GetBatchSyncStatus(r.Context(), batchID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, err)
			return
		}
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// listBatchSyncs 列出批量同步
func (h *BatchHandler) listBatchSyncs(w http.ResponseWriter, r *http.Request) {
	meta := metaFromRequestContext(r)

	limit := 20
	offset := 0

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}

	results, total, err := h.batchSyncService.ListBatchSyncs(r.Context(), meta, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":    results,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": int64(offset+len(results)) < total,
	})
}

// metaFromRequestContext 从请求上下文中提取 RequestMeta
// 这个函数假设认证中间件已经设置了上下文值
func metaFromRequestContext(r *http.Request) service.RequestMeta {
	meta := service.RequestMeta{}

	// 从上下文获取用户信息（认证中间件设置）
	if user, ok := r.Context().Value(auth.UserContextKey).(*database.User); ok {
		meta.UserIDNumeric = user.ID
		meta.UserID = strconv.FormatUint(uint64(user.ID), 10)
		meta.UserRole = user.Role
	}

	// 从请求头获取 API Key 等信息
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		meta.APIKey = apiKey
	}
	if adminKey := r.Header.Get("X-Admin-Key"); adminKey != "" {
		meta.AdminKey = adminKey
	}

	return meta
}
