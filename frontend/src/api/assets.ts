import { http } from "./http";

/**
 * Asset 资产类型定义
 */
export interface Asset {
  id: number;
  filename: string;
  content_type: string | null;
  size_bytes: number;
  status: string;
  bucket: string;
  object_key: string;
  etag: string | null;
  created_by: string;
  updated_by: string;
  created_at: string;
  updated_at: string;
  deleted_at: string | null;
}

/**
 * 初始化上传请求参数
 */
export interface AssetInitRequest {
  filename: string;
  content_type?: string;
  size_bytes: number;
}

/**
 * 初始化上传响应
 */
export interface AssetInitResponse {
  asset: Asset;
  upload_id: string;
  part_size_bytes: number;
  expires_in: number;
}

/**
 * 分片 URL
 */
export interface AssetPartURL {
  part_number: number;
  url: string;
}

/**
 * 分片 URL 响应
 */
export interface AssetPartURLsResponse {
  upload_id: string;
  urls: AssetPartURL[];
  expires_in: number;
}

/**
 * 已完成的分片信息
 */
export interface CompletedPart {
  part_number: number;
  etag: string;
}

/**
 * 下载 URL 响应
 */
export interface AssetDownloadURLResponse {
  url: string;
  expires_in: number;
}

/**
 * 初始化分片上传
 */
export async function initMultipartUpload(
  params: AssetInitRequest
): Promise<AssetInitResponse> {
  return http<AssetInitResponse>("/api/v1/assets/multipart/init", {
    method: "POST",
    body: JSON.stringify(params),
  });
}

/**
 * 获取分片预签名 URL
 */
export async function getPartURLs(
  assetId: number,
  partNumbers: number[]
): Promise<AssetPartURLsResponse> {
  return http<AssetPartURLsResponse>(
    `/api/v1/assets/${assetId}/multipart/part-urls`,
    {
      method: "POST",
      body: JSON.stringify({ part_numbers: partNumbers }),
    }
  );
}

/**
 * 完成分片上传
 */
export async function completeMultipartUpload(
  assetId: number,
  parts: CompletedPart[]
): Promise<Asset> {
  return http<Asset>(`/api/v1/assets/${assetId}/multipart/complete`, {
    method: "POST",
    body: JSON.stringify({ parts }),
  });
}

/**
 * 中止分片上传
 */
export async function abortMultipartUpload(assetId: number): Promise<void> {
  return http<void>(`/api/v1/assets/${assetId}/multipart/abort`, {
    method: "POST",
  });
}

/**
 * 获取资产信息
 */
export async function getAsset(assetId: number): Promise<Asset> {
  return http<Asset>(`/api/v1/assets/${assetId}`);
}

/**
 * 获取下载 URL
 */
export async function getDownloadURL(
  assetId: number
): Promise<AssetDownloadURLResponse> {
  return http<AssetDownloadURLResponse>(
    `/api/v1/assets/${assetId}/download-url`
  );
}

/**
 * 删除资产
 */
export async function deleteAsset(assetId: number): Promise<void> {
  return http<void>(`/api/v1/assets/${assetId}`, {
    method: "DELETE",
  });
}

/**
 * 上传单个分片到 S3 (直接使用 presigned URL，不通过 http())
 * @param url presigned URL
 * @param blob 文件数据
 * @param onProgress 进度回调
 * @returns ETag
 */
export async function uploadPart(
  url: string,
  blob: Blob,
  onProgress?: (loaded: number) => void
): Promise<string> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();

    xhr.upload.addEventListener("progress", (event) => {
      if (event.lengthComputable && onProgress) {
        onProgress(event.loaded);
      }
    });

    xhr.addEventListener("load", () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        // S3 返回的 ETag 在响应头中
        const etag = xhr.getResponseHeader("ETag");
        if (etag) {
          // 移除引号
          resolve(etag.replace(/"/g, ""));
        } else {
          reject(new Error("Missing ETag in response"));
        }
      } else {
        reject(new Error(`Upload failed with status ${xhr.status}`));
      }
    });

    xhr.addEventListener("error", () => {
      reject(new Error("Upload failed"));
    });

    xhr.addEventListener("abort", () => {
      reject(new Error("Upload aborted"));
    });

    xhr.open("PUT", url);
    xhr.send(blob);
  });
}
