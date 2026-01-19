import { useState, useCallback } from "react";
import {
  initMultipartUpload,
  getPartURLs,
  completeMultipartUpload,
  uploadPart,
  getDownloadURL,
  type CompletedPart,
} from "../api/assets";

/**
 * 上传进度信息
 */
export interface UploadProgress {
  loaded: number;
  total: number;
  percent: number;
}

/**
 * 上传结果
 */
export interface UploadResult {
  assetId: number;
  filename: string;
  contentType: string;
  downloadUrl: string;
}

/**
 * useFileUpload hook 返回值
 */
export interface UseFileUploadReturn {
  uploadFile: (file: File) => Promise<UploadResult>;
  uploading: boolean;
  progress: UploadProgress | null;
  error: Error | null;
  reset: () => void;
}

/**
 * 文件上传 Hook，支持分片上传和进度显示
 */
export function useFileUpload(): UseFileUploadReturn {
  const [uploading, setUploading] = useState(false);
  const [progress, setProgress] = useState<UploadProgress | null>(null);
  const [error, setError] = useState<Error | null>(null);

  const reset = useCallback(() => {
    setUploading(false);
    setProgress(null);
    setError(null);
  }, []);

  const uploadFile = useCallback(async (file: File): Promise<UploadResult> => {
    setUploading(true);
    setProgress({ loaded: 0, total: file.size, percent: 0 });
    setError(null);

    try {
      // 1. 初始化上传
      const initResp = await initMultipartUpload({
        filename: file.name,
        content_type: file.type || "application/octet-stream",
        size_bytes: file.size,
      });

      // 2. 计算分片
      const partSize = initResp.part_size_bytes;
      const partCount = Math.ceil(file.size / partSize);
      const partNumbers = Array.from({ length: partCount }, (_, i) => i + 1);

      // 3. 获取预签名 URL
      const urlsResp = await getPartURLs(initResp.asset.id, partNumbers);

      // 4. 上传每个分片（带进度）
      let uploadedBytes = 0;
      const completedParts: CompletedPart[] = [];

      for (const partUrl of urlsResp.urls) {
        const start = (partUrl.part_number - 1) * partSize;
        const end = Math.min(start + partSize, file.size);
        const blob = file.slice(start, end);

        const etag = await uploadPart(partUrl.url, blob, (loaded) => {
          const currentLoaded = uploadedBytes + loaded;
          setProgress({
            loaded: currentLoaded,
            total: file.size,
            percent: Math.round((currentLoaded / file.size) * 100),
          });
        });

        completedParts.push({ part_number: partUrl.part_number, etag });
        uploadedBytes += blob.size;
      }

      // 5. 完成上传
      await completeMultipartUpload(initResp.asset.id, completedParts);

      // 6. 获取下载 URL
      const downloadResp = await getDownloadURL(initResp.asset.id);

      setProgress({ loaded: file.size, total: file.size, percent: 100 });

      return {
        assetId: initResp.asset.id,
        filename: file.name,
        contentType: file.type || "application/octet-stream",
        downloadUrl: downloadResp.url,
      };
    } catch (err) {
      const uploadError = err instanceof Error ? err : new Error(String(err));
      setError(uploadError);
      throw uploadError;
    } finally {
      setUploading(false);
    }
  }, []);

  return { uploadFile, uploading, progress, error, reset };
}

/**
 * 判断是否为图片类型
 */
export function isImageContentType(contentType: string): boolean {
  return contentType.startsWith("image/");
}

/**
 * 从完整 URL 提取相对路径
 * 例: http://localhost:9005/ndr-assets/assets/12/image.png -> /ndr-assets/assets/12/image.png
 */
function extractRelativePath(url: string): string {
  try {
    const urlObj = new URL(url);
    return urlObj.pathname;
  } catch {
    // 已经是相对路径，直接返回
    return url;
  }
}

/**
 * 根据上传结果生成 Markdown 链接
 * 使用相对路径以支持跨环境访问
 */
export function formatUploadLink(result: UploadResult): string {
  const relativePath = extractRelativePath(result.downloadUrl);
  if (isImageContentType(result.contentType)) {
    return `![${result.filename}](${relativePath})`;
  }
  return `[${result.filename}](${relativePath})`;
}
