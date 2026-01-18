import { http } from "./http";

// 同步目标配置
export interface SyncTarget {
  record_id: number;
  table?: string;      // 可选：自定义处理器可能不需要
  field?: string;      // 可选：自定义处理器可能不需要
  connection?: string; // 可选：数据库连接名
}

// 最后同步信息
export interface LastSync {
  event_id?: string;
  version: number;
  status: "pending" | "success" | "failed" | "skipped";
  error?: string;
  run_id?: string;
  synced_at?: string;
}

// 同步状态响应
export interface SyncStatusResponse {
  document_id: number;
  sync_target?: SyncTarget;
  last_sync?: LastSync;
  sync_enabled: boolean;
}

// 触发同步响应
export interface TriggerSyncResponse {
  event_id: string;
  status: string;
  message?: string;
  document_id: number;
  document_version: number;
  prefect_flow_run_id?: string;
  sync_target?: SyncTarget;
  idempotency_key?: string;
}

// 获取文档同步状态
export async function getSyncStatus(
  documentId: number
): Promise<SyncStatusResponse> {
  return http<SyncStatusResponse>(
    `/api/v1/documents/${documentId}/sync-status`
  );
}

// 触发文档同步
export async function triggerSync(
  documentId: number
): Promise<TriggerSyncResponse> {
  return http<TriggerSyncResponse>(`/api/v1/documents/${documentId}/sync`, {
    method: "POST",
  });
}

// 轮询同步状态（带超时）
export async function pollSyncStatus(
  documentId: number,
  options?: {
    interval?: number; // 轮询间隔（毫秒），默认 2000
    timeout?: number; // 超时时间（毫秒），默认 120000（2分钟）
    onProgress?: (status: SyncStatusResponse) => void;
  }
): Promise<SyncStatusResponse> {
  const interval = options?.interval ?? 2000;
  const timeout = options?.timeout ?? 120000;
  const startTime = Date.now();

  return new Promise((resolve, reject) => {
    const poll = async () => {
      try {
        const status = await getSyncStatus(documentId);
        options?.onProgress?.(status);

        const lastStatus = status.last_sync?.status;
        if (lastStatus === "success" || lastStatus === "failed" || lastStatus === "skipped") {
          resolve(status);
          return;
        }

        if (Date.now() - startTime > timeout) {
          reject(new Error("同步轮询超时"));
          return;
        }

        setTimeout(poll, interval);
      } catch (error) {
        reject(error);
      }
    };

    poll();
  });
}
