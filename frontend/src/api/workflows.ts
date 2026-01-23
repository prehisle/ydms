import { http } from "./http";

// 工作流定义
export interface WorkflowDefinition {
  id: number;
  workflow_key: string;
  name: string;
  description: string;
  prefect_deployment_name: string;
  prefect_deployment_id?: string;
  prefect_version?: string;
  prefect_tags?: { tags?: string[] };
  parameter_schema: Record<string, unknown>;
  source: "prefect" | "manual";
  workflow_type: "node" | "document";
  sync_status: "active" | "missing" | "error";
  last_synced_at?: string;
  last_seen_at?: string;
  spec_hash?: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

// 工作流运行记录
export interface WorkflowRun {
  id: number;
  workflow_key: string;
  node_id?: number;
  document_id?: number;
  parameters: Record<string, unknown>;
  status: "pending" | "running" | "success" | "failed" | "cancelled";
  prefect_flow_run_id?: string;
  result?: Record<string, unknown>;
  error_message?: string;
  created_by_id?: number;
  created_by?: {
    id: number;
    username: string;
    display_name: string;
  };
  created_at: string;
  updated_at: string;
  started_at?: string;
  finished_at?: string;
  // 重试关联
  retry_of_id?: number;  // 重试来源任务 ID
  retry_count?: number;  // 被重试的次数
  latest_retry_status?: "pending" | "running" | "success" | "failed" | "cancelled";  // 最新重试状态
}

// 触发工作流请求
export interface TriggerWorkflowRequest {
  parameters?: Record<string, unknown>;
  retry_of_id?: number;  // 重试来源任务 ID
}

// 触发工作流响应
export interface TriggerWorkflowResponse {
  run_id: number;
  status: string;
  prefect_flow_run_id?: string;
  message?: string;
}

// 工作流运行列表响应
export interface WorkflowRunsResponse {
  runs: WorkflowRun[];
  total: number;
  has_more: boolean;
}

// 获取所有可用的工作流定义
export async function listWorkflowDefinitions(): Promise<WorkflowDefinition[]> {
  return http<WorkflowDefinition[]>("/api/v1/workflows");
}

// 获取节点可用的工作流（目前与全局相同）
export async function listNodeWorkflows(nodeId: number): Promise<WorkflowDefinition[]> {
  return http<WorkflowDefinition[]>(`/api/v1/nodes/${nodeId}/workflows`);
}

// 触发节点工作流
export async function triggerNodeWorkflow(
  nodeId: number,
  workflowKey: string,
  request?: TriggerWorkflowRequest
): Promise<TriggerWorkflowResponse> {
  return http<TriggerWorkflowResponse>(
    `/api/v1/nodes/${nodeId}/workflows/${workflowKey}/runs`,
    {
      method: "POST",
      body: JSON.stringify(request || {}),
    }
  );
}

// 获取节点的工作流运行历史
export async function listNodeWorkflowRuns(
  nodeId: number,
  params?: { limit?: number; offset?: number; status?: string }
): Promise<WorkflowRunsResponse> {
  const searchParams = new URLSearchParams();
  if (params?.limit) searchParams.set("limit", String(params.limit));
  if (params?.offset) searchParams.set("offset", String(params.offset));
  if (params?.status) searchParams.set("status", params.status);

  const query = searchParams.toString();
  return http<WorkflowRunsResponse>(
    `/api/v1/nodes/${nodeId}/workflow-runs${query ? `?${query}` : ""}`
  );
}

// 获取工作流运行详情
export async function getWorkflowRun(runId: number): Promise<WorkflowRun> {
  return http<WorkflowRun>(`/api/v1/workflows/runs/${runId}`);
}

// 取消工作流运行
export async function cancelWorkflowRun(runId: number): Promise<{ success: boolean; message: string }> {
  return http<{ success: boolean; message: string }>(`/api/v1/workflows/runs/${runId}/cancel`, {
    method: "POST",
  });
}

// 强制终止僵尸任务（运行超过 30 分钟的任务）
export async function forceTerminateWorkflowRun(runId: number): Promise<{ success: boolean; message: string }> {
  return http<{ success: boolean; message: string }>(`/api/v1/workflows/runs/${runId}/force-terminate`, {
    method: "POST",
  });
}

// 获取所有工作流运行记录（全局）
export async function listWorkflowRuns(params?: {
  node_id?: number;
  document_id?: number;
  workflow_key?: string;
  status?: string;
  limit?: number;
  offset?: number;
}): Promise<WorkflowRunsResponse> {
  const searchParams = new URLSearchParams();
  if (params?.node_id) searchParams.set("node_id", String(params.node_id));
  if (params?.document_id) searchParams.set("document_id", String(params.document_id));
  if (params?.workflow_key) searchParams.set("workflow_key", params.workflow_key);
  if (params?.status) searchParams.set("status", params.status);
  if (params?.limit) searchParams.set("limit", String(params.limit));
  if (params?.offset) searchParams.set("offset", String(params.offset));

  const query = searchParams.toString();
  return http<WorkflowRunsResponse>(
    `/api/v1/workflows/runs${query ? `?${query}` : ""}`
  );
}

// ===== Document Workflow API =====

// 获取文档可用的工作流定义
export async function listDocumentWorkflows(docId: number): Promise<WorkflowDefinition[]> {
  return http<WorkflowDefinition[]>(`/api/v1/documents/${docId}/workflows`);
}

// 触发文档工作流
export async function triggerDocumentWorkflow(
  docId: number,
  workflowKey: string,
  request?: TriggerWorkflowRequest
): Promise<TriggerWorkflowResponse> {
  return http<TriggerWorkflowResponse>(
    `/api/v1/documents/${docId}/workflows/${workflowKey}/runs`,
    {
      method: "POST",
      body: JSON.stringify(request || {}),
    }
  );
}

// 获取文档的工作流运行历史
export async function listDocumentWorkflowRuns(
  docId: number,
  params?: { limit?: number; offset?: number; status?: string }
): Promise<WorkflowRunsResponse> {
  const searchParams = new URLSearchParams();
  if (params?.limit) searchParams.set("limit", String(params.limit));
  if (params?.offset) searchParams.set("offset", String(params.offset));
  if (params?.status) searchParams.set("status", params.status);

  const query = searchParams.toString();
  return http<WorkflowRunsResponse>(
    `/api/v1/documents/${docId}/workflow-runs${query ? `?${query}` : ""}`
  );
}

// ===== Admin API =====

// 同步状态
export interface SyncStatus {
  last_sync_time?: string;
  status: "idle" | "in_progress" | "success" | "failed" | "completed_with_errors";
  error?: string;
  prefect_enabled: boolean;
}

// 同步结果
export interface SyncResult {
  created: number;
  updated: number;
  missing: number;
  errors?: string[];
  duration: string;
}

// 管理 API：获取同步状态
export async function getSyncStatus(): Promise<SyncStatus> {
  return http<SyncStatus>("/api/v1/admin/workflows/sync/status");
}

// 管理 API：触发同步
export async function triggerSync(): Promise<SyncResult> {
  return http<SyncResult>("/api/v1/admin/workflows/sync", {
    method: "POST",
  });
}

// 管理 API：列出所有工作流定义（带过滤）
export interface AdminWorkflowFilter {
  source?: "prefect" | "manual";
  type?: "node" | "document";
  sync_status?: "active" | "missing" | "error";
  enabled?: boolean;
}

export async function listAdminWorkflows(
  filter?: AdminWorkflowFilter
): Promise<WorkflowDefinition[]> {
  const searchParams = new URLSearchParams();
  if (filter?.source) searchParams.set("source", filter.source);
  if (filter?.type) searchParams.set("type", filter.type);
  if (filter?.sync_status) searchParams.set("sync_status", filter.sync_status);
  if (filter?.enabled !== undefined)
    searchParams.set("enabled", String(filter.enabled));

  const query = searchParams.toString();
  return http<WorkflowDefinition[]>(
    `/api/v1/admin/workflows${query ? `?${query}` : ""}`
  );
}

// 管理 API：更新工作流定义
export async function updateAdminWorkflow(
  id: number,
  data: { enabled: boolean }
): Promise<{ success: boolean; message: string }> {
  return http<{ success: boolean; message: string }>(
    `/api/v1/admin/workflows/${id}`,
    {
      method: "PATCH",
      body: JSON.stringify(data),
    }
  );
}

// ===== 清理执行历史 API =====

// 清理执行历史参数
export interface CleanupWorkflowRunsParams {
  before_date?: string;  // ISO 8601 格式或 YYYY-MM-DD
  status?: string;       // 逗号分隔的状态列表
  workflow_key?: string;
  node_id?: number;
  document_id?: number;
  include_zombie?: boolean;  // 是否包含僵尸任务
  dry_run?: boolean;     // 试运行模式
}

// 清理执行历史响应
export interface CleanupWorkflowRunsResponse {
  deleted_count: number;
  zombie_count?: number;  // 清理的僵尸任务数量
  dry_run: boolean;
}

// 清理执行历史
export async function cleanupWorkflowRuns(
  params?: CleanupWorkflowRunsParams
): Promise<CleanupWorkflowRunsResponse> {
  const searchParams = new URLSearchParams();
  if (params?.before_date) searchParams.set("before_date", params.before_date);
  if (params?.status) searchParams.set("status", params.status);
  if (params?.workflow_key) searchParams.set("workflow_key", params.workflow_key);
  if (params?.node_id) searchParams.set("node_id", String(params.node_id));
  if (params?.document_id) searchParams.set("document_id", String(params.document_id));
  if (params?.include_zombie) searchParams.set("include_zombie", "true");
  if (params?.dry_run) searchParams.set("dry_run", "true");

  const query = searchParams.toString();
  return http<CleanupWorkflowRunsResponse>(
    `/api/v1/workflows/runs${query ? `?${query}` : ""}`,
    { method: "DELETE" }
  );
}
