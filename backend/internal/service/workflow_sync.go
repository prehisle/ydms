// Package service provides business logic services.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yjxt/ydms/backend/internal/database"
	"github.com/yjxt/ydms/backend/internal/prefectclient"

	"gorm.io/gorm"
)

// WorkflowSyncService handles synchronization of workflow definitions from Prefect.
type WorkflowSyncService struct {
	db             *gorm.DB
	prefectClient  *prefectclient.Client
	prefectEnabled bool

	// Sync status
	mu             sync.RWMutex
	lastSyncTime   *time.Time
	lastSyncStatus string // success, failed, in_progress
	lastSyncError  string
}

// NewWorkflowSyncService creates a new WorkflowSyncService.
func NewWorkflowSyncService(db *gorm.DB, prefectClient *prefectclient.Client, prefectEnabled bool) *WorkflowSyncService {
	return &WorkflowSyncService{
		db:             db,
		prefectClient:  prefectClient,
		prefectEnabled: prefectEnabled,
		lastSyncStatus: "idle",
	}
}

// SyncStatus represents the current sync status.
type SyncStatus struct {
	LastSyncTime   *time.Time `json:"last_sync_time,omitempty"`
	Status         string     `json:"status"`
	Error          string     `json:"error,omitempty"`
	PrefectEnabled bool       `json:"prefect_enabled"`
}

// GetSyncStatus returns the current sync status.
func (s *WorkflowSyncService) GetSyncStatus() SyncStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return SyncStatus{
		LastSyncTime:   s.lastSyncTime,
		Status:         s.lastSyncStatus,
		Error:          s.lastSyncError,
		PrefectEnabled: s.prefectEnabled,
	}
}

// SyncResult represents the result of a sync operation.
type SyncResult struct {
	Created  int      `json:"created"`
	Updated  int      `json:"updated"`
	Missing  int      `json:"missing"`
	Errors   []string `json:"errors,omitempty"`
	Duration string   `json:"duration"`
}

// SyncFromPrefect synchronizes workflow definitions from Prefect.
func (s *WorkflowSyncService) SyncFromPrefect(ctx context.Context) (*SyncResult, error) {
	if !s.prefectEnabled {
		return nil, fmt.Errorf("Prefect integration is not enabled")
	}

	// Set sync status to in_progress (with check for concurrent sync)
	s.mu.Lock()
	if s.lastSyncStatus == "in_progress" {
		s.mu.Unlock()
		return nil, fmt.Errorf("sync already in progress")
	}
	s.lastSyncStatus = "in_progress"
	s.lastSyncError = ""
	s.mu.Unlock()

	startTime := time.Now()
	result := &SyncResult{}

	// 1. Fetch deployments from Prefect
	// Filter by pdms:type or node-workflow/document-workflow tags
	deployments, err := s.prefectClient.ListDeployments(ctx, nil)
	if err != nil {
		s.mu.Lock()
		s.lastSyncStatus = "failed"
		s.lastSyncError = err.Error()
		s.mu.Unlock()
		return nil, fmt.Errorf("failed to fetch deployments from Prefect: %w", err)
	}

	// 2. Filter and process deployments
	seenIDs := make(map[string]bool)
	now := time.Now()

	for _, dep := range deployments {
		// Parse workflow type and key from tags
		workflowType, workflowKey := s.parseDeploymentTags(dep.Tags)
		if workflowType == "" {
			// Skip deployments without pdms tags
			continue
		}

		// Validate workflow key
		if workflowKey == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("deployment %s(%s) missing pdms:key tag", dep.Name, dep.ID))
			continue
		}

		// Validate workflow type
		if workflowType != "node" && workflowType != "document" {
			result.Errors = append(result.Errors, fmt.Sprintf("deployment %s(%s) has invalid pdms:type=%s", dep.Name, dep.ID, workflowType))
			continue
		}

		seenIDs[dep.ID] = true

		// Build spec hash for change detection
		specHash := s.computeSpecHash(dep)

		// Find existing definition
		var existing database.WorkflowDefinition
		err := s.db.Where("prefect_deployment_id = ?", dep.ID).First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			// Create new definition
			newDef := database.WorkflowDefinition{
				WorkflowKey:           workflowKey,
				Name:                  dep.Name,
				Description:           dep.Description,
				PrefectDeploymentName: dep.Name,
				PrefectDeploymentID:   dep.ID,
				PrefectVersion:        dep.Version,
				PrefectTags:           s.tagsToJSONMap(dep.Tags),
				ParameterSchema:       database.JSONMap(dep.ParameterSchema),
				Source:                "prefect",
				WorkflowType:          workflowType,
				SyncStatus:            "active",
				LastSyncedAt:          &now,
				LastSeenAt:            &now,
				SpecHash:              specHash,
				Enabled:               true,
			}

			if err := s.db.Create(&newDef).Error; err != nil {
				// Check if conflict on workflow_key
				if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "UNIQUE constraint") {
					// Try to update by workflow_key instead
					// Note: Use jsonMapToBytes() because GORM's Updates() with map doesn't call Value() method
					if updateErr := s.db.Model(&database.WorkflowDefinition{}).Where("workflow_key = ?", workflowKey).Updates(map[string]interface{}{
						"prefect_deployment_id":   dep.ID,
						"prefect_deployment_name": dep.Name,
						"prefect_version":         dep.Version,
						"prefect_tags":            jsonMapToBytes(s.tagsToJSONMap(dep.Tags)),
						"parameter_schema":        jsonMapToBytes(database.JSONMap(dep.ParameterSchema)),
						"description":             dep.Description,
						"source":                  "prefect",
						"workflow_type":           workflowType,
						"sync_status":             "active",
						"last_synced_at":          now,
						"last_seen_at":            now,
						"spec_hash":               specHash,
					}).Error; updateErr != nil {
						result.Errors = append(result.Errors, fmt.Sprintf("failed to upsert %s: %v", dep.Name, updateErr))
						continue
					}
					result.Updated++
					continue
				}
				result.Errors = append(result.Errors, fmt.Sprintf("failed to create %s: %v", dep.Name, err))
				continue
			}
			result.Created++
			log.Printf("[WorkflowSync] Created workflow: %s (%s)", workflowKey, workflowType)
		} else if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to query %s: %v", dep.Name, err))
			continue
		} else {
			// Update existing definition if spec changed
			updates := map[string]interface{}{
				"last_seen_at": now,
				"sync_status":  "active",
			}

			if existing.SpecHash != specHash {
				// Spec changed, update all fields
				// Note: Use jsonMapToBytes() because GORM's Updates() with map doesn't call Value() method
				updates["name"] = dep.Name
				updates["description"] = dep.Description
				updates["prefect_deployment_name"] = dep.Name
				updates["prefect_version"] = dep.Version
				updates["prefect_tags"] = jsonMapToBytes(s.tagsToJSONMap(dep.Tags))
				updates["parameter_schema"] = jsonMapToBytes(database.JSONMap(dep.ParameterSchema))
				updates["workflow_type"] = workflowType
				updates["spec_hash"] = specHash
				updates["last_synced_at"] = now
				result.Updated++
				log.Printf("[WorkflowSync] Updated workflow: %s", workflowKey)
			}

			if err := s.db.Model(&existing).Updates(updates).Error; err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to update %s: %v", dep.Name, err))
			}
		}
	}

	// 3. Mark missing deployments
	var prefectDefs []database.WorkflowDefinition
	if err := s.db.Where("source = ?", "prefect").Find(&prefectDefs).Error; err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to query existing definitions: %v", err))
	} else {
		for _, def := range prefectDefs {
			if def.PrefectDeploymentID != "" && !seenIDs[def.PrefectDeploymentID] {
				// Deployment no longer exists in Prefect
				if err := s.db.Model(&def).Updates(map[string]interface{}{
					"sync_status":  "missing",
					"last_seen_at": def.LastSeenAt, // Keep original last_seen_at
				}).Error; err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("failed to mark %s as missing: %v", def.WorkflowKey, err))
				} else {
					result.Missing++
					log.Printf("[WorkflowSync] Marked as missing: %s", def.WorkflowKey)
				}
			}
		}
	}

	// Update sync status
	s.mu.Lock()
	s.lastSyncTime = &now
	if len(result.Errors) > 0 {
		s.lastSyncStatus = "completed_with_errors"
		s.lastSyncError = strings.Join(result.Errors, "; ")
	} else {
		s.lastSyncStatus = "success"
		s.lastSyncError = ""
	}
	s.mu.Unlock()

	result.Duration = time.Since(startTime).String()
	log.Printf("[WorkflowSync] Sync completed: created=%d, updated=%d, missing=%d, errors=%d",
		result.Created, result.Updated, result.Missing, len(result.Errors))

	return result, nil
}

// parseDeploymentTags extracts workflow type and key from Prefect deployment tags.
func (s *WorkflowSyncService) parseDeploymentTags(tags []string) (workflowType, workflowKey string) {
	for _, tag := range tags {
		if strings.HasPrefix(tag, "pdms:type=") {
			workflowType = strings.TrimPrefix(tag, "pdms:type=")
		} else if strings.HasPrefix(tag, "pdms:key=") {
			workflowKey = strings.TrimPrefix(tag, "pdms:key=")
		}
	}

	// Fallback rules for backward compatibility
	if workflowType == "" {
		for _, tag := range tags {
			if tag == "node-workflow" {
				workflowType = "node"
				break
			} else if tag == "document-workflow" {
				workflowType = "document"
				break
			}
		}
	}

	return workflowType, workflowKey
}

// computeSpecHash computes a hash of the deployment spec for change detection.
func (s *WorkflowSyncService) computeSpecHash(dep prefectclient.DeploymentDetails) string {
	// Sort tags to ensure stable hashing (tag order is not guaranteed)
	tags := make([]string, len(dep.Tags))
	copy(tags, dep.Tags)
	sort.Strings(tags)

	// Create a stable representation for hashing
	spec := map[string]interface{}{
		"name":             dep.Name,
		"description":      dep.Description,
		"version":          dep.Version,
		"tags":             tags,
		"parameter_schema": dep.ParameterSchema,
	}

	data, _ := json.Marshal(spec)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash[:16]) // Use first 16 bytes (32 hex chars)
}

// tagsToJSONMap converts a slice of tags to JSONMap.
func (s *WorkflowSyncService) tagsToJSONMap(tags []string) database.JSONMap {
	return database.JSONMap{
		"tags": tags,
	}
}

// jsonMapToBytes converts JSONMap to []byte for GORM Updates() with map[string]interface{}.
// GORM's Updates() with map doesn't call Value() method, so we need to serialize manually.
func jsonMapToBytes(m database.JSONMap) []byte {
	if m == nil {
		return nil
	}
	b, _ := json.Marshal(m)
	return b
}

// WorkflowDefinitionFilter represents filter options for listing workflow definitions.
type WorkflowDefinitionFilter struct {
	Source       string // prefect, manual, or empty for all
	WorkflowType string // node, document, or empty for all
	SyncStatus   string // active, missing, error, or empty for all
	Enabled      *bool  // nil for all
}

// ListWorkflowDefinitionsAdmin lists all workflow definitions with admin filters.
func (s *WorkflowSyncService) ListWorkflowDefinitionsAdmin(filter WorkflowDefinitionFilter) ([]database.WorkflowDefinition, error) {
	query := s.db.Model(&database.WorkflowDefinition{})

	if filter.Source != "" {
		query = query.Where("source = ?", filter.Source)
	}
	if filter.WorkflowType != "" {
		query = query.Where("workflow_type = ?", filter.WorkflowType)
	}
	if filter.SyncStatus != "" {
		query = query.Where("sync_status = ?", filter.SyncStatus)
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}

	var definitions []database.WorkflowDefinition
	if err := query.Order("workflow_key").Find(&definitions).Error; err != nil {
		return nil, fmt.Errorf("failed to list workflow definitions: %w", err)
	}

	return definitions, nil
}

// UpdateWorkflowDefinition updates a workflow definition (admin only).
func (s *WorkflowSyncService) UpdateWorkflowDefinition(id uint, enabled bool) error {
	result := s.db.Model(&database.WorkflowDefinition{}).Where("id = ?", id).Update("enabled", enabled)
	if result.Error != nil {
		return fmt.Errorf("failed to update workflow definition: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("workflow definition not found")
	}
	return nil
}
