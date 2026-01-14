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

// SyncHandler handles document sync API endpoints.
type SyncHandler struct {
	service       *service.SyncService
	webhookSecret string
}

// NewSyncHandler creates a new SyncHandler.
func NewSyncHandler(svc *service.SyncService, webhookSecret string) *SyncHandler {
	return &SyncHandler{
		service:       svc,
		webhookSecret: webhookSecret,
	}
}

// SyncRoutes handles /api/v1/sync/* endpoints.
func (h *SyncHandler) SyncRoutes(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/api/v1/sync/")

	// /api/v1/sync/callback - 回调端点（不需要用户认证，需要 webhook secret）
	if relPath == "callback" {
		h.handleCallback(w, r)
		return
	}

	respondError(w, http.StatusNotFound, errors.New("not found"))
}

// DocumentSyncRoutes handles /api/v1/documents/{id}/sync* endpoints.
func (h *SyncHandler) DocumentSyncRoutes(w http.ResponseWriter, r *http.Request, docID int64) {
	// 根据 URL 路径判断具体端点
	path := r.URL.Path

	// POST /api/v1/documents/{id}/sync - 触发同步
	if strings.HasSuffix(path, "/sync") && r.Method == http.MethodPost {
		h.triggerSync(w, r, docID)
		return
	}

	// GET /api/v1/documents/{id}/sync-status - 获取同步状态
	if strings.HasSuffix(path, "/sync-status") && r.Method == http.MethodGet {
		h.getSyncStatus(w, r, docID)
		return
	}

	respondError(w, http.StatusNotFound, errors.New("not found"))
}

// InternalDocumentRoutes handles /api/internal/documents/* endpoints.
func (h *SyncHandler) InternalDocumentRoutes(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/api/internal/documents/")
	parts := strings.Split(relPath, "/")

	if len(parts) < 2 {
		respondError(w, http.StatusNotFound, errors.New("not found"))
		return
	}

	// 解析文档 ID
	docID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid document ID"))
		return
	}

	// GET /api/internal/documents/{id}/snapshot - 获取文档快照
	if parts[1] == "snapshot" && r.Method == http.MethodGet {
		h.getDocumentSnapshot(w, r, docID)
		return
	}

	respondError(w, http.StatusNotFound, errors.New("not found"))
}

// triggerSync triggers document sync to MySQL.
func (h *SyncHandler) triggerSync(w http.ResponseWriter, r *http.Request, docID int64) {
	// Get current user
	currentUser, ok := r.Context().Value(auth.UserContextKey).(*database.User)
	if !ok {
		respondError(w, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}

	// 权限检查：校对员不能触发同步
	if currentUser.Role == "proofreader" {
		respondError(w, http.StatusForbidden, errors.New("proofreader cannot trigger sync"))
		return
	}

	// Build RequestMeta from request headers
	meta := service.RequestMeta{
		APIKey:        r.Header.Get("x-api-key"),
		UserID:        currentUser.Username,
		RequestID:     r.Header.Get("x-request-id"),
		UserRole:      currentUser.Role,
		UserIDNumeric: currentUser.ID,
	}

	resp, err := h.service.TriggerSync(r.Context(), meta, docID)
	if err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusAccepted, resp)
}

// getSyncStatus returns the sync status for a document.
func (h *SyncHandler) getSyncStatus(w http.ResponseWriter, r *http.Request, docID int64) {
	// Get current user
	currentUser, ok := r.Context().Value(auth.UserContextKey).(*database.User)
	if !ok {
		respondError(w, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}

	// Build RequestMeta from request headers
	meta := service.RequestMeta{
		APIKey:        r.Header.Get("x-api-key"),
		UserID:        currentUser.Username,
		RequestID:     r.Header.Get("x-request-id"),
		UserRole:      currentUser.Role,
		UserIDNumeric: currentUser.ID,
	}

	resp, err := h.service.GetSyncStatus(r.Context(), meta, docID)
	if err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleCallback handles a callback from IDPP.
func (h *SyncHandler) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}

	// Verify webhook secret (required for callback)
	if h.webhookSecret == "" {
		respondError(w, http.StatusInternalServerError, errors.New("webhook secret not configured"))
		return
	}

	providedSecret := r.Header.Get("X-Webhook-Secret")
	if providedSecret != h.webhookSecret {
		respondError(w, http.StatusUnauthorized, errors.New("invalid webhook secret"))
		return
	}

	var callback service.SyncCallbackRequest
	if err := json.NewDecoder(r.Body).Decode(&callback); err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}

	if err := h.service.HandleSyncCallback(r.Context(), callback); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// getDocumentSnapshot returns a document snapshot for IDPP.
func (h *SyncHandler) getDocumentSnapshot(w http.ResponseWriter, r *http.Request, docID int64) {
	// 内部 API 使用 API Key 认证
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		apiKey = r.Header.Get("Authorization")
		if strings.HasPrefix(apiKey, "Bearer ") {
			apiKey = strings.TrimPrefix(apiKey, "Bearer ")
		}
	}

	if apiKey == "" {
		respondError(w, http.StatusUnauthorized, errors.New("API key required"))
		return
	}

	// Build RequestMeta with API key
	meta := service.RequestMeta{
		APIKey:    apiKey,
		UserID:    "idpp-internal", // 内部调用标识
		RequestID: r.Header.Get("x-request-id"),
	}

	snapshot, err := h.service.GetDocumentSnapshot(r.Context(), meta, docID)
	if err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusOK, snapshot)
}
