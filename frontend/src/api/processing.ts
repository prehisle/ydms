import { buildQuery, http } from "./http";

// Pipeline 定义
export interface Pipeline {
  name: string;
  label: string;
  description: string;
  deployment_name: string;
  supports_dry_run: boolean;
}

// Processing Job 定义
export interface ProcessingJob {
  id: number;
  document_id: number;
  document_version: number;
  document_title: string;
  pipeline_name: string;
  pipeline_params?: Record<string, unknown>;
  prefect_deployment_id?: string;
  prefect_flow_run_id?: string;
  status: "pending" | "running" | "completed" | "failed" | "cancelled";
  progress: number;
  result?: Record<string, unknown>;
  error_message?: string;
  idempotency_key: string;
  triggered_by_id?: number;
  dry_run: boolean;
  created_at: string;
  started_at?: string;
  completed_at?: string;
}

// 触发流水线请求
export interface TriggerPipelineRequest {
  document_id: number;
  pipeline_name: string;
  dry_run?: boolean;
  params?: Record<string, unknown>;
}

// 触发流水线响应
export interface TriggerPipelineResponse {
  job_id: number;
  message: string;
  status: string;
  prefect_flow_run_id?: string;
}

// 任务列表响应
export interface ProcessingJobsResponse {
  jobs: ProcessingJob[];
  total: number;
}

// 流水线列表响应
export interface PipelinesResponse {
  pipelines: Pipeline[];
}

// 获取可用流水线
export async function getPipelines(): Promise<PipelinesResponse> {
  return http<PipelinesResponse>("/api/v1/processing/pipelines");
}

// 触发流水线
export async function triggerPipeline(
  request: TriggerPipelineRequest
): Promise<TriggerPipelineResponse> {
  return http<TriggerPipelineResponse>("/api/v1/processing", {
    method: "POST",
    body: JSON.stringify(request),
  });
}

// 获取任务详情
export async function getProcessingJob(jobId: number): Promise<ProcessingJob> {
  return http<ProcessingJob>(`/api/v1/processing/jobs/${jobId}`);
}

// 获取文档的处理任务列表
export async function getDocumentProcessingJobs(
  documentId: number
): Promise<ProcessingJobsResponse> {
  const query = buildQuery({ document_id: documentId });
  return http<ProcessingJobsResponse>(`/api/v1/processing/jobs${query}`);
}

// 轮询任务状态（带超时）
export async function pollJobStatus(
  jobId: number,
  options?: {
    interval?: number; // 轮询间隔（毫秒），默认 2000
    timeout?: number; // 超时时间（毫秒），默认 300000（5分钟）
    onProgress?: (job: ProcessingJob) => void;
  }
): Promise<ProcessingJob> {
  const interval = options?.interval ?? 2000;
  const timeout = options?.timeout ?? 300000;
  const startTime = Date.now();

  return new Promise((resolve, reject) => {
    const poll = async () => {
      try {
        const job = await getProcessingJob(jobId);
        options?.onProgress?.(job);

        if (job.status === "completed" || job.status === "failed") {
          resolve(job);
          return;
        }

        if (Date.now() - startTime > timeout) {
          reject(new Error("轮询超时"));
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
