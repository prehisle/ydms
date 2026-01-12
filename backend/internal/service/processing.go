// Package service provides business logic for the YDMS backend.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/yjxt/ydms/backend/internal/database"
	"github.com/yjxt/ydms/backend/internal/ndrclient"
	"github.com/yjxt/ydms/backend/internal/prefectclient"
)

// Pipeline names
const (
	PipelineGenerateKnowledgeOverview = "generate_knowledge_overview"
	PipelinePolishDocument            = "polish_document"
)

// Job status constants
const (
	JobStatusPending   = "pending"
	JobStatusRunning   = "running"
	JobStatusCompleted = "completed"
	JobStatusFailed    = "failed"
	JobStatusCancelled = "cancelled"
)

// ProcessingService handles AI processing tasks.
type ProcessingService struct {
	db             *gorm.DB
	prefect        *prefectclient.Client
	ndr            ndrclient.Client
	pdmsBaseURL    string // PDMS base URL for callback
	prefectEnabled bool   // Whether Prefect integration is enabled
}

// NewProcessingService creates a new ProcessingService.
func NewProcessingService(db *gorm.DB, prefect *prefectclient.Client, ndr ndrclient.Client, pdmsBaseURL string) *ProcessingService {
	return &ProcessingService{
		db:             db,
		prefect:        prefect,
		ndr:            ndr,
		pdmsBaseURL:    pdmsBaseURL,
		prefectEnabled: prefect != nil,
	}
}

// TriggerPipelineRequest represents a request to trigger a pipeline.
type TriggerPipelineRequest struct {
	DocumentID   int64                  `json:"document_id"`
	PipelineName string                 `json:"pipeline_name"`
	DryRun       bool                   `json:"dry_run"`
	Params       map[string]interface{} `json:"params,omitempty"`
}

// TriggerPipelineResponse represents the response after triggering a pipeline.
type TriggerPipelineResponse struct {
	JobID            uint   `json:"job_id"`
	Status           string `json:"status"`
	IdempotencyKey   string `json:"idempotency_key"`
	PrefectFlowRunID string `json:"prefect_flow_run_id,omitempty"`
	Message          string `json:"message,omitempty"`
}

// CallbackRequest represents a callback from IDPP.
type CallbackRequest struct {
	Status       string                 `json:"status"`
	Progress     int                    `json:"progress"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Result       map[string]interface{} `json:"result,omitempty"`
}

// PipelineInfo describes a pipeline.
type PipelineInfo struct {
	Name               string   `json:"name"`
	Label              string   `json:"label"`
	Description        string   `json:"description"`
	DeploymentName     string   `json:"deployment_name"`
	SupportsDryRun     bool     `json:"supports_dry_run"`
	DocTypes           []string `json:"doc_types"`
	RequiresReferences bool     `json:"requires_references"`
}

// TriggerPipeline triggers an AI processing pipeline.
func (s *ProcessingService) TriggerPipeline(
	ctx context.Context,
	meta RequestMeta,
	req TriggerPipelineRequest,
) (*TriggerPipelineResponse, error) {
	// 1. Validate pipeline name
	if !isValidPipeline(req.PipelineName) {
		return nil, fmt.Errorf("invalid pipeline name: %s", req.PipelineName)
	}

	// 2. Get document info and version
	doc, err := s.ndr.GetDocument(ctx, toNDRMeta(meta), req.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	docVersion := 1
	if doc.Version != nil {
		docVersion = *doc.Version
	}

	// 3. Generate idempotency key
	idempotencyKey := generateIdempotencyKey(req.DocumentID, docVersion, req.PipelineName)

	// 4. Check for existing job
	var existingJob database.ProcessingJob
	if err := s.db.Where("idempotency_key = ?", idempotencyKey).First(&existingJob).Error; err == nil {
		// Job exists
		if existingJob.Status == JobStatusPending || existingJob.Status == JobStatusRunning {
			return &TriggerPipelineResponse{
				JobID:          existingJob.ID,
				Status:         existingJob.Status,
				IdempotencyKey: idempotencyKey,
				Message:        "任务已在处理中",
			}, nil
		}
		// Job is completed or failed - delete old record to allow re-trigger
		if existingJob.Status == JobStatusCompleted || existingJob.Status == JobStatusFailed {
			s.db.Delete(&existingJob)
		}
	}

	// 5. Create job record
	pipelineParams := database.JSONMap{}
	if req.Params != nil {
		pipelineParams = database.JSONMap(req.Params)
	}
	job := database.ProcessingJob{
		DocumentID:      req.DocumentID,
		DocumentVersion: docVersion,
		DocumentTitle:   doc.Title,
		PipelineName:    req.PipelineName,
		PipelineParams:  pipelineParams,
		Status:          JobStatusPending,
		IdempotencyKey:  idempotencyKey,
		TriggeredByID:   &meta.UserIDNumeric,
		DryRun:          req.DryRun,
	}

	if err := s.db.Create(&job).Error; err != nil {
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	// 6. If Prefect is not enabled, just return the job
	if !s.prefectEnabled {
		return &TriggerPipelineResponse{
			JobID:          job.ID,
			Status:         JobStatusPending,
			IdempotencyKey: idempotencyKey,
			Message:        "任务已创建（Prefect 未配置）",
		}, nil
	}

	// 7. Find Prefect deployment
	deploymentName := fmt.Sprintf("%s-deployment", req.PipelineName)
	deployment, err := s.prefect.GetDeploymentByName(ctx, req.PipelineName, deploymentName)
	if err != nil {
		// Update job status to failed
		s.db.Model(&job).Updates(map[string]interface{}{
			"status":        JobStatusFailed,
			"error_message": fmt.Sprintf("Deployment not found: %s", err.Error()),
		})
		return nil, fmt.Errorf("failed to find deployment: %w", err)
	}

	// 8. Build pipeline parameters with reserved key protection
	callbackURL := fmt.Sprintf("%s/api/v1/processing/callback/%d", s.pdmsBaseURL, job.ID)
	flowParams := map[string]interface{}{
		"doc_path":     fmt.Sprintf("@doc:%d", req.DocumentID),
		"dry_run":      req.DryRun,
		"callback_url": callbackURL,
	}

	// Reserved keys that cannot be overridden by user params
	reservedKeys := map[string]bool{
		"doc_path":      true,
		"dry_run":       true,
		"callback_url":  true,
		"pdms_base_url": true,
		"api_key":       true,
		"llm_base_url":  true,
	}

	// Only allow non-reserved keys from user params
	for k, v := range req.Params {
		if !reservedKeys[k] {
			flowParams[k] = v
		}
	}

	// 9. Create flow run
	flowRun, err := s.prefect.CreateFlowRun(ctx, deployment.ID, flowParams)
	if err != nil {
		s.db.Model(&job).Updates(map[string]interface{}{
			"status":        JobStatusFailed,
			"error_message": fmt.Sprintf("Failed to create flow run: %s", err.Error()),
		})
		return nil, fmt.Errorf("failed to create flow run: %w", err)
	}

	// 10. Update job record
	now := time.Now()
	s.db.Model(&job).Updates(map[string]interface{}{
		"prefect_deployment_id": deployment.ID,
		"prefect_flow_run_id":   flowRun.ID,
		"status":                JobStatusRunning,
		"started_at":            &now,
	})

	return &TriggerPipelineResponse{
		JobID:            job.ID,
		Status:           JobStatusRunning,
		IdempotencyKey:   idempotencyKey,
		PrefectFlowRunID: flowRun.ID,
		Message:          "任务已提交",
	}, nil
}

// GetJob retrieves a job by ID.
func (s *ProcessingService) GetJob(ctx context.Context, jobID uint) (*database.ProcessingJob, error) {
	var job database.ProcessingJob
	if err := s.db.Preload("TriggeredBy").First(&job, jobID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("job not found")
		}
		return nil, err
	}
	return &job, nil
}

// ListJobs lists processing jobs for a document.
func (s *ProcessingService) ListJobs(ctx context.Context, documentID int64, limit int) ([]database.ProcessingJob, error) {
	var jobs []database.ProcessingJob
	query := s.db.Where("document_id = ?", documentID).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Find(&jobs).Error; err != nil {
		return nil, err
	}
	return jobs, nil
}

// HandleCallback handles a callback from IDPP.
func (s *ProcessingService) HandleCallback(ctx context.Context, jobID uint, callback CallbackRequest) error {
	var job database.ProcessingJob
	if err := s.db.First(&job, jobID).Error; err != nil {
		return err
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":   callback.Status,
		"progress": callback.Progress,
	}

	if callback.Status == JobStatusCompleted || callback.Status == JobStatusFailed {
		updates["completed_at"] = &now
	}

	if callback.Status == JobStatusFailed && callback.ErrorMessage != "" {
		updates["error_message"] = callback.ErrorMessage
	}

	if callback.Result != nil {
		updates["result"] = database.JSONMap(callback.Result)
	}

	return s.db.Model(&job).Updates(updates).Error
}

// ListPipelines returns available pipelines.
func (s *ProcessingService) ListPipelines() []PipelineInfo {
	return []PipelineInfo{
		{
			Name:               PipelineGenerateKnowledgeOverview,
			Label:              "生成知识概览",
			Description:        "基于文档引用关系，调用 AI 生成 HTML 格式的知识点学习资料",
			DeploymentName:     "generate_knowledge_overview-deployment",
			SupportsDryRun:     true,
			DocTypes:           []string{"markdown_v1"},
			RequiresReferences: true,
		},
		{
			Name:               PipelinePolishDocument,
			Label:              "文档润色",
			Description:        "使用 AI 对文档内容进行润色优化",
			DeploymentName:     "polish_document-deployment",
			SupportsDryRun:     true,
			DocTypes:           []string{"markdown_v1"},
			RequiresReferences: false,
		},
	}
}

// Helper functions

func isValidPipeline(name string) bool {
	validPipelines := map[string]bool{
		PipelineGenerateKnowledgeOverview: true,
		PipelinePolishDocument:            true,
	}
	return validPipelines[name]
}

func generateIdempotencyKey(docID int64, version int, pipeline string) string {
	data := fmt.Sprintf("%d:%d:%s", docID, version, pipeline)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:16])
}
