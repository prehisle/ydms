package ndrclient

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Asset represents a file asset stored in NDR.
type Asset struct {
	ID          int64      `json:"id"`
	Filename    string     `json:"filename"`
	ContentType *string    `json:"content_type,omitempty"`
	SizeBytes   int64      `json:"size_bytes"`
	Status      string     `json:"status"`
	Bucket      string     `json:"bucket"`
	ObjectKey   string     `json:"object_key"`
	ETag        *string    `json:"etag,omitempty"`
	CreatedBy   string     `json:"created_by"`
	UpdatedBy   string     `json:"updated_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}

// AssetInitRequest is the request body for initializing a multipart upload.
type AssetInitRequest struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes"`
}

// AssetInitResponse is the response from initializing a multipart upload.
type AssetInitResponse struct {
	Asset         Asset  `json:"asset"`
	UploadID      string `json:"upload_id"`
	PartSizeBytes int    `json:"part_size_bytes"`
	ExpiresIn     int    `json:"expires_in"`
}

// AssetPartURLsRequest is the request body for getting presigned part URLs.
type AssetPartURLsRequest struct {
	PartNumbers []int `json:"part_numbers"`
}

// AssetPartURL represents a presigned URL for uploading a part.
type AssetPartURL struct {
	PartNumber int    `json:"part_number"`
	URL        string `json:"url"`
}

// AssetPartURLsResponse is the response from getting presigned part URLs.
type AssetPartURLsResponse struct {
	UploadID  string         `json:"upload_id"`
	URLs      []AssetPartURL `json:"urls"`
	ExpiresIn int            `json:"expires_in"`
}

// AssetCompletedPart represents a completed upload part.
type AssetCompletedPart struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
}

// AssetCompleteRequest is the request body for completing a multipart upload.
type AssetCompleteRequest struct {
	Parts []AssetCompletedPart `json:"parts"`
}

// AssetDownloadURLResponse is the response from getting a download URL.
type AssetDownloadURLResponse struct {
	URL       string `json:"url"`
	ExpiresIn int    `json:"expires_in"`
}

// InitMultipartUpload initializes a multipart upload session.
func (c *httpClient) InitMultipartUpload(ctx context.Context, meta RequestMeta, req AssetInitRequest) (AssetInitResponse, error) {
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/api/v1/assets/multipart/init", meta, req)
	if err != nil {
		return AssetInitResponse{}, err
	}
	var resp AssetInitResponse
	_, err = c.do(httpReq, &resp)
	return resp, err
}

// GetAssetPartURLs gets presigned URLs for uploading parts.
func (c *httpClient) GetAssetPartURLs(ctx context.Context, meta RequestMeta, assetID int64, partNumbers []int) (AssetPartURLsResponse, error) {
	endpoint := fmt.Sprintf("/api/v1/assets/%d/multipart/part-urls", assetID)
	req := AssetPartURLsRequest{PartNumbers: partNumbers}
	httpReq, err := c.newRequest(ctx, http.MethodPost, endpoint, meta, req)
	if err != nil {
		return AssetPartURLsResponse{}, err
	}
	var resp AssetPartURLsResponse
	_, err = c.do(httpReq, &resp)
	return resp, err
}

// CompleteMultipartUpload completes a multipart upload.
func (c *httpClient) CompleteMultipartUpload(ctx context.Context, meta RequestMeta, assetID int64, parts []AssetCompletedPart) (Asset, error) {
	endpoint := fmt.Sprintf("/api/v1/assets/%d/multipart/complete", assetID)
	req := AssetCompleteRequest{Parts: parts}
	httpReq, err := c.newRequest(ctx, http.MethodPost, endpoint, meta, req)
	if err != nil {
		return Asset{}, err
	}
	var resp Asset
	_, err = c.do(httpReq, &resp)
	return resp, err
}

// AbortMultipartUpload aborts a multipart upload.
func (c *httpClient) AbortMultipartUpload(ctx context.Context, meta RequestMeta, assetID int64) error {
	endpoint := fmt.Sprintf("/api/v1/assets/%d/multipart/abort", assetID)
	httpReq, err := c.newRequest(ctx, http.MethodPost, endpoint, meta, nil)
	if err != nil {
		return err
	}
	_, err = c.do(httpReq, nil)
	return err
}

// GetAsset gets asset metadata by ID.
func (c *httpClient) GetAsset(ctx context.Context, meta RequestMeta, assetID int64) (Asset, error) {
	endpoint := fmt.Sprintf("/api/v1/assets/%d", assetID)
	httpReq, err := c.newRequest(ctx, http.MethodGet, endpoint, meta, nil)
	if err != nil {
		return Asset{}, err
	}
	var resp Asset
	_, err = c.do(httpReq, &resp)
	return resp, err
}

// GetAssetDownloadURL gets a presigned download URL for an asset.
func (c *httpClient) GetAssetDownloadURL(ctx context.Context, meta RequestMeta, assetID int64) (AssetDownloadURLResponse, error) {
	endpoint := fmt.Sprintf("/api/v1/assets/%d/download-url", assetID)
	httpReq, err := c.newRequest(ctx, http.MethodGet, endpoint, meta, nil)
	if err != nil {
		return AssetDownloadURLResponse{}, err
	}
	var resp AssetDownloadURLResponse
	_, err = c.do(httpReq, &resp)
	return resp, err
}

// DeleteAsset soft-deletes an asset.
func (c *httpClient) DeleteAsset(ctx context.Context, meta RequestMeta, assetID int64) error {
	endpoint := fmt.Sprintf("/api/v1/assets/%d", assetID)
	httpReq, err := c.newRequest(ctx, http.MethodDelete, endpoint, meta, nil)
	if err != nil {
		return err
	}
	_, err = c.do(httpReq, nil)
	return err
}
