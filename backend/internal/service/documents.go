package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yjxt/ydms/backend/internal/ndrclient"
)

// DocumentCreateRequest represents the payload required to create a document.
type DocumentCreateRequest struct {
	Title    string         `json:"title"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Content  map[string]any `json:"content,omitempty"`
	Type     *string        `json:"type,omitempty"`
	Position *int           `json:"position,omitempty"`
}

// ListDocuments fetches a paginated list of documents from NDR.
func (s *Service) ListDocuments(ctx context.Context, meta RequestMeta, query url.Values) (ndrclient.DocumentsPage, error) {
	page, err := s.ndr.ListDocuments(ctx, toNDRMeta(meta), query)
	if err != nil {
		return ndrclient.DocumentsPage{}, err
	}

	ids := extractIDFilter(query)
	if len(ids) == 0 {
		return page, nil
	}

	filtered := filterDocumentsByID(page.Items, ids)
	page.Items = filtered
	page.Total = len(filtered)
	page.Size = len(filtered)
	return page, nil
}

// ListNodeDocuments fetches documents attached to the node subtree with pagination support.
func (s *Service) ListNodeDocuments(ctx context.Context, meta RequestMeta, nodeID int64, query url.Values) (ndrclient.DocumentsPage, error) {
	page, err := s.ndr.ListNodeDocuments(ctx, toNDRMeta(meta), nodeID, query)
	if err != nil {
		return ndrclient.DocumentsPage{}, err
	}

	ids := extractIDFilter(query)
	if len(ids) == 0 {
		return page, nil
	}

	filtered := filterDocumentsByID(page.Items, ids)
	page.Items = filtered
	page.Total = len(filtered)
	page.Size = len(filtered)
	return page, nil
}

// ListDocumentsByPath fetches documents attached to the node subtree by node path.
func (s *Service) ListDocumentsByPath(ctx context.Context, meta RequestMeta, nodePath string, query url.Values) (ndrclient.DocumentsPage, error) {
	page, err := s.ndr.ListNodeDocumentsByPath(ctx, toNDRMeta(meta), nodePath, query)
	if err != nil {
		return ndrclient.DocumentsPage{}, err
	}

	ids := extractIDFilter(query)
	if len(ids) == 0 {
		return page, nil
	}

	filtered := filterDocumentsByID(page.Items, ids)
	page.Items = filtered
	page.Total = len(filtered)
	page.Size = len(filtered)
	return page, nil
}

// ListDeletedDocuments returns documents that are currently soft-deleted.
func (s *Service) ListDeletedDocuments(ctx context.Context, meta RequestMeta, query url.Values) (ndrclient.DocumentsPage, error) {
	if query == nil {
		query = url.Values{}
	}
	query.Set("include_deleted", "true")

	page, err := s.ndr.ListDocuments(ctx, toNDRMeta(meta), query)
	if err != nil {
		return ndrclient.DocumentsPage{}, err
	}

	filtered := make([]ndrclient.Document, 0, len(page.Items))
	for _, doc := range page.Items {
		if doc.DeletedAt != nil {
			filtered = append(filtered, doc)
		}
	}

	page.Items = filtered
	page.Total = len(filtered)
	page.Size = len(filtered)
	return page, nil
}

// CreateDocument creates a new document upstream.
func (s *Service) CreateDocument(ctx context.Context, meta RequestMeta, payload DocumentCreateRequest) (ndrclient.Document, error) {
	// Validate document type if provided
	if payload.Type != nil {
		if !IsValidDocumentType(*payload.Type) {
			return ndrclient.Document{}, fmt.Errorf("invalid document type: %s. Valid types: %v", *payload.Type, ValidDocumentTypes())
		}

		// Validate content structure
		if err := ValidateDocumentContent(payload.Content, *payload.Type); err != nil {
			return ndrclient.Document{}, fmt.Errorf("invalid content: %w", err)
		}
	}

	// Validate metadata
	if err := ValidateDocumentMetadata(payload.Metadata); err != nil {
		return ndrclient.Document{}, fmt.Errorf("invalid metadata: %w", err)
	}

	body := ndrclient.DocumentCreate{
		Title:    payload.Title,
		Metadata: payload.Metadata,
		Content:  payload.Content,
		Type:     payload.Type,
		Position: payload.Position,
	}

	// If no position is specified, NDR will assign the next available position automatically
	return s.ndr.CreateDocument(ctx, toNDRMeta(meta), body)
}

// BindDocument associates a document with a specific node.
func (s *Service) BindDocument(ctx context.Context, meta RequestMeta, nodeID, docID int64) error {
	return s.ndr.BindDocument(ctx, toNDRMeta(meta), nodeID, docID)
}

// UnbindDocument removes the binding between a node and a document.
func (s *Service) UnbindDocument(ctx context.Context, meta RequestMeta, nodeID, docID int64) error {
	return s.ndr.UnbindDocument(ctx, toNDRMeta(meta), nodeID, docID)
}

// GetDocument fetches a single document by ID.
func (s *Service) GetDocument(ctx context.Context, meta RequestMeta, docID int64) (ndrclient.Document, error) {
	return s.ndr.GetDocument(ctx, toNDRMeta(meta), docID)
}

// DeleteDocument performs a soft delete on the document.
func (s *Service) DeleteDocument(ctx context.Context, meta RequestMeta, docID int64) error {
	return s.ndr.DeleteDocument(ctx, toNDRMeta(meta), docID)
}

// RestoreDocument restores a previously soft-deleted document.
func (s *Service) RestoreDocument(ctx context.Context, meta RequestMeta, docID int64) (ndrclient.Document, error) {
	return s.ndr.RestoreDocument(ctx, toNDRMeta(meta), docID)
}

// PurgeDocument permanently removes a document.
func (s *Service) PurgeDocument(ctx context.Context, meta RequestMeta, docID int64) error {
	return s.ndr.PurgeDocument(ctx, toNDRMeta(meta), docID)
}

// GetDocumentBindingStatus returns the binding status of a document.
func (s *Service) GetDocumentBindingStatus(ctx context.Context, meta RequestMeta, docID int64) (ndrclient.DocumentBindingStatus, error) {
	return s.ndr.GetDocumentBindingStatus(ctx, toNDRMeta(meta), docID)
}

// DocumentUpdateRequest represents the payload required to update a document.
type DocumentUpdateRequest struct {
	Title    *string        `json:"title,omitempty"`
	Content  map[string]any `json:"content,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Type     *string        `json:"type,omitempty"`
	Position *int           `json:"position,omitempty"`
}

// UpdateDocument updates an existing document upstream.
func (s *Service) UpdateDocument(ctx context.Context, meta RequestMeta, docID int64, payload DocumentUpdateRequest) (ndrclient.Document, error) {
	// Validate document type if provided
	if payload.Type != nil {
		if !IsValidDocumentType(*payload.Type) {
			return ndrclient.Document{}, fmt.Errorf("invalid document type: %s. Valid types: %v", *payload.Type, ValidDocumentTypes())
		}

		// Validate content structure if both type and content are provided
		if payload.Content != nil {
			if err := ValidateDocumentContent(payload.Content, *payload.Type); err != nil {
				return ndrclient.Document{}, fmt.Errorf("invalid content: %w", err)
			}
		}
	}

	// Validate metadata if provided
	if payload.Metadata != nil {
		if err := ValidateDocumentMetadata(payload.Metadata); err != nil {
			return ndrclient.Document{}, fmt.Errorf("invalid metadata: %w", err)
		}
	}

	body := ndrclient.DocumentUpdate{
		Title:    payload.Title,
		Content:  payload.Content,
		Metadata: payload.Metadata,
		Type:     payload.Type,
		Position: payload.Position,
	}
	return s.ndr.UpdateDocument(ctx, toNDRMeta(meta), docID, body)
}

// ErrInvalidDocumentReorder indicates the reorder payload is invalid.
var ErrInvalidDocumentReorder = errors.New("ordered_ids cannot be empty")

// DocumentReorderRequest represents a request to reorder documents.
type DocumentReorderRequest struct {
	OrderedIDs      []int64 `json:"ordered_ids"`
	Type            *string `json:"type"`
	ApplyTypeFilter bool    `json:"-"`
}

// UnmarshalJSON captures whether the optional type field is provided.
func (r *DocumentReorderRequest) UnmarshalJSON(data []byte) error {
	type alias struct {
		OrderedIDs []int64          `json:"ordered_ids"`
		Type       *json.RawMessage `json:"type"`
	}

	var payload alias
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	r.OrderedIDs = payload.OrderedIDs
	r.Type = nil
	r.ApplyTypeFilter = false

	if payload.Type != nil {
		r.ApplyTypeFilter = true
		raw := bytes.TrimSpace(*payload.Type)
		if !bytes.Equal(raw, []byte("null")) {
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return err
			}
			trimmed := strings.TrimSpace(value)
			r.Type = &trimmed
		}
	}

	return nil
}

// ReorderDocuments delegates document reordering to the upstream service.
func (s *Service) ReorderDocuments(ctx context.Context, meta RequestMeta, req DocumentReorderRequest) ([]ndrclient.Document, error) {
	if len(req.OrderedIDs) == 0 {
		return nil, ErrInvalidDocumentReorder
	}

	payload := ndrclient.DocumentReorderPayload{
		OrderedIDs: req.OrderedIDs,
	}

	applyFilter := req.ApplyTypeFilter
	var outboundType *string
	if req.Type != nil {
		trimmed := strings.TrimSpace(*req.Type)
		if trimmed != "" {
			t := trimmed
			outboundType = &t
		}
		// 一旦显式提供了 type，即便未设置 ApplyTypeFilter，也认为需要按类型筛选
		applyFilter = true
	}

	if applyFilter {
		payload.ApplyTypeFilter = true
		payload.Type = outboundType
	}

	return s.ndr.ReorderDocuments(ctx, toNDRMeta(meta), payload)
}

func extractIDFilter(query url.Values) map[int64]struct{} {
	if query == nil {
		return nil
	}
	raw := query["id"]
	if len(raw) == 0 {
		return nil
	}
	ids := make(map[int64]struct{}, len(raw))
	for _, value := range raw {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		id, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			continue
		}
		ids[id] = struct{}{}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

func filterDocumentsByID(items []ndrclient.Document, ids map[int64]struct{}) []ndrclient.Document {
	if len(ids) == 0 {
		return items
	}
	filtered := make([]ndrclient.Document, 0, len(items))
	for _, doc := range items {
		if _, ok := ids[doc.ID]; ok {
			filtered = append(filtered, doc)
		}
	}
	return filtered
}

// DocumentVersion represents a version of a document.
type DocumentVersion struct {
	DocumentID    int64          `json:"document_id"`
	VersionNumber int            `json:"version_number"`
	Title         string         `json:"title"`
	Content       map[string]any `json:"content"`
	Metadata      map[string]any `json:"metadata"`
	Type          *string        `json:"type"`
	CreatedBy     string         `json:"created_by"`
	CreatedAt     string         `json:"created_at"`
	ChangeMessage *string        `json:"change_message"`
}

// DocumentVersionsPage wraps paginated version results.
type DocumentVersionsPage struct {
	Page     int               `json:"page"`
	Size     int               `json:"size"`
	Total    int               `json:"total"`
	Versions []DocumentVersion `json:"versions"`
}

// DocumentVersionDiff represents differences between two versions.
type DocumentVersionDiff struct {
	FromVersion int            `json:"from_version"`
	ToVersion   int            `json:"to_version"`
	TitleDiff   *DiffDetail    `json:"title_diff,omitempty"`
	ContentDiff map[string]any `json:"content_diff,omitempty"`
	MetaDiff    map[string]any `json:"metadata_diff,omitempty"`
}

// DiffDetail represents the difference in a specific field.
type DiffDetail struct {
	Old any `json:"old"`
	New any `json:"new"`
}

// ListDocumentVersions retrieves all versions of a document.
func (s *Service) ListDocumentVersions(ctx context.Context, meta RequestMeta, docID int64, page, size int) (DocumentVersionsPage, error) {
	ndrPage, err := s.ndr.ListDocumentVersions(ctx, toNDRMeta(meta), docID, page, size)
	if err != nil {
		return DocumentVersionsPage{}, err
	}

	source := ndrPage.Versions
	if len(source) == 0 && len(ndrPage.Items) > 0 {
		source = ndrPage.Items
	}

	versions := make([]DocumentVersion, 0, len(source))
	for _, v := range source {
		versions = append(versions, DocumentVersion{
			DocumentID:    v.DocumentID,
			VersionNumber: v.VersionNumber,
			Title:         v.Title,
			Content:       v.Content,
			Metadata:      v.Metadata,
			Type:          v.Type,
			CreatedBy:     v.CreatedBy,
			CreatedAt:     v.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			ChangeMessage: v.ChangeMessage,
		})
	}

	return DocumentVersionsPage{
		Page:     ndrPage.Page,
		Size:     ndrPage.Size,
		Total:    ndrPage.Total,
		Versions: versions,
	}, nil
}

// GetDocumentVersion retrieves a specific version of a document.
func (s *Service) GetDocumentVersion(ctx context.Context, meta RequestMeta, docID int64, versionNumber int) (DocumentVersion, error) {
	v, err := s.ndr.GetDocumentVersion(ctx, toNDRMeta(meta), docID, versionNumber)
	if err != nil {
		return DocumentVersion{}, err
	}

	return DocumentVersion{
		DocumentID:    v.DocumentID,
		VersionNumber: v.VersionNumber,
		Title:         v.Title,
		Content:       v.Content,
		Metadata:      v.Metadata,
		Type:          v.Type,
		CreatedBy:     v.CreatedBy,
		CreatedAt:     v.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		ChangeMessage: v.ChangeMessage,
	}, nil
}

// GetDocumentVersionDiff compares two versions of a document.
func (s *Service) GetDocumentVersionDiff(ctx context.Context, meta RequestMeta, docID int64, fromVersion, toVersion int) (DocumentVersionDiff, error) {
	diff, err := s.ndr.GetDocumentVersionDiff(ctx, toNDRMeta(meta), docID, fromVersion, toVersion)
	if err != nil {
		return DocumentVersionDiff{}, err
	}

	return DocumentVersionDiff{
		FromVersion: diff.FromVersion,
		ToVersion:   diff.ToVersion,
		TitleDiff:   (*DiffDetail)(diff.TitleDiff),
		ContentDiff: diff.ContentDiff,
		MetaDiff:    diff.MetaDiff,
	}, nil
}

// RestoreDocumentVersion restores a document to a specific version.
func (s *Service) RestoreDocumentVersion(ctx context.Context, meta RequestMeta, docID int64, versionNumber int) (ndrclient.Document, error) {
	return s.ndr.RestoreDocumentVersion(ctx, toNDRMeta(meta), docID, versionNumber)
}

// DocumentReference represents a reference to another document stored in metadata.
type DocumentReference struct {
	DocumentID int64  `json:"document_id"`
	Title      string `json:"title"`
	AddedAt    string `json:"added_at"`
}

// AddDocumentReference adds a reference to another document in the source document's metadata.
// It prevents self-references and ensures the referenced document exists.
func (s *Service) AddDocumentReference(ctx context.Context, meta RequestMeta, docID int64, refDocID int64) (ndrclient.Document, error) {
	// Prevent self-reference
	if docID == refDocID {
		return ndrclient.Document{}, fmt.Errorf("cannot add self-reference")
	}

	// Get the source document
	doc, err := s.GetDocument(ctx, meta, docID)
	if err != nil {
		return ndrclient.Document{}, fmt.Errorf("failed to get source document: %w", err)
	}

	// Verify the referenced document exists
	refDoc, err := s.GetDocument(ctx, meta, refDocID)
	if err != nil {
		return ndrclient.Document{}, fmt.Errorf("referenced document not found: %w", err)
	}

	// Get or initialize references array in metadata
	metadata := doc.Metadata
	if metadata == nil {
		metadata = make(map[string]any)
	}

	var references []DocumentReference
	if refsRaw, ok := metadata["references"]; ok {
		// Parse existing references
		if refsArray, ok := refsRaw.([]any); ok {
			for _, refRaw := range refsArray {
				if refMap, ok := refRaw.(map[string]any); ok {
					ref := DocumentReference{}
					if id, ok := refMap["document_id"].(float64); ok {
						ref.DocumentID = int64(id)
					}
					if title, ok := refMap["title"].(string); ok {
						ref.Title = title
					}
					if addedAt, ok := refMap["added_at"].(string); ok {
						ref.AddedAt = addedAt
					}
					references = append(references, ref)
				}
			}
		}
	}

	// Check if reference already exists
	for _, ref := range references {
		if ref.DocumentID == refDocID {
			return ndrclient.Document{}, fmt.Errorf("reference already exists: document %d already references document %d (title: %s)", docID, refDocID, ref.Title)
		}
	}

	// Add new reference
	newRef := DocumentReference{
		DocumentID: refDocID,
		Title:      refDoc.Title,
		AddedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	references = append(references, newRef)

	// Convert back to []any for metadata
	refsAny := make([]any, len(references))
	for i, ref := range references {
		refsAny[i] = map[string]any{
			"document_id": ref.DocumentID,
			"title":       ref.Title,
			"added_at":    ref.AddedAt,
		}
	}
	metadata["references"] = refsAny

	// Update document with new metadata
	updateReq := DocumentUpdateRequest{
		Metadata: metadata,
	}
	return s.UpdateDocument(ctx, meta, docID, updateReq)
}

// RemoveDocumentReference removes a reference from a document's metadata.
func (s *Service) RemoveDocumentReference(ctx context.Context, meta RequestMeta, docID int64, refDocID int64) (ndrclient.Document, error) {
	// Get the source document
	doc, err := s.GetDocument(ctx, meta, docID)
	if err != nil {
		return ndrclient.Document{}, fmt.Errorf("failed to get source document: %w", err)
	}

	// Get references from metadata
	metadata := doc.Metadata
	if metadata == nil {
		return ndrclient.Document{}, fmt.Errorf("no references found")
	}

	refsRaw, ok := metadata["references"]
	if !ok {
		return ndrclient.Document{}, fmt.Errorf("no references found")
	}

	refsArray, ok := refsRaw.([]any)
	if !ok {
		return ndrclient.Document{}, fmt.Errorf("invalid references format")
	}

	// Filter out the reference to remove
	var newRefs []any
	found := false
	for _, refRaw := range refsArray {
		refMap, ok := refRaw.(map[string]any)
		if !ok {
			continue
		}
		id, ok := refMap["document_id"].(float64)
		if !ok {
			continue
		}
		if int64(id) != refDocID {
			newRefs = append(newRefs, refRaw)
		} else {
			found = true
		}
	}

	if !found {
		return ndrclient.Document{}, fmt.Errorf("reference not found")
	}

	// Update metadata with filtered references
	// Per RFC 7396 (JSON Merge Patch), set field to nil to delete it
	// This will be serialized as {"references": null} in JSON
	if len(newRefs) == 0 {
		log.Printf("[DEBUG] 删除最后一个引用，设置 references 为 nil (docID=%d, refDocID=%d)", docID, refDocID)
		metadata["references"] = nil
		log.Printf("[DEBUG] 设置后 metadata: %+v", metadata)
	} else {
		log.Printf("[DEBUG] 还有 %d 个引用，更新 references 字段", len(newRefs))
		metadata["references"] = newRefs
	}

	// Update document
	updateReq := DocumentUpdateRequest{
		Metadata: metadata,
	}
	log.Printf("[DEBUG] 准备更新文档 %d，metadata: %+v", docID, metadata)
	updatedDoc, err := s.UpdateDocument(ctx, meta, docID, updateReq)
	if err != nil {
		log.Printf("[ERROR] 更新文档失败: %v", err)
		return ndrclient.Document{}, err
	}
	log.Printf("[DEBUG] 更新后文档 metadata: %+v", updatedDoc.Metadata)
	return updatedDoc, nil
}

// GetReferencingDocuments finds all documents that reference the given document.
// This performs a reverse lookup by searching through all documents' metadata.
func (s *Service) GetReferencingDocuments(ctx context.Context, meta RequestMeta, docID int64, query url.Values) ([]ndrclient.Document, error) {
	// Get all documents (with pagination handled by caller via query params)
	page, err := s.ListDocuments(ctx, meta, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list documents: %w", err)
	}

	// Filter documents that reference the target docID
	var referencingDocs []ndrclient.Document
	for _, doc := range page.Items {
		if doc.Metadata == nil {
			continue
		}

		refsRaw, ok := doc.Metadata["references"]
		if !ok {
			continue
		}

		refsArray, ok := refsRaw.([]any)
		if !ok {
			continue
		}

		// Check if this document references the target
		for _, refRaw := range refsArray {
			refMap, ok := refRaw.(map[string]any)
			if !ok {
				continue
			}
			id, ok := refMap["document_id"].(float64)
			if !ok {
				continue
			}
			if int64(id) == docID {
				referencingDocs = append(referencingDocs, doc)
				break
			}
		}
	}

	return referencingDocs, nil
}
