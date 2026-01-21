import { type FC } from "react";
import {
  Card,
  Button,
  Space,
  Typography,
  Table,
  Tag,
  Switch,
  message,
  Breadcrumb,
  Tooltip,
  Alert,
} from "antd";
import {
  HomeOutlined,
  SyncOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  QuestionCircleOutlined,
  ClockCircleOutlined,
} from "@ant-design/icons";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import type { ColumnsType } from "antd/es/table";
import {
  listAdminWorkflows,
  getSyncStatus,
  triggerSync,
  updateAdminWorkflow,
  type WorkflowDefinition,
} from "../../../api/workflows";
import { useAuth } from "../../../contexts/AuthContext";

const { Title, Text } = Typography;

/**
 * 工作流管理页面
 */
export const SystemWorkflowsPage: FC = () => {
  const { user: currentUser } = useAuth();
  const queryClient = useQueryClient();
  const [messageApi, contextHolder] = message.useMessage();

  const isAdmin =
    currentUser?.role === "super_admin" ||
    currentUser?.role === "course_admin";

  // 获取工作流列表
  const {
    data: workflows,
    isLoading: workflowsLoading,
    isFetching,
  } = useQuery({
    queryKey: ["adminWorkflows"],
    queryFn: () => listAdminWorkflows(),
    enabled: isAdmin,
  });

  // 获取同步状态
  const { data: syncStatus, isLoading: syncStatusLoading } = useQuery({
    queryKey: ["workflowSyncStatus"],
    queryFn: getSyncStatus,
    enabled: isAdmin,
    refetchInterval: (query) =>
      query.state.data?.status === "in_progress" ? 2000 : false, // 同步中时每 2 秒刷新
  });

  // 触发同步
  const syncMutation = useMutation({
    mutationFn: triggerSync,
    onSuccess: (result) => {
      messageApi.success(
        `同步完成：新增 ${result.created}，更新 ${result.updated}，失效 ${result.missing}`
      );
      void queryClient.invalidateQueries({ queryKey: ["adminWorkflows"] });
      void queryClient.invalidateQueries({ queryKey: ["workflowSyncStatus"] });
    },
    onError: (err) => {
      messageApi.error(
        err instanceof Error ? err.message : "同步失败"
      );
      void queryClient.invalidateQueries({ queryKey: ["workflowSyncStatus"] });
    },
  });

  // 更新工作流状态
  const updateMutation = useMutation({
    mutationFn: ({
      id,
      enabled,
    }: {
      id: number;
      enabled: boolean;
    }) => updateAdminWorkflow(id, { enabled }),
    onSuccess: () => {
      messageApi.success("工作流状态已更新");
      void queryClient.invalidateQueries({ queryKey: ["adminWorkflows"] });
    },
    onError: (err) => {
      messageApi.error(
        err instanceof Error ? err.message : "更新失败"
      );
    },
  });

  // 格式化时间
  const formatTime = (timeStr?: string) => {
    if (!timeStr) return "-";
    return new Date(timeStr).toLocaleString("zh-CN");
  };

  // 获取同步状态图标
  const getSyncStatusIcon = (status: string) => {
    switch (status) {
      case "active":
        return <CheckCircleOutlined style={{ color: "#52c41a" }} />;
      case "missing":
        return <QuestionCircleOutlined style={{ color: "#faad14" }} />;
      case "error":
        return <CloseCircleOutlined style={{ color: "#ff4d4f" }} />;
      default:
        return null;
    }
  };

  // 表格列定义
  const columns: ColumnsType<WorkflowDefinition> = [
    {
      title: "名称",
      dataIndex: "name",
      key: "name",
      width: 200,
      render: (name, record) => (
        <Space direction="vertical" size={0}>
          <Text strong>{name}</Text>
          <Text type="secondary" style={{ fontSize: 12 }}>
            {record.workflow_key}
          </Text>
        </Space>
      ),
    },
    {
      title: "类型",
      dataIndex: "workflow_type",
      key: "workflow_type",
      width: 100,
      render: (type: string) => (
        <Tag color={type === "node" ? "blue" : "purple"}>
          {type === "node" ? "节点" : "文档"}
        </Tag>
      ),
    },
    {
      title: "版本",
      dataIndex: "prefect_version",
      key: "prefect_version",
      width: 80,
      render: (version) => version || "-",
    },
    {
      title: "来源",
      dataIndex: "source",
      key: "source",
      width: 80,
      render: (source: string) => (
        <Tag color={source === "prefect" ? "geekblue" : "default"}>
          {source === "prefect" ? "Prefect" : "手动"}
        </Tag>
      ),
    },
    {
      title: "同步状态",
      dataIndex: "sync_status",
      key: "sync_status",
      width: 100,
      render: (status: string) => (
        <Space>
          {getSyncStatusIcon(status)}
          <span>
            {status === "active"
              ? "活跃"
              : status === "missing"
              ? "失效"
              : "错误"}
          </span>
        </Space>
      ),
    },
    {
      title: "最后同步",
      dataIndex: "last_synced_at",
      key: "last_synced_at",
      width: 160,
      render: formatTime,
    },
    {
      title: "启用",
      dataIndex: "enabled",
      key: "enabled",
      width: 80,
      render: (enabled: boolean, record) => (
        <Switch
          checked={enabled}
          loading={updateMutation.isPending}
          onChange={(checked) =>
            updateMutation.mutate({ id: record.id, enabled: checked })
          }
        />
      ),
    },
    {
      title: "描述",
      dataIndex: "description",
      key: "description",
      ellipsis: true,
      render: (desc) => (
        <Tooltip title={desc}>
          <Text type="secondary" ellipsis>
            {desc || "-"}
          </Text>
        </Tooltip>
      ),
    },
  ];

  if (!isAdmin) {
    return (
      <Alert
        message="权限不足"
        description="仅管理员可以访问工作流管理页面"
        type="error"
        showIcon
      />
    );
  }

  return (
    <div style={{ padding: 24 }}>
      {contextHolder}
      <Breadcrumb
        style={{ marginBottom: 16 }}
        items={[
          {
            title: (
              <Link to="/">
                <HomeOutlined />
              </Link>
            ),
          },
          { title: "系统管理" },
          { title: "工作流管理" },
        ]}
      />

      <div
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          marginBottom: 16,
        }}
      >
        <Title level={4} style={{ margin: 0 }}>
          工作流管理
        </Title>
        <Space>
          {syncStatus && (
            <Text type="secondary">
              {syncStatus.prefect_enabled ? (
                <>
                  <ClockCircleOutlined style={{ marginRight: 4 }} />
                  最后同步：{formatTime(syncStatus.last_sync_time)}
                </>
              ) : (
                <Tag color="warning">Prefect 未配置</Tag>
              )}
            </Text>
          )}
          <Button
            type="primary"
            icon={<SyncOutlined spin={syncMutation.isPending} />}
            onClick={() => syncMutation.mutate()}
            loading={syncMutation.isPending}
            disabled={!syncStatus?.prefect_enabled}
          >
            立即同步
          </Button>
        </Space>
      </div>

      {syncStatus?.error && (
        <Alert
          message="同步错误"
          description={syncStatus.error}
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          closable
        />
      )}

      <Card>
        <Table
          columns={columns}
          dataSource={workflows}
          rowKey="id"
          loading={workflowsLoading || isFetching}
          pagination={{
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (total) => `共 ${total} 条`,
          }}
          size="middle"
        />
      </Card>
    </div>
  );
};
