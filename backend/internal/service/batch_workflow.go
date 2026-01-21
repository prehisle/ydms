// Package service provides business logic for the YDMS backend.
package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/yjxt/ydms/backend/internal/database"
	"github.com/yjxt/ydms/backend/internal/ndrclient"
)

const (
	// DefaultBatchConcurrency 默认批量执行并发数
	// 设为 1 避免 Prefect Server (SQLite) 并发写入导致 503
	DefaultBatchConcurrency = 1
	// MaxBatchConcurrency 最大批量执行并发数
	MaxBatchConcurrency = 10
)

// BatchWorkflowService 批量工作流服务
type BatchWorkflowService struct {
	db              *gorm.DB
	ndr             ndrclient.Client
	workflowService *WorkflowService
}

// NewBatchWorkflowService 创建批量工作流服务
func NewBatchWorkflowService(db *gorm.DB, ndr ndrclient.Client, workflowSvc *WorkflowService) *BatchWorkflowService {
	return &BatchWorkflowService{
		db:              db,
		ndr:             ndr,
		workflowService: workflowSvc,
	}
}

// BatchWorkflowPreviewRequest 批量工作流预览请求
type BatchWorkflowPreviewRequest struct {
	WorkflowKey        string `json:"workflow_key"`
	IncludeDescendants bool   `json:"include_descendants"`
	SkipNoSource       bool   `json:"skip_no_source"` // 跳过没有源文档的节点
}

// NodePreviewItem 节点预览项
type NodePreviewItem struct {
	NodeID          int64  `json:"node_id"`
	NodeName        string `json:"node_name"`
	NodePath        string `json:"node_path"`
	SourceDocCount  int    `json:"source_doc_count"`
	CanExecute      bool   `json:"can_execute"`
	SkipReason      string `json:"skip_reason,omitempty"`
	Depth           int    `json:"depth"` // 节点深度（相对于起始节点）
}

// BatchWorkflowPreviewResponse 批量工作流预览响应
type BatchWorkflowPreviewResponse struct {
	RootNodeID   int64             `json:"root_node_id"`
	WorkflowKey  string            `json:"workflow_key"`
	WorkflowName string            `json:"workflow_name"`
	TotalNodes   int               `json:"total_nodes"`
	CanExecute   int               `json:"can_execute"`
	WillSkip     int               `json:"will_skip"`
	Nodes        []NodePreviewItem `json:"nodes"`
}

// BatchWorkflowExecuteRequest 批量工作流执行请求
type BatchWorkflowExecuteRequest struct {
	WorkflowKey        string                 `json:"workflow_key"`
	IncludeDescendants bool                   `json:"include_descendants"`
	SkipNoSource       bool                   `json:"skip_no_source"`
	Parameters         map[string]interface{} `json:"parameters,omitempty"`
	Concurrency        int                    `json:"concurrency,omitempty"` // 并发数，默认 3
}

// BatchWorkflowExecuteResponse 批量工作流执行响应
type BatchWorkflowExecuteResponse struct {
	BatchID     string `json:"batch_id"`
	Status      string `json:"status"`
	TotalNodes  int    `json:"total_nodes"`
	Message     string `json:"message,omitempty"`
}

// BatchWorkflowStatusResponse 批量工作流状态响应
type BatchWorkflowStatusResponse struct {
	BatchID      string                 `json:"batch_id"`
	WorkflowKey  string                 `json:"workflow_key"`
	RootNodeID   int64                  `json:"root_node_id"`
	Status       string                 `json:"status"`
	TotalNodes   int                    `json:"total_nodes"`
	SuccessCount int                    `json:"success_count"`
	FailedCount  int                    `json:"failed_count"`
	SkippedCount int                    `json:"skipped_count"`
	Progress     float64                `json:"progress"` // 0-100
	Details      map[string]interface{} `json:"details,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	StartedAt    *time.Time             `json:"started_at,omitempty"`
	FinishedAt   *time.Time             `json:"finished_at,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
}

// PreviewBatchWorkflow 预览批量工作流
func (s *BatchWorkflowService) PreviewBatchWorkflow(
	ctx context.Context,
	meta RequestMeta,
	nodeID int64,
	req BatchWorkflowPreviewRequest,
) (*BatchWorkflowPreviewResponse, error) {
	// 1. 验证工作流存在
	def, err := s.workflowService.GetWorkflowDefinition(ctx, req.WorkflowKey)
	if err != nil {
		return nil, fmt.Errorf("invalid workflow: %w", err)
	}

	// 2. 收集所有目标节点
	nodes, err := s.collectNodes(ctx, meta, nodeID, req.IncludeDescendants, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to collect nodes: %w", err)
	}

	// 3. 检查每个节点的状态
	previewItems := make([]NodePreviewItem, 0, len(nodes))
	canExecuteCount := 0
	willSkipCount := 0

	for _, node := range nodes {
		item := NodePreviewItem{
			NodeID:   node.ID,
			NodeName: node.Name,
			NodePath: node.Path,
			Depth:    node.Depth,
		}

		// 检查源文档
		sources, err := s.ndr.ListSourceDocuments(ctx, toNDRMeta(meta), node.ID)
		if err != nil {
			item.CanExecute = false
			item.SkipReason = fmt.Sprintf("获取源文档失败: %v", err)
			willSkipCount++
		} else {
			item.SourceDocCount = len(sources)
			if len(sources) == 0 {
				if req.SkipNoSource {
					item.CanExecute = false
					item.SkipReason = "无源文档"
					willSkipCount++
				} else {
					item.CanExecute = true
					canExecuteCount++
				}
			} else {
				item.CanExecute = true
				canExecuteCount++
			}
		}

		previewItems = append(previewItems, item)
	}

	return &BatchWorkflowPreviewResponse{
		RootNodeID:   nodeID,
		WorkflowKey:  req.WorkflowKey,
		WorkflowName: def.Name,
		TotalNodes:   len(nodes),
		CanExecute:   canExecuteCount,
		WillSkip:     willSkipCount,
		Nodes:        previewItems,
	}, nil
}

// nodeWithDepth 带深度信息的节点
type nodeWithDepth struct {
	ndrclient.Node
	Depth int
}

// collectNodes 递归收集节点（包括子孙节点）
func (s *BatchWorkflowService) collectNodes(
	ctx context.Context,
	meta RequestMeta,
	nodeID int64,
	includeDescendants bool,
	depth int,
) ([]nodeWithDepth, error) {
	// 获取当前节点
	node, err := s.ndr.GetNode(ctx, toNDRMeta(meta), nodeID, ndrclient.GetNodeOptions{})
	if err != nil {
		return nil, fmt.Errorf("get node %d: %w", nodeID, err)
	}

	result := []nodeWithDepth{{Node: node, Depth: depth}}

	if !includeDescendants {
		return result, nil
	}

	// 获取子节点
	children, err := s.ndr.ListChildren(ctx, toNDRMeta(meta), nodeID, ndrclient.ListChildrenParams{})
	if err != nil {
		return nil, fmt.Errorf("list children of %d: %w", nodeID, err)
	}

	for _, child := range children {
		if child.DeletedAt != nil {
			continue // 跳过已删除的节点
		}
		childNodes, err := s.collectNodes(ctx, meta, child.ID, true, depth+1)
		if err != nil {
			return nil, err
		}
		result = append(result, childNodes...)
	}

	return result, nil
}

// ExecuteBatchWorkflow 执行批量工作流
func (s *BatchWorkflowService) ExecuteBatchWorkflow(
	ctx context.Context,
	meta RequestMeta,
	nodeID int64,
	req BatchWorkflowExecuteRequest,
) (*BatchWorkflowExecuteResponse, error) {
	// 1. 验证工作流存在
	_, err := s.workflowService.GetWorkflowDefinition(ctx, req.WorkflowKey)
	if err != nil {
		return nil, fmt.Errorf("invalid workflow: %w", err)
	}

	// 2. 收集所有目标节点
	nodes, err := s.collectNodes(ctx, meta, nodeID, req.IncludeDescendants, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to collect nodes: %w", err)
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes to execute")
	}

	// 3. 创建批次记录
	batchID := uuid.New().String()
	batch := database.WorkflowBatch{
		BatchID:     batchID,
		WorkflowKey: req.WorkflowKey,
		RootNodeID:  nodeID,
		Status:      database.BatchStatusPending,
		TotalNodes:  len(nodes),
		CreatedByID: &meta.UserIDNumeric,
	}

	if err := s.db.Create(&batch).Error; err != nil {
		return nil, fmt.Errorf("failed to create batch record: %w", err)
	}

	// 4. 启动异步执行
	go s.executeBatchAsync(context.Background(), meta, batch.ID, nodes, req)

	return &BatchWorkflowExecuteResponse{
		BatchID:    batchID,
		Status:     database.BatchStatusRunning,
		TotalNodes: len(nodes),
		Message:    "批量工作流已启动",
	}, nil
}

// executeBatchAsync 异步执行批量工作流
func (s *BatchWorkflowService) executeBatchAsync(
	ctx context.Context,
	meta RequestMeta,
	batchID uint,
	nodes []nodeWithDepth,
	req BatchWorkflowExecuteRequest,
) {
	// 更新状态为运行中
	now := time.Now()
	s.db.Model(&database.WorkflowBatch{}).Where("id = ?", batchID).Updates(map[string]interface{}{
		"status":     database.BatchStatusRunning,
		"started_at": &now,
	})

	// 设置并发数
	concurrency := req.Concurrency
	if concurrency <= 0 {
		concurrency = DefaultBatchConcurrency
	}
	if concurrency > MaxBatchConcurrency {
		concurrency = MaxBatchConcurrency
	}

	// 使用信号量控制并发
	semaphore := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex

	successCount := 0
	failedCount := 0
	skippedCount := 0
	details := make(map[string]interface{})
	nodeResults := make([]map[string]interface{}, 0, len(nodes))

	for _, node := range nodes {
		wg.Add(1)
		semaphore <- struct{}{} // 获取信号量

		go func(n nodeWithDepth) {
			defer wg.Done()
			defer func() { <-semaphore }() // 释放信号量

			result := map[string]interface{}{
				"node_id":   n.ID,
				"node_name": n.Name,
				"node_path": n.Path,
			}

			// 检查是否需要跳过
			if req.SkipNoSource {
				sources, err := s.ndr.ListSourceDocuments(ctx, toNDRMeta(meta), n.ID)
				if err != nil {
					mu.Lock()
					failedCount++
					result["status"] = "failed"
					result["error"] = fmt.Sprintf("获取源文档失败: %v", err)
					nodeResults = append(nodeResults, result)
					mu.Unlock()
					return
				}
				if len(sources) == 0 {
					mu.Lock()
					skippedCount++
					result["status"] = "skipped"
					result["reason"] = "无源文档"
					nodeResults = append(nodeResults, result)
					mu.Unlock()
					return
				}
			}

			// 执行工作流
			triggerReq := TriggerWorkflowRequest{
				NodeID:      n.ID,
				WorkflowKey: req.WorkflowKey,
				Parameters:  req.Parameters,
			}

			resp, err := s.workflowService.TriggerWorkflow(ctx, meta, triggerReq)
			if err != nil {
				mu.Lock()
				failedCount++
				result["status"] = "failed"
				result["error"] = err.Error()
				nodeResults = append(nodeResults, result)
				mu.Unlock()
				log.Printf("[batch_workflow] node %d failed: %v", n.ID, err)
				return
			}

			mu.Lock()
			successCount++
			result["status"] = "success"
			result["run_id"] = resp.RunID
			result["prefect_flow_run_id"] = resp.PrefectFlowRunID
			nodeResults = append(nodeResults, result)
			mu.Unlock()

			log.Printf("[batch_workflow] node %d success, run_id=%d", n.ID, resp.RunID)
		}(node)
	}

	wg.Wait()

	// 更新批次记录
	finishedAt := time.Now()
	finalStatus := database.BatchStatusCompleted
	if failedCount > 0 && successCount == 0 {
		finalStatus = database.BatchStatusFailed
	}

	details["node_results"] = nodeResults

	s.db.Model(&database.WorkflowBatch{}).Where("id = ?", batchID).Updates(map[string]interface{}{
		"status":        finalStatus,
		"success_count": successCount,
		"failed_count":  failedCount,
		"skipped_count": skippedCount,
		"details":       database.JSONMap(details),
		"finished_at":   &finishedAt,
	})

	log.Printf("[batch_workflow] batch %d completed: success=%d, failed=%d, skipped=%d",
		batchID, successCount, failedCount, skippedCount)
}

// GetBatchWorkflowStatus 获取批量工作流状态
func (s *BatchWorkflowService) GetBatchWorkflowStatus(
	ctx context.Context,
	batchID string,
) (*BatchWorkflowStatusResponse, error) {
	var batch database.WorkflowBatch
	if err := s.db.Where("batch_id = ?", batchID).First(&batch).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("batch not found: %s", batchID)
		}
		return nil, fmt.Errorf("failed to get batch: %w", err)
	}

	// 计算进度
	var progress float64
	if batch.TotalNodes > 0 {
		completed := batch.SuccessCount + batch.FailedCount + batch.SkippedCount
		progress = float64(completed) / float64(batch.TotalNodes) * 100
	}

	return &BatchWorkflowStatusResponse{
		BatchID:      batch.BatchID,
		WorkflowKey:  batch.WorkflowKey,
		RootNodeID:   batch.RootNodeID,
		Status:       batch.Status,
		TotalNodes:   batch.TotalNodes,
		SuccessCount: batch.SuccessCount,
		FailedCount:  batch.FailedCount,
		SkippedCount: batch.SkippedCount,
		Progress:     progress,
		Details:      batch.Details,
		ErrorMessage: batch.ErrorMessage,
		StartedAt:    batch.StartedAt,
		FinishedAt:   batch.FinishedAt,
		CreatedAt:    batch.CreatedAt,
	}, nil
}

// ListBatchWorkflows 列出批量工作流
func (s *BatchWorkflowService) ListBatchWorkflows(
	ctx context.Context,
	meta RequestMeta,
	limit int,
	offset int,
) ([]BatchWorkflowStatusResponse, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var total int64
	query := s.db.Model(&database.WorkflowBatch{})

	// 非超级管理员只能看到自己创建的批次
	if meta.UserRole != "super_admin" {
		query = query.Where("created_by_id = ?", meta.UserIDNumeric)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var batches []database.WorkflowBatch
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&batches).Error; err != nil {
		return nil, 0, err
	}

	results := make([]BatchWorkflowStatusResponse, 0, len(batches))
	for _, batch := range batches {
		var progress float64
		if batch.TotalNodes > 0 {
			completed := batch.SuccessCount + batch.FailedCount + batch.SkippedCount
			progress = float64(completed) / float64(batch.TotalNodes) * 100
		}

		results = append(results, BatchWorkflowStatusResponse{
			BatchID:      batch.BatchID,
			WorkflowKey:  batch.WorkflowKey,
			RootNodeID:   batch.RootNodeID,
			Status:       batch.Status,
			TotalNodes:   batch.TotalNodes,
			SuccessCount: batch.SuccessCount,
			FailedCount:  batch.FailedCount,
			SkippedCount: batch.SkippedCount,
			Progress:     progress,
			ErrorMessage: batch.ErrorMessage,
			StartedAt:    batch.StartedAt,
			FinishedAt:   batch.FinishedAt,
			CreatedAt:    batch.CreatedAt,
		})
	}

	return results, total, nil
}
