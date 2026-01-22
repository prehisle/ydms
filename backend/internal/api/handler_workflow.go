package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/yjxt/ydms/backend/internal/service"
)

// WorkflowHandler handles workflow-related HTTP requests.
type WorkflowHandler struct {
	workflowService *service.WorkflowService
	handler         *Handler // 用于获取 meta
}

// NewWorkflowHandler creates a new WorkflowHandler.
func NewWorkflowHandler(workflowService *service.WorkflowService, handler *Handler) *WorkflowHandler {
	return &WorkflowHandler{
		workflowService: workflowService,
		handler:         handler,
	}
}

// WorkflowRoutes handles /api/v1/workflows/* routes.
func (h *WorkflowHandler) WorkflowRoutes(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/api/v1/workflows")
	relPath = strings.TrimPrefix(relPath, "/")

	// GET /api/v1/workflows - list workflow definitions
	if relPath == "" && r.Method == http.MethodGet {
		h.listWorkflowDefinitions(w, r)
		return
	}

	// POST /api/v1/workflows/callback/{runId} - handle callback
	if strings.HasPrefix(relPath, "callback/") {
		runIDStr := strings.TrimPrefix(relPath, "callback/")
		runID, err := strconv.ParseUint(runIDStr, 10, 64)
		if err != nil {
			respondError(w, http.StatusBadRequest, errors.New("invalid run id"))
			return
		}
		h.handleCallback(w, r, uint(runID))
		return
	}

	// GET /api/v1/workflows/runs - list workflow runs
	if relPath == "runs" && r.Method == http.MethodGet {
		h.listWorkflowRuns(w, r)
		return
	}

	// GET /api/v1/workflows/runs/{runId} - get workflow run
	// POST /api/v1/workflows/runs/{runId}/cancel - cancel workflow run
	if strings.HasPrefix(relPath, "runs/") {
		rest := strings.TrimPrefix(relPath, "runs/")
		// Check for /cancel suffix
		if strings.HasSuffix(rest, "/cancel") {
			runIDStr := strings.TrimSuffix(rest, "/cancel")
			runID, err := strconv.ParseUint(runIDStr, 10, 64)
			if err != nil {
				respondError(w, http.StatusBadRequest, errors.New("invalid run id"))
				return
			}
			if r.Method == http.MethodPost {
				h.cancelWorkflowRun(w, r, uint(runID))
				return
			}
			respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		// Otherwise it's GET /runs/{runId}
		runID, err := strconv.ParseUint(rest, 10, 64)
		if err != nil {
			respondError(w, http.StatusBadRequest, errors.New("invalid run id"))
			return
		}
		h.getWorkflowRun(w, r, uint(runID))
		return
	}

	respondError(w, http.StatusNotFound, errors.New("not found"))
}

// NodeWorkflowRoutes handles /api/v1/nodes/{id}/workflows/* routes.
func (h *WorkflowHandler) NodeWorkflowRoutes(w http.ResponseWriter, r *http.Request, nodeID int64, subPath string) {
	meta := h.handler.metaFromRequest(r)

	// GET /api/v1/nodes/{id}/workflows - list available workflows for node
	if subPath == "" && r.Method == http.MethodGet {
		h.listNodeWorkflows(w, r)
		return
	}

	// GET /api/v1/nodes/{id}/workflow-runs - list workflow runs for node
	if subPath == "-runs" && r.Method == http.MethodGet {
		h.listNodeWorkflowRuns(w, r, nodeID)
		return
	}

	// POST /api/v1/nodes/{id}/workflows/{workflowKey}/runs - trigger workflow
	parts := strings.Split(subPath, "/")
	if len(parts) == 2 && parts[1] == "runs" && r.Method == http.MethodPost {
		workflowKey := parts[0]
		h.triggerWorkflow(w, r, meta, nodeID, workflowKey)
		return
	}

	respondError(w, http.StatusNotFound, errors.New("not found"))
}

// listWorkflowDefinitions handles GET /api/v1/workflows
func (h *WorkflowHandler) listWorkflowDefinitions(w http.ResponseWriter, r *http.Request) {
	definitions, err := h.workflowService.ListWorkflowDefinitions(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, definitions)
}

// listNodeWorkflows handles GET /api/v1/nodes/{id}/workflows
func (h *WorkflowHandler) listNodeWorkflows(w http.ResponseWriter, r *http.Request) {
	// 返回所有可用的节点工作流定义
	definitions, err := h.workflowService.ListWorkflowDefinitionsByType(r.Context(), "node")
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, definitions)
}

// triggerWorkflow handles POST /api/v1/nodes/{id}/workflows/{workflowKey}/runs
func (h *WorkflowHandler) triggerWorkflow(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, nodeID int64, workflowKey string) {
	var params struct {
		Parameters map[string]interface{} `json:"parameters"`
		RetryOfID  *uint                  `json:"retry_of_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil && err.Error() != "EOF" {
		respondAPIError(w, NewAPIError(ErrCodeValidation, http.StatusBadRequest, "请求格式错误", err.Error()))
		return
	}

	req := service.TriggerWorkflowRequest{
		NodeID:      nodeID,
		WorkflowKey: workflowKey,
		Parameters:  params.Parameters,
		RetryOfID:   params.RetryOfID,
	}

	resp, err := h.workflowService.TriggerWorkflow(r.Context(), meta, req)
	if err != nil {
		var vErr *service.ValidationError
		if errors.As(err, &vErr) {
			respondAPIError(w, NewAPIError(ErrCodeValidation, http.StatusBadRequest, "请求参数错误", vErr.Error()))
			return
		}
		respondAPIError(w, WrapUpstreamError(err))
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

// listNodeWorkflowRuns handles GET /api/v1/nodes/{id}/workflow-runs
func (h *WorkflowHandler) listNodeWorkflowRuns(w http.ResponseWriter, r *http.Request, nodeID int64) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	params := service.ListWorkflowRunsParams{
		NodeID: &nodeID,
		Limit:  limit,
		Offset: offset,
	}

	if status := r.URL.Query().Get("status"); status != "" {
		params.Status = strings.Split(status, ",")
	}

	resp, err := h.workflowService.ListWorkflowRuns(r.Context(), params)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// listWorkflowRuns handles GET /api/v1/workflows/runs
func (h *WorkflowHandler) listWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	params := service.ListWorkflowRunsParams{
		Limit:  limit,
		Offset: offset,
	}

	if nodeIDStr := r.URL.Query().Get("node_id"); nodeIDStr != "" {
		nodeID, err := strconv.ParseInt(nodeIDStr, 10, 64)
		if err == nil {
			params.NodeID = &nodeID
		}
	}

	if documentIDStr := r.URL.Query().Get("document_id"); documentIDStr != "" {
		documentID, err := strconv.ParseInt(documentIDStr, 10, 64)
		if err == nil {
			params.DocumentID = &documentID
		}
	}

	if workflowKey := r.URL.Query().Get("workflow_key"); workflowKey != "" {
		params.WorkflowKey = &workflowKey
	}

	if status := r.URL.Query().Get("status"); status != "" {
		params.Status = strings.Split(status, ",")
	}

	resp, err := h.workflowService.ListWorkflowRuns(r.Context(), params)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// getWorkflowRun handles GET /api/v1/workflows/runs/{runId}
func (h *WorkflowHandler) getWorkflowRun(w http.ResponseWriter, r *http.Request, runID uint) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}

	run, err := h.workflowService.GetWorkflowRun(r.Context(), runID)
	if err != nil {
		respondError(w, http.StatusNotFound, err)
		return
	}

	writeJSON(w, http.StatusOK, run)
}

// cancelWorkflowRun handles POST /api/v1/workflows/runs/{runId}/cancel
func (h *WorkflowHandler) cancelWorkflowRun(w http.ResponseWriter, r *http.Request, runID uint) {
	err := h.workflowService.CancelWorkflowRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, service.ErrWorkflowRunNotFound) {
			respondError(w, http.StatusNotFound, err)
			return
		}
		var vErr *service.ValidationError
		if errors.As(err, &vErr) {
			respondAPIError(w, NewAPIError(ErrCodeValidation, http.StatusBadRequest, "取消失败", vErr.Error()))
			return
		}
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "工作流已取消",
	})
}

// handleCallback handles POST /api/v1/workflows/callback/{runId}
func (h *WorkflowHandler) handleCallback(w http.ResponseWriter, r *http.Request, runID uint) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}

	var callback service.WorkflowCallbackRequest
	if err := json.NewDecoder(r.Body).Decode(&callback); err != nil {
		respondAPIError(w, NewAPIError(ErrCodeValidation, http.StatusBadRequest, "请求格式错误", err.Error()))
		return
	}

	if err := h.workflowService.HandleCallback(r.Context(), runID, callback); err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DocumentWorkflowRoutes handles /api/v1/documents/{id}/workflows/* routes.
func (h *WorkflowHandler) DocumentWorkflowRoutes(w http.ResponseWriter, r *http.Request, docID int64, subPath string) {
	meta := h.handler.metaFromRequest(r)

	// GET /api/v1/documents/{id}/workflows - list available workflows for document
	if subPath == "" && r.Method == http.MethodGet {
		h.listDocumentWorkflows(w, r)
		return
	}

	// GET /api/v1/documents/{id}/workflow-runs - list workflow runs for document
	if subPath == "-runs" && r.Method == http.MethodGet {
		h.listDocumentWorkflowRuns(w, r, docID)
		return
	}

	// POST /api/v1/documents/{id}/workflows/{workflowKey}/runs - trigger workflow
	parts := strings.Split(subPath, "/")
	if len(parts) == 2 && parts[1] == "runs" && r.Method == http.MethodPost {
		workflowKey := parts[0]
		h.triggerDocumentWorkflow(w, r, meta, docID, workflowKey)
		return
	}

	respondError(w, http.StatusNotFound, errors.New("not found"))
}

// listDocumentWorkflows handles GET /api/v1/documents/{id}/workflows
func (h *WorkflowHandler) listDocumentWorkflows(w http.ResponseWriter, r *http.Request) {
	// 返回所有可用的文档工作流定义
	definitions, err := h.workflowService.ListWorkflowDefinitionsByType(r.Context(), "document")
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, definitions)
}

// triggerDocumentWorkflow handles POST /api/v1/documents/{id}/workflows/{workflowKey}/runs
func (h *WorkflowHandler) triggerDocumentWorkflow(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, docID int64, workflowKey string) {
	var params struct {
		Parameters map[string]interface{} `json:"parameters"`
		RetryOfID  *uint                  `json:"retry_of_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil && err.Error() != "EOF" {
		respondAPIError(w, NewAPIError(ErrCodeValidation, http.StatusBadRequest, "请求格式错误", err.Error()))
		return
	}

	req := service.TriggerDocumentWorkflowRequest{
		DocumentID:  docID,
		WorkflowKey: workflowKey,
		Parameters:  params.Parameters,
		RetryOfID:   params.RetryOfID,
	}

	resp, err := h.workflowService.TriggerDocumentWorkflow(r.Context(), meta, req)
	if err != nil {
		var vErr *service.ValidationError
		if errors.As(err, &vErr) {
			respondAPIError(w, NewAPIError(ErrCodeValidation, http.StatusBadRequest, "请求参数错误", vErr.Error()))
			return
		}
		respondAPIError(w, WrapUpstreamError(err))
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

// listDocumentWorkflowRuns handles GET /api/v1/documents/{id}/workflow-runs
func (h *WorkflowHandler) listDocumentWorkflowRuns(w http.ResponseWriter, r *http.Request, docID int64) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	params := service.ListWorkflowRunsParams{
		DocumentID: &docID,
		Limit:      limit,
		Offset:     offset,
	}

	if status := r.URL.Query().Get("status"); status != "" {
		params.Status = strings.Split(status, ",")
	}

	resp, err := h.workflowService.ListWorkflowRuns(r.Context(), params)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
