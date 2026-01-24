// Package service provides business logic for the YDMS backend.
package service

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/yjxt/ydms/backend/internal/database"
	"github.com/yjxt/ydms/backend/internal/ndrclient"
)

// BatchSyncService 批量同步服务
type BatchSyncService struct {
	db          *gorm.DB
	ndr         ndrclient.Client
	syncService *SyncService
}

// NewBatchSyncService 创建批量同步服务
func NewBatchSyncService(db *gorm.DB, ndr ndrclient.Client, syncSvc *SyncService) *BatchSyncService {
	return &BatchSyncService{
		db:          db,
		ndr:         ndr,
		syncService: syncSvc,
	}
}

// BatchSyncPreviewRequest 批量同步预览请求
type BatchSyncPreviewRequest struct {
	IncludeDescendants bool `json:"include_descendants"`
}

// DocumentPreviewItem 文档预览项
type DocumentPreviewItem struct {
	DocumentID   int64       `json:"document_id"`
	DocumentName string      `json:"document_name"`
	DocumentType string      `json:"document_type"`
	NodeID       int64       `json:"node_id"`
	NodePath     string      `json:"node_path"`
	SyncTarget   *SyncTarget `json:"sync_target,omitempty"`
	CanSync      bool        `json:"can_sync"`
	SkipReason   string      `json:"skip_reason,omitempty"`
}

// BatchSyncPreviewResponse 批量同步预览响应
type BatchSyncPreviewResponse struct {
	RootNodeID     int64                 `json:"root_node_id"`
	TotalDocuments int                   `json:"total_documents"`
	CanSync        int                   `json:"can_sync"`
	WillSkip       int                   `json:"will_skip"`
	Documents      []DocumentPreviewItem `json:"documents"`
}

// BatchSyncExecuteRequest 批量同步执行请求
type BatchSyncExecuteRequest struct {
	IncludeDescendants bool `json:"include_descendants"`
	Concurrency        int  `json:"concurrency,omitempty"` // 并发数，默认 3
}

// BatchSyncExecuteResponse 批量同步执行响应
type BatchSyncExecuteResponse struct {
	BatchID        string `json:"batch_id"`
	Status         string `json:"status"`
	TotalDocuments int    `json:"total_documents"`
	Message        string `json:"message,omitempty"`
}

// BatchSyncStatusResponse 批量同步状态响应
type BatchSyncStatusResponse struct {
	BatchID        string                 `json:"batch_id"`
	RootNodeID     int64                  `json:"root_node_id"`
	Status         string                 `json:"status"`
	TotalDocuments int                    `json:"total_documents"`
	SuccessCount   int                    `json:"success_count"`
	FailedCount    int                    `json:"failed_count"`
	SkippedCount   int                    `json:"skipped_count"`
	Progress       float64                `json:"progress"` // 0-100
	Details        map[string]interface{} `json:"details,omitempty"`
	ErrorMessage   string                 `json:"error_message,omitempty"`
	StartedAt      *time.Time             `json:"started_at,omitempty"`
	FinishedAt     *time.Time             `json:"finished_at,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
}

// documentWithNode 带节点信息的文档
type documentWithNode struct {
	Document ndrclient.Document
	NodeID   int64
	NodePath string
}

// PreviewBatchSync 预览批量同步
func (s *BatchSyncService) PreviewBatchSync(
	ctx context.Context,
	meta RequestMeta,
	nodeID int64,
	req BatchSyncPreviewRequest,
) (*BatchSyncPreviewResponse, error) {
	// 收集所有目标文档
	documents, err := s.collectDocuments(ctx, meta, nodeID, req.IncludeDescendants)
	if err != nil {
		return nil, fmt.Errorf("failed to collect documents: %w", err)
	}

	// 检查每个文档的状态
	previewItems := make([]DocumentPreviewItem, 0, len(documents))
	canSyncCount := 0
	willSkipCount := 0

	for _, doc := range documents {
		docType := ""
		if doc.Document.Type != nil {
			docType = *doc.Document.Type
		}

		item := DocumentPreviewItem{
			DocumentID:   doc.Document.ID,
			DocumentName: doc.Document.Title,
			DocumentType: docType,
			NodeID:       doc.NodeID,
			NodePath:     doc.NodePath,
		}

		// 检查 sync_target 配置
		syncTarget, err := parseSyncTarget(doc.Document.Metadata)
		if err != nil {
			item.CanSync = false
			item.SkipReason = fmt.Sprintf("sync_target 配置错误: %v", err)
			willSkipCount++
		} else if syncTarget == nil {
			item.CanSync = false
			item.SkipReason = "未配置 sync_target"
			willSkipCount++
		} else {
			item.SyncTarget = syncTarget
			item.CanSync = true
			canSyncCount++
		}

		previewItems = append(previewItems, item)
	}

	return &BatchSyncPreviewResponse{
		RootNodeID:     nodeID,
		TotalDocuments: len(documents),
		CanSync:        canSyncCount,
		WillSkip:       willSkipCount,
		Documents:      previewItems,
	}, nil
}

// collectDocuments 递归收集节点下的所有文档（排除源文档）
func (s *BatchSyncService) collectDocuments(
	ctx context.Context,
	meta RequestMeta,
	nodeID int64,
	includeDescendants bool,
) ([]documentWithNode, error) {
	result := make([]documentWithNode, 0)

	// 获取当前节点信息
	node, err := s.ndr.GetNode(ctx, toNDRMeta(meta), nodeID, ndrclient.GetNodeOptions{})
	if err != nil {
		return nil, fmt.Errorf("get node %d: %w", nodeID, err)
	}

	// 获取当前节点的源文档 ID 列表（用于排除）
	sourceDocIDs := make(map[int64]bool)
	sourceDocs, err := s.ndr.ListSourceDocuments(ctx, toNDRMeta(meta), nodeID)
	if err != nil {
		// 源文档获取失败不阻塞流程，仅记录日志
		log.Printf("[batch_sync] warning: failed to get source documents for node %d: %v", nodeID, err)
	} else {
		for _, sd := range sourceDocs {
			sourceDocIDs[sd.DocumentID] = true
		}
	}

	// 获取当前节点的文档
	docs, err := s.getNodeDocuments(ctx, meta, nodeID)
	if err != nil {
		return nil, fmt.Errorf("get documents for node %d: %w", nodeID, err)
	}

	for _, doc := range docs {
		// 排除源文档（工作流输入）
		if sourceDocIDs[doc.ID] {
			continue
		}
		result = append(result, documentWithNode{
			Document: doc,
			NodeID:   nodeID,
			NodePath: node.Path,
		})
	}

	if !includeDescendants {
		return result, nil
	}

	// 递归获取子节点的文档
	children, err := s.ndr.ListChildren(ctx, toNDRMeta(meta), nodeID, ndrclient.ListChildrenParams{})
	if err != nil {
		return nil, fmt.Errorf("list children of %d: %w", nodeID, err)
	}

	for _, child := range children {
		if child.DeletedAt != nil {
			continue // 跳过已删除的节点
		}
		childDocs, err := s.collectDocuments(ctx, meta, child.ID, true)
		if err != nil {
			return nil, err
		}
		result = append(result, childDocs...)
	}

	return result, nil
}

// getNodeDocuments 获取节点的文档（不包含子孙节点）
func (s *BatchSyncService) getNodeDocuments(
	ctx context.Context,
	meta RequestMeta,
	nodeID int64,
) ([]ndrclient.Document, error) {
	query := url.Values{}
	query.Set("include_descendants", "false")
	query.Set("size", "100")

	page := 1
	docs := make([]ndrclient.Document, 0)

	for {
		query.Set("page", fmt.Sprintf("%d", page))
		docsPage, err := s.ndr.ListNodeDocuments(ctx, toNDRMeta(meta), nodeID, query)
		if err != nil {
			return nil, err
		}

		docs = append(docs, docsPage.Items...)

		if len(docsPage.Items) < 100 || (docsPage.Total > 0 && len(docs) >= docsPage.Total) {
			break
		}
		page++
	}

	return docs, nil
}

// ExecuteBatchSync 执行批量同步
func (s *BatchSyncService) ExecuteBatchSync(
	ctx context.Context,
	meta RequestMeta,
	nodeID int64,
	req BatchSyncExecuteRequest,
) (*BatchSyncExecuteResponse, error) {
	// 收集所有目标文档
	documents, err := s.collectDocuments(ctx, meta, nodeID, req.IncludeDescendants)
	if err != nil {
		return nil, fmt.Errorf("failed to collect documents: %w", err)
	}

	if len(documents) == 0 {
		return nil, fmt.Errorf("no documents to sync")
	}

	// 创建批次记录
	batchID := uuid.New().String()
	batch := database.SyncBatch{
		BatchID:        batchID,
		RootNodeID:     nodeID,
		Status:         database.BatchStatusPending,
		TotalDocuments: len(documents),
		CreatedByID:    &meta.UserIDNumeric,
	}

	if err := s.db.Create(&batch).Error; err != nil {
		return nil, fmt.Errorf("failed to create batch record: %w", err)
	}

	// 启动异步执行
	go s.executeBatchSyncAsync(context.Background(), meta, batch.ID, documents, req)

	return &BatchSyncExecuteResponse{
		BatchID:        batchID,
		Status:         database.BatchStatusRunning,
		TotalDocuments: len(documents),
		Message:        "批量同步已启动",
	}, nil
}

// executeBatchSyncAsync 异步执行批量同步
func (s *BatchSyncService) executeBatchSyncAsync(
	ctx context.Context,
	meta RequestMeta,
	batchID uint,
	documents []documentWithNode,
	req BatchSyncExecuteRequest,
) {
	// 更新状态为运行中
	now := time.Now()
	s.db.Model(&database.SyncBatch{}).Where("id = ?", batchID).Updates(map[string]interface{}{
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
	docResults := make([]map[string]interface{}, 0, len(documents))

	for _, doc := range documents {
		wg.Add(1)
		semaphore <- struct{}{} // 获取信号量

		go func(d documentWithNode) {
			defer wg.Done()
			defer func() { <-semaphore }() // 释放信号量

			docType := ""
			if d.Document.Type != nil {
				docType = *d.Document.Type
			}

			result := map[string]interface{}{
				"document_id":   d.Document.ID,
				"document_name": d.Document.Title,
				"document_type": docType,
				"node_id":       d.NodeID,
				"node_path":     d.NodePath,
			}

			// 检查 sync_target 配置
			syncTarget, err := parseSyncTarget(d.Document.Metadata)
			if err != nil {
				mu.Lock()
				failedCount++
				result["status"] = "failed"
				result["error"] = fmt.Sprintf("sync_target 配置错误: %v", err)
				docResults = append(docResults, result)
				mu.Unlock()
				return
			}
			if syncTarget == nil {
				mu.Lock()
				skippedCount++
				result["status"] = "skipped"
				result["reason"] = "未配置 sync_target"
				docResults = append(docResults, result)
				mu.Unlock()
				return
			}

			// 执行同步
			resp, err := s.syncService.TriggerSync(ctx, meta, d.Document.ID)
			if err != nil {
				mu.Lock()
				failedCount++
				result["status"] = "failed"
				result["error"] = err.Error()
				docResults = append(docResults, result)
				mu.Unlock()
				log.Printf("[batch_sync] document %d failed: %v", d.Document.ID, err)
				return
			}

			mu.Lock()
			successCount++
			result["status"] = "success"
			result["event_id"] = resp.EventID
			result["prefect_flow_run_id"] = resp.PrefectFlowRunID
			docResults = append(docResults, result)
			mu.Unlock()

			log.Printf("[batch_sync] document %d success, event_id=%s", d.Document.ID, resp.EventID)
		}(doc)
	}

	wg.Wait()

	// 更新批次记录
	finishedAt := time.Now()
	finalStatus := database.BatchStatusCompleted
	if failedCount > 0 && successCount == 0 {
		finalStatus = database.BatchStatusFailed
	}

	details["document_results"] = docResults

	s.db.Model(&database.SyncBatch{}).Where("id = ?", batchID).Updates(map[string]interface{}{
		"status":        finalStatus,
		"success_count": successCount,
		"failed_count":  failedCount,
		"skipped_count": skippedCount,
		"details":       database.JSONMap(details),
		"finished_at":   &finishedAt,
	})

	log.Printf("[batch_sync] batch %d completed: success=%d, failed=%d, skipped=%d",
		batchID, successCount, failedCount, skippedCount)
}

// GetBatchSyncStatus 获取批量同步状态
func (s *BatchSyncService) GetBatchSyncStatus(
	ctx context.Context,
	batchID string,
) (*BatchSyncStatusResponse, error) {
	var batch database.SyncBatch
	if err := s.db.Where("batch_id = ?", batchID).First(&batch).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("batch not found: %s", batchID)
		}
		return nil, fmt.Errorf("failed to get batch: %w", err)
	}

	// 计算进度
	var progress float64
	if batch.TotalDocuments > 0 {
		completed := batch.SuccessCount + batch.FailedCount + batch.SkippedCount
		progress = float64(completed) / float64(batch.TotalDocuments) * 100
	}

	return &BatchSyncStatusResponse{
		BatchID:        batch.BatchID,
		RootNodeID:     batch.RootNodeID,
		Status:         batch.Status,
		TotalDocuments: batch.TotalDocuments,
		SuccessCount:   batch.SuccessCount,
		FailedCount:    batch.FailedCount,
		SkippedCount:   batch.SkippedCount,
		Progress:       progress,
		Details:        batch.Details,
		ErrorMessage:   batch.ErrorMessage,
		StartedAt:      batch.StartedAt,
		FinishedAt:     batch.FinishedAt,
		CreatedAt:      batch.CreatedAt,
	}, nil
}

// ListBatchSyncs 列出批量同步
func (s *BatchSyncService) ListBatchSyncs(
	ctx context.Context,
	meta RequestMeta,
	limit int,
	offset int,
) ([]BatchSyncStatusResponse, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var total int64
	query := s.db.Model(&database.SyncBatch{})

	// 非超级管理员只能看到自己创建的批次
	if meta.UserRole != "super_admin" {
		query = query.Where("created_by_id = ?", meta.UserIDNumeric)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var batches []database.SyncBatch
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&batches).Error; err != nil {
		return nil, 0, err
	}

	results := make([]BatchSyncStatusResponse, 0, len(batches))
	for _, batch := range batches {
		var progress float64
		if batch.TotalDocuments > 0 {
			completed := batch.SuccessCount + batch.FailedCount + batch.SkippedCount
			progress = float64(completed) / float64(batch.TotalDocuments) * 100
		}

		results = append(results, BatchSyncStatusResponse{
			BatchID:        batch.BatchID,
			RootNodeID:     batch.RootNodeID,
			Status:         batch.Status,
			TotalDocuments: batch.TotalDocuments,
			SuccessCount:   batch.SuccessCount,
			FailedCount:    batch.FailedCount,
			SkippedCount:   batch.SkippedCount,
			Progress:       progress,
			ErrorMessage:   batch.ErrorMessage,
			StartedAt:      batch.StartedAt,
			FinishedAt:     batch.FinishedAt,
			CreatedAt:      batch.CreatedAt,
		})
	}

	return results, total, nil
}
