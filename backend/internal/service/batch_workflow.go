// Package service provides business logic for the YDMS backend.
package service

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"slices"
	"strconv"
	"strings"
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
	// DefaultSyncConcurrency sync_to_mysql 默认并发数（不调用 Prefect，可以更高）
	DefaultSyncConcurrency = 10
	// MaxBatchConcurrency 最大批量执行并发数
	MaxBatchConcurrency = 20
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
	WorkflowKey        string   `json:"workflow_key"`
	IncludeDescendants bool     `json:"include_descendants"`
	SkipNoSource       bool     `json:"skip_no_source"`       // 跳过没有源文档的节点
	SkipNoOutput       bool     `json:"skip_no_output"`       // 跳过无产出文档的节点
	SkipNameContains   string   `json:"skip_name_contains"`   // 跳过节点名包含指定字符串的节点
	SkipDocTypes       []string `json:"skip_doc_types"`       // 跳过指定类型的源文档
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
	SkipNoOutput       bool                   `json:"skip_no_output"`       // 跳过无产出文档的节点
	SkipNameContains   string                 `json:"skip_name_contains"`   // 跳过节点名包含指定字符串的节点
	SkipDocTypes       []string               `json:"skip_doc_types"`       // 跳过指定类型的源文档
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

		// 检查节点名是否包含指定字符串
		if req.SkipNameContains != "" && strings.Contains(node.Name, req.SkipNameContains) {
			item.CanExecute = false
			item.SkipReason = fmt.Sprintf("节点名包含「%s」", req.SkipNameContains)
			willSkipCount++
			previewItems = append(previewItems, item)
			continue
		}

		// 检查源文档
		sources, err := s.ndr.ListSourceDocuments(ctx, toNDRMeta(meta), node.ID)
		if err != nil {
			item.CanExecute = false
			item.SkipReason = fmt.Sprintf("获取源文档失败: %v", err)
			willSkipCount++
			previewItems = append(previewItems, item)
			continue
		}

		// 过滤掉指定类型的源文档
		if len(req.SkipDocTypes) > 0 {
			filteredSources := make([]ndrclient.SourceDocument, 0, len(sources))
			for _, src := range sources {
				docType := ""
				if src.Document != nil && src.Document.Type != nil {
					docType = *src.Document.Type
				}
				if !slices.Contains(req.SkipDocTypes, docType) {
					filteredSources = append(filteredSources, src)
				}
			}
			sources = filteredSources
		}

		item.SourceDocCount = len(sources)

		if len(sources) == 0 && req.SkipNoSource {
			item.CanExecute = false
			item.SkipReason = "无源文档"
			willSkipCount++
			previewItems = append(previewItems, item)
			continue
		}

		// 检查产出文档（如果启用了跳过无产出）
		if req.SkipNoOutput {
			// 构建源文档 ID 列表，用于排除
			sourceDocIDs := make([]int64, len(sources))
			for i, src := range sources {
				sourceDocIDs[i] = src.DocumentID
			}
			// 检查是否有非源文档的直接文档
			hasOutput, err := s.nodeHasOutputDocuments(ctx, meta, node.ID, sourceDocIDs)
			if err != nil {
				item.CanExecute = false
				item.SkipReason = fmt.Sprintf("获取产出文档失败: %v", err)
				willSkipCount++
				previewItems = append(previewItems, item)
				continue
			}
			if !hasOutput {
				item.CanExecute = false
				item.SkipReason = "无产出文档"
				willSkipCount++
				previewItems = append(previewItems, item)
				continue
			}
		}

		item.CanExecute = true
		canExecuteCount++
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

// nodeHasOutputDocuments 检查节点是否有产出文档（非源文档的直接文档）
// 使用 include_descendants=false 只检查节点的直接文档，排除源文档
func (s *BatchWorkflowService) nodeHasOutputDocuments(
	ctx context.Context,
	meta RequestMeta,
	nodeID int64,
	sourceDocIDs []int64,
) (bool, error) {
	sourceSet := make(map[int64]struct{}, len(sourceDocIDs))
	for _, id := range sourceDocIDs {
		sourceSet[id] = struct{}{}
	}

	const pageSize = 100
	page := 1
	for {
		query := url.Values{}
		query.Set("include_descendants", "false")
		query.Set("page", strconv.Itoa(page))
		query.Set("size", strconv.Itoa(pageSize))

		docsPage, err := s.ndr.ListNodeDocuments(ctx, toNDRMeta(meta), nodeID, query)
		if err != nil {
			return false, err
		}
		for _, doc := range docsPage.Items {
			if _, ok := sourceSet[doc.ID]; !ok {
				return true, nil // 找到非源文档
			}
		}

		fetched := docsPage.Page * docsPage.Size
		if len(docsPage.Items) == 0 || fetched >= docsPage.Total {
			break
		}
		page++
	}

	return false, nil
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
		// sync_to_mysql 不调用 Prefect，可以使用更高的默认并发数
		if req.WorkflowKey == SyncWorkflowKey {
			concurrency = DefaultSyncConcurrency
		} else {
			concurrency = DefaultBatchConcurrency
		}
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

			// 检查节点名是否包含指定字符串
			if req.SkipNameContains != "" && strings.Contains(n.Name, req.SkipNameContains) {
				mu.Lock()
				skippedCount++
				result["status"] = "skipped"
				result["reason"] = fmt.Sprintf("节点名包含「%s」", req.SkipNameContains)
				nodeResults = append(nodeResults, result)
				mu.Unlock()
				return
			}

			// 检查是否需要跳过无源文档的节点
			// SkipNoOutput 也需要源文档集合用于"产出文档"判断，因此这里统一预取源文档
			var sourceDocIDs []int64
			if req.SkipNoSource || req.SkipNoOutput || len(req.SkipDocTypes) > 0 {
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

				// 过滤掉指定类型的源文档
				if len(req.SkipDocTypes) > 0 {
					filteredSources := make([]ndrclient.SourceDocument, 0, len(sources))
					for _, src := range sources {
						docType := ""
						if src.Document != nil && src.Document.Type != nil {
							docType = *src.Document.Type
						}
						if !slices.Contains(req.SkipDocTypes, docType) {
							filteredSources = append(filteredSources, src)
						}
					}
					sources = filteredSources
				}

				if req.SkipNoSource && len(sources) == 0 {
					mu.Lock()
					skippedCount++
					result["status"] = "skipped"
					result["reason"] = "无源文档"
					nodeResults = append(nodeResults, result)
					mu.Unlock()
					return
				}
				// 保存源文档 ID，传递给 TriggerWorkflow 避免重复查询
				sourceDocIDs = make([]int64, len(sources))
				for i, src := range sources {
					sourceDocIDs[i] = src.DocumentID
				}
			}

			// 检查是否需要跳过无产出文档的节点
			if req.SkipNoOutput {
				hasOutput, err := s.nodeHasOutputDocuments(ctx, meta, n.ID, sourceDocIDs)
				if err != nil {
					mu.Lock()
					failedCount++
					result["status"] = "failed"
					result["error"] = fmt.Sprintf("获取产出文档失败: %v", err)
					nodeResults = append(nodeResults, result)
					mu.Unlock()
					return
				}
				if !hasOutput {
					mu.Lock()
					skippedCount++
					result["status"] = "skipped"
					result["reason"] = "无产出文档"
					nodeResults = append(nodeResults, result)
					mu.Unlock()
					return
				}
			}

			// 执行工作流
			triggerReq := TriggerWorkflowRequest{
				NodeID:       n.ID,
				WorkflowKey:  req.WorkflowKey,
				Parameters:   req.Parameters,
				SourceDocIDs: sourceDocIDs,
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
