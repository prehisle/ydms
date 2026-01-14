import { http } from "./http";

// 工作流定义
export interface WorkflowDefinition {
  id: number;
  workflow_key: string;
  name: string;
  description: string;
  parameter_schema: Record<string, unknown>;
  enabled: boolean;
}

// 工作流运行记录
export interface WorkflowRun {
  id: number;
  workflow_key: string;
  node_id: number;
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
}

// 触发工作流请求
export interface TriggerWorkflowRequest {
  parameters?: Record<string, unknown>;
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

// 获取所有工作流运行记录（全局）
export async function listWorkflowRuns(params?: {
  node_id?: number;
  workflow_key?: string;
  status?: string;
  limit?: number;
  offset?: number;
}): Promise<WorkflowRunsResponse> {
  const searchParams = new URLSearchParams();
  if (params?.node_id) searchParams.set("node_id", String(params.node_id));
  if (params?.workflow_key) searchParams.set("workflow_key", params.workflow_key);
  if (params?.status) searchParams.set("status", params.status);
  if (params?.limit) searchParams.set("limit", String(params.limit));
  if (params?.offset) searchParams.set("offset", String(params.offset));

  const query = searchParams.toString();
  return http<WorkflowRunsResponse>(
    `/api/v1/workflows/runs${query ? `?${query}` : ""}`
  );
}
