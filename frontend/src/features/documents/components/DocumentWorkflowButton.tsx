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
  Empty,
} from "antd";
import {
  ThunderboltOutlined,
  PlayCircleOutlined,
  HistoryOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  LoadingOutlined,
  ClockCircleOutlined,
} from "@ant-design/icons";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import type { MenuProps } from "antd";

import {
  listDocumentWorkflows,
  triggerDocumentWorkflow,
  listDocumentWorkflowRuns,
  getWorkflowRun,
  type WorkflowDefinition,
  type WorkflowRun,
} from "../../../api/workflows";

const { Text, Paragraph } = Typography;

interface DocumentWorkflowButtonProps {
  documentId: number;
  documentTitle?: string;
  disabled?: boolean;
}

// 任务状态到标签颜色的映射
const statusColors: Record<string, string> = {
  pending: "default",
  running: "processing",
  success: "success",
  failed: "error",
  cancelled: "warning",
};

// 任务状态到中文名的映射
const statusLabels: Record<string, string> = {
  pending: "等待中",
  running: "运行中",
  success: "已完成",
  failed: "失败",
  cancelled: "已取消",
};

export const DocumentWorkflowButton: FC<DocumentWorkflowButtonProps> = ({
  documentId,
  documentTitle,
  disabled,
}) => {
  const queryClient = useQueryClient();
  const [historyModalOpen, setHistoryModalOpen] = useState(false);
  const [progressModalOpen, setProgressModalOpen] = useState(false);
  const [currentRunId, setCurrentRunId] = useState<number | null>(null);

  // 获取可用文档工作流
  const { data: workflows, isLoading: loadingWorkflows } = useQuery({
    queryKey: ["document-workflows", documentId],
    queryFn: () => listDocumentWorkflows(documentId),
    staleTime: 5 * 60 * 1000, // 5分钟缓存
  });

  // 获取当前文档的工作流运行历史
  const { data: runsData, refetch: refetchRuns } = useQuery({
    queryKey: ["document-workflow-runs", documentId],
    queryFn: () => listDocumentWorkflowRuns(documentId, { limit: 20 }),
    enabled: historyModalOpen,
  });

  // 轮询当前任务状态
  const { data: currentRun } = useQuery({
    queryKey: ["workflow-run", currentRunId],
    queryFn: () => getWorkflowRun(currentRunId!),
    enabled: currentRunId !== null && progressModalOpen,
    refetchInterval: (query) => {
      const run = query.state.data;
      if (!run) return 2000;
      if (run.status === "success" || run.status === "failed") {
        return false; // 停止轮询
      }
      return 2000; // 每2秒轮询一次
    },
  });

  // 当任务完成或失败时显示消息
  useEffect(() => {
    if (!currentRun) return;
    if (currentRun.status === "success") {
      message.success("工作流执行完成");
      setProgressModalOpen(false);
      queryClient.invalidateQueries({
        queryKey: ["document-detail", documentId],
      });
      queryClient.invalidateQueries({
        queryKey: ["document-workflow-runs", documentId],
      });
    } else if (currentRun.status === "failed") {
      message.error(`工作流执行失败: ${currentRun.error_message || "未知错误"}`);
    }
  }, [currentRun?.status, currentRun?.error_message, documentId, queryClient]);

  // 触发工作流
  const triggerMutation = useMutation({
    mutationFn: (workflowKey: string) =>
      triggerDocumentWorkflow(documentId, workflowKey),
    onSuccess: (response) => {
      setCurrentRunId(response.run_id);
      message.success({
        content: (
          <span>
            工作流已提交，可
            <a
              onClick={() => {
                setProgressModalOpen(true);
                message.destroy();
              }}
              style={{ marginLeft: 4, marginRight: 4 }}
            >
              查看进度
            </a>
          </span>
        ),
        duration: 5,
      });
    },
    onError: (error: Error) => {
      message.error(`提交失败: ${error.message}`);
    },
  });

  // 处理菜单点击
  const handleMenuClick = useCallback(
    (workflowKey: string) => {
      triggerMutation.mutate(workflowKey);
    },
    [triggerMutation]
  );

  // 构建下拉菜单
  const buildMenuItems = useCallback((): MenuProps["items"] => {
    if (loadingWorkflows) {
      return [
        {
          key: "loading",
          label: <Spin size="small" />,
          disabled: true,
        },
      ];
    }

    if (!workflows || workflows.length === 0) {
      return [
        {
          key: "empty",
          label: "暂无可用工作流",
          disabled: true,
        },
        { type: "divider" },
        {
          key: "history",
          icon: <HistoryOutlined />,
          label: "执行历史",
          onClick: () => setHistoryModalOpen(true),
        },
      ];
    }

    const items: MenuProps["items"] = [];

    workflows.forEach((workflow: WorkflowDefinition, index: number) => {
      if (index > 0) {
        items.push({ type: "divider" });
      }

      // 工作流标题和描述
      items.push({
        key: `title-${workflow.workflow_key}`,
        label: (
          <div style={{ maxWidth: 280 }}>
            <Text strong>{workflow.name}</Text>
            <br />
            <Text type="secondary" style={{ fontSize: 12 }}>
              {workflow.description}
            </Text>
          </div>
        ),
        disabled: true,
      });

      // 执行按钮
      items.push({
        key: `execute-${workflow.workflow_key}`,
        icon: <PlayCircleOutlined />,
        label: "执行工作流",
        onClick: () => handleMenuClick(workflow.workflow_key),
      });
    });

    // 历史记录入口
    items.push({ type: "divider" });
    items.push({
      key: "history",
      icon: <HistoryOutlined />,
      label: "执行历史",
      onClick: () => setHistoryModalOpen(true),
    });

    return items;
  }, [loadingWorkflows, workflows, handleMenuClick]);

  // 格式化时间
  const formatTime = (isoString?: string) => {
    if (!isoString) return "-";
    return new Date(isoString).toLocaleString("zh-CN");
  };

  // 获取状态图标
  const getStatusIcon = (status: string) => {
    switch (status) {
      case "success":
        return <CheckCircleOutlined style={{ color: "#52c41a" }} />;
      case "failed":
        return <CloseCircleOutlined style={{ color: "#ff4d4f" }} />;
      case "running":
        return <LoadingOutlined style={{ color: "#1890ff" }} />;
      case "pending":
        return <ClockCircleOutlined style={{ color: "#8c8c8c" }} />;
      default:
        return null;
    }
  };

  // 如果没有可用工作流且加载完成，不显示按钮
  if (!loadingWorkflows && (!workflows || workflows.length === 0)) {
    return null;
  }

  return (
    <>
      <Dropdown
        menu={{ items: buildMenuItems() }}
        trigger={["click"]}
        disabled={disabled || triggerMutation.isPending}
      >
        <Button
          icon={
            triggerMutation.isPending ? (
              <LoadingOutlined />
            ) : (
              <ThunderboltOutlined />
            )
          }
          loading={triggerMutation.isPending}
        >
          文档工作流
        </Button>
      </Dropdown>

      {/* 进度模态框 */}
      <Modal
        title="工作流执行进度"
        open={progressModalOpen}
        onCancel={() => setProgressModalOpen(false)}
        footer={null}
        maskClosable={false}
      >
        {currentRun ? (
          <Space direction="vertical" style={{ width: "100%" }} size="middle">
            <div>
              <Text strong>文档：</Text>
              <Text>{documentTitle || `ID: ${documentId}`}</Text>
            </div>
            <div>
              <Text strong>工作流：</Text>
              <Text>{currentRun.workflow_key}</Text>
            </div>
            <div>
              <Text strong>状态：</Text>
              <Tag color={statusColors[currentRun.status]}>
                {statusLabels[currentRun.status]}
              </Tag>
            </div>
            <Progress
              percent={
                currentRun.status === "success"
                  ? 100
                  : currentRun.status === "running"
                  ? 50
                  : currentRun.status === "pending"
                  ? 10
                  : 0
              }
              status={
                currentRun.status === "failed"
                  ? "exception"
                  : currentRun.status === "success"
                  ? "success"
                  : "active"
              }
            />
            {currentRun.error_message && (
              <Alert
                type="error"
                message="错误信息"
                description={currentRun.error_message}
                showIcon
              />
            )}
            {currentRun.status === "success" && currentRun.result && (
              <Alert
                type="success"
                message="执行完成"
                description={
                  <pre style={{ margin: 0, fontSize: 12, maxHeight: 200, overflow: "auto" }}>
                    {JSON.stringify(currentRun.result, null, 2)}
                  </pre>
                }
                showIcon
              />
            )}
            {currentRun.started_at && (
              <div>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  开始时间：{formatTime(currentRun.started_at)}
                </Text>
              </div>
            )}
            {currentRun.finished_at && (
              <div>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  完成时间：{formatTime(currentRun.finished_at)}
                </Text>
              </div>
            )}
          </Space>
        ) : (
          <div style={{ textAlign: "center", padding: 24 }}>
            <Spin size="large" />
            <Paragraph style={{ marginTop: 16 }}>正在提交工作流...</Paragraph>
          </div>
        )}
      </Modal>

      {/* 历史记录模态框 */}
      <Modal
        title="工作流执行历史"
        open={historyModalOpen}
        onCancel={() => setHistoryModalOpen(false)}
        footer={<Button onClick={() => refetchRuns()}>刷新</Button>}
        width={700}
      >
        {runsData?.runs && runsData.runs.length > 0 ? (
          <Space direction="vertical" style={{ width: "100%" }} size="small">
            {runsData.runs.map((run: WorkflowRun) => (
              <div
                key={run.id}
                style={{
                  padding: 12,
                  border: "1px solid #f0f0f0",
                  borderRadius: 6,
                }}
              >
                <Space
                  style={{ width: "100%", justifyContent: "space-between" }}
                >
                  <Space>
                    {getStatusIcon(run.status)}
                    <Text strong>{run.workflow_key}</Text>
                    <Tag color={statusColors[run.status]}>
                      {statusLabels[run.status]}
                    </Tag>
                  </Space>
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    {formatTime(run.created_at)}
                  </Text>
                </Space>
                {run.error_message && (
                  <Paragraph
                    type="danger"
                    style={{ marginTop: 8, marginBottom: 0, fontSize: 12 }}
                    ellipsis={{ rows: 2 }}
                  >
                    {run.error_message}
                  </Paragraph>
                )}
                {run.created_by && (
                  <Text
                    type="secondary"
                    style={{ fontSize: 12, display: "block", marginTop: 4 }}
                  >
                    执行者：{run.created_by.display_name || run.created_by.username}
                  </Text>
                )}
              </div>
            ))}
          </Space>
        ) : (
          <Empty description="暂无执行历史" />
        )}
      </Modal>
    </>
  );
};
