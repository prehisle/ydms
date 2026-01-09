package service

import (
	"context"

	"github.com/yjxt/ydms/backend/internal/ndrclient"
)

// InitMultipartUpload initializes a multipart upload session.
func (s *Service) InitMultipartUpload(ctx context.Context, meta RequestMeta, req ndrclient.AssetInitRequest) (ndrclient.AssetInitResponse, error) {
	return s.ndr.InitMultipartUpload(ctx, toNDRMeta(meta), req)
}

// GetAssetPartURLs gets presigned URLs for uploading parts.
func (s *Service) GetAssetPartURLs(ctx context.Context, meta RequestMeta, assetID int64, partNumbers []int) (ndrclient.AssetPartURLsResponse, error) {
	return s.ndr.GetAssetPartURLs(ctx, toNDRMeta(meta), assetID, partNumbers)
}

// CompleteMultipartUpload completes a multipart upload.
func (s *Service) CompleteMultipartUpload(ctx context.Context, meta RequestMeta, assetID int64, parts []ndrclient.AssetCompletedPart) (ndrclient.Asset, error) {
	return s.ndr.CompleteMultipartUpload(ctx, toNDRMeta(meta), assetID, parts)
}

// AbortMultipartUpload aborts a multipart upload.
func (s *Service) AbortMultipartUpload(ctx context.Context, meta RequestMeta, assetID int64) error {
	return s.ndr.AbortMultipartUpload(ctx, toNDRMeta(meta), assetID)
}

// GetAsset gets asset metadata by ID.
func (s *Service) GetAsset(ctx context.Context, meta RequestMeta, assetID int64) (ndrclient.Asset, error) {
	return s.ndr.GetAsset(ctx, toNDRMeta(meta), assetID)
}

// GetAssetDownloadURL gets a presigned download URL for an asset.
func (s *Service) GetAssetDownloadURL(ctx context.Context, meta RequestMeta, assetID int64) (ndrclient.AssetDownloadURLResponse, error) {
	return s.ndr.GetAssetDownloadURL(ctx, toNDRMeta(meta), assetID)
}

// DeleteAsset soft-deletes an asset.
func (s *Service) DeleteAsset(ctx context.Context, meta RequestMeta, assetID int64) error {
	return s.ndr.DeleteAsset(ctx, toNDRMeta(meta), assetID)
}
