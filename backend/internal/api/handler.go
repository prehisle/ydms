package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/yjxt/ydms/backend/internal/auth"
	"github.com/yjxt/ydms/backend/internal/database"
	"github.com/yjxt/ydms/backend/internal/ndrclient"
	"github.com/yjxt/ydms/backend/internal/service"
)

// Handler exposes HTTP handlers that delegate to the service layer.
type Handler struct {
	service           *service.Service
	permissionService *service.PermissionService
	defaults          HeaderDefaults
}

type HeaderDefaults struct {
	APIKey   string
	UserID   string
	AdminKey string
}

// NewHandler returns a Handler wiring dependencies.
func NewHandler(svc *service.Service, permSvc *service.PermissionService, defaults HeaderDefaults) *Handler {
	return &Handler{
		service:           svc,
		permissionService: permSvc,
		defaults:          defaults,
	}
}

// Health reports basic liveness.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ping returns a hello world message.
func (h *Handler) Ping(w http.ResponseWriter, r *http.Request) {
	message, err := h.service.Hello(r.Context())
	if err != nil {
		respondError(w, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": message})
}

// Categories handles collection-level operations.
func (h *Handler) Categories(w http.ResponseWriter, r *http.Request) {
	meta := h.metaFromRequest(r)
	switch r.Method {
	case http.MethodPost:
		h.createCategory(w, r, meta)
	default:
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

// CategoryRoutes handles item-level operations based on suffix.
func (h *Handler) CategoryRoutes(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/api/v1/categories/")
	if relPath == "" {
		h.Categories(w, r)
		return
	}

	if relPath == "reorder" {
		h.reorderCategories(w, r, h.metaFromRequest(r))
		return
	}

	if relPath == "trash" {
		h.listDeletedCategories(w, r, h.metaFromRequest(r))
		return
	}

	if strings.HasPrefix(relPath, "bulk/") {
		meta := h.metaFromRequest(r)
		if relPath == "bulk/restore" {
			h.bulkRestoreCategories(w, r, meta)
			return
		}
		if relPath == "bulk/delete" {
			h.bulkDeleteCategories(w, r, meta)
			return
		}
		if relPath == "bulk/purge" {
			h.bulkPurgeCategories(w, r, meta)
			return
		}
		if relPath == "bulk/check" {
			h.bulkCheckCategories(w, r, meta)
			return
		}
		if relPath == "bulk/copy" {
			h.bulkCopyCategories(w, r, meta)
			return
		}
		if relPath == "bulk/move" {
			h.bulkMoveCategories(w, r, meta)
			return
		}
		respondError(w, http.StatusNotFound, errors.New("not found"))
		return
	}

	meta := h.metaFromRequest(r)

	if relPath == "tree" {
		h.listCategoryTree(w, r, meta)
		return
	}

	parts := strings.Split(relPath, "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid category id"))
		return
	}

	if len(parts) == 1 {
		h.handleCategoryItem(w, r, meta, id)
		return
	}

	switch parts[1] {
	case "restore":
		h.restoreCategory(w, r, meta, id)
	case "move":
		h.moveCategory(w, r, meta, id)
	case "purge":
		h.purgeCategory(w, r, meta, id)
	case "reposition":
		h.repositionCategory(w, r, meta, id)
	default:
		respondError(w, http.StatusNotFound, errors.New("not found"))
	}
}

// Documents handles collection-level document operations.
func (h *Handler) Documents(w http.ResponseWriter, r *http.Request) {
	meta := h.metaFromRequest(r)
	switch r.Method {
	case http.MethodGet:
		page, err := h.service.ListDocuments(r.Context(), meta, cloneQuery(r.URL.Query()))
		if err != nil {
			respondError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, page)
	case http.MethodPost:
		// 权限检查：校对员不能创建文档
		_, httpErr := h.requireNotProofreader(r, "create documents")
		if httpErr != nil {
			respondError(w, httpErr.code, httpErr.message)
			return
		}

		var payload service.DocumentCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			respondAPIError(w, NewAPIError(ErrCodeValidation, http.StatusBadRequest, "请求格式错误", err.Error()))
			return
		}
		if strings.TrimSpace(payload.Title) == "" {
			respondAPIError(w, ErrDocumentTitleRequired)
			return
		}
		doc, err := h.service.CreateDocument(r.Context(), meta, payload)
		if err != nil {
			respondAPIError(w, WrapUpstreamError(err))
			return
		}
		writeJSON(w, http.StatusCreated, doc)
	default:
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

// DocumentRoutes handles document-related operations and sub-resources.
func (h *Handler) DocumentRoutes(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/api/v1/documents/")
	if relPath == "" {
		h.Documents(w, r)
		return
	}

	if relPath == "reorder" {
		h.reorderDocuments(w, r, h.metaFromRequest(r))
		return
	}

	if relPath == "trash" {
		h.listDeletedDocuments(w, r, h.metaFromRequest(r))
		return
	}

	parts := strings.Split(relPath, "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid document id"))
		return
	}

	meta := h.metaFromRequest(r)

	if len(parts) == 1 {
		h.handleDocumentItem(w, r, meta, id)
		return
	}

	if parts[1] == "restore" {
		h.restoreDocument(w, r, meta, id)
		return
	}

	if parts[1] == "purge" {
		h.purgeDocument(w, r, meta, id)
		return
	}

	if parts[1] == "binding-status" {
		h.getDocumentBindingStatus(w, r, meta, id)
		return
	}

	// Handle reference-related routes
	if parts[1] == "references" {
		if len(parts) == 2 {
			// POST /api/v1/documents/{id}/references - add reference
			h.addDocumentReference(w, r, meta, id)
			return
		}
		if len(parts) == 3 {
			refID, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				respondError(w, http.StatusBadRequest, errors.New("invalid reference document id"))
				return
			}
			// DELETE /api/v1/documents/{id}/references/{refId} - remove reference
			h.removeDocumentReference(w, r, meta, id, refID)
			return
		}
	}

	if parts[1] == "referencing" {
		// GET /api/v1/documents/{id}/referencing - get documents that reference this one
		h.getReferencingDocuments(w, r, meta, id)
		return
	}

	// Handle version-related routes
	if parts[1] == "versions" {
		if len(parts) == 2 {
			// GET /api/v1/documents/{id}/versions - list versions
			h.listDocumentVersions(w, r, meta, id)
			return
		}

		versionNum, err := strconv.Atoi(parts[2])
		if err != nil {
			respondError(w, http.StatusBadRequest, errors.New("invalid version number"))
			return
		}

		if len(parts) == 3 {
			// GET /api/v1/documents/{id}/versions/{version_number} - get specific version
			h.getDocumentVersion(w, r, meta, id, versionNum)
			return
		}

		if len(parts) == 4 {
			switch parts[3] {
			case "diff":
				// GET /api/v1/documents/{id}/versions/{version_number}/diff?to={to_version}
				h.getDocumentVersionDiff(w, r, meta, id, versionNum)
			case "restore":
				// POST /api/v1/documents/{id}/versions/{version_number}/restore
				h.restoreDocumentVersion(w, r, meta, id, versionNum)
			default:
				respondError(w, http.StatusNotFound, errors.New("not found"))
			}
			return
		}
	}

	respondError(w, http.StatusNotFound, errors.New("not found"))
}

func (h *Handler) handleDocumentItem(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, id int64) {
	switch r.Method {
	case http.MethodGet:
		h.getDocument(w, r, meta, id)
	case http.MethodPut:
		h.updateDocument(w, r, meta, id)
	case http.MethodDelete:
		h.deleteDocument(w, r, meta, id)
	default:
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (h *Handler) updateDocument(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, id int64) {
	var payload service.DocumentUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondAPIError(w, NewAPIError(ErrCodeValidation, http.StatusBadRequest, "请求格式错误", err.Error()))
		return
	}
	doc, err := h.service.UpdateDocument(r.Context(), meta, id, payload)
	if err != nil {
		respondAPIError(w, WrapUpstreamError(err))
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (h *Handler) getDocument(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, id int64) {
	doc, err := h.service.GetDocument(r.Context(), meta, id)
	if err != nil {
		respondAPIError(w, WrapUpstreamError(err))
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (h *Handler) deleteDocument(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, id int64) {
	// 权限检查：校对员不能删除文档
	_, httpErr := h.requireNotProofreader(r, "delete documents")
	if httpErr != nil {
		respondError(w, httpErr.code, httpErr.message)
		return
	}

	if err := h.service.DeleteDocument(r.Context(), meta, id); err != nil {
		respondAPIError(w, WrapUpstreamError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) restoreDocument(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, id int64) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	doc, err := h.service.RestoreDocument(r.Context(), meta, id)
	if err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (h *Handler) purgeDocument(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, id int64) {
	if r.Method != http.MethodDelete {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	if err := h.service.PurgeDocument(r.Context(), meta, id); err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getDocumentBindingStatus(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, id int64) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	status, err := h.service.GetDocumentBindingStatus(r.Context(), meta, id)
	if err != nil {
		respondAPIError(w, WrapUpstreamError(err))
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) reorderDocuments(w http.ResponseWriter, r *http.Request, meta service.RequestMeta) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var payload service.DocumentReorderRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	docs, err := h.service.ReorderDocuments(r.Context(), meta, payload)
	if err != nil {
		if errors.Is(err, service.ErrInvalidDocumentReorder) {
			respondError(w, http.StatusBadRequest, err)
			return
		}
		var ndrErr *ndrclient.Error
		if errors.As(err, &ndrErr) {
			switch ndrErr.StatusCode {
			case http.StatusBadRequest:
				respondError(w, http.StatusBadRequest, err)
			case http.StatusNotFound:
				respondError(w, http.StatusNotFound, err)
			default:
				respondError(w, http.StatusBadGateway, err)
			}
			return
		}
		respondError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, docs)
}

func (h *Handler) listDeletedDocuments(w http.ResponseWriter, r *http.Request, meta service.RequestMeta) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	page, err := h.service.ListDeletedDocuments(r.Context(), meta, cloneQuery(r.URL.Query()))
	if err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) listDocumentVersions(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, docID int64) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}

	page := 1
	size := 20
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if sizeStr := r.URL.Query().Get("size"); sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 {
			size = s
		}
	}

	versionsPage, err := h.service.ListDocumentVersions(r.Context(), meta, docID, page, size)
	if err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, versionsPage)
}

func (h *Handler) getDocumentVersion(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, docID int64, versionNum int) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}

	version, err := h.service.GetDocumentVersion(r.Context(), meta, docID, versionNum)
	if err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, version)
}

func (h *Handler) getDocumentVersionDiff(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, docID int64, fromVersion int) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}

	toVersionStr := r.URL.Query().Get("to")
	if toVersionStr == "" {
		respondError(w, http.StatusBadRequest, errors.New("to version parameter is required"))
		return
	}

	toVersion, err := strconv.Atoi(toVersionStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid to version"))
		return
	}

	diff, err := h.service.GetDocumentVersionDiff(r.Context(), meta, docID, fromVersion, toVersion)
	if err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, diff)
}

func (h *Handler) restoreDocumentVersion(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, docID int64, versionNum int) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}

	doc, err := h.service.RestoreDocumentVersion(r.Context(), meta, docID, versionNum)
	if err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

// addDocumentReference handles adding a reference to another document.
func (h *Handler) addDocumentReference(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, docID int64) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}

	// 权限检查：校对员不能添加引用（因为会修改 metadata）
	_, httpErr := h.requireNotProofreader(r, "add document references")
	if httpErr != nil {
		respondError(w, httpErr.code, httpErr.message)
		return
	}

	var payload struct {
		DocumentID int64 `json:"document_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondAPIError(w, NewAPIError(ErrCodeValidation, http.StatusBadRequest, "请求格式错误", err.Error()))
		return
	}

	if payload.DocumentID == 0 {
		respondAPIError(w, NewAPIError(ErrCodeValidation, http.StatusBadRequest, "document_id 不能为空", ""))
		return
	}

	doc, err := h.service.AddDocumentReference(r.Context(), meta, docID, payload.DocumentID)
	if err != nil {
		respondAPIError(w, WrapUpstreamError(err))
		return
	}

	writeJSON(w, http.StatusOK, doc)
}

// removeDocumentReference handles removing a reference from a document.
func (h *Handler) removeDocumentReference(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, docID int64, refDocID int64) {
	if r.Method != http.MethodDelete {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}

	// 权限检查：校对员不能删除引用
	_, httpErr := h.requireNotProofreader(r, "remove document references")
	if httpErr != nil {
		respondError(w, httpErr.code, httpErr.message)
		return
	}

	doc, err := h.service.RemoveDocumentReference(r.Context(), meta, docID, refDocID)
	if err != nil {
		respondAPIError(w, WrapUpstreamError(err))
		return
	}

	writeJSON(w, http.StatusOK, doc)
}

// getReferencingDocuments handles retrieving documents that reference the given document.
func (h *Handler) getReferencingDocuments(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, docID int64) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}

	docs, err := h.service.GetReferencingDocuments(r.Context(), meta, docID, cloneQuery(r.URL.Query()))
	if err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"referencing_documents": docs,
		"total":                 len(docs),
	})
}

// NodeRoutes handles node-related sub-resources.
func (h *Handler) NodeRoutes(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/api/v1/nodes/")
	if relPath == "" {
		respondError(w, http.StatusNotFound, errors.New("not found"))
		return
	}

	parts := strings.Split(relPath, "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid node id"))
		return
	}

	meta := h.metaFromRequest(r)

	if len(parts) == 1 {
		respondError(w, http.StatusNotFound, errors.New("not found"))
		return
	}

	switch parts[1] {
	case "subtree-documents":
		if r.Method != http.MethodGet {
			respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		page, err := h.service.ListNodeDocuments(r.Context(), meta, id, cloneQuery(r.URL.Query()))
		if err != nil {
			respondError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, page)
	case "bind":
		if len(parts) < 3 {
			respondError(w, http.StatusNotFound, errors.New("not found"))
			return
		}
		docID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			respondError(w, http.StatusBadRequest, errors.New("invalid document id"))
			return
		}
		if r.Method != http.MethodPost {
			respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		if err := h.service.BindDocument(r.Context(), meta, id, docID); err != nil {
			respondError(w, http.StatusBadGateway, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case "unbind":
		if len(parts) < 3 {
			respondError(w, http.StatusNotFound, errors.New("not found"))
			return
		}
		docID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			respondError(w, http.StatusBadRequest, errors.New("invalid document id"))
			return
		}
		if r.Method != http.MethodDelete {
			respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		if err := h.service.UnbindDocument(r.Context(), meta, id, docID); err != nil {
			respondError(w, http.StatusBadGateway, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		respondError(w, http.StatusNotFound, errors.New("not found"))
	}
}

func (h *Handler) handleCategoryItem(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, id int64) {
	switch r.Method {
	case http.MethodGet:
		h.getCategory(w, r, meta, id)
	case http.MethodPatch:
		h.updateCategory(w, r, meta, id)
	case http.MethodDelete:
		h.deleteCategory(w, r, meta, id)
	default:
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (h *Handler) createCategory(w http.ResponseWriter, r *http.Request, meta service.RequestMeta) {
	// 权限检查：校对员不能创建分类（节点）
	_, httpErr := h.requireNotProofreader(r, "create categories")
	if httpErr != nil {
		respondError(w, httpErr.code, httpErr.message)
		return
	}

	var payload service.CategoryCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondAPIError(w, NewAPIError(ErrCodeValidation, http.StatusBadRequest, "请求格式错误", err.Error()))
		return
	}
	category, err := h.service.CreateCategory(r.Context(), meta, payload)
	if err != nil {
		respondAPIError(w, WrapUpstreamError(err))
		return
	}
	writeJSON(w, http.StatusCreated, category)
}

func (h *Handler) getCategory(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, id int64) {
	includeDeleted := r.URL.Query().Get("include_deleted") == "true"
	category, err := h.service.GetCategory(r.Context(), meta, id, includeDeleted)
	if err != nil {
		respondAPIError(w, WrapUpstreamError(err))
		return
	}
	writeJSON(w, http.StatusOK, category)
}

func (h *Handler) updateCategory(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, id int64) {
	// 权限检查：校对员不能编辑分类（节点）
	_, httpErr := h.requireNotProofreader(r, "edit categories")
	if httpErr != nil {
		respondError(w, httpErr.code, httpErr.message)
		return
	}

	var payload service.CategoryUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondAPIError(w, NewAPIError(ErrCodeValidation, http.StatusBadRequest, "请求格式错误", err.Error()))
		return
	}
	category, err := h.service.UpdateCategory(r.Context(), meta, id, payload)
	if err != nil {
		respondAPIError(w, WrapUpstreamError(err))
		return
	}
	writeJSON(w, http.StatusOK, category)
}

func (h *Handler) deleteCategory(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, id int64) {
	// 权限检查：校对员不能删除分类（节点）
	_, httpErr := h.requireNotProofreader(r, "delete categories")
	if httpErr != nil {
		respondError(w, httpErr.code, httpErr.message)
		return
	}

	if err := h.service.DeleteCategory(r.Context(), meta, id); err != nil {
		respondAPIError(w, WrapUpstreamError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) restoreCategory(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, id int64) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	category, err := h.service.RestoreCategory(r.Context(), meta, id)
	if err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, category)
}

func (h *Handler) moveCategory(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, id int64) {
	if r.Method != http.MethodPatch {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var payload service.MoveCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	category, err := h.service.MoveCategory(r.Context(), meta, id, payload)
	if err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, category)
}

func (h *Handler) listCategoryTree(w http.ResponseWriter, r *http.Request, meta service.RequestMeta) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	includeDeleted := r.URL.Query().Get("include_deleted") == "true"
	tree, err := h.service.GetCategoryTree(r.Context(), meta, includeDeleted)
	if err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, tree)
}

func (h *Handler) reorderCategories(w http.ResponseWriter, r *http.Request, meta service.RequestMeta) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var payload service.CategoryReorderRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	categories, err := h.service.ReorderCategories(r.Context(), meta, payload)
	if err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, categories)
}

func (h *Handler) listDeletedCategories(w http.ResponseWriter, r *http.Request, meta service.RequestMeta) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	items, err := h.service.GetDeletedCategories(r.Context(), meta)
	if err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) purgeCategory(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, id int64) {
	if r.Method != http.MethodDelete {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	if err := h.service.PurgeCategory(r.Context(), meta, id); err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) repositionCategory(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, id int64) {
	if r.Method != http.MethodPatch {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var payload service.CategoryRepositionRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	result, err := h.service.RepositionCategory(r.Context(), meta, id, payload)
	if err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) bulkRestoreCategories(w http.ResponseWriter, r *http.Request, meta service.RequestMeta) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var payload service.CategoryBulkIDsRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	items, err := h.service.BulkRestoreCategories(r.Context(), meta, payload.IDs)
	if err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) bulkDeleteCategories(w http.ResponseWriter, r *http.Request, meta service.RequestMeta) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var payload service.CategoryBulkIDsRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	ids, err := h.service.BulkDeleteCategories(r.Context(), meta, payload.IDs)
	if err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted_ids": ids})
}

func (h *Handler) bulkPurgeCategories(w http.ResponseWriter, r *http.Request, meta service.RequestMeta) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var payload service.CategoryBulkIDsRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	ids, err := h.service.BulkPurgeCategories(r.Context(), meta, payload.IDs)
	if err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"purged_ids": ids})
}

func (h *Handler) bulkCheckCategories(w http.ResponseWriter, r *http.Request, meta service.RequestMeta) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var payload struct {
		IDs                []int64 `json:"ids"`
		IncludeDescendants *bool   `json:"include_descendants,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	includeDesc := true
	if payload.IncludeDescendants != nil {
		includeDesc = *payload.IncludeDescendants
	}
	if len(payload.IDs) == 0 {
		respondError(w, http.StatusBadRequest, errors.New("no ids provided"))
		return
	}
	resp, err := h.service.CheckCategoryDependencies(r.Context(), meta, service.CategoryCheckRequest{
		IDs:                payload.IDs,
		IncludeDescendants: includeDesc,
	})
	if err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) bulkCopyCategories(w http.ResponseWriter, r *http.Request, meta service.RequestMeta) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var payload service.CategoryBulkCopyRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	items, err := h.service.BulkCopyCategories(r.Context(), meta, payload)
	if err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"items": items})
}

func (h *Handler) bulkMoveCategories(w http.ResponseWriter, r *http.Request, meta service.RequestMeta) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var payload service.CategoryBulkMoveRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}
	items, err := h.service.BulkMoveCategories(r.Context(), meta, payload)
	if err != nil {
		respondError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func cloneQuery(values url.Values) url.Values {
	if values == nil {
		return nil
	}
	cloned := url.Values{}
	for k, v := range values {
		cloned[k] = append([]string(nil), v...)
	}
	return cloned
}

func (h *Handler) metaFromRequest(r *http.Request) service.RequestMeta {
	apiKey := r.Header.Get("x-api-key")
	if apiKey == "" {
		apiKey = h.defaults.APIKey
	}
	userID := r.Header.Get("x-user-id")
	if userID == "" {
		userID = h.defaults.UserID
	}
	requestID := r.Header.Get("x-request-id")

	meta := service.RequestMeta{
		APIKey:    apiKey,
		UserID:    userID,
		RequestID: requestID,
		AdminKey:  headerFallback(r.Header.Get("x-admin-key"), h.defaults.AdminKey),
	}

	// 尝试从 context 获取认证用户信息（由 JWT 中间件设置）
	if user, ok := r.Context().Value(auth.UserContextKey).(*database.User); ok {
		meta.UserRole = user.Role
		meta.UserIDNumeric = user.ID
	}

	return meta
}

func headerFallback(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func respondError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// getCurrentUser 从 context 中获取当前用户
func (h *Handler) getCurrentUser(r *http.Request) (*database.User, error) {
	user, ok := r.Context().Value(auth.UserContextKey).(*database.User)
	if !ok || user == nil {
		return nil, errors.New("user not found in context")
	}
	return user, nil
}
