// Package service provides business logic for the YDMS backend.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/yjxt/ydms/backend/internal/database"
	"github.com/yjxt/ydms/backend/internal/ndrclient"
	"github.com/yjxt/ydms/backend/internal/prefectclient"
)

// Sync pipeline name
const (
	PipelineSyncToMySQL = "sync_to_mysql"
)

// Sync status constants
const (
	SyncStatusPending = "pending"
	SyncStatusSuccess = "success"
	SyncStatusFailed  = "failed"
	SyncStatusSkipped = "skipped"
)

// SyncPendingTimeout 同步任务超时时间（超过此时间的 pending 状态视为超时，允许重新同步）
const SyncPendingTimeout = 1 * time.Minute

// SyncWorkflowKey 同步工作流的 key（用于记录到 workflow_runs）
const SyncWorkflowKey = "sync_to_mysql"

// SyncService 处理文档同步到外部 MySQL 数据库的服务
type SyncService struct {
	db             *gorm.DB
	prefect        *prefectclient.Client
	ndr            ndrclient.Client
	pdmsBaseURL    string // PDMS base URL for callback
	prefectEnabled bool   // Whether Prefect integration is enabled
}

// NewSyncService 创建新的 SyncService
func NewSyncService(db *gorm.DB, prefect *prefectclient.Client, ndr ndrclient.Client, pdmsBaseURL string) *SyncService {
	return &SyncService{
		db:             db,
		prefect:        prefect,
		ndr:            ndr,
		pdmsBaseURL:    pdmsBaseURL,
		prefectEnabled: prefect != nil,
	}
}

// SyncTarget 同步目标配置（存储在文档 metadata.sync_target 中）
type SyncTarget struct {
	Table      string `json:"table,omitempty"`      // 可选：默认处理器需要，自定义处理器可能不需要
	RecordID   int64  `json:"record_id"`            // 必需：目标记录 ID
	Field      string `json:"field,omitempty"`      // 可选：默认处理器需要，自定义处理器可能不需要
	Connection string `json:"connection,omitempty"` // 可选：数据库连接名，默认根据文档类型决定
}

// TriggerSyncRequest 触发同步请求
type TriggerSyncRequest struct {
	DocumentID int64 `json:"document_id"`
}

// TriggerSyncResponse 触发同步响应
type TriggerSyncResponse struct {
	EventID          string      `json:"event_id"`
	Status           string      `json:"status"`
	Message          string      `json:"message,omitempty"`
	DocumentID       int64       `json:"document_id"`
	DocumentVersion  int         `json:"document_version"`
	PrefectFlowRunID string      `json:"prefect_flow_run_id,omitempty"`
	SyncTarget       *SyncTarget `json:"sync_target,omitempty"`
	IdempotencyKey   string      `json:"idempotency_key,omitempty"`
}

// SyncCallbackRequest 同步回调请求（来自 IDPP）
type SyncCallbackRequest struct {
	EventID        string                 `json:"event_id"`
	DocID          int64                  `json:"doc_id"`
	DocVersion     int                    `json:"doc_version"`
	Status         string                 `json:"status"`
	Error          string                 `json:"error,omitempty"`
	AffectedTables []AffectedTable        `json:"affected_tables,omitempty"`
	RunID          string                 `json:"run_id,omitempty"`
	Extra          map[string]interface{} `json:"extra,omitempty"`
}

// AffectedTable 受影响的表信息
type AffectedTable struct {
	Table        string `json:"table"`
	AffectedRows int64  `json:"affected_rows"`
	Operation    string `json:"operation"` // update, insert, delete
}

// SyncStatusResponse 同步状态响应
type SyncStatusResponse struct {
	DocumentID  int64       `json:"document_id"`
	SyncTarget  *SyncTarget `json:"sync_target,omitempty"`
	LastSync    *LastSync   `json:"last_sync,omitempty"`
	SyncEnabled bool        `json:"sync_enabled"`
}

// LastSync 最后一次同步信息
type LastSync struct {
	EventID  string     `json:"event_id,omitempty"`
	Version  int        `json:"version"`
	Status   string     `json:"status"`
	Error    string     `json:"error,omitempty"`
	RunID    string     `json:"run_id,omitempty"`
	SyncedAt *time.Time `json:"synced_at,omitempty"`
}

// TriggerSync 触发文档同步到 MySQL
func (s *SyncService) TriggerSync(
	ctx context.Context,
	meta RequestMeta,
	docID int64,
) (*TriggerSyncResponse, error) {
	// 1. 获取文档信息
	doc, err := s.ndr.GetDocument(ctx, toNDRMeta(meta), docID)
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	docVersion := 1
	if doc.Version != nil {
		docVersion = *doc.Version
	}

	docType := ""
	if doc.Type != nil {
		docType = *doc.Type
	}

	// 2. 解析并验证 sync_target 配置
	syncTarget, err := parseSyncTarget(doc.Metadata)
	if err != nil {
		return nil, fmt.Errorf("invalid sync_target config: %w", err)
	}

	if syncTarget == nil {
		return nil, errors.New("sync_target not configured in document metadata")
	}

	// 3. 生成幂等性 key
	idempotencyKey := generateSyncIdempotencyKey(docID, docVersion)

	// 4. 检查是否已有进行中的同步任务
	var existingStatus database.DocSyncStatus
	err = s.db.WithContext(ctx).Where("document_id = ?", docID).First(&existingStatus).Error
	if err == nil {
		// 如果已有记录且状态为 pending，检查是否超时
		if existingStatus.LastStatus == SyncStatusPending {
			// 检查是否超过超时时间（1 分钟）
			if time.Since(existingStatus.UpdatedAt) < SyncPendingTimeout {
				return &TriggerSyncResponse{
					EventID:          existingStatus.LastEventID,
					Status:           SyncStatusPending,
					Message:          "sync task already in progress",
					DocumentID:       docID,
					DocumentVersion:  existingStatus.LastVersion,
					PrefectFlowRunID: existingStatus.LastRunID,
					SyncTarget:       syncTarget,
					IdempotencyKey:   idempotencyKey,
				}, nil
			}
			// 超时了，将旧任务标记为失败，然后继续创建新任务
			s.markSyncTimeout(ctx, docID, existingStatus.LastEventID, existingStatus.LastWorkflowRunID)
		}
	}

	// 5. 生成新的 event_id
	eventID := uuid.New().String()

	// 6. 在事务中创建/更新同步状态记录，并创建 workflow_run 记录
	var workflowRunID uint
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 6.1 创建 workflow_run 记录
		documentID := docID
		run := database.WorkflowRun{
			WorkflowKey: SyncWorkflowKey,
			DocumentID:  &documentID,
			Parameters: database.JSONMap{
				"event_id":    eventID,
				"doc_version": docVersion,
				"sync_target": syncTarget,
			},
			Status:      WorkflowStatusPending,
			CreatedByID: &meta.UserIDNumeric,
		}
		if err := tx.Create(&run).Error; err != nil {
			return fmt.Errorf("failed to create workflow run: %w", err)
		}
		workflowRunID = run.ID

		// 6.2 创建/更新同步状态记录
		var status database.DocSyncStatus
		result := tx.Where("document_id = ?", docID).First(&status)

		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// 创建新记录
			status = database.DocSyncStatus{
				DocumentID:        docID,
				LastEventID:       eventID,
				LastVersion:       docVersion,
				LastStatus:        SyncStatusPending,
				LastError:         "",
				LastWorkflowRunID: &workflowRunID,
			}
			return tx.Create(&status).Error
		}

		if result.Error != nil {
			return result.Error
		}

		// 更新现有记录
		return tx.Model(&status).Updates(map[string]interface{}{
			"last_event_id":        eventID,
			"last_version":         docVersion,
			"last_status":          SyncStatusPending,
			"last_error":           "",
			"last_run_id":          "",
			"last_workflow_run_id": workflowRunID,
		}).Error
	})

	if err != nil {
		return nil, fmt.Errorf("failed to update sync status: %w", err)
	}

	// 7. 如果 Prefect 未启用，返回 pending 状态
	if !s.prefectEnabled {
		return &TriggerSyncResponse{
			EventID:         eventID,
			Status:          SyncStatusPending,
			Message:         "sync task created (Prefect not configured)",
			DocumentID:      docID,
			DocumentVersion: docVersion,
			SyncTarget:      syncTarget,
			IdempotencyKey:  idempotencyKey,
		}, nil
	}

	// 8. 查找 Prefect deployment
	deploymentName := fmt.Sprintf("%s-deployment", PipelineSyncToMySQL)
	deployment, err := s.prefect.GetDeploymentByName(ctx, PipelineSyncToMySQL, deploymentName)
	if err != nil {
		// 更新状态为失败（使用条件更新防止覆盖回调结果）
		_ = s.updateSyncStatusWithCondition(ctx, docID, eventID, SyncStatusFailed, fmt.Sprintf("deployment not found: %s", err.Error()), "")
		return nil, fmt.Errorf("failed to find Prefect deployment: %w", err)
	}

	// 9. 构建 flow 参数
	callbackURL := fmt.Sprintf("%s/api/v1/sync/callback", s.pdmsBaseURL)
	flowParams := map[string]interface{}{
		"event_id":     eventID,
		"doc_id":       docID,
		"doc_type":     docType,
		"doc_version":  docVersion,
		"callback_url": callbackURL,
	}

	// 10. 创建 flow run
	flowRun, err := s.prefect.CreateFlowRun(ctx, deployment.ID, flowParams)
	if err != nil {
		_ = s.updateSyncStatusWithCondition(ctx, docID, eventID, SyncStatusFailed, fmt.Sprintf("failed to create flow run: %s", err.Error()), "")
		return nil, fmt.Errorf("failed to create flow run: %w", err)
	}

	// 11. 更新 flow run ID（使用条件更新）
	_ = s.updateSyncStatusWithCondition(ctx, docID, eventID, SyncStatusPending, "", flowRun.ID)

	return &TriggerSyncResponse{
		EventID:          eventID,
		Status:           SyncStatusPending,
		Message:          "sync task submitted",
		DocumentID:       docID,
		DocumentVersion:  docVersion,
		PrefectFlowRunID: flowRun.ID,
		SyncTarget:       syncTarget,
		IdempotencyKey:   idempotencyKey,
	}, nil
}

// HandleSyncCallback 处理来自 IDPP 的同步回调
func (s *SyncService) HandleSyncCallback(ctx context.Context, callback SyncCallbackRequest) error {
	// 验证状态值
	if !isValidSyncStatus(callback.Status) {
		return fmt.Errorf("invalid status: %s", callback.Status)
	}

	// 获取当前状态记录
	var status database.DocSyncStatus
	err := s.db.WithContext(ctx).Where("document_id = ?", callback.DocID).First(&status).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("sync status not found for document %d", callback.DocID)
		}
		return fmt.Errorf("failed to get sync status: %w", err)
	}

	// 验证 event_id 匹配（防止重放攻击）
	if status.LastEventID != callback.EventID {
		return fmt.Errorf("event_id mismatch: expected %s, got %s", status.LastEventID, callback.EventID)
	}

	// 状态机约束：只允许从 pending 转移到终态
	if status.LastStatus != SyncStatusPending {
		return fmt.Errorf("invalid state transition: current status is %s, not pending", status.LastStatus)
	}

	// 构建更新
	now := time.Now()
	updates := map[string]interface{}{
		"last_status": callback.Status,
	}

	// 成功时清空错误信息，设置同步时间
	if callback.Status == SyncStatusSuccess {
		updates["last_error"] = ""
		updates["last_synced_at"] = &now
	} else if callback.Status == SyncStatusFailed {
		updates["last_error"] = callback.Error
	}

	if callback.RunID != "" {
		updates["last_run_id"] = callback.RunID
	}

	// 更新 doc_sync_status
	if err := s.db.WithContext(ctx).Model(&status).Updates(updates).Error; err != nil {
		return err
	}

	// 同步更新关联的 workflow_run 记录
	if status.LastWorkflowRunID != nil && *status.LastWorkflowRunID > 0 {
		workflowStatus := WorkflowStatusSuccess
		if callback.Status == SyncStatusFailed {
			workflowStatus = WorkflowStatusFailed
		} else if callback.Status == SyncStatusSkipped {
			workflowStatus = WorkflowStatusCancelled
		}

		workflowUpdates := map[string]interface{}{
			"status":      workflowStatus,
			"finished_at": &now,
		}
		if callback.Status == SyncStatusFailed && callback.Error != "" {
			workflowUpdates["error_message"] = callback.Error
		}
		if callback.RunID != "" {
			workflowUpdates["prefect_flow_run_id"] = callback.RunID
		}

		s.db.WithContext(ctx).
			Model(&database.WorkflowRun{}).
			Where("id = ?", *status.LastWorkflowRunID).
			Updates(workflowUpdates)
	}

	return nil
}

// GetSyncStatus 获取文档的同步状态
func (s *SyncService) GetSyncStatus(ctx context.Context, meta RequestMeta, docID int64) (*SyncStatusResponse, error) {
	// 1. 获取文档信息
	doc, err := s.ndr.GetDocument(ctx, toNDRMeta(meta), docID)
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	// 2. 解析 sync_target 配置
	syncTarget, _ := parseSyncTarget(doc.Metadata)

	// 3. 获取同步状态记录
	var syncStatus database.DocSyncStatus
	err = s.db.WithContext(ctx).Where("document_id = ?", docID).First(&syncStatus).Error

	var lastSync *LastSync
	if err == nil {
		lastSync = &LastSync{
			EventID:  syncStatus.LastEventID,
			Version:  syncStatus.LastVersion,
			Status:   syncStatus.LastStatus,
			Error:    syncStatus.LastError,
			RunID:    syncStatus.LastRunID,
			SyncedAt: syncStatus.LastSyncedAt,
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// 真实的数据库错误
		return nil, fmt.Errorf("failed to get sync status: %w", err)
	}

	return &SyncStatusResponse{
		DocumentID:  docID,
		SyncTarget:  syncTarget,
		LastSync:    lastSync,
		SyncEnabled: syncTarget != nil,
	}, nil
}

// GetDocumentSnapshot 获取文档快照（供 IDPP 调用）
func (s *SyncService) GetDocumentSnapshot(ctx context.Context, meta RequestMeta, docID int64) (*DocumentSnapshot, error) {
	doc, err := s.ndr.GetDocument(ctx, toNDRMeta(meta), docID)
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	docVersion := 1
	if doc.Version != nil {
		docVersion = *doc.Version
	}

	docType := ""
	if doc.Type != nil {
		docType = *doc.Type
	}

	return &DocumentSnapshot{
		ID:       doc.ID,
		Type:     docType,
		Version:  docVersion,
		Title:    doc.Title,
		Content:  doc.Content,
		Metadata: doc.Metadata,
	}, nil
}

// DocumentSnapshot 文档快照
type DocumentSnapshot struct {
	ID       int64                  `json:"id"`
	Type     string                 `json:"type"`
	Version  int                    `json:"version"`
	Title    string                 `json:"title"`
	Content  map[string]interface{} `json:"content"`
	Metadata map[string]interface{} `json:"metadata"`
}

// Helper functions

// markSyncTimeout 将超时的同步任务标记为失败
func (s *SyncService) markSyncTimeout(ctx context.Context, docID int64, eventID string, workflowRunID *uint) {
	now := time.Now()
	errorMsg := "sync task timeout (exceeded 1 minute)"

	// 更新 doc_sync_status
	s.db.WithContext(ctx).
		Model(&database.DocSyncStatus{}).
		Where("document_id = ? AND last_event_id = ? AND last_status = ?", docID, eventID, SyncStatusPending).
		Updates(map[string]interface{}{
			"last_status": SyncStatusFailed,
			"last_error":  errorMsg,
		})

	// 更新关联的 workflow_run
	if workflowRunID != nil && *workflowRunID > 0 {
		s.db.WithContext(ctx).
			Model(&database.WorkflowRun{}).
			Where("id = ? AND status IN ?", *workflowRunID, []string{WorkflowStatusPending, WorkflowStatusRunning}).
			Updates(map[string]interface{}{
				"status":        WorkflowStatusFailed,
				"error_message": errorMsg,
				"finished_at":   &now,
			})
	}
}

// updateSyncStatusWithCondition 条件更新同步状态（防止覆盖回调结果）
func (s *SyncService) updateSyncStatusWithCondition(ctx context.Context, docID int64, eventID string, status string, errorMsg string, runID string) error {
	updates := map[string]interface{}{
		"last_status": status,
		"last_error":  errorMsg,
	}

	if runID != "" {
		updates["last_run_id"] = runID
	}

	if status == SyncStatusSuccess {
		now := time.Now()
		updates["last_synced_at"] = &now
		updates["last_error"] = "" // 成功时清空错误
	}

	// 只更新当 event_id 匹配且状态仍为 pending 时
	result := s.db.WithContext(ctx).
		Model(&database.DocSyncStatus{}).
		Where("document_id = ? AND last_event_id = ? AND last_status = ?", docID, eventID, SyncStatusPending).
		Updates(updates)

	if result.Error != nil {
		return result.Error
	}

	// 如果没有更新到任何行，可能是回调已经处理过了
	if result.RowsAffected == 0 {
		// 不返回错误，只是静默跳过
		return nil
	}

	return nil
}

// isValidSyncStatus 验证同步状态值
func isValidSyncStatus(status string) bool {
	switch status {
	case SyncStatusPending, SyncStatusSuccess, SyncStatusFailed, SyncStatusSkipped:
		return true
	default:
		return false
	}
}

// generateSyncIdempotencyKey 生成同步幂等性 key
func generateSyncIdempotencyKey(docID int64, version int) string {
	data := fmt.Sprintf("sync:%d:%d", docID, version)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:16])
}

// parseSyncTarget 从文档 metadata 解析并验证 sync_target 配置
func parseSyncTarget(metadata map[string]interface{}) (*SyncTarget, error) {
	if metadata == nil {
		return nil, nil
	}

	syncTargetRaw, ok := metadata["sync_target"]
	if !ok {
		return nil, nil
	}

	var syncTarget SyncTarget

	switch v := syncTargetRaw.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil, nil
		}
		// 尝试解析为 JSON 对象
		if err := json.Unmarshal([]byte(trimmed), &syncTarget); err != nil {
			return nil, fmt.Errorf("failed to unmarshal sync_target: %w", err)
		}
	case float64:
		// JSON 数字会被解析为 float64，支持直接配置为整数
		syncTarget.RecordID = int64(v)
	case int64:
		syncTarget.RecordID = v
	case int:
		syncTarget.RecordID = int64(v)
	case map[string]interface{}:
		// 将 map 转换为 JSON 再解析为结构体
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal sync_target: %w", err)
		}
		if err := json.Unmarshal(jsonBytes, &syncTarget); err != nil {
			return nil, fmt.Errorf("failed to unmarshal sync_target: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported sync_target type: %T", syncTargetRaw)
	}

	// 验证必要字段：只要求 record_id
	// table 和 field 由具体的处理器决定是否需要
	if syncTarget.RecordID == 0 {
		return nil, fmt.Errorf("sync_target.record_id is required")
	}

	// 安全验证：表名和字段名只允许安全标识符（防止 SQL 注入）
	identifierPattern := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	if syncTarget.Table != "" && !identifierPattern.MatchString(syncTarget.Table) {
		return nil, fmt.Errorf("invalid table name: %s (must match pattern ^[a-zA-Z_][a-zA-Z0-9_]*$)", syncTarget.Table)
	}
	if syncTarget.Field != "" && !identifierPattern.MatchString(syncTarget.Field) {
		return nil, fmt.Errorf("invalid field name: %s (must match pattern ^[a-zA-Z_][a-zA-Z0-9_]*$)", syncTarget.Field)
	}

	// 连接名验证（如果提供）
	if syncTarget.Connection != "" && !identifierPattern.MatchString(syncTarget.Connection) {
		return nil, fmt.Errorf("invalid connection name: %s", syncTarget.Connection)
	}

	return &syncTarget, nil
}
