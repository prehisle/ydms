package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/yjxt/ydms/backend/internal/auth"
	"github.com/yjxt/ydms/backend/internal/database"
	"github.com/yjxt/ydms/backend/internal/service"
)

// AdminWorkflowHandler handles admin workflow management endpoints.
type AdminWorkflowHandler struct {
	syncService *service.WorkflowSyncService
}

// NewAdminWorkflowHandler creates a new AdminWorkflowHandler.
func NewAdminWorkflowHandler(syncService *service.WorkflowSyncService) *AdminWorkflowHandler {
	return &AdminWorkflowHandler{
		syncService: syncService,
	}
}

// getCurrentUser gets the current user from request context.
func (h *AdminWorkflowHandler) getCurrentUser(r *http.Request) (*database.User, error) {
	user, ok := r.Context().Value(auth.UserContextKey).(*database.User)
	if !ok || user == nil {
		return nil, errors.New("user not found in context")
	}
	return user, nil
}

// isAdmin checks if the user has admin role.
func (h *AdminWorkflowHandler) isAdmin(user *database.User) bool {
	return user.Role == "super_admin" || user.Role == "course_admin"
}

// TriggerSync triggers a sync from Prefect.
// POST /api/v1/admin/workflows/sync
func (h *AdminWorkflowHandler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	user, err := h.getCurrentUser(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, err)
		return
	}

	if !h.isAdmin(user) {
		respondError(w, http.StatusForbidden, errors.New("管理员权限不足"))
		return
	}

	result, err := h.syncService.SyncFromPrefect(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// GetSyncStatus returns the current sync status.
// GET /api/v1/admin/workflows/sync/status
func (h *AdminWorkflowHandler) GetSyncStatus(w http.ResponseWriter, r *http.Request) {
	user, err := h.getCurrentUser(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, err)
		return
	}

	if !h.isAdmin(user) {
		respondError(w, http.StatusForbidden, errors.New("管理员权限不足"))
		return
	}

	status := h.syncService.GetSyncStatus()
	writeJSON(w, http.StatusOK, status)
}

// ListWorkflowDefinitions lists all workflow definitions with filters.
// GET /api/v1/admin/workflows
func (h *AdminWorkflowHandler) ListWorkflowDefinitions(w http.ResponseWriter, r *http.Request) {
	user, err := h.getCurrentUser(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, err)
		return
	}

	if !h.isAdmin(user) {
		respondError(w, http.StatusForbidden, errors.New("管理员权限不足"))
		return
	}

	// Parse query parameters
	filter := service.WorkflowDefinitionFilter{
		Source:       r.URL.Query().Get("source"),
		WorkflowType: r.URL.Query().Get("type"),
		SyncStatus:   r.URL.Query().Get("sync_status"),
	}

	if enabledStr := r.URL.Query().Get("enabled"); enabledStr != "" {
		enabled := enabledStr == "true"
		filter.Enabled = &enabled
	}

	definitions, err := h.syncService.ListWorkflowDefinitionsAdmin(filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, definitions)
}

// UpdateWorkflowDefinitionRequest represents the request to update a workflow definition.
type UpdateWorkflowDefinitionRequest struct {
	Enabled bool `json:"enabled"`
}

// UpdateWorkflowDefinition updates a workflow definition.
// PATCH /api/v1/admin/workflows/{id}
func (h *AdminWorkflowHandler) UpdateWorkflowDefinition(w http.ResponseWriter, r *http.Request) {
	user, err := h.getCurrentUser(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, err)
		return
	}

	if !h.isAdmin(user) {
		respondError(w, http.StatusForbidden, errors.New("管理员权限不足"))
		return
	}

	// Parse ID from path
	idStr := r.PathValue("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("无效的工作流 ID"))
		return
	}

	// Parse request body
	var req UpdateWorkflowDefinitionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, errors.New("无效的请求体"))
		return
	}

	// Update
	if err := h.syncService.UpdateWorkflowDefinition(uint(id), req.Enabled); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "工作流定义已更新",
	})
}
