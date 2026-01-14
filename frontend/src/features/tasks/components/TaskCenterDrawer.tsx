import { type FC, useState, useCallback } from "react";
import {
  Drawer,
  Table,
  Tag,
  Space,
  Button,
  Typography,
  Progress,
  Segmented,
  Empty,
  Tooltip,
  message,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import {
  ReloadOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  LoadingOutlined,
  ClockCircleOutlined,
  FileTextOutlined,
  EyeOutlined,
} from "@ant-design/icons";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";

import {
  listProcessingJobs,
  type ProcessingJob,
  type ListJobsParams,
} from "../../../api/processing";

const { Text } = Typography;

interface TaskCenterDrawerProps {
  open: boolean;
  onClose: () => void;
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

// 流水线名称映射
const pipelineLabels: Record<string, string> = {
  generate_knowledge_overview: "生成知识概览",
  polish_document: "文档润色",
};

type StatusFilter = "all" | "running" | "completed" | "failed";

export const TaskCenterDrawer: FC<TaskCenterDrawerProps> = ({
  open,
  onClose,
}) => {
  const navigate = useNavigate();
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [page, setPage] = useState(1);
  const pageSize = 10;

  // 构建查询参数
  const getQueryParams = useCallback((): ListJobsParams => {
    const params: ListJobsParams = {
      limit: pageSize,
      offset: (page - 1) * pageSize,
    };

    if (statusFilter === "running") {
      params.status = "pending,running";
    } else if (statusFilter === "completed") {
      params.status = "completed";
    } else if (statusFilter === "failed") {
      params.status = "failed,cancelled";
    }

    return params;
  }, [statusFilter, page]);

  // 查询任务列表
  const {
    data,
    isLoading,
    isFetching,
    refetch,
  } = useQuery({
    queryKey: ["task-center-jobs", statusFilter, page],
    queryFn: () => listProcessingJobs(getQueryParams()),
    enabled: open,
    refetchInterval: (query) => {
      // 如果有进行中的任务，自动轮询
      const jobs = query.state.data?.jobs || [];
      const hasRunning = jobs.some(
        (job) => job.status === "pending" || job.status === "running"
      );
      return hasRunning ? 3000 : false;
    },
  });

  // 统计进行中任务数
  const runningCount = data?.jobs?.filter(
    (job) => job.status === "pending" || job.status === "running"
  ).length || 0;

  // 格式化时间
  const formatTime = (isoString?: string) => {
    if (!isoString) return "-";
    const date = new Date(isoString);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMinutes = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMinutes / 60);
    const diffDays = Math.floor(diffHours / 24);

    if (diffMinutes < 1) return "刚刚";
    if (diffMinutes < 60) return `${diffMinutes} 分钟前`;
    if (diffHours < 24) return `${diffHours} 小时前`;
    if (diffDays < 7) return `${diffDays} 天前`;
    return date.toLocaleString("zh-CN");
  };

  // 查看文档
  const handleViewDocument = (docId: number) => {
    onClose();
    navigate(`/documents/${docId}/edit`);
  };

  // 表格列定义
  const columns: ColumnsType<ProcessingJob> = [
    {
      title: "文档",
      dataIndex: "document_title",
      key: "document_title",
      width: 200,
      ellipsis: true,
      render: (title: string, record) => (
        <Space>
          <FileTextOutlined />
          <Tooltip title={title}>
            <Text ellipsis style={{ maxWidth: 150 }}>{title}</Text>
          </Tooltip>
          {record.dry_run && (
            <Tag color="orange" style={{ marginLeft: 4 }}>预览</Tag>
          )}
        </Space>
      ),
    },
    {
      title: "流水线",
      dataIndex: "pipeline_name",
      key: "pipeline_name",
      width: 130,
      render: (name: string) => (
        <Text>{pipelineLabels[name] || name}</Text>
      ),
    },
    {
      title: "状态",
      dataIndex: "status",
      key: "status",
      width: 100,
      render: (status: string) => {
        let icon;
        switch (status) {
          case "completed":
            icon = <CheckCircleOutlined />;
            break;
          case "failed":
          case "cancelled":
            icon = <CloseCircleOutlined />;
            break;
          case "running":
            icon = <LoadingOutlined />;
            break;
          default:
            icon = <ClockCircleOutlined />;
        }
        return (
          <Tag icon={icon} color={statusColors[status]}>
            {statusLabels[status]}
          </Tag>
        );
      },
    },
    {
      title: "进度",
      dataIndex: "progress",
      key: "progress",
      width: 120,
      render: (progress: number, record) => {
        if (record.status === "completed") {
          return <Progress percent={100} size="small" />;
        }
        if (record.status === "failed" || record.status === "cancelled") {
          return <Progress percent={progress} size="small" status="exception" />;
        }
        return <Progress percent={progress} size="small" status="active" />;
      },
    },
    {
      title: "时间",
      key: "time",
      width: 120,
      render: (_, record) => {
        if (record.status === "completed" && record.completed_at) {
          return <Text type="secondary">完成于 {formatTime(record.completed_at)}</Text>;
        }
        if (record.status === "failed" && record.completed_at) {
          return <Text type="secondary">失败于 {formatTime(record.completed_at)}</Text>;
        }
        return <Text type="secondary">开始于 {formatTime(record.started_at || record.created_at)}</Text>;
      },
    },
    {
      title: "操作",
      key: "actions",
      width: 80,
      render: (_, record) => (
        <Tooltip title="查看文档">
          <Button
            type="text"
            icon={<EyeOutlined />}
            onClick={() => handleViewDocument(record.document_id)}
          />
        </Tooltip>
      ),
    },
  ];

  // 展开行内容（显示错误信息）
  const expandedRowRender = (record: ProcessingJob) => {
    if (record.error_message) {
      return (
        <div style={{ padding: "8px 0" }}>
          <Text type="danger">错误信息：{record.error_message}</Text>
        </div>
      );
    }
    if (record.result) {
      return (
        <div style={{ padding: "8px 0" }}>
          <Text type="secondary">
            结果：{JSON.stringify(record.result)}
          </Text>
        </div>
      );
    }
    return null;
  };

  // 状态过滤选项
  const statusOptions = [
    { label: "全部", value: "all" },
    {
      label: (
        <Space>
          进行中
          {runningCount > 0 && statusFilter !== "running" && (
            <Tag color="processing" style={{ marginLeft: 4 }}>{runningCount}</Tag>
          )}
        </Space>
      ),
      value: "running",
    },
    { label: "已完成", value: "completed" },
    { label: "失败", value: "failed" },
  ];

  const handleStatusChange = (value: string | number) => {
    setStatusFilter(value as StatusFilter);
    setPage(1);
  };

  return (
    <Drawer
      title="任务中心"
      placement="right"
      width={800}
      open={open}
      onClose={onClose}
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
      <Space direction="vertical" size="middle" style={{ width: "100%" }}>
        <Segmented
          options={statusOptions}
          value={statusFilter}
          onChange={handleStatusChange}
        />
        <Table
          dataSource={data?.jobs || []}
          columns={columns}
          rowKey="id"
          loading={isLoading}
          size="small"
          expandable={{
            expandedRowRender,
            rowExpandable: (record) =>
              !!(record.error_message || record.result),
          }}
          pagination={{
            current: page,
            pageSize,
            total: data?.total || 0,
            showSizeChanger: false,
            showTotal: (total) => `共 ${total} 条`,
            onChange: setPage,
          }}
          locale={{
            emptyText: (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description="暂无任务"
              />
            ),
          }}
        />
      </Space>
    </Drawer>
  );
};
