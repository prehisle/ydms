// Package service provides business logic for the YDMS backend.
package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
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
	NodeID      int64                  `json:"node_id"`
	WorkflowKey string                 `json:"workflow_key"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// TriggerDocumentWorkflowRequest represents a request to trigger a workflow on a document.
type TriggerDocumentWorkflowRequest struct {
	DocumentID  int64                  `json:"document_id"`
	WorkflowKey string                 `json:"workflow_key"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// TriggerWorkflowResponse represents the response after triggering a workflow.
type TriggerWorkflowResponse struct {
	RunID            uint   `json:"run_id"`
	Status           string `json:"status"`
	PrefectFlowRunID string `json:"prefect_flow_run_id,omitempty"`
	Message          string `json:"message,omitempty"`
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

	// 2. Verify node exists and get source documents
	sources, err := s.ndr.ListSourceDocuments(ctx, toNDRMeta(meta), req.NodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get source documents: %w", err)
	}

	// 3. Get target documents (node's direct documents) for generate_node_documents workflow
	var targetDocs []map[string]interface{}
	if req.WorkflowKey == "generate_node_documents" || req.WorkflowKey == "generate_node_documents_v2" || req.WorkflowKey == "generate_node_documents_v3" || req.WorkflowKey == "generate_node_documents_exercises" {
		query := url.Values{}
		query.Set("include_descendants", "false")
		query.Set("size", "100")
		docsPage, err := s.ndr.ListNodeDocuments(ctx, toNDRMeta(meta), req.NodeID, query)
		if err != nil {
			return nil, fmt.Errorf("failed to get target documents: %w", err)
		}

		// Build source document ID set for filtering
		sourceDocIDs := make(map[int64]bool)
		for _, src := range sources {
			sourceDocIDs[src.DocumentID] = true
		}

		// Filter out source documents from target docs
		for _, doc := range docsPage.Items {
			// Skip source documents - they are inputs, not outputs
			if sourceDocIDs[doc.ID] {
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

	// Collect source document IDs
	sourceDocIDs := make([]int64, len(sources))
	for i, src := range sources {
		sourceDocIDs[i] = src.DocumentID
	}

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
			return nil, errors.New("workflow run not found")
		}
		return nil, err
	}
	return &run, nil
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

// ListWorkflowRunsResponse response for listing workflow runs.
type ListWorkflowRunsResponse struct {
	Runs    []database.WorkflowRun `json:"runs"`
	Total   int64                  `json:"total"`
	HasMore bool                   `json:"has_more"`
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

	return &ListWorkflowRunsResponse{
		Runs:    runs,
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
	}

	for _, def := range defaults {
		var existing database.WorkflowDefinition
		err := s.db.Where("workflow_key = ?", def.WorkflowKey).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := s.db.Create(&def).Error; err != nil {
				return fmt.Errorf("failed to create workflow definition %s: %w", def.WorkflowKey, err)
			}
		} else if err == nil {
			// Update enabled status for existing records
			if existing.Enabled != def.Enabled {
				if err := s.db.Model(&existing).Update("enabled", def.Enabled).Error; err != nil {
					return fmt.Errorf("failed to update workflow definition %s: %w", def.WorkflowKey, err)
				}
			}
		}
	}

	return nil
}
