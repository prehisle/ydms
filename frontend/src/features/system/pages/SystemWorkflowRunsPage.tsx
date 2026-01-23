import { type FC, useState, useMemo } from "react";
import {
  Card,
  Space,
  Typography,
  Table,
  Tag,
  Breadcrumb,
  Select,
  Descriptions,
  Alert,
  Button,
  message,
  Tooltip,
  Popconfirm,
  Modal,
  Checkbox,
  DatePicker,
  Form,
} from "antd";
import {
  HomeOutlined,
  ClockCircleOutlined,
  SyncOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  StopOutlined,
  FileOutlined,
  FolderOutlined,
  ReloadOutlined,
  DeleteOutlined,
} from "@ant-design/icons";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import type { ColumnsType } from "antd/es/table";
import {
  listWorkflowRuns,
  listAdminWorkflows,
  triggerNodeWorkflow,
  triggerDocumentWorkflow,
  cancelWorkflowRun,
  forceTerminateWorkflowRun,
  cleanupWorkflowRuns,
  type WorkflowRun,
} from "../../../api/workflows";
import { getCategoryTree, type Category } from "../../../api/categories";
import { useAuth } from "../../../contexts/AuthContext";

const { Title, Text } = Typography;

// 状态配置
const statusConfig: Record<
  string,
  { color: string; icon: React.ReactNode; label: string }
> = {
  pending: {
    color: "default",
    icon: <ClockCircleOutlined />,
    label: "待执行",
  },
  running: {
    color: "processing",
    icon: <SyncOutlined spin />,
    label: "运行中",
  },
  success: {
    color: "success",
    icon: <CheckCircleOutlined />,
    label: "成功",
  },
  failed: {
    color: "error",
    icon: <CloseCircleOutlined />,
    label: "失败",
  },
  cancelled: {
    color: "warning",
    icon: <StopOutlined />,
    label: "已取消",
  },
};

// 筛选状态接口
interface FilterState {
  workflow_key?: string;
  status?: string;
}

// 构建节点 ID 到节点信息的映射（包含友好路径）
function buildNodeMap(
  categories: Category[],
  parentPath: string = "",
  map: Map<number, { name: string; path: string }> = new Map()
): Map<number, { name: string; path: string }> {
  for (const cat of categories) {
    const friendlyPath = parentPath ? `${parentPath} / ${cat.name}` : cat.name;
    map.set(cat.id, { name: cat.name, path: friendlyPath });
    if (cat.children?.length) {
      buildNodeMap(cat.children, friendlyPath, map);
    }
  }
  return map;
}

/**
 * 工作流执行历史页面
 */
export const SystemWorkflowRunsPage: FC = () => {
  const { user: currentUser } = useAuth();
  const queryClient = useQueryClient();
  const [filters, setFilters] = useState<FilterState>({});
  const [pagination, setPagination] = useState({ current: 1, pageSize: 20 });
  const [cancellingRunId, setCancellingRunId] = useState<number | null>(null);

  const isAdmin =
    currentUser?.role === "super_admin" ||
    currentUser?.role === "course_admin";

  // 获取分类树（用于显示节点名称）
  const { data: categoryTree } = useQuery({
    queryKey: ["categoryTree"],
    queryFn: () => getCategoryTree(),
    enabled: isAdmin,
    staleTime: 5 * 60 * 1000, // 5 分钟缓存
  });

  // 构建节点映射
  const nodeMap = useMemo(() => {
    if (!categoryTree) return new Map();
    return buildNodeMap(categoryTree);
  }, [categoryTree]);

  // 获取工作流定义列表（用于筛选下拉）
  const { data: workflows } = useQuery({
    queryKey: ["adminWorkflows"],
    queryFn: () => listAdminWorkflows(),
    enabled: isAdmin,
  });

  // 获取工作流运行记录
  const {
    data: runsData,
    isLoading,
    isFetching,
  } = useQuery({
    queryKey: [
      "workflowRuns",
      filters,
      pagination.current,
      pagination.pageSize,
    ],
    queryFn: () =>
      listWorkflowRuns({
        workflow_key: filters.workflow_key,
        status: filters.status,
        limit: pagination.pageSize,
        offset: (pagination.current - 1) * pagination.pageSize,
      }),
    enabled: isAdmin,
    placeholderData: (previousData) => previousData,
    // 智能轮询：有 pending/running 任务时每 3 秒刷新
    refetchInterval: (query) => {
      const runs = query.state.data?.runs;
      if (!runs) return false;
      const hasActiveRuns = runs.some(
        (run) => run.status === "pending" || run.status === "running"
      );
      return hasActiveRuns ? 3000 : false;
    },
  });

  // 重试工作流
  const retryMutation = useMutation({
    mutationFn: async (run: WorkflowRun) => {
      if (run.node_id) {
        return triggerNodeWorkflow(run.node_id, run.workflow_key, {
          parameters: run.parameters,
          retry_of_id: run.id,
        });
      } else if (run.document_id) {
        return triggerDocumentWorkflow(run.document_id, run.workflow_key, {
          parameters: run.parameters,
          retry_of_id: run.id,
        });
      }
      throw new Error("无法重试：缺少节点或文档信息");
    },
    onSuccess: () => {
      message.success("工作流已重新提交");
      queryClient.invalidateQueries({ queryKey: ["workflowRuns"] });
    },
    onError: (error: Error) => {
      message.error(`重试失败：${error.message}`);
    },
  });

  // 取消工作流
  const cancelMutation = useMutation({
    mutationFn: (runId: number) => cancelWorkflowRun(runId),
    onSuccess: () => {
      message.success("工作流已取消");
      queryClient.invalidateQueries({ queryKey: ["workflowRuns"] });
    },
    onError: (error: Error) => {
      message.error(`取消失败：${error.message}`);
    },
    onSettled: () => {
      setCancellingRunId(null);
    },
  });

  // 处理取消操作
  const handleCancel = (runId: number) => {
    setCancellingRunId(runId);
    cancelMutation.mutate(runId);
  };

  // 强制终止僵尸任务
  const [terminatingRunId, setTerminatingRunId] = useState<number | null>(null);
  const forceTerminateMutation = useMutation({
    mutationFn: (runId: number) => forceTerminateWorkflowRun(runId),
    onSuccess: () => {
      message.success("僵尸任务已强制终止");
      queryClient.invalidateQueries({ queryKey: ["workflowRuns"] });
    },
    onError: (error: Error) => {
      message.error(`强制终止失败：${error.message}`);
    },
    onSettled: () => {
      setTerminatingRunId(null);
    },
  });

  const handleForceTerminate = (runId: number) => {
    setTerminatingRunId(runId);
    forceTerminateMutation.mutate(runId);
  };

  // 判断是否为僵尸任务（运行超过 30 分钟）
  const isZombieTask = (run: WorkflowRun): boolean => {
    if (run.status !== "pending" && run.status !== "running") return false;
    const startTime = run.started_at ? new Date(run.started_at) : new Date(run.created_at);
    const runningMinutes = (Date.now() - startTime.getTime()) / 1000 / 60;
    return runningMinutes >= 30;
  };

  // 清理选项状态
  const [showCleanupModal, setShowCleanupModal] = useState(false);
  const [cleanupOptions, setCleanupOptions] = useState({
    statuses: ["success", "failed", "cancelled"] as string[],
    beforeDate: null as string | null,
    includeZombie: false,
  });
  const [showCleanupConfirm, setShowCleanupConfirm] = useState(false);
  const [cleanupCount, setCleanupCount] = useState(0);
  const [zombieCount, setZombieCount] = useState(0);

  // 清理执行历史
  const cleanupMutation = useMutation({
    mutationFn: (dryRun: boolean) =>
      cleanupWorkflowRuns({
        workflow_key: filters.workflow_key,
        status: cleanupOptions.includeZombie
          ? [...cleanupOptions.statuses, "pending", "running"].join(",")
          : cleanupOptions.statuses.join(","),
        before_date: cleanupOptions.beforeDate || undefined,
        include_zombie: cleanupOptions.includeZombie,
        dry_run: dryRun,
      }),
    onSuccess: (data, dryRun) => {
      if (dryRun) {
        // 试运行模式，显示将要删除的数量
        if (data.deleted_count === 0) {
          message.info("没有符合条件的记录需要清理");
        } else {
          // 弹出确认框
          setCleanupCount(data.deleted_count);
          setZombieCount(data.zombie_count || 0);
          setShowCleanupModal(false);
          setShowCleanupConfirm(true);
        }
      } else {
        const zombieMsg = data.zombie_count ? `（含 ${data.zombie_count} 个僵尸任务）` : "";
        message.success(`已清理 ${data.deleted_count} 条记录${zombieMsg}`);
        setShowCleanupConfirm(false);
        queryClient.invalidateQueries({ queryKey: ["workflowRuns"] });
      }
    },
    onError: (error: Error) => {
      message.error(`清理失败：${error.message}`);
    },
  });

  // 打开清理选项对话框
  const handleCleanup = () => {
    setShowCleanupModal(true);
  };

  // 预览清理数量
  const previewCleanup = () => {
    if (cleanupOptions.statuses.length === 0) {
      message.warning("请至少选择一种状态");
      return;
    }
    cleanupMutation.mutate(true); // 试运行
  };

  // 确认清理
  const confirmCleanup = () => {
    cleanupMutation.mutate(false); // 实际执行
  };

  // 格式化时间
  const formatTime = (timeStr?: string) => {
    if (!timeStr) return "-";
    return new Date(timeStr).toLocaleString("zh-CN");
  };

  // 计算耗时
  const calculateDuration = (run: WorkflowRun): string => {
    if (!run.started_at) return "-";
    const start = new Date(run.started_at).getTime();
    const end = run.finished_at
      ? new Date(run.finished_at).getTime()
      : Date.now();
    const durationMs = end - start;

    if (durationMs < 1000) return `${durationMs}ms`;
    if (durationMs < 60000) return `${(durationMs / 1000).toFixed(1)}s`;
    const minutes = Math.floor(durationMs / 60000);
    const seconds = Math.floor((durationMs % 60000) / 1000);
    return `${minutes}m ${seconds}s`;
  };

  // 渲染目标（节点/文档）
  const renderTarget = (run: WorkflowRun) => {
    if (run.node_id) {
      const nodeInfo = nodeMap.get(run.node_id);
      const fullPath = nodeInfo?.path || `节点 #${run.node_id}`;

      return (
        <Link to={`/documents?nodeId=${run.node_id}`}>
          <Space size={4}>
            <FolderOutlined style={{ flexShrink: 0 }} />
            <span style={{ wordBreak: "break-all" }}>{fullPath}</span>
          </Space>
        </Link>
      );
    }
    if (run.document_id) {
      return (
        <Link to={`/documents/${run.document_id}/edit`}>
          <Space size={4}>
            <FileOutlined />
            <span>文档 #{run.document_id}</span>
          </Space>
        </Link>
      );
    }
    return <Text type="secondary">-</Text>;
  };

  // 表格列定义
  const columns: ColumnsType<WorkflowRun> = [
    {
      title: "ID",
      dataIndex: "id",
      key: "id",
      width: 70,
    },
    {
      title: "工作流",
      dataIndex: "workflow_key",
      key: "workflow_key",
      width: 180,
      render: (key: string) => {
        const workflow = workflows?.find((w) => w.workflow_key === key);
        return (
          <Space direction="vertical" size={0}>
            <Text strong>{workflow?.name || key}</Text>
            <Text type="secondary" style={{ fontSize: 12 }}>
              {key}
            </Text>
          </Space>
        );
      },
    },
    {
      title: "目标",
      key: "target",
      render: (_, run) => renderTarget(run),
    },
    {
      title: "状态",
      dataIndex: "status",
      key: "status",
      width: 180,
      render: (status: string, run: WorkflowRun) => {
        const config = statusConfig[status] || statusConfig.pending;
        return (
          <Space size={4} wrap>
            <Tag color={config.color} icon={config.icon}>
              {config.label}
            </Tag>
            {run.retry_of_id && (
              <Tooltip title={`此任务是对任务 #${run.retry_of_id} 的重试`}>
                <Tag color="blue">重试自 #{run.retry_of_id}</Tag>
              </Tooltip>
            )}
            {(run.retry_count ?? 0) > 0 && (
              <Tooltip title={`此任务已被重试 ${run.retry_count} 次，最新状态：${
                run.latest_retry_status ? statusConfig[run.latest_retry_status]?.label : "未知"
              }`}>
                <Tag color={
                  run.latest_retry_status === "success" ? "success" :
                  run.latest_retry_status === "failed" ? "error" :
                  run.latest_retry_status === "running" ? "processing" : "orange"
                }>
                  已重试 → {run.latest_retry_status ? statusConfig[run.latest_retry_status]?.label : `${run.retry_count}次`}
                </Tag>
              </Tooltip>
            )}
          </Space>
        );
      },
    },
    {
      title: "执行者",
      dataIndex: "created_by",
      key: "created_by",
      width: 120,
      render: (createdBy?: { display_name: string; username: string }) => {
        if (!createdBy) return <Text type="secondary">系统</Text>;
        return createdBy.display_name || createdBy.username;
      },
    },
    {
      title: "创建时间",
      dataIndex: "created_at",
      key: "created_at",
      width: 170,
      render: formatTime,
    },
    {
      title: "耗时",
      key: "duration",
      width: 100,
      render: (_, run) => calculateDuration(run),
    },
    {
      title: "操作",
      key: "actions",
      width: 150,
      fixed: "right" as const,
      render: (_, run) => {
        const canRetry =
          run.status === "failed" || run.status === "cancelled";
        const canCancel =
          run.status === "pending" || run.status === "running";
        const canForceTerminate = isZombieTask(run);
        const isCancelling = cancellingRunId === run.id;
        const isTerminating = terminatingRunId === run.id;

        return (
          <Space size={0}>
            {canCancel && !canForceTerminate && (
              <Popconfirm
                title="确认取消"
                description="确定要取消此工作流吗？"
                onConfirm={() => handleCancel(run.id)}
                okText="确定"
                cancelText="取消"
                okButtonProps={{ danger: true }}
              >
                <Button
                  type="link"
                  size="small"
                  danger
                  icon={<StopOutlined />}
                  loading={isCancelling}
                >
                  取消
                </Button>
              </Popconfirm>
            )}
            {canForceTerminate && (
              <Popconfirm
                title="强制终止"
                description="此任务已运行超过 30 分钟，确定要强制终止吗？"
                onConfirm={() => handleForceTerminate(run.id)}
                okText="强制终止"
                cancelText="取消"
                okButtonProps={{ danger: true }}
              >
                <Tooltip title="任务已运行超过 30 分钟，可强制终止">
                  <Button
                    type="link"
                    size="small"
                    danger
                    icon={<CloseCircleOutlined />}
                    loading={isTerminating}
                  >
                    强制终止
                  </Button>
                </Tooltip>
              </Popconfirm>
            )}
            {canRetry && (
              <Tooltip title="重试">
                <Button
                  type="link"
                  size="small"
                  icon={<ReloadOutlined />}
                  loading={retryMutation.isPending}
                  onClick={() => retryMutation.mutate(run)}
                >
                  重试
                </Button>
              </Tooltip>
            )}
          </Space>
        );
      },
    },
  ];

  // 展开行渲染
  const expandedRowRender = (run: WorkflowRun) => {
    return (
      <Descriptions
        bordered
        size="small"
        column={1}
        style={{ background: "#fafafa" }}
      >
        <Descriptions.Item label="参数">
          <pre style={{ margin: 0, whiteSpace: "pre-wrap", fontSize: 12 }}>
            {JSON.stringify(run.parameters, null, 2)}
          </pre>
        </Descriptions.Item>
        {run.result && (
          <Descriptions.Item label="结果">
            <pre style={{ margin: 0, whiteSpace: "pre-wrap", fontSize: 12 }}>
              {JSON.stringify(run.result, null, 2)}
            </pre>
          </Descriptions.Item>
        )}
        {run.error_message && (
          <Descriptions.Item label="错误信息">
            <Text type="danger">{run.error_message}</Text>
          </Descriptions.Item>
        )}
        {run.prefect_flow_run_id && (
          <Descriptions.Item label="Prefect Flow Run ID">
            <Text code copyable>
              {run.prefect_flow_run_id}
            </Text>
          </Descriptions.Item>
        )}
        <Descriptions.Item label="时间信息">
          <Space direction="vertical" size={0}>
            <Text>开始：{formatTime(run.started_at)}</Text>
            <Text>结束：{formatTime(run.finished_at)}</Text>
          </Space>
        </Descriptions.Item>
      </Descriptions>
    );
  };

  if (!isAdmin) {
    return (
      <Alert
        message="权限不足"
        description="仅管理员可以访问执行历史页面"
        type="error"
        showIcon
      />
    );
  }

  // 生成工作流选项
  const workflowOptions =
    workflows?.map((w) => ({
      label: w.name,
      value: w.workflow_key,
    })) || [];

  // 生成状态选项
  const statusOptions = Object.entries(statusConfig).map(([key, config]) => ({
    label: config.label,
    value: key,
  }));

  return (
    <div style={{ padding: 24 }}>
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
          { title: "执行历史" },
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
          执行历史
        </Title>
      </div>

      <Card>
        {/* 筛选区 */}
        <Space wrap style={{ marginBottom: 16, width: "100%", justifyContent: "space-between" }}>
          <Space wrap>
            <Select
              placeholder="选择工作流"
              allowClear
              style={{ width: 200 }}
              options={workflowOptions}
              value={filters.workflow_key}
              onChange={(value) => {
                setFilters((prev) => ({ ...prev, workflow_key: value }));
                setPagination((prev) => ({ ...prev, current: 1 }));
              }}
            />
            <Select
              placeholder="选择状态"
              allowClear
              style={{ width: 140 }}
              options={statusOptions}
              value={filters.status}
              onChange={(value) => {
                setFilters((prev) => ({ ...prev, status: value }));
                setPagination((prev) => ({ ...prev, current: 1 }));
              }}
            />
          </Space>
          <Tooltip title="清理执行历史记录">
            <Button
              icon={<DeleteOutlined />}
              onClick={handleCleanup}
              loading={cleanupMutation.isPending}
            >
              清理历史
            </Button>
          </Tooltip>
        </Space>

        {/* 清理选项对话框 */}
        <Modal
          title="清理执行历史"
          open={showCleanupModal}
          onOk={previewCleanup}
          onCancel={() => setShowCleanupModal(false)}
          okText="预览"
          cancelText="取消"
          okButtonProps={{ loading: cleanupMutation.isPending }}
        >
          <Form layout="vertical">
            <Form.Item label="选择要清理的状态" required>
              <Checkbox.Group
                value={cleanupOptions.statuses}
                onChange={(values) =>
                  setCleanupOptions((prev) => ({
                    ...prev,
                    statuses: values as string[],
                  }))
                }
              >
                <Space direction="vertical">
                  <Checkbox value="success">
                    <Tag color="success" icon={<CheckCircleOutlined />}>成功</Tag>
                  </Checkbox>
                  <Checkbox value="failed">
                    <Tag color="error" icon={<CloseCircleOutlined />}>失败</Tag>
                  </Checkbox>
                  <Checkbox value="cancelled">
                    <Tag color="warning" icon={<StopOutlined />}>已取消</Tag>
                  </Checkbox>
                </Space>
              </Checkbox.Group>
            </Form.Item>
            <Form.Item>
              <Checkbox
                checked={cleanupOptions.includeZombie}
                onChange={(e) =>
                  setCleanupOptions((prev) => ({
                    ...prev,
                    includeZombie: e.target.checked,
                  }))
                }
              >
                <Space>
                  <span>包含僵尸任务</span>
                  <Tooltip title="运行超过 30 分钟的待执行/运行中任务，将被标记为失败后清理">
                    <Tag color="orange">运行中 &gt; 30分钟</Tag>
                  </Tooltip>
                </Space>
              </Checkbox>
            </Form.Item>
            <Form.Item label="清理此日期之前的记录（可选）">
              <DatePicker
                style={{ width: "100%" }}
                placeholder="不限制日期"
                onChange={(date) =>
                  setCleanupOptions((prev) => ({
                    ...prev,
                    beforeDate: date ? date.format("YYYY-MM-DD") : null,
                  }))
                }
              />
            </Form.Item>
            {filters.workflow_key && (
              <Alert
                type="info"
                message={`将只清理工作流「${workflows?.find(w => w.workflow_key === filters.workflow_key)?.name || filters.workflow_key}」的记录`}
                style={{ marginBottom: 0 }}
              />
            )}
          </Form>
        </Modal>

        {/* 清理确认对话框 */}
        <Modal
          title="确认清理"
          open={showCleanupConfirm}
          onOk={confirmCleanup}
          onCancel={() => setShowCleanupConfirm(false)}
          okText="确认清理"
          cancelText="取消"
          okButtonProps={{ danger: true, loading: cleanupMutation.isPending }}
        >
          <p>
            将清理 <Text strong type="danger">{cleanupCount}</Text> 条执行记录
            {zombieCount > 0 && (
              <span>（含 <Text strong type="warning">{zombieCount}</Text> 个僵尸任务）</span>
            )}。
          </p>
          <p>
            <Text type="secondary">
              清理条件：
              {cleanupOptions.statuses.map((s) => statusConfig[s]?.label).join("、")}
              {cleanupOptions.includeZombie && "、僵尸任务"}
              {cleanupOptions.beforeDate && `，${cleanupOptions.beforeDate} 之前`}
              {filters.workflow_key && `，工作流：${filters.workflow_key}`}
            </Text>
          </p>
          <p>
            <Text type="warning">此操作不可撤销，请确认是否继续？</Text>
          </p>
        </Modal>

        {/* 表格 */}
        <Table
          columns={columns}
          dataSource={runsData?.runs}
          rowKey="id"
          loading={isLoading || isFetching}
          expandable={{
            expandedRowRender,
            rowExpandable: () => true,
          }}
          pagination={{
            current: pagination.current,
            pageSize: pagination.pageSize,
            total: runsData?.total || 0,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (total) => `共 ${total} 条`,
            onChange: (page, pageSize) => {
              setPagination({ current: page, pageSize });
            },
          }}
          scroll={{ y: "calc(100vh - 340px)", x: "max-content" }}
          size="middle"
        />
      </Card>
    </div>
  );
};
