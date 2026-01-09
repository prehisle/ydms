package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/yjxt/ydms/backend/internal/ndrclient"
	"github.com/yjxt/ydms/backend/internal/service"
)

// AssetsHandler handles asset-related HTTP requests.
type AssetsHandler struct {
	service  *service.Service
	defaults HeaderDefaults
}

// NewAssetsHandler creates a new AssetsHandler.
func NewAssetsHandler(svc *service.Service, defaults HeaderDefaults) *AssetsHandler {
	return &AssetsHandler{
		service:  svc,
		defaults: defaults,
	}
}

// Assets handles the /api/v1/assets endpoint.
func (h *AssetsHandler) Assets(w http.ResponseWriter, r *http.Request) {
	respondError(w, http.StatusMethodNotAllowed, errors.New("use specific asset endpoints"))
}

// AssetRoutes handles asset-related routes.
func (h *AssetsHandler) AssetRoutes(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/api/v1/assets/")
	if relPath == "" {
		h.Assets(w, r)
		return
	}

	meta := h.metaFromRequest(r)

	// POST /api/v1/assets/multipart/init
	if relPath == "multipart/init" && r.Method == http.MethodPost {
		h.initMultipartUpload(w, r, meta)
		return
	}

	// Parse asset ID from path
	parts := strings.Split(relPath, "/")
	if len(parts) < 1 {
		respondError(w, http.StatusNotFound, errors.New("not found"))
		return
	}

	assetID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, errors.New("invalid asset id"))
		return
	}

	if len(parts) == 1 {
		// GET /api/v1/assets/:id
		if r.Method == http.MethodGet {
			h.getAsset(w, r, meta, assetID)
			return
		}
		// DELETE /api/v1/assets/:id
		if r.Method == http.MethodDelete {
			h.deleteAsset(w, r, meta, assetID)
			return
		}
		respondError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}

	// Route based on suffix
	suffix := strings.Join(parts[1:], "/")
	switch suffix {
	case "multipart/part-urls":
		if r.Method == http.MethodPost {
			h.getPartURLs(w, r, meta, assetID)
			return
		}
	case "multipart/complete":
		if r.Method == http.MethodPost {
			h.completeMultipartUpload(w, r, meta, assetID)
			return
		}
	case "multipart/abort":
		if r.Method == http.MethodPost {
			h.abortMultipartUpload(w, r, meta, assetID)
			return
		}
	case "download-url":
		if r.Method == http.MethodGet {
			h.getDownloadURL(w, r, meta, assetID)
			return
		}
	}

	respondError(w, http.StatusNotFound, errors.New("not found"))
}

func (h *AssetsHandler) metaFromRequest(r *http.Request) service.RequestMeta {
	apiKey := r.Header.Get("x-api-key")
	if apiKey == "" {
		apiKey = h.defaults.APIKey
	}
	userID := r.Header.Get("x-user-id")
	if userID == "" {
		userID = h.defaults.UserID
	}
	adminKey := r.Header.Get("x-admin-key")
	if adminKey == "" {
		adminKey = h.defaults.AdminKey
	}
	return service.RequestMeta{
		APIKey:    apiKey,
		UserID:    userID,
		RequestID: r.Header.Get("x-request-id"),
		AdminKey:  adminKey,
	}
}

// initMultipartUpload handles POST /api/v1/assets/multipart/init
func (h *AssetsHandler) initMultipartUpload(w http.ResponseWriter, r *http.Request, meta service.RequestMeta) {
	var req ndrclient.AssetInitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}

	resp, err := h.service.InitMultipartUpload(r.Context(), meta, req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

// getPartURLs handles POST /api/v1/assets/:id/multipart/part-urls
func (h *AssetsHandler) getPartURLs(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, assetID int64) {
	var req struct {
		PartNumbers []int `json:"part_numbers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}

	resp, err := h.service.GetAssetPartURLs(r.Context(), meta, assetID, req.PartNumbers)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// completeMultipartUpload handles POST /api/v1/assets/:id/multipart/complete
func (h *AssetsHandler) completeMultipartUpload(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, assetID int64) {
	var req struct {
		Parts []ndrclient.AssetCompletedPart `json:"parts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}

	asset, err := h.service.CompleteMultipartUpload(r.Context(), meta, assetID, req.Parts)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, asset)
}

// abortMultipartUpload handles POST /api/v1/assets/:id/multipart/abort
func (h *AssetsHandler) abortMultipartUpload(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, assetID int64) {
	err := h.service.AbortMultipartUpload(r.Context(), meta, assetID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// getAsset handles GET /api/v1/assets/:id
func (h *AssetsHandler) getAsset(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, assetID int64) {
	asset, err := h.service.GetAsset(r.Context(), meta, assetID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, asset)
}

// getDownloadURL handles GET /api/v1/assets/:id/download-url
func (h *AssetsHandler) getDownloadURL(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, assetID int64) {
	resp, err := h.service.GetAssetDownloadURL(r.Context(), meta, assetID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// deleteAsset handles DELETE /api/v1/assets/:id
func (h *AssetsHandler) deleteAsset(w http.ResponseWriter, r *http.Request, meta service.RequestMeta, assetID int64) {
	err := h.service.DeleteAsset(r.Context(), meta, assetID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleServiceError maps service errors to HTTP status codes.
func handleServiceError(w http.ResponseWriter, err error) {
	var ndrErr *ndrclient.Error
	if errors.As(err, &ndrErr) {
		respondError(w, ndrErr.StatusCode, err)
		return
	}
	respondError(w, http.StatusInternalServerError, err)
}
