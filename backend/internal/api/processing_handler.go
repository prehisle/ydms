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

// ProcessingHandler handles AI processing API endpoints.
type ProcessingHandler struct {
	service       *service.ProcessingService
	webhookSecret string
}

// NewProcessingHandler creates a new ProcessingHandler.
func NewProcessingHandler(svc *service.ProcessingService, webhookSecret string) *ProcessingHandler {
	return &ProcessingHandler{
		service:       svc,
		webhookSecret: webhookSecret,
	}
}

// Processing handles POST /api/v1/processing (trigger pipeline).
func (h *ProcessingHandler) Processing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	h.triggerPipeline(w, r)
}

// ProcessingRoutes handles /api/v1/processing/* endpoints.
func (h *ProcessingHandler) ProcessingRoutes(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/api/v1/processing/")
	if relPath == "" || relPath == "processing" {
		h.Processing(w, r)
		return
	}

	// /api/v1/processing/pipelines
	if relPath == "pipelines" {
		h.listPipelines(w, r)
		return
	}

	// /api/v1/processing/jobs?document_id=xxx
	if relPath == "jobs" {
		h.listJobs(w, r)
		return
	}

	// /api/v1/processing/jobs/{job_id}
	if strings.HasPrefix(relPath, "jobs/") {
		jobIDStr := strings.TrimPrefix(relPath, "jobs/")
		jobID, err := strconv.ParseUint(jobIDStr, 10, 32)
		if err != nil {
			respondError(w, http.StatusBadRequest, errors.New("invalid job ID"))
			return
		}
		h.getJob(w, r, uint(jobID))
		return
	}

	// /api/v1/processing/callback/{job_id}
	if strings.HasPrefix(relPath, "callback/") {
		jobIDStr := strings.TrimPrefix(relPath, "callback/")
		jobID, err := strconv.ParseUint(jobIDStr, 10, 32)
		if err != nil {
			respondError(w, http.StatusBadRequest, errors.New("invalid job ID"))
			return
		}
		h.handleCallback(w, r, uint(jobID))
		return
	}

	respondError(w, http.StatusNotFound, errors.New("not found"))
}

// triggerPipeline triggers an AI processing pipeline.
func (h *ProcessingHandler) triggerPipeline(w http.ResponseWriter, r *http.Request) {
	// Get current user
	currentUser, ok := r.Context().Value(auth.UserContextKey).(*database.User)
	if !ok {
		respondError(w, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}

	var req service.TriggerPipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid request body"))
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

	resp, err := h.service.TriggerPipeline(r.Context(), meta, req)
	if err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusAccepted, resp)
}

// getJob retrieves a job by ID.
func (h *ProcessingHandler) getJob(w http.ResponseWriter, r *http.Request, jobID uint) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}

	job, err := h.service.GetJob(r.Context(), jobID)
	if err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}

	writeJSON(w, http.StatusOK, job)
}

// listJobs lists processing jobs for a document.
func (h *ProcessingHandler) listJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}

	// 获取当前用户
	currentUser, ok := r.Context().Value(auth.UserContextKey).(*database.User)
	if !ok {
		respondError(w, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}

	// 解析查询参数
	query := r.URL.Query()

	// document_id 参数（可选）
	var documentID *int64
	if docIDStr := query.Get("document_id"); docIDStr != "" {
		id, err := strconv.ParseInt(docIDStr, 10, 64)
		if err != nil {
			respondError(w, http.StatusBadRequest, errors.New("invalid document_id"))
			return
		}
		documentID = &id
	}

	// status 参数（可选，逗号分隔）
	var statusFilter []string
	if statusStr := query.Get("status"); statusStr != "" {
		statusFilter = strings.Split(statusStr, ",")
		// 验证状态值
		validStatuses := map[string]bool{
			"pending": true, "running": true, "completed": true, "failed": true, "cancelled": true,
		}
		for _, s := range statusFilter {
			if !validStatuses[s] {
				respondError(w, http.StatusBadRequest, errors.New("invalid status value: "+s))
				return
			}
		}
	}

	// limit 参数
	limit := 20
	if limitStr := query.Get("limit"); limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil || l < 1 {
			respondError(w, http.StatusBadRequest, errors.New("invalid limit"))
			return
		}
		limit = l
	}

	// offset 参数
	offset := 0
	if offsetStr := query.Get("offset"); offsetStr != "" {
		o, err := strconv.Atoi(offsetStr)
		if err != nil || o < 0 {
			respondError(w, http.StatusBadRequest, errors.New("invalid offset"))
			return
		}
		offset = o
	}

	// 构建查询参数
	params := service.ListJobsParams{
		DocumentID: documentID,
		UserID:     &currentUser.ID, // 只返回当前用户的任务
		Status:     statusFilter,
		Limit:      limit,
		Offset:     offset,
	}

	result, err := h.service.ListJobsWithParams(r.Context(), params)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleCallback handles a callback from IDPP.
func (h *ProcessingHandler) handleCallback(w http.ResponseWriter, r *http.Request, jobID uint) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}

	// Verify webhook secret (if configured)
	if h.webhookSecret != "" {
		providedSecret := r.Header.Get("X-Webhook-Secret")
		if providedSecret != h.webhookSecret {
			respondError(w, http.StatusUnauthorized, errors.New("invalid webhook secret"))
			return
		}
	}

	var callback service.CallbackRequest
	if err := json.NewDecoder(r.Body).Decode(&callback); err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}

	if err := h.service.HandleCallback(r.Context(), jobID, callback); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// listPipelines returns available pipelines.
func (h *ProcessingHandler) listPipelines(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}

	pipelines := h.service.ListPipelines()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pipelines": pipelines,
	})
}
