import { http } from "./http";

// ===== 批量工作流 API =====

// 节点预览项
export interface NodePreviewItem {
  node_id: number;
  node_name: string;
  node_path: string;
  source_doc_count: number;
  can_execute: boolean;
  skip_reason?: string;
  depth: number;
}

// 批量工作流预览请求
export interface BatchWorkflowPreviewRequest {
  workflow_key: string;
  include_descendants: boolean;
  skip_no_source?: boolean;
  skip_no_output?: boolean;
  skip_name_contains?: string;
  skip_doc_types?: string[];
}

// 批量工作流预览响应
export interface BatchWorkflowPreviewResponse {
  root_node_id: number;
  workflow_key: string;
  workflow_name: string;
  total_nodes: number;
  can_execute: number;
  will_skip: number;
  nodes: NodePreviewItem[];
}

// 批量工作流执行请求
export interface BatchWorkflowExecuteRequest {
  workflow_key: string;
  include_descendants: boolean;
  skip_no_source?: boolean;
  skip_no_output?: boolean;
  skip_name_contains?: string;
  skip_doc_types?: string[];
  parameters?: Record<string, unknown>;
  concurrency?: number;
}

// 批量工作流执行响应
export interface BatchWorkflowExecuteResponse {
  batch_id: string;
  status: string;
  total_nodes: number;
  message?: string;
}

// 批量工作流状态响应
export interface BatchWorkflowStatusResponse {
  batch_id: string;
  workflow_key: string;
  root_node_id: number;
  status: "pending" | "running" | "completed" | "failed" | "cancelled";
  total_nodes: number;
  success_count: number;
  failed_count: number;
  skipped_count: number;
  progress: number;
  details?: {
    node_results?: Array<{
      node_id: number;
      node_name: string;
      node_path: string;
      status: "success" | "failed" | "skipped";
      run_id?: number;
      prefect_flow_run_id?: string;
      error?: string;
      reason?: string;
    }>;
  };
  error_message?: string;
  started_at?: string;
  finished_at?: string;
  created_at: string;
}

// 批量工作流列表响应
export interface BatchWorkflowListResponse {
  items: BatchWorkflowStatusResponse[];
  total: number;
  limit: number;
  offset: number;
  has_more: boolean;
}

// 预览批量工作流
export async function previewBatchWorkflow(
  nodeId: number,
  request: BatchWorkflowPreviewRequest
): Promise<BatchWorkflowPreviewResponse> {
  return http<BatchWorkflowPreviewResponse>(
    `/api/v1/nodes/${nodeId}/workflows/batch/preview`,
    {
      method: "POST",
      body: JSON.stringify(request),
    }
  );
}

// 执行批量工作流
export async function executeBatchWorkflow(
  nodeId: number,
  request: BatchWorkflowExecuteRequest
): Promise<BatchWorkflowExecuteResponse> {
  return http<BatchWorkflowExecuteResponse>(
    `/api/v1/nodes/${nodeId}/workflows/batch/execute`,
    {
      method: "POST",
      body: JSON.stringify(request),
    }
  );
}

// 获取批量工作流状态
export async function getBatchWorkflowStatus(
  batchId: string
): Promise<BatchWorkflowStatusResponse> {
  return http<BatchWorkflowStatusResponse>(
    `/api/v1/workflows/batches/${batchId}`
  );
}

// 列出批量工作流
export async function listBatchWorkflows(params?: {
  limit?: number;
  offset?: number;
}): Promise<BatchWorkflowListResponse> {
  const searchParams = new URLSearchParams();
  if (params?.limit) searchParams.set("limit", String(params.limit));
  if (params?.offset) searchParams.set("offset", String(params.offset));

  const query = searchParams.toString();
  return http<BatchWorkflowListResponse>(
    `/api/v1/workflows/batches${query ? `?${query}` : ""}`
  );
}

// ===== 批量同步 API =====

// 同步目标配置
export interface SyncTarget {
  table?: string;
  record_id: number;
  field?: string;
  connection?: string;
}

// 文档预览项
export interface DocumentPreviewItem {
  document_id: number;
  document_name: string;
  document_type: string;
  node_id: number;
  node_path: string;
  sync_target?: SyncTarget;
  can_sync: boolean;
  skip_reason?: string;
}

// 批量同步预览请求
export interface BatchSyncPreviewRequest {
  include_descendants: boolean;
  skip_doc_types?: string[];
}

// 批量同步预览响应
export interface BatchSyncPreviewResponse {
  root_node_id: number;
  total_documents: number;
  can_sync: number;
  will_skip: number;
  documents: DocumentPreviewItem[];
}

// 批量同步执行请求
export interface BatchSyncExecuteRequest {
  include_descendants: boolean;
  concurrency?: number;
  skip_doc_types?: string[];
}

// 批量同步执行响应
export interface BatchSyncExecuteResponse {
  batch_id: string;
  status: string;
  total_documents: number;
  message?: string;
}

// 批量同步状态响应
export interface BatchSyncStatusResponse {
  batch_id: string;
  root_node_id: number;
  status: "pending" | "running" | "completed" | "failed" | "cancelled";
  total_documents: number;
  success_count: number;
  failed_count: number;
  skipped_count: number;
  progress: number;
  details?: {
    document_results?: Array<{
      document_id: number;
      document_name: string;
      document_type: string;
      node_id: number;
      node_path: string;
      status: "success" | "failed" | "skipped";
      event_id?: string;
      prefect_flow_run_id?: string;
      error?: string;
      reason?: string;
    }>;
  };
  error_message?: string;
  started_at?: string;
  finished_at?: string;
  created_at: string;
}

// 批量同步列表响应
export interface BatchSyncListResponse {
  items: BatchSyncStatusResponse[];
  total: number;
  limit: number;
  offset: number;
  has_more: boolean;
}

// 预览批量同步
export async function previewBatchSync(
  nodeId: number,
  request: BatchSyncPreviewRequest
): Promise<BatchSyncPreviewResponse> {
  return http<BatchSyncPreviewResponse>(
    `/api/v1/nodes/${nodeId}/sync/batch/preview`,
    {
      method: "POST",
      body: JSON.stringify(request),
    }
  );
}

// 执行批量同步
export async function executeBatchSync(
  nodeId: number,
  request: BatchSyncExecuteRequest
): Promise<BatchSyncExecuteResponse> {
  return http<BatchSyncExecuteResponse>(
    `/api/v1/nodes/${nodeId}/sync/batch/execute`,
    {
      method: "POST",
      body: JSON.stringify(request),
    }
  );
}

// 获取批量同步状态
export async function getBatchSyncStatus(
  batchId: string
): Promise<BatchSyncStatusResponse> {
  return http<BatchSyncStatusResponse>(`/api/v1/sync/batches/${batchId}`);
}

// 列出批量同步
export async function listBatchSyncs(params?: {
  limit?: number;
  offset?: number;
}): Promise<BatchSyncListResponse> {
  const searchParams = new URLSearchParams();
  if (params?.limit) searchParams.set("limit", String(params.limit));
  if (params?.offset) searchParams.set("offset", String(params.offset));

  const query = searchParams.toString();
  return http<BatchSyncListResponse>(
    `/api/v1/sync/batches${query ? `?${query}` : ""}`
  );
}
