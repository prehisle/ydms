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
} from "@ant-design/icons";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import type { ColumnsType } from "antd/es/table";
import {
  listWorkflowRuns,
  listAdminWorkflows,
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
  const [filters, setFilters] = useState<FilterState>({});
  const [pagination, setPagination] = useState({ current: 1, pageSize: 20 });

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
  });

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
      width: 100,
      render: (status: string) => {
        const config = statusConfig[status] || statusConfig.pending;
        return (
          <Tag color={config.color} icon={config.icon}>
            {config.label}
          </Tag>
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
        <Space wrap style={{ marginBottom: 16 }}>
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
          size="middle"
        />
      </Card>
    </div>
  );
};
