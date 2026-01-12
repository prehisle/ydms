import { type FC, useState, useEffect, useCallback } from "react";
import {
  Button,
  Dropdown,
  Modal,
  Progress,
  Space,
  Spin,
  Typography,
  Tag,
  Alert,
  message,
} from "antd";
import {
  RobotOutlined,
  PlayCircleOutlined,
  EyeOutlined,
  HistoryOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  LoadingOutlined,
} from "@ant-design/icons";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import type { MenuProps } from "antd";

import {
  getPipelines,
  triggerPipeline,
  getDocumentProcessingJobs,
  getProcessingJob,
  type Pipeline,
  type ProcessingJob,
  type TriggerPipelineRequest,
} from "../../../api/processing";

const { Text, Paragraph } = Typography;

interface AIProcessingButtonProps {
  documentId: number;
  documentTitle?: string;
  disabled?: boolean;
}

// 任务状态到标签颜色的映射
const statusColors: Record<string, string> = {
  pending: "default",
  running: "processing",
  completed: "success",
  failed: "error",
  cancelled: "warning",
};

// 任务状态到中文名的映射
const statusLabels: Record<string, string> = {
  pending: "等待中",
  running: "运行中",
  completed: "已完成",
  failed: "失败",
  cancelled: "已取消",
};

export const AIProcessingButton: FC<AIProcessingButtonProps> = ({
  documentId,
  documentTitle,
  disabled,
}) => {
  const queryClient = useQueryClient();
  const [historyModalOpen, setHistoryModalOpen] = useState(false);
  const [progressModalOpen, setProgressModalOpen] = useState(false);
  const [currentJobId, setCurrentJobId] = useState<number | null>(null);

  // 获取可用流水线
  const { data: pipelinesData, isLoading: loadingPipelines } = useQuery({
    queryKey: ["processing-pipelines"],
    queryFn: getPipelines,
    staleTime: 5 * 60 * 1000, // 5分钟缓存
  });

  // 获取当前文档的处理历史
  const {
    data: jobsData,
    refetch: refetchJobs,
  } = useQuery({
    queryKey: ["processing-jobs", documentId],
    queryFn: () => getDocumentProcessingJobs(documentId),
    enabled: historyModalOpen,
  });

  // 轮询当前任务状态
  const { data: currentJob } = useQuery({
    queryKey: ["processing-job", currentJobId],
    queryFn: () => getProcessingJob(currentJobId!),
    enabled: currentJobId !== null && progressModalOpen,
    refetchInterval: (query) => {
      const job = query.state.data;
      if (!job) return 2000;
      if (job.status === "completed" || job.status === "failed") {
        return false; // 停止轮询
      }
      return 2000; // 每2秒轮询一次
    },
  });

  // 当任务完成或失败时显示消息
  useEffect(() => {
    if (!currentJob) return;
    if (currentJob.status === "completed") {
      message.success("AI 处理完成");
      setProgressModalOpen(false);
      queryClient.invalidateQueries({ queryKey: ["document-detail", documentId] });
    } else if (currentJob.status === "failed") {
      message.error(`AI 处理失败: ${currentJob.error_message || "未知错误"}`);
    }
  }, [currentJob?.status, currentJob?.error_message, documentId, queryClient]);

  // 触发流水线
  const triggerMutation = useMutation({
    mutationFn: (request: TriggerPipelineRequest) => triggerPipeline(request),
    onSuccess: (response) => {
      setCurrentJobId(response.job_id);
      setProgressModalOpen(true);
      message.info("AI 处理任务已提交");
    },
    onError: (error: Error) => {
      message.error(`提交失败: ${error.message}`);
    },
  });

  // 处理菜单点击
  const handleMenuClick = useCallback(
    (pipelineName: string, dryRun: boolean) => {
      triggerMutation.mutate({
        document_id: documentId,
        pipeline_name: pipelineName,
        dry_run: dryRun,
      });
    },
    [documentId, triggerMutation]
  );

  // 构建下拉菜单
  const buildMenuItems = useCallback((): MenuProps["items"] => {
    if (loadingPipelines) {
      return [
        {
          key: "loading",
          label: <Spin size="small" />,
          disabled: true,
        },
      ];
    }

    const pipelines = pipelinesData?.pipelines || [];
    if (pipelines.length === 0) {
      return [
        {
          key: "empty",
          label: "暂无可用流水线",
          disabled: true,
        },
      ];
    }

    const items: MenuProps["items"] = [];

    pipelines.forEach((pipeline: Pipeline, index: number) => {
      if (index > 0) {
        items.push({ type: "divider" });
      }

      // 流水线标题
      items.push({
        key: `title-${pipeline.name}`,
        label: (
          <div>
            <Text strong>{pipeline.label}</Text>
            <br />
            <Text type="secondary" style={{ fontSize: 12 }}>
              {pipeline.description}
            </Text>
          </div>
        ),
        disabled: true,
      });

      // 预览模式
      if (pipeline.supports_dry_run) {
        items.push({
          key: `preview-${pipeline.name}`,
          icon: <EyeOutlined />,
          label: "预览（不保存）",
          onClick: () => handleMenuClick(pipeline.name, true),
        });
      }

      // 执行模式
      items.push({
        key: `execute-${pipeline.name}`,
        icon: <PlayCircleOutlined />,
        label: "执行并保存",
        onClick: () => handleMenuClick(pipeline.name, false),
      });
    });

    // 历史记录入口
    items.push({ type: "divider" });
    items.push({
      key: "history",
      icon: <HistoryOutlined />,
      label: "处理历史",
      onClick: () => setHistoryModalOpen(true),
    });

    return items;
  }, [loadingPipelines, pipelinesData, handleMenuClick]);

  // 格式化时间
  const formatTime = (isoString?: string) => {
    if (!isoString) return "-";
    return new Date(isoString).toLocaleString("zh-CN");
  };

  return (
    <>
      <Dropdown
        menu={{ items: buildMenuItems() }}
        trigger={["click"]}
        disabled={disabled || triggerMutation.isPending}
      >
        <Button
          icon={
            triggerMutation.isPending ? <LoadingOutlined /> : <RobotOutlined />
          }
          loading={triggerMutation.isPending}
        >
          AI 处理
        </Button>
      </Dropdown>

      {/* 进度模态框 */}
      <Modal
        title="AI 处理进度"
        open={progressModalOpen}
        onCancel={() => setProgressModalOpen(false)}
        footer={null}
        maskClosable={false}
      >
        {currentJob ? (
          <Space direction="vertical" style={{ width: "100%" }} size="middle">
            <div>
              <Text strong>文档：</Text>
              <Text>{documentTitle || `ID: ${documentId}`}</Text>
            </div>
            <div>
              <Text strong>流水线：</Text>
              <Text>{currentJob.pipeline_name}</Text>
              {currentJob.dry_run && (
                <Tag color="orange" style={{ marginLeft: 8 }}>
                  预览模式
                </Tag>
              )}
            </div>
            <div>
              <Text strong>状态：</Text>
              <Tag color={statusColors[currentJob.status]}>
                {statusLabels[currentJob.status]}
              </Tag>
            </div>
            <Progress
              percent={currentJob.progress}
              status={
                currentJob.status === "failed"
                  ? "exception"
                  : currentJob.status === "completed"
                  ? "success"
                  : "active"
              }
            />
            {currentJob.error_message && (
              <Alert
                type="error"
                message="错误信息"
                description={currentJob.error_message}
                showIcon
              />
            )}
            {currentJob.status === "completed" && currentJob.result && (
              <Alert
                type="success"
                message="处理完成"
                description={
                  <pre style={{ margin: 0, fontSize: 12 }}>
                    {JSON.stringify(currentJob.result, null, 2)}
                  </pre>
                }
                showIcon
              />
            )}
          </Space>
        ) : (
          <div style={{ textAlign: "center", padding: 24 }}>
            <Spin size="large" />
            <Paragraph style={{ marginTop: 16 }}>正在提交任务...</Paragraph>
          </div>
        )}
      </Modal>

      {/* 历史记录模态框 */}
      <Modal
        title="AI 处理历史"
        open={historyModalOpen}
        onCancel={() => setHistoryModalOpen(false)}
        footer={
          <Button onClick={() => refetchJobs()}>刷新</Button>
        }
        width={700}
      >
        {jobsData?.jobs && jobsData.jobs.length > 0 ? (
          <Space direction="vertical" style={{ width: "100%" }} size="small">
            {jobsData.jobs.map((job: ProcessingJob) => (
              <div
                key={job.id}
                style={{
                  padding: 12,
                  border: "1px solid #f0f0f0",
                  borderRadius: 6,
                }}
              >
                <Space style={{ width: "100%", justifyContent: "space-between" }}>
                  <Space>
                    {job.status === "completed" && (
                      <CheckCircleOutlined style={{ color: "#52c41a" }} />
                    )}
                    {job.status === "failed" && (
                      <CloseCircleOutlined style={{ color: "#ff4d4f" }} />
                    )}
                    {job.status === "running" && (
                      <LoadingOutlined style={{ color: "#1890ff" }} />
                    )}
                    <Text strong>{job.pipeline_name}</Text>
                    {job.dry_run && <Tag color="orange">预览</Tag>}
                    <Tag color={statusColors[job.status]}>
                      {statusLabels[job.status]}
                    </Tag>
                  </Space>
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    {formatTime(job.created_at)}
                  </Text>
                </Space>
                {job.error_message && (
                  <Paragraph
                    type="danger"
                    style={{ marginTop: 8, marginBottom: 0, fontSize: 12 }}
                    ellipsis={{ rows: 2 }}
                  >
                    {job.error_message}
                  </Paragraph>
                )}
              </div>
            ))}
          </Space>
        ) : (
          <div style={{ textAlign: "center", padding: 24 }}>
            <Text type="secondary">暂无处理历史</Text>
          </div>
        )}
      </Modal>
    </>
  );
};
