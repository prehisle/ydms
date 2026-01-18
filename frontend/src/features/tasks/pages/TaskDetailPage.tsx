import { type FC } from "react";
import {
  Card,
  Descriptions,
  Tag,
  Space,
  Button,
  Typography,
  Progress,
  Breadcrumb,
  Spin,
  Alert,
  Timeline,
  Divider,
  Modal,
} from "antd";
import {
  ReloadOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  LoadingOutlined,
  ClockCircleOutlined,
  FileTextOutlined,
  HomeOutlined,
  ArrowLeftOutlined,
} from "@ant-design/icons";
import { useQuery } from "@tanstack/react-query";
import { Link, useParams, useNavigate } from "react-router-dom";

import { getProcessingJob, type ProcessingJob } from "../../../api/processing";
import { getDocumentDetail } from "../../../api/documents";

const { Text, Title, Paragraph } = Typography;

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

// 流水线名称映射
const pipelineLabels: Record<string, string> = {
  generate_knowledge_overview: "生成知识概览",
  polish_document: "文档润色",
};

/**
 * 获取状态图标
 */
function getStatusIcon(status: string) {
  switch (status) {
    case "completed":
      return <CheckCircleOutlined style={{ color: "#52c41a" }} />;
    case "failed":
    case "cancelled":
      return <CloseCircleOutlined style={{ color: "#ff4d4f" }} />;
    case "running":
      return <LoadingOutlined style={{ color: "#1890ff" }} />;
    default:
      return <ClockCircleOutlined style={{ color: "#8c8c8c" }} />;
  }
}

/**
 * 格式化时间
 */
function formatDateTime(isoString?: string): string {
  if (!isoString) return "-";
  return new Date(isoString).toLocaleString("zh-CN");
}

/**
 * 计算运行时长
 */
function calculateDuration(start?: string, end?: string): string {
  if (!start) return "-";
  const startTime = new Date(start).getTime();
  const endTime = end ? new Date(end).getTime() : Date.now();
  const diffMs = endTime - startTime;
  const seconds = Math.floor(diffMs / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);

  if (hours > 0) {
    return `${hours} 小时 ${minutes % 60} 分钟`;
  }
  if (minutes > 0) {
    return `${minutes} 分钟 ${seconds % 60} 秒`;
  }
  return `${seconds} 秒`;
}

/**
 * 任务详情页面
 */
export const TaskDetailPage: FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const taskId = id ? parseInt(id, 10) : 0;

  const { data: job, isLoading, error, refetch, isFetching } = useQuery({
    queryKey: ["processing-job", taskId],
    queryFn: () => getProcessingJob(taskId),
    enabled: taskId > 0,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status === "pending" || status === "running" ? 3000 : false;
    },
  });

  if (isLoading) {
    return (
      <div style={{ padding: "24px", textAlign: "center" }}>
        <Spin size="large" />
        <div style={{ marginTop: 16 }}>
          <Text type="secondary">加载任务详情...</Text>
        </div>
      </div>
    );
  }

  if (error || !job) {
    return (
      <div style={{ padding: "24px" }}>
        <Alert
          type="error"
          message="加载失败"
          description={(error as Error)?.message || "未找到任务"}
          showIcon
          action={
            <Button size="small" onClick={() => navigate("/tasks")}>
              返回列表
            </Button>
          }
        />
      </div>
    );
  }

  // 构建时间线
  const timelineItems = [];
  if (job.created_at) {
    timelineItems.push({
      color: "gray",
      children: (
        <div>
          <Text strong>任务创建</Text>
          <br />
          <Text type="secondary">{formatDateTime(job.created_at)}</Text>
        </div>
      ),
    });
  }
  if (job.started_at) {
    timelineItems.push({
      color: "blue",
      children: (
        <div>
          <Text strong>开始执行</Text>
          <br />
          <Text type="secondary">{formatDateTime(job.started_at)}</Text>
        </div>
      ),
    });
  }
  if (job.completed_at) {
    timelineItems.push({
      color: job.status === "completed" ? "green" : "red",
      children: (
        <div>
          <Text strong>{job.status === "completed" ? "执行完成" : "执行失败"}</Text>
          <br />
          <Text type="secondary">{formatDateTime(job.completed_at)}</Text>
        </div>
      ),
    });
  }

  // 查看文档
  const handleViewDocument = async (docId: number) => {
    try {
      await getDocumentDetail(docId);
      navigate(`/documents/${docId}/edit`);
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : "";
      const isNotFound = errorMessage.includes("[404]") || errorMessage.includes("404");
      Modal.warning({
        title: isNotFound ? "文档不可用" : "加载失败",
        content: isNotFound
          ? "该文档可能已删除，可在回收站查找。"
          : "无法加载文档，请稍后重试。",
        okText: "知道了",
      });
    }
  };

  return (
    <div style={{ padding: "24px", height: "100%", overflow: "auto" }}>
      <Space direction="vertical" size="large" style={{ width: "100%" }}>
        <Breadcrumb
          items={[
            { title: <Link to="/"><HomeOutlined /></Link> },
            { title: <Link to="/tasks">任务中心</Link> },
            { title: `任务 #${taskId}` },
          ]}
        />

        <Card
          title={
            <Space>
              <Button
                type="text"
                icon={<ArrowLeftOutlined />}
                onClick={() => navigate("/tasks")}
              />
              <Title level={4} style={{ margin: 0 }}>
                任务详情 #{taskId}
              </Title>
              <Tag icon={getStatusIcon(job.status)} color={statusColors[job.status]}>
                {statusLabels[job.status]}
              </Tag>
              {job.dry_run && <Tag color="orange">预览模式</Tag>}
            </Space>
          }
          extra={
            <Button
              icon={<ReloadOutlined spin={isFetching} />}
              onClick={() => refetch()}
              disabled={isFetching}
            >
              刷新
            </Button>
          }
        >
          <Space direction="vertical" size="large" style={{ width: "100%" }}>
            {/* 进度条 */}
            <div>
              <Text strong>执行进度</Text>
              <Progress
                percent={job.status === "completed" ? 100 : job.progress}
                status={
                  job.status === "completed"
                    ? "success"
                    : job.status === "failed" || job.status === "cancelled"
                    ? "exception"
                    : "active"
                }
                style={{ marginTop: 8 }}
              />
            </div>

            <Divider />

            {/* 基本信息 */}
            <Descriptions title="基本信息" bordered column={2}>
              <Descriptions.Item label="任务 ID">{job.id}</Descriptions.Item>
              <Descriptions.Item label="流水线">
                {pipelineLabels[job.pipeline_name] || job.pipeline_name}
              </Descriptions.Item>
              <Descriptions.Item label="文档">
                <Space>
                  <FileTextOutlined />
                  <Button
                    type="link"
                    onClick={() => handleViewDocument(job.document_id)}
                    style={{ padding: 0 }}
                  >
                    {job.document_title}
                  </Button>
                  <Text type="secondary">(ID: {job.document_id})</Text>
                </Space>
              </Descriptions.Item>
              <Descriptions.Item label="文档版本">v{job.document_version}</Descriptions.Item>
              <Descriptions.Item label="运行时长">
                {calculateDuration(job.started_at, job.completed_at)}
              </Descriptions.Item>
              <Descriptions.Item label="Prefect Flow Run">
                {job.prefect_flow_run_id || "-"}
              </Descriptions.Item>
            </Descriptions>

            {/* 时间线 */}
            <div>
              <Text strong>执行时间线</Text>
              <div style={{ marginTop: 16 }}>
                <Timeline items={timelineItems} />
              </div>
            </div>

            {/* 流水线参数 */}
            {job.pipeline_params && Object.keys(job.pipeline_params).length > 0 && (
              <div>
                <Text strong>流水线参数</Text>
                <Paragraph>
                  <pre style={{ background: "#f5f5f5", padding: 12, borderRadius: 4 }}>
                    {JSON.stringify(job.pipeline_params, null, 2)}
                  </pre>
                </Paragraph>
              </div>
            )}

            {/* 错误信息 */}
            {job.error_message && (
              <Alert type="error" message="错误信息" description={job.error_message} showIcon />
            )}

            {/* 执行结果 */}
            {job.result && Object.keys(job.result).length > 0 && (
              <div>
                <Text strong>执行结果</Text>
                <Paragraph>
                  <pre style={{ background: "#f5f5f5", padding: 12, borderRadius: 4 }}>
                    {JSON.stringify(job.result, null, 2)}
                  </pre>
                </Paragraph>
              </div>
            )}
          </Space>
        </Card>
      </Space>
    </div>
  );
};
