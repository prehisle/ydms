package database

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// User 用户模型
type User struct {
	ID           uint           `gorm:"primarykey" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Username     string         `gorm:"uniqueIndex;not null" json:"username"`
	PasswordHash string         `gorm:"not null" json:"-"` // 不在 JSON 中返回密码
	Role         string         `gorm:"not null;index" json:"role"` // super_admin, course_admin, proofreader
	DisplayName  string         `json:"display_name"`
	CreatedByID  *uint          `gorm:"index" json:"created_by_id,omitempty"` // 创建者 ID
	CreatedBy    *User          `gorm:"foreignKey:CreatedByID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL" json:"created_by,omitempty"`
}

// CoursePermission 课程权限模型（多对多关联）
type CoursePermission struct {
	ID         uint      `gorm:"primarykey" json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	UserID     uint      `gorm:"not null;index" json:"user_id"`
	RootNodeID int64     `gorm:"not null;index" json:"root_node_id"` // NDR 中的根节点 ID
	User       User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// TableName 指定表名
func (User) TableName() string {
	return "users"
}

// TableName 指定表名
func (CoursePermission) TableName() string {
	return "course_permissions"
}

// APIKey API密钥模型
type APIKey struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Name        string         `gorm:"not null" json:"name"`                                        // API Key 描述名称
	KeyHash     string         `gorm:"uniqueIndex;not null" json:"-"`                               // API Key 的哈希值（不返回）
	KeyPrefix   string         `gorm:"not null;index" json:"key_prefix"`                            // 前缀（便于识别）
	UserID      uint           `gorm:"not null;index" json:"user_id"`                               // 关联的用户账号
	User        User           `gorm:"foreignKey:UserID" json:"user,omitempty"`                     // 关联用户
	Scopes      string         `json:"scopes"`                                                      // 权限范围（JSON数组字符串）
	ExpiresAt   *time.Time     `json:"expires_at,omitempty"`                                        // 过期时间
	LastUsedAt  *time.Time     `json:"last_used_at,omitempty"`                                      // 最后使用时间
	CreatedByID uint           `gorm:"index" json:"created_by_id"`                                  // 创建者 ID
	CreatedBy   *User          `gorm:"foreignKey:CreatedByID;constraint:OnDelete:SET NULL" json:"created_by,omitempty"`
}

// TableName 指定表名
func (APIKey) TableName() string {
	return "api_keys"
}

// ProcessingJob AI 处理任务模型
type ProcessingJob struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// 文档信息
	DocumentID      int64  `gorm:"not null;index" json:"document_id"`
	DocumentVersion int    `gorm:"not null" json:"document_version"`
	DocumentTitle   string `json:"document_title"`

	// 流水线信息
	PipelineName   string  `gorm:"not null" json:"pipeline_name"`
	PipelineParams JSONMap `gorm:"type:jsonb;default:'{}'" json:"pipeline_params"` // JSONB 存储

	// Prefect 集成
	PrefectDeploymentID string `json:"prefect_deployment_id,omitempty"`
	PrefectFlowRunID    string `gorm:"index" json:"prefect_flow_run_id,omitempty"`

	// 状态: pending, running, completed, failed, cancelled
	Status   string `gorm:"not null;default:'pending';index" json:"status"`
	Progress int    `gorm:"default:0" json:"progress"` // 进度百分比 0-100

	// 结果
	Result       JSONMap `gorm:"type:jsonb" json:"result,omitempty"` // JSONB 存储处理结果
	ErrorMessage string  `json:"error_message,omitempty"`            // 失败时的错误信息

	// 幂等性: hash(doc_id + version + pipeline)
	IdempotencyKey string `gorm:"uniqueIndex" json:"idempotency_key"`

	// 触发者
	TriggeredByID *uint `gorm:"index" json:"triggered_by_id,omitempty"`
	TriggeredBy   *User `gorm:"foreignKey:TriggeredByID" json:"triggered_by,omitempty"`
	DryRun        bool  `gorm:"default:false" json:"dry_run"` // 是否预览模式

	// 时间戳
	StartedAt   *time.Time `json:"started_at,omitempty"`   // 开始处理时间
	CompletedAt *time.Time `json:"completed_at,omitempty"` // 完成时间
}

// TableName 指定表名
func (ProcessingJob) TableName() string {
	return "processing_jobs"
}

// DocSyncStatus 文档同步状态模型
// 记录文档到外部 MySQL 数据库的同步状态
type DocSyncStatus struct {
	ID           uint       `gorm:"primarykey" json:"id"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DocumentID   int64      `gorm:"not null;uniqueIndex" json:"document_id"`                    // NDR 文档 ID
	LastEventID  string     `gorm:"size:64;not null;default:''" json:"last_event_id,omitempty"` // 最后同步事件 ID
	LastVersion  int        `gorm:"not null;default:0" json:"last_version"`                     // 最后同步的文档版本
	LastStatus   string     `gorm:"size:32;not null;index;default:'pending'" json:"last_status"` // pending, success, failed, skipped
	LastError    string     `gorm:"type:text" json:"last_error,omitempty"`                      // 失败时的错误信息（可能较长）
	LastRunID    string     `gorm:"size:64" json:"last_run_id,omitempty"`                       // Prefect flow run ID
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`                                   // 最后成功同步时间
}

// TableName 指定表名（与其他表保持复数形式一致）
func (DocSyncStatus) TableName() string {
	return "doc_sync_statuses"
}

// JSONMap is a map[string]interface{} that implements GORM's Scanner and Valuer interfaces
// for proper JSONB serialization in PostgreSQL.
type JSONMap map[string]interface{}

// Value implements driver.Valuer for JSONMap
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements sql.Scanner for JSONMap
func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, j)
}

// WorkflowDefinition 工作流定义模型
// 定义可在节点上运行的工作流类型
type WorkflowDefinition struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// 工作流标识
	WorkflowKey string `gorm:"uniqueIndex;not null;size:64" json:"workflow_key"` // 如 generate_overview, generate_exercises

	// 展示信息
	Name        string `gorm:"not null;size:128" json:"name"`        // 显示名称
	Description string `gorm:"type:text" json:"description"`         // 描述

	// Prefect 集成
	PrefectDeploymentName string `gorm:"not null;size:128" json:"prefect_deployment_name"` // Prefect deployment 名称

	// 参数配置
	ParameterSchema JSONMap `gorm:"type:jsonb;default:'{}'" json:"parameter_schema"` // JSON Schema，用于前端动态表单

	// 状态
	Enabled bool `gorm:"default:true" json:"enabled"` // 是否启用
}

// TableName 指定表名
func (WorkflowDefinition) TableName() string {
	return "workflow_definitions"
}

// WorkflowRun 工作流运行记录模型
// 记录每次工作流运行的状态和结果
type WorkflowRun struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// 工作流信息
	WorkflowKey string `gorm:"not null;size:64;index" json:"workflow_key"` // 关联的工作流定义

	// 节点信息
	NodeID int64 `gorm:"not null;index" json:"node_id"` // 运行的节点 ID

	// 运行参数
	Parameters JSONMap `gorm:"type:jsonb;default:'{}'" json:"parameters"` // 运行时传入的参数

	// 状态: pending, running, success, failed, cancelled
	Status string `gorm:"not null;default:'pending';size:32;index" json:"status"`

	// Prefect 集成
	PrefectFlowRunID string `gorm:"size:64;index" json:"prefect_flow_run_id,omitempty"`

	// 结果
	Result       JSONMap `gorm:"type:jsonb" json:"result,omitempty"`     // 运行结果
	ErrorMessage string  `gorm:"type:text" json:"error_message,omitempty"` // 错误信息

	// 触发者
	CreatedByID *uint `gorm:"index" json:"created_by_id,omitempty"`
	CreatedBy   *User `gorm:"foreignKey:CreatedByID" json:"created_by,omitempty"`

	// 时间戳
	StartedAt  *time.Time `json:"started_at,omitempty"`  // 开始时间
	FinishedAt *time.Time `json:"finished_at,omitempty"` // 完成时间
}

// TableName 指定表名
func (WorkflowRun) TableName() string {
	return "workflow_runs"
}
