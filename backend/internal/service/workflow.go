// Package service provides business logic for the YDMS backend.
package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/yjxt/ydms/backend/internal/database"
	"github.com/yjxt/ydms/backend/internal/ndrclient"
	"github.com/yjxt/ydms/backend/internal/prefectclient"
)

// WorkflowRun status constants
const (
	WorkflowStatusPending   = "pending"
	WorkflowStatusRunning   = "running"
	WorkflowStatusSuccess   = "success"
	WorkflowStatusFailed    = "failed"
	WorkflowStatusCancelled = "cancelled"
)

// ZombieTaskTimeout 僵尸任务超时时间（超过此时间的 running 任务可被强制终止）
const ZombieTaskTimeout = 30 * time.Minute

// ErrWorkflowRunNotFound is returned when a workflow run is not found.
var ErrWorkflowRunNotFound = errors.New("workflow run not found")

// ValidationError is used for request validation failures that should map to HTTP 400.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func newValidationError(format string, args ...interface{}) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// WorkflowService handles node workflow operations.
type WorkflowService struct {
	db             *gorm.DB
	prefect        *prefectclient.Client
	ndr            ndrclient.Client
	pdmsBaseURL    string
	prefectEnabled bool
}

// NewWorkflowService creates a new WorkflowService.
func NewWorkflowService(db *gorm.DB, prefect *prefectclient.Client, ndr ndrclient.Client, pdmsBaseURL string) *WorkflowService {
	return &WorkflowService{
		db:             db,
		prefect:        prefect,
		ndr:            ndr,
		pdmsBaseURL:    pdmsBaseURL,
		prefectEnabled: prefect != nil,
	}
}

// WorkflowDefinitionInfo represents workflow definition for API responses.
type WorkflowDefinitionInfo struct {
	ID              uint                   `json:"id"`
	WorkflowKey     string                 `json:"workflow_key"`
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	ParameterSchema map[string]interface{} `json:"parameter_schema"`
	Enabled         bool                   `json:"enabled"`
}

// ListWorkflowDefinitions returns all enabled workflow definitions.
func (s *WorkflowService) ListWorkflowDefinitions(ctx context.Context) ([]WorkflowDefinitionInfo, error) {
	var definitions []database.WorkflowDefinition
	if err := s.db.Where("enabled = ?", true).Find(&definitions).Error; err != nil {
		return nil, fmt.Errorf("failed to list workflow definitions: %w", err)
	}

	result := make([]WorkflowDefinitionInfo, len(definitions))
	for i, def := range definitions {
		result[i] = WorkflowDefinitionInfo{
			ID:              def.ID,
			WorkflowKey:     def.WorkflowKey,
			Name:            def.Name,
			Description:     def.Description,
			ParameterSchema: def.ParameterSchema,
			Enabled:         def.Enabled,
		}
	}
	return result, nil
}

// ListWorkflowDefinitionsByType returns enabled workflow definitions filtered by type.
func (s *WorkflowService) ListWorkflowDefinitionsByType(ctx context.Context, workflowType string) ([]WorkflowDefinitionInfo, error) {
	var definitions []database.WorkflowDefinition
	// 兼容空的 sync_status（手动创建的工作流可能没有 sync_status）
	query := s.db.Where("enabled = ? AND (sync_status = ? OR sync_status = '' OR sync_status IS NULL)", true, "active")
	if workflowType != "" {
		query = query.Where("workflow_type = ?", workflowType)
	}
	if err := query.Find(&definitions).Error; err != nil {
		return nil, fmt.Errorf("failed to list workflow definitions: %w", err)
	}

	result := make([]WorkflowDefinitionInfo, len(definitions))
	for i, def := range definitions {
		result[i] = WorkflowDefinitionInfo{
			ID:              def.ID,
			WorkflowKey:     def.WorkflowKey,
			Name:            def.Name,
			Description:     def.Description,
			ParameterSchema: def.ParameterSchema,
			Enabled:         def.Enabled,
		}
	}
	return result, nil
}

// GetWorkflowDefinition retrieves a single workflow definition by key.
func (s *WorkflowService) GetWorkflowDefinition(ctx context.Context, workflowKey string) (*database.WorkflowDefinition, error) {
	var def database.WorkflowDefinition
	if err := s.db.Where("workflow_key = ? AND enabled = ?", workflowKey, true).First(&def).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("workflow not found: %s", workflowKey)
		}
		return nil, err
	}
	return &def, nil
}

// TriggerWorkflowRequest represents a request to trigger a workflow on a node.
type TriggerWorkflowRequest struct {
	NodeID       int64                  `json:"node_id"`
	WorkflowKey  string                 `json:"workflow_key"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
	SourceDocIDs []int64                `json:"-"`                      // 预获取的源文档 ID（内部使用，跳过重复查询）
	RetryOfID    *uint                  `json:"retry_of_id,omitempty"`  // 重试来源任务 ID
}

// TriggerDocumentWorkflowRequest represents a request to trigger a workflow on a document.
type TriggerDocumentWorkflowRequest struct {
	DocumentID  int64                  `json:"document_id"`
	WorkflowKey string                 `json:"workflow_key"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	RetryOfID   *uint                  `json:"retry_of_id,omitempty"` // 重试来源任务 ID
}

// TriggerWorkflowResponse represents the response after triggering a workflow.
type TriggerWorkflowResponse struct {
	RunID            uint   `json:"run_id"`
	Status           string `json:"status"`
	PrefectFlowRunID string `json:"prefect_flow_run_id,omitempty"`
	Message          string `json:"message,omitempty"`
}

// validateRetryOf validates the retry_of_id parameter.
func (s *WorkflowService) validateRetryOf(
	ctx context.Context,
	retryOfID *uint,
	workflowKey string,
	nodeID *int64,
	documentID *int64,
) error {
	if retryOfID == nil {
		return nil
	}
	if *retryOfID == 0 {
		return newValidationError("retry_of_id must be a positive integer")
	}

	var src database.WorkflowRun
	if err := s.db.First(&src, *retryOfID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return newValidationError("retry_of_id refers to a non-existent workflow run (%d)", *retryOfID)
		}
		return err
	}
	if src.WorkflowKey != workflowKey {
		return newValidationError("retry_of_id (%d) workflow_key mismatch", *retryOfID)
	}
	if nodeID != nil && (src.NodeID == nil || *src.NodeID != *nodeID) {
		return newValidationError("retry_of_id (%d) node_id mismatch", *retryOfID)
	}
	if documentID != nil && (src.DocumentID == nil || *src.DocumentID != *documentID) {
		return newValidationError("retry_of_id (%d) document_id mismatch", *retryOfID)
	}
	return nil
}

// TriggerWorkflow triggers a workflow on a node.
func (s *WorkflowService) TriggerWorkflow(
	ctx context.Context,
	meta RequestMeta,
	req TriggerWorkflowRequest,
) (*TriggerWorkflowResponse, error) {
	// 1. Validate workflow exists
	def, err := s.GetWorkflowDefinition(ctx, req.WorkflowKey)
	if err != nil {
		return nil, err
	}

	// 检查 sync_status（非活跃的同步工作流不允许触发）
	if def.SyncStatus != "" && def.SyncStatus != "active" {
		return nil, fmt.Errorf("workflow %s is not active (status=%s)", req.WorkflowKey, def.SyncStatus)
	}

	// 检查 workflow_type（防止通过节点端点触发文档工作流）
	if def.WorkflowType != "" && def.WorkflowType != "node" {
		return nil, fmt.Errorf("workflow %s is not a node workflow", req.WorkflowKey)
	}

	// 校验 retry_of_id 参数
	if err := s.validateRetryOf(ctx, req.RetryOfID, req.WorkflowKey, &req.NodeID, nil); err != nil {
		return nil, err
	}

	// 2. Verify node exists and get source documents
	var sourceDocIDs []int64

	if len(req.SourceDocIDs) > 0 {
		// 使用预获取的源文档 ID（批量执行优化）
		sourceDocIDs = req.SourceDocIDs
	} else {
		// 查询源文档
		sources, err := s.ndr.ListSourceDocuments(ctx, toNDRMeta(meta), req.NodeID)
		if err != nil {
			return nil, fmt.Errorf("failed to get source documents: %w", err)
		}
		sourceDocIDs = make([]int64, len(sources))
		for i, src := range sources {
			sourceDocIDs[i] = src.DocumentID
		}
	}

	// 3. Get target documents (node's direct documents) for generate_node_documents workflow
	var targetDocs []map[string]interface{}
	needsTargetDocs := strings.HasPrefix(req.WorkflowKey, "generate_node_documents")
	if needsTargetDocs {
		query := url.Values{}
		query.Set("include_descendants", "false")
		query.Set("size", "100")
		docsPage, err := s.ndr.ListNodeDocuments(ctx, toNDRMeta(meta), req.NodeID, query)
		if err != nil {
			return nil, fmt.Errorf("failed to get target documents: %w", err)
		}

		// Build source document ID set for filtering
		sourceDocIDSet := make(map[int64]bool)
		for _, id := range sourceDocIDs {
			sourceDocIDSet[id] = true
		}

		// Filter out source documents from target docs
		for _, doc := range docsPage.Items {
			// Skip source documents - they are inputs, not outputs
			if sourceDocIDSet[doc.ID] {
				continue
			}
			docType := ""
			if doc.Type != nil {
				docType = *doc.Type
			}
			targetDocs = append(targetDocs, map[string]interface{}{
				"document_id": doc.ID,
				"title":       doc.Title,
				"type":        docType,
			})
		}
	}

	// 4. Create workflow run record
	params := database.JSONMap{}
	if req.Parameters != nil {
		params = database.JSONMap(req.Parameters)
	}

	nodeID := req.NodeID
	run := database.WorkflowRun{
		WorkflowKey: req.WorkflowKey,
		NodeID:      &nodeID,
		Parameters:  params,
		Status:      WorkflowStatusPending,
		CreatedByID: &meta.UserIDNumeric,
		RetryOfID:   req.RetryOfID,
	}

	if err := s.db.Create(&run).Error; err != nil {
		return nil, fmt.Errorf("failed to create workflow run: %w", err)
	}

	// 5. If Prefect is not enabled, return pending status
	if !s.prefectEnabled {
		return &TriggerWorkflowResponse{
			RunID:   run.ID,
			Status:  WorkflowStatusPending,
			Message: "工作流已创建（Prefect 未配置）",
		}, nil
	}

	// 6. Find Prefect deployment
	deployment, err := s.prefect.GetDeploymentByName(ctx, req.WorkflowKey, def.PrefectDeploymentName)
	if err != nil {
		s.db.Model(&run).Updates(map[string]interface{}{
			"status":        WorkflowStatusFailed,
			"error_message": fmt.Sprintf("Deployment not found: %s", err.Error()),
		})
		return nil, fmt.Errorf("failed to find deployment: %w", err)
	}

	// 7. Build flow parameters
	callbackURL := fmt.Sprintf("%s/api/v1/workflows/callback/%d", s.pdmsBaseURL, run.ID)

	flowParams := map[string]interface{}{
		"run_id":         run.ID,
		"node_id":        req.NodeID,
		"workflow_key":   req.WorkflowKey,
		"source_doc_ids": sourceDocIDs,
		"callback_url":   callbackURL,
		"pdms_base_url":  s.pdmsBaseURL,
	}

	// Add target_docs for generate_node_documents workflow
	if len(targetDocs) > 0 {
		flowParams["target_docs"] = targetDocs
	}

	// Add user parameters (with reserved key protection)
	reservedKeys := map[string]bool{
		"run_id":         true,
		"node_id":        true,
		"workflow_key":   true,
		"source_doc_ids": true,
		"callback_url":   true,
		"pdms_base_url":  true,
		"target_docs":    true,
	}
	for k, v := range req.Parameters {
		if !reservedKeys[k] {
			flowParams[k] = v
		}
	}

	// 8. Create flow run
	flowRun, err := s.prefect.CreateFlowRun(ctx, deployment.ID, flowParams)
	if err != nil {
		s.db.Model(&run).Updates(map[string]interface{}{
			"status":        WorkflowStatusFailed,
			"error_message": fmt.Sprintf("Failed to create flow run: %s", err.Error()),
		})
		return nil, fmt.Errorf("failed to create flow run: %w", err)
	}

	// 9. Update run record
	now := time.Now()
	s.db.Model(&run).Updates(map[string]interface{}{
		"prefect_flow_run_id": flowRun.ID,
		"status":              WorkflowStatusRunning,
		"started_at":          &now,
	})

	return &TriggerWorkflowResponse{
		RunID:            run.ID,
		Status:           WorkflowStatusRunning,
		PrefectFlowRunID: flowRun.ID,
		Message:          "工作流已提交",
	}, nil
}

// TriggerDocumentWorkflow triggers a workflow on a document.
func (s *WorkflowService) TriggerDocumentWorkflow(
	ctx context.Context,
	meta RequestMeta,
	req TriggerDocumentWorkflowRequest,
) (*TriggerWorkflowResponse, error) {
	// 1. Validate workflow exists and is a document workflow
	def, err := s.GetWorkflowDefinition(ctx, req.WorkflowKey)
	if err != nil {
		return nil, err
	}

	// 检查 sync_status（非活跃的同步工作流不允许触发）
	if def.SyncStatus != "" && def.SyncStatus != "active" {
		return nil, fmt.Errorf("workflow %s is not active (status=%s)", req.WorkflowKey, def.SyncStatus)
	}

	// Verify it's a document workflow
	if def.WorkflowType != "document" {
		return nil, fmt.Errorf("workflow %s is not a document workflow", req.WorkflowKey)
	}

	// 校验 retry_of_id 参数
	if err := s.validateRetryOf(ctx, req.RetryOfID, req.WorkflowKey, nil, &req.DocumentID); err != nil {
		return nil, err
	}

	// 2. Get document info from NDR
	doc, err := s.ndr.GetDocument(ctx, toNDRMeta(meta), req.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	// 3. Create workflow run record
	params := database.JSONMap{}
	if req.Parameters != nil {
		params = database.JSONMap(req.Parameters)
	}

	documentID := req.DocumentID
	run := database.WorkflowRun{
		WorkflowKey: req.WorkflowKey,
		DocumentID:  &documentID,
		Parameters:  params,
		Status:      WorkflowStatusPending,
		CreatedByID: &meta.UserIDNumeric,
		RetryOfID:   req.RetryOfID,
	}

	if err := s.db.Create(&run).Error; err != nil {
		return nil, fmt.Errorf("failed to create workflow run: %w", err)
	}

	// 4. If Prefect is not enabled, return pending status
	if !s.prefectEnabled {
		return &TriggerWorkflowResponse{
			RunID:   run.ID,
			Status:  WorkflowStatusPending,
			Message: "工作流已创建（Prefect 未配置）",
		}, nil
	}

	// 5. Find Prefect deployment
	deployment, err := s.prefect.GetDeploymentByName(ctx, req.WorkflowKey, def.PrefectDeploymentName)
	if err != nil {
		s.db.Model(&run).Updates(map[string]interface{}{
			"status":        WorkflowStatusFailed,
			"error_message": fmt.Sprintf("Deployment not found: %s", err.Error()),
		})
		return nil, fmt.Errorf("failed to find deployment: %w", err)
	}

	// 6. Build flow parameters
	callbackURL := fmt.Sprintf("%s/api/v1/workflows/callback/%d", s.pdmsBaseURL, run.ID)

	flowParams := map[string]interface{}{
		"run_id":        run.ID,
		"document_id":   req.DocumentID,
		"document_type": doc.Type,
		"workflow_key":  req.WorkflowKey,
		"callback_url":  callbackURL,
		"pdms_base_url": s.pdmsBaseURL,
	}

	// Add user parameters (with reserved key protection)
	reservedKeys := map[string]bool{
		"run_id":        true,
		"document_id":   true,
		"document_type": true,
		"workflow_key":  true,
		"callback_url":  true,
		"pdms_base_url": true,
	}
	for k, v := range req.Parameters {
		if !reservedKeys[k] {
			flowParams[k] = v
		}
	}

	// 7. Create flow run
	flowRun, err := s.prefect.CreateFlowRun(ctx, deployment.ID, flowParams)
	if err != nil {
		s.db.Model(&run).Updates(map[string]interface{}{
			"status":        WorkflowStatusFailed,
			"error_message": fmt.Sprintf("Failed to create flow run: %s", err.Error()),
		})
		return nil, fmt.Errorf("failed to create flow run: %w", err)
	}

	// 8. Update run record
	now := time.Now()
	s.db.Model(&run).Updates(map[string]interface{}{
		"prefect_flow_run_id": flowRun.ID,
		"status":              WorkflowStatusRunning,
		"started_at":          &now,
	})

	return &TriggerWorkflowResponse{
		RunID:            run.ID,
		Status:           WorkflowStatusRunning,
		PrefectFlowRunID: flowRun.ID,
		Message:          "工作流已提交",
	}, nil
}

// GetWorkflowRun retrieves a workflow run by ID.
func (s *WorkflowService) GetWorkflowRun(ctx context.Context, runID uint) (*database.WorkflowRun, error) {
	var run database.WorkflowRun
	if err := s.db.Preload("CreatedBy").First(&run, runID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWorkflowRunNotFound
		}
		return nil, err
	}
	return &run, nil
}

// CancelWorkflowRun cancels a workflow run (marks it as cancelled).
// This only updates the local database status; it does not cancel the Prefect flow run.
func (s *WorkflowService) CancelWorkflowRun(ctx context.Context, runID uint) error {
	now := time.Now()
	updates := map[string]interface{}{
		"status":      WorkflowStatusCancelled,
		"finished_at": now,
	}

	// 原子条件更新：只允许 pending/running -> cancelled，避免并发下"取消已完成任务"
	res := s.db.Model(&database.WorkflowRun{}).
		Where("id = ? AND status IN ?", runID, []string{WorkflowStatusPending, WorkflowStatusRunning}).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}

	// 区分：不存在 vs 状态不允许取消
	var run database.WorkflowRun
	if err := s.db.Select("id, status").First(&run, runID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrWorkflowRunNotFound
		}
		return err
	}
	return newValidationError("只能取消待执行或运行中的任务（当前状态: %s）", run.Status)
}

// ForceTerminateWorkflowRun 强制终止僵尸任务（运行超过 30 分钟的任务）
func (s *WorkflowService) ForceTerminateWorkflowRun(ctx context.Context, runID uint) error {
	// 先获取任务信息
	var run database.WorkflowRun
	if err := s.db.First(&run, runID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrWorkflowRunNotFound
		}
		return err
	}

	// 检查状态：只能终止 pending 或 running 的任务
	if run.Status != WorkflowStatusPending && run.Status != WorkflowStatusRunning {
		return newValidationError("只能终止待执行或运行中的任务（当前状态: %s）", run.Status)
	}

	// 检查是否为僵尸任务（运行超过 30 分钟）
	var startTime time.Time
	if run.StartedAt != nil {
		startTime = *run.StartedAt
	} else {
		startTime = run.CreatedAt
	}

	if time.Since(startTime) < ZombieTaskTimeout {
		return newValidationError("任务运行时间未超过 30 分钟，请使用普通取消功能")
	}

	// 强制终止
	now := time.Now()
	updates := map[string]interface{}{
		"status":        WorkflowStatusFailed,
		"error_message": fmt.Sprintf("任务被强制终止（运行时间超过 %v）", ZombieTaskTimeout),
		"finished_at":   &now,
	}

	return s.db.Model(&run).Updates(updates).Error
}

// ListWorkflowRunsParams parameters for listing workflow runs.
type ListWorkflowRunsParams struct {
	NodeID      *int64   // Filter by node ID
	DocumentID  *int64   // Filter by document ID
	WorkflowKey *string  // Filter by workflow key
	Status      []string // Filter by status
	Limit       int
	Offset      int
}

// WorkflowRunInfo extends WorkflowRun with retry count for API responses.
type WorkflowRunInfo struct {
	database.WorkflowRun
	RetryCount        int     `json:"retry_count"`                    // 被重试的次数
	LatestRetryStatus *string `json:"latest_retry_status,omitempty"`  // 最新重试的状态（用于闭环）
}

// ListWorkflowRunsResponse response for listing workflow runs.
type ListWorkflowRunsResponse struct {
	Runs    []WorkflowRunInfo `json:"runs"`
	Total   int64             `json:"total"`
	HasMore bool              `json:"has_more"`
}

// ListWorkflowRuns lists workflow runs with optional filters.
func (s *WorkflowService) ListWorkflowRuns(ctx context.Context, params ListWorkflowRunsParams) (*ListWorkflowRunsResponse, error) {
	query := s.db.Model(&database.WorkflowRun{})

	if params.NodeID != nil {
		query = query.Where("node_id = ?", *params.NodeID)
	}
	if params.DocumentID != nil {
		query = query.Where("document_id = ?", *params.DocumentID)
	}
	if params.WorkflowKey != nil {
		query = query.Where("workflow_key = ?", *params.WorkflowKey)
	}
	if len(params.Status) > 0 {
		query = query.Where("status IN ?", params.Status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	offset := params.Offset
	if offset < 0 {
		offset = 0
	}

	var runs []database.WorkflowRun
	if err := query.Preload("CreatedBy").Order("created_at DESC").Limit(limit).Offset(offset).Find(&runs).Error; err != nil {
		return nil, err
	}

	// 收集所有 run ID，批量查询重试次数
	runIDs := make([]uint, len(runs))
	for i, run := range runs {
		runIDs[i] = run.ID
	}

	// 查询每个任务被重试的次数
	var retryCounts []struct {
		RetryOfID  uint `gorm:"column:retry_of_id"`
		RetryCount int  `gorm:"column:retry_count"`
	}
	if len(runIDs) > 0 {
		if err := s.db.Model(&database.WorkflowRun{}).
			Select("retry_of_id, COUNT(*) as retry_count").
			Where("retry_of_id IN ?", runIDs).
			Group("retry_of_id").
			Scan(&retryCounts).Error; err != nil {
			return nil, err
		}
	}

	// 构建 retry_of_id -> retry_count 映射
	retryCountMap := make(map[uint]int)
	for _, rc := range retryCounts {
		retryCountMap[rc.RetryOfID] = rc.RetryCount
	}

	// 查询每个任务最新重试的状态（用于闭环显示）
	var latestRetries []struct {
		RetryOfID uint   `gorm:"column:retry_of_id"`
		Status    string `gorm:"column:status"`
	}
	if len(runIDs) > 0 {
		// 使用子查询找到每个原任务的最新重试记录的状态
		// 按 created_at DESC, id DESC 排序取第一条（id DESC 确保同时间戳时结果稳定）
		subQuery := s.db.Model(&database.WorkflowRun{}).
			Select("retry_of_id, status, ROW_NUMBER() OVER (PARTITION BY retry_of_id ORDER BY created_at DESC, id DESC) as rn").
			Where("retry_of_id IN ?", runIDs)

		if err := s.db.Table("(?) as sub", subQuery).
			Select("retry_of_id, status").
			Where("rn = 1").
			Scan(&latestRetries).Error; err != nil {
			return nil, err
		}
	}

	// 构建 retry_of_id -> latest_status 映射
	latestRetryStatusMap := make(map[uint]string)
	for _, lr := range latestRetries {
		latestRetryStatusMap[lr.RetryOfID] = lr.Status
	}

	// 组装结果
	result := make([]WorkflowRunInfo, len(runs))
	for i, run := range runs {
		info := WorkflowRunInfo{
			WorkflowRun: run,
			RetryCount:  retryCountMap[run.ID],
		}
		if status, ok := latestRetryStatusMap[run.ID]; ok {
			info.LatestRetryStatus = &status
		}
		result[i] = info
	}

	return &ListWorkflowRunsResponse{
		Runs:    result,
		Total:   total,
		HasMore: int64(offset+len(runs)) < total,
	}, nil
}

// WorkflowCallbackRequest represents a callback from IDPP workflow.
type WorkflowCallbackRequest struct {
	Status       string                 `json:"status"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Result       map[string]interface{} `json:"result,omitempty"`
}

// HandleCallback handles a callback from IDPP workflow.
func (s *WorkflowService) HandleCallback(ctx context.Context, runID uint, callback WorkflowCallbackRequest) error {
	var run database.WorkflowRun
	if err := s.db.First(&run, runID).Error; err != nil {
		return err
	}

	// 已取消的任务保持 cancelled，避免回调覆盖导致"取消不生效/状态跳变"
	if run.Status == WorkflowStatusCancelled {
		return nil
	}

	updates := map[string]interface{}{
		"status": callback.Status,
	}

	if callback.Status == WorkflowStatusSuccess || callback.Status == WorkflowStatusFailed {
		now := time.Now()
		updates["finished_at"] = &now
	}

	if callback.Status == WorkflowStatusFailed && callback.ErrorMessage != "" {
		updates["error_message"] = callback.ErrorMessage
	}

	if callback.Result != nil {
		updates["result"] = database.JSONMap(callback.Result)
	}

	return s.db.Model(&run).Updates(updates).Error
}

// EnsureDefaultWorkflows ensures default workflow definitions exist in the database.
func (s *WorkflowService) EnsureDefaultWorkflows(ctx context.Context) error {
	defaults := []database.WorkflowDefinition{
		{
			WorkflowKey:           "generate_node_documents",
			Name:                  "生成节点文档",
			Description:           "根据源文档，一次性生成节点下所有文档的内容",
			PrefectDeploymentName: "node-generate-documents-deployment",
			ParameterSchema:       database.JSONMap{},
			Enabled:               true,
		},
		{
			WorkflowKey:           "generate_node_documents_v2",
			Name:                  "生成节点文档(增强版)",
			Description:           "增强版文档生成：更完整的知识覆盖、高质量SVG结构图、详细的题目逐项解析",
			PrefectDeploymentName: "node-generate-documents-v2-deployment",
			ParameterSchema:       database.JSONMap{},
			Enabled:               true,
		},
		{
			WorkflowKey:           "generate_node_documents_v3",
			Name:                  "生成节点文档(V3)",
			Description:           "V3版本：新增规划阶段、灰度SVG、禁止emoji、必背融合到正文",
			PrefectDeploymentName: "node-generate-documents-v3-deployment",
			ParameterSchema:       database.JSONMap{},
			Enabled:               true,
		},
		{
			WorkflowKey:           "generate_node_documents_v4",
			Name:                  "生成节点文档(V4)",
			Description:           "V4版本：单轮生成、简化提示词、内嵌SVG、轻量验证，Token效率提升70%",
			PrefectDeploymentName: "node-generate-documents-v4-deployment",
			ParameterSchema:       database.JSONMap{},
			Enabled:               true,
		},
		{
			WorkflowKey:           "generate_node_documents_v5",
			Name:                  "生成节点文档(V5)",
			Description:           "V5版本：XML标签隔离、Recency Bias、SVG规则增强",
			PrefectDeploymentName: "node-generate-documents-v5-deployment",
			ParameterSchema:       database.JSONMap{},
			Enabled:               true,
		},
		{
			WorkflowKey:           "generate_node_documents_v6",
			Name:                  "生成节点文档(V6)",
			Description:           "V6版本：分步生成、SVG独立生成、错误隔离",
			PrefectDeploymentName: "node-generate-documents-v6-deployment",
			ParameterSchema:       database.JSONMap{},
			Enabled:               true,
		},
		{
			WorkflowKey:           "generate_node_documents_v7",
			Name:                  "生成节点文档(V7)",
			Description:           "V7版本：三步分离架构、LLM内容规划、并行处理、HTML+SVG注入",
			PrefectDeploymentName: "node-generate-documents-v7-deployment",
			ParameterSchema:       database.JSONMap{},
			Enabled:               true,
		},
		{
			WorkflowKey:           "generate_node_documents_exercises",
			Name:                  "生成章节练习",
			Description:           "章节练习：两阶段生成高质量选择题，题目具备深度和迷惑性",
			PrefectDeploymentName: "node-generate-exercises-deployment",
			ParameterSchema:       database.JSONMap{},
			Enabled:               true,
		},
		{
			WorkflowKey:           "generate_knowledge_overview",
			Name:                  "生成知识概览",
			Description:           "基于节点的源文档，调用 AI 生成 HTML 格式的知识点学习资料",
			PrefectDeploymentName: "node-generate-knowledge-overview-deployment",
			ParameterSchema: database.JSONMap{
				"type": "object",
				"properties": map[string]interface{}{
					"theme": map[string]interface{}{
						"type":        "string",
						"title":       "主题风格",
						"description": "知识概览的视觉主题",
						"enum":        []string{"classic_blue", "warm_sunrise", "night_immersion", "glass_morphism", "bamboo_ink"},
						"default":     "classic_blue",
					},
				},
			},
			Enabled: false, // 已由 generate_node_documents 替代
		},
		{
			WorkflowKey:           "generate_exercises",
			Name:                  "生成练习题",
			Description:           "基于节点的源文档，调用 AI 生成练习题",
			PrefectDeploymentName: "generate_exercises-deployment",
			ParameterSchema: database.JSONMap{
				"type": "object",
				"properties": map[string]interface{}{
					"count": map[string]interface{}{
						"type":        "integer",
						"title":       "题目数量",
						"description": "生成的练习题数量",
						"minimum":     1,
						"maximum":     20,
						"default":     5,
					},
					"difficulty": map[string]interface{}{
						"type":        "string",
						"title":       "难度",
						"description": "练习题难度级别",
						"enum":        []string{"easy", "medium", "hard"},
						"default":     "medium",
					},
				},
			},
			Enabled: false, // 已由 generate_node_documents 替代
		},
		{
			WorkflowKey:           "generate_xiaohongshu_cards",
			Name:                  "生成小红书卡片",
			Description:           "将知识内容转化为适合小红书传播的系列卡片（深色 Bento Grid 风格，3:4 比例）",
			PrefectDeploymentName: "node-generate-xiaohongshu-cards-deployment",
			ParameterSchema: database.JSONMap{
				"type": "object",
				"properties": map[string]interface{}{
					"course_name": map[string]interface{}{
						"type":        "string",
						"title":       "课程名称",
						"description": "显示在卡片上的课程名称",
						"default":     "系统架构设计",
					},
				},
				"required": []string{"course_name"},
			},
			Enabled: true,
		},
	}

	for _, def := range defaults {
		var existing database.WorkflowDefinition
		err := s.db.Where("workflow_key = ?", def.WorkflowKey).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := s.db.Create(&def).Error; err != nil {
				return fmt.Errorf("failed to create workflow definition %s: %w", def.WorkflowKey, err)
			}
		}
		// 注意：不再覆盖已存在工作流的 enabled 状态
		// 用户在管理界面的自定义设置应该被保留
	}

	return nil
}

// CleanupWorkflowRunsParams 清理执行历史的参数
type CleanupWorkflowRunsParams struct {
	BeforeDate         *time.Time // 清理此日期之前的记录
	Status             []string   // 只清理指定状态的记录（为空则清理所有终态）
	WorkflowKey        *string    // 只清理指定工作流的记录
	NodeID             *int64     // 只清理指定节点的记录
	DocumentID         *int64     // 只清理指定文档的记录
	IncludeZombie      bool       // 是否包含僵尸任务（运行超过 30 分钟的 pending/running）
	ForceCleanupActive bool       // 强制清理所有 pending/running 任务（不仅仅是僵尸任务）
	DryRun             bool       // 试运行，只返回将被删除的数量
}

// CleanupWorkflowRunsResponse 清理执行历史的响应
type CleanupWorkflowRunsResponse struct {
	DeletedCount int64 `json:"deleted_count"`
	ZombieCount  int64 `json:"zombie_count,omitempty"` // 清理的僵尸任务数量
	DryRun       bool  `json:"dry_run"`
}

// CleanupWorkflowRuns 清理执行历史记录
// 只清理已完成的任务（success, failed, cancelled），不清理 pending 和 running
// 如果 IncludeZombie=true，也会清理运行超过 30 分钟的僵尸任务
// 如果 ForceCleanupActive=true，会清理所有 pending/running 任务（不仅仅是僵尸任务）
func (s *WorkflowService) CleanupWorkflowRuns(ctx context.Context, params CleanupWorkflowRunsParams) (*CleanupWorkflowRunsResponse, error) {
	// 默认只清理终态记录
	allowedStatuses := params.Status
	if len(allowedStatuses) == 0 {
		allowedStatuses = []string{WorkflowStatusSuccess, WorkflowStatusFailed, WorkflowStatusCancelled}
	}

	// 验证状态值，确保不会清理 pending 或 running 的任务（除非启用了相关选项）
	hasActiveStatus := false
	for _, status := range allowedStatuses {
		if status == WorkflowStatusPending || status == WorkflowStatusRunning {
			hasActiveStatus = true
			if !params.IncludeZombie && !params.ForceCleanupActive {
				return nil, fmt.Errorf("cannot cleanup %s tasks without include_zombie=true or force_cleanup_active=true", status)
			}
		}
	}

	var totalDeleted int64
	var zombieDeleted int64
	var forceDeleted int64

	// 如果强制清理所有活跃任务
	if params.ForceCleanupActive && hasActiveStatus {
		forceQuery := s.db.WithContext(ctx).Model(&database.WorkflowRun{}).
			Where("status IN ?", []string{WorkflowStatusPending, WorkflowStatusRunning})

		if params.BeforeDate != nil {
			forceQuery = forceQuery.Where("created_at < ?", *params.BeforeDate)
		}
		if params.WorkflowKey != nil {
			forceQuery = forceQuery.Where("workflow_key = ?", *params.WorkflowKey)
		}
		if params.NodeID != nil {
			forceQuery = forceQuery.Where("node_id = ?", *params.NodeID)
		}
		if params.DocumentID != nil {
			forceQuery = forceQuery.Where("document_id = ?", *params.DocumentID)
		}

		if params.DryRun {
			if err := forceQuery.Count(&forceDeleted).Error; err != nil {
				return nil, fmt.Errorf("failed to count active tasks: %w", err)
			}
		} else {
			result := forceQuery.Delete(&database.WorkflowRun{})
			if result.Error != nil {
				return nil, fmt.Errorf("failed to delete active tasks: %w", result.Error)
			}
			forceDeleted = result.RowsAffected
		}

		// 从状态列表中移除 pending 和 running，后续只处理终态
		var filteredStatuses []string
		for _, status := range allowedStatuses {
			if status != WorkflowStatusPending && status != WorkflowStatusRunning {
				filteredStatuses = append(filteredStatuses, status)
			}
		}
		allowedStatuses = filteredStatuses
	} else if params.IncludeZombie && hasActiveStatus {
		// 如果只包含僵尸任务，处理僵尸任务
		zombieTimeout := time.Now().Add(-ZombieTaskTimeout)
		zombieQuery := s.db.WithContext(ctx).Model(&database.WorkflowRun{}).
			Where("status IN ?", []string{WorkflowStatusPending, WorkflowStatusRunning}).
			Where("COALESCE(started_at, created_at) < ?", zombieTimeout)

		if params.WorkflowKey != nil {
			zombieQuery = zombieQuery.Where("workflow_key = ?", *params.WorkflowKey)
		}
		if params.NodeID != nil {
			zombieQuery = zombieQuery.Where("node_id = ?", *params.NodeID)
		}
		if params.DocumentID != nil {
			zombieQuery = zombieQuery.Where("document_id = ?", *params.DocumentID)
		}

		if params.DryRun {
			if err := zombieQuery.Count(&zombieDeleted).Error; err != nil {
				return nil, fmt.Errorf("failed to count zombie tasks: %w", err)
			}
		} else {
			// 先将僵尸任务标记为失败
			now := time.Now()
			result := zombieQuery.Updates(map[string]interface{}{
				"status":        WorkflowStatusFailed,
				"error_message": fmt.Sprintf("任务被清理（运行时间超过 %v）", ZombieTaskTimeout),
				"finished_at":   &now,
			})
			if result.Error != nil {
				return nil, fmt.Errorf("failed to mark zombie tasks as failed: %w", result.Error)
			}
			zombieDeleted = result.RowsAffected
		}

		// 从状态列表中移除 pending 和 running，后续只处理终态
		var filteredStatuses []string
		for _, status := range allowedStatuses {
			if status != WorkflowStatusPending && status != WorkflowStatusRunning {
				filteredStatuses = append(filteredStatuses, status)
			}
		}
		allowedStatuses = filteredStatuses
	}

	// 处理终态任务
	if len(allowedStatuses) > 0 {
		query := s.db.WithContext(ctx).Model(&database.WorkflowRun{}).Where("status IN ?", allowedStatuses)

		if params.BeforeDate != nil {
			query = query.Where("created_at < ?", *params.BeforeDate)
		}
		if params.WorkflowKey != nil {
			query = query.Where("workflow_key = ?", *params.WorkflowKey)
		}
		if params.NodeID != nil {
			query = query.Where("node_id = ?", *params.NodeID)
		}
		if params.DocumentID != nil {
			query = query.Where("document_id = ?", *params.DocumentID)
		}

		if params.DryRun {
			var count int64
			if err := query.Count(&count).Error; err != nil {
				return nil, fmt.Errorf("failed to count workflow runs: %w", err)
			}
			totalDeleted = count + zombieDeleted + forceDeleted
		} else {
			result := query.Delete(&database.WorkflowRun{})
			if result.Error != nil {
				return nil, fmt.Errorf("failed to delete workflow runs: %w", result.Error)
			}
			totalDeleted = result.RowsAffected + zombieDeleted + forceDeleted
		}
	} else {
		totalDeleted = zombieDeleted + forceDeleted
	}

	return &CleanupWorkflowRunsResponse{
		DeletedCount: totalDeleted,
		ZombieCount:  zombieDeleted,
		DryRun:       params.DryRun,
	}, nil
}
