import { useState, useEffect, useCallback } from "react";
import {
  Modal,
  Steps,
  Button,
  Select,
  Switch,
  Table,
  Tag,
  Progress,
  Alert,
  Space,
  Typography,
  Spin,
  Result,
  Descriptions,
  Input,
  message,
} from "antd";
import {
  ThunderboltOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  ExclamationCircleOutlined,
  LoadingOutlined,
} from "@ant-design/icons";
import { useQuery } from "@tanstack/react-query";

import {
  previewBatchWorkflow,
  executeBatchWorkflow,
  getBatchWorkflowStatus,
  type BatchWorkflowPreviewRequest,
  type BatchWorkflowPreviewResponse,
  type NodePreviewItem,
  type BatchWorkflowStatusResponse,
} from "../../api/batchOperations";
import { listNodeWorkflows } from "../../api/workflows";

const { Text, Paragraph } = Typography;

// localStorage 键名
const STORAGE_KEY = "ydms_batch_workflow_defaults";

// 默认选项类型
interface BatchWorkflowDefaults {
  workflowKey?: string;
  includeDescendants: boolean;
  skipNoSource: boolean;
  skipNoOutput: boolean;
  skipNameContains: boolean;
  skipNamePattern: string;
}

// 从 localStorage 加载默认选项
function loadDefaults(): BatchWorkflowDefaults {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) {
      const parsed = JSON.parse(stored);
      return {
        includeDescendants: true,
        skipNoSource: true,
        skipNoOutput: true,
        skipNameContains: false,
        skipNamePattern: "",
        ...parsed,
      };
    }
  } catch {
    // 忽略解析错误
  }
  return {
    includeDescendants: true,
    skipNoSource: true,
    skipNoOutput: true,
    skipNameContains: false,
    skipNamePattern: "",
  };
}

// 保存默认选项到 localStorage
function saveDefaults(defaults: BatchWorkflowDefaults): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(defaults));
  } catch {
    // 忽略存储错误
  }
}

// 步骤定义
const STEPS = [
  { title: "配置", description: "选择工作流" },
  { title: "预览", description: "确认范围" },
  { title: "执行", description: "运行中" },
  { title: "完成", description: "查看结果" },
];

// 状态颜色映射
const statusColors: Record<string, string> = {
  pending: "default",
  running: "processing",
  completed: "success",
  failed: "error",
  cancelled: "warning",
  success: "success",
  skipped: "warning",
};

// 状态标签映射
const statusLabels: Record<string, string> = {
  pending: "等待中",
  running: "运行中",
  completed: "已完成",
  failed: "失败",
  cancelled: "已取消",
  success: "成功",
  skipped: "跳过",
};

interface BatchWorkflowModalProps {
  open: boolean;
  nodeId: number;
  nodeName: string;
  onClose: () => void;
  onSuccess?: () => void;
}

export function BatchWorkflowModal({
  open,
  nodeId,
  nodeName,
  onClose,
  onSuccess,
}: BatchWorkflowModalProps) {
  // 步骤状态
  const [currentStep, setCurrentStep] = useState(0);

  // 从 localStorage 加载默认值
  const [defaults] = useState(loadDefaults);

  // 配置状态（使用保存的默认值初始化）
  const [workflowKey, setWorkflowKey] = useState<string | undefined>(defaults.workflowKey);
  const [includeDescendants, setIncludeDescendants] = useState(defaults.includeDescendants);
  const [skipNoSource, setSkipNoSource] = useState(defaults.skipNoSource);
  const [skipNoOutput, setSkipNoOutput] = useState(defaults.skipNoOutput);
  const [skipNameContains, setSkipNameContains] = useState(defaults.skipNameContains);
  const [skipNamePattern, setSkipNamePattern] = useState(defaults.skipNamePattern);

  // 预览和执行状态
  const [previewData, setPreviewData] = useState<BatchWorkflowPreviewResponse | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [batchId, setBatchId] = useState<string | null>(null);
  const [executeLoading, setExecuteLoading] = useState(false);

  // 获取可用工作流列表
  const { data: workflows, isLoading: workflowsLoading } = useQuery({
    queryKey: ["node-workflows", nodeId],
    queryFn: () => listNodeWorkflows(nodeId),
    enabled: open,
  });

  // 轮询批次状态
  const { data: batchStatus } = useQuery({
    queryKey: ["batch-workflow-status", batchId],
    queryFn: () => getBatchWorkflowStatus(batchId!),
    enabled: currentStep === 2 && batchId !== null,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (status === "completed" || status === "failed" || status === "cancelled") {
        return false; // 停止轮询
      }
      return 2000; // 每 2 秒轮询
    },
  });

  // 批次完成时自动跳转到完成步骤
  useEffect(() => {
    if (batchStatus && currentStep === 2) {
      const { status } = batchStatus;
      if (status === "completed" || status === "failed" || status === "cancelled") {
        setCurrentStep(3);
        if (status === "completed" && onSuccess) {
          onSuccess();
        }
      }
    }
  }, [batchStatus, currentStep, onSuccess]);

  // 重置状态（使用保存的默认值）
  const resetState = useCallback(() => {
    const savedDefaults = loadDefaults();
    setCurrentStep(0);
    setWorkflowKey(savedDefaults.workflowKey);
    setIncludeDescendants(savedDefaults.includeDescendants);
    setSkipNoSource(savedDefaults.skipNoSource);
    setSkipNoOutput(savedDefaults.skipNoOutput);
    setSkipNameContains(savedDefaults.skipNameContains);
    setSkipNamePattern(savedDefaults.skipNamePattern);
    setPreviewData(null);
    setBatchId(null);
  }, []);

  // 关闭时重置
  const handleClose = useCallback(() => {
    resetState();
    onClose();
  }, [resetState, onClose]);

  // 预览
  const handlePreview = useCallback(async () => {
    if (!workflowKey) {
      message.warning("请选择工作流");
      return;
    }

    setPreviewLoading(true);
    try {
      const request: BatchWorkflowPreviewRequest = {
        workflow_key: workflowKey,
        include_descendants: includeDescendants,
        skip_no_source: skipNoSource,
        skip_no_output: skipNoOutput,
        skip_name_contains: skipNameContains && skipNamePattern ? skipNamePattern : undefined,
      };
      const result = await previewBatchWorkflow(nodeId, request);
      setPreviewData(result);
      setCurrentStep(1);
    } catch (error) {
      message.error(`预览失败: ${(error as Error).message}`);
    } finally {
      setPreviewLoading(false);
    }
  }, [nodeId, workflowKey, includeDescendants, skipNoSource, skipNoOutput, skipNameContains, skipNamePattern]);

  // 执行
  const handleExecute = useCallback(async () => {
    if (!workflowKey) {
      return;
    }

    setExecuteLoading(true);
    try {
      const result = await executeBatchWorkflow(nodeId, {
        workflow_key: workflowKey,
        include_descendants: includeDescendants,
        skip_no_source: skipNoSource,
        skip_no_output: skipNoOutput,
        skip_name_contains: skipNameContains && skipNamePattern ? skipNamePattern : undefined,
        concurrency: 5,
      });
      setBatchId(result.batch_id);
      setCurrentStep(2);
      message.success("批量工作流已启动");

      // 执行成功后保存当前选项为默认值
      saveDefaults({
        workflowKey,
        includeDescendants,
        skipNoSource,
        skipNoOutput,
        skipNameContains,
        skipNamePattern,
      });
    } catch (error) {
      message.error(`执行失败: ${(error as Error).message}`);
    } finally {
      setExecuteLoading(false);
    }
  }, [nodeId, workflowKey, includeDescendants, skipNoSource, skipNoOutput, skipNameContains, skipNamePattern]);

  // 预览表格列
  const previewColumns = [
    {
      title: "节点名称",
      dataIndex: "node_name",
      key: "node_name",
      ellipsis: true,
    },
    {
      title: "路径",
      dataIndex: "node_path",
      key: "node_path",
      ellipsis: true,
      render: (path: string) => (
        <Text type="secondary" style={{ fontSize: 12 }}>
          {path}
        </Text>
      ),
    },
    {
      title: "源文档数",
      dataIndex: "source_doc_count",
      key: "source_doc_count",
      width: 100,
      align: "center" as const,
    },
    {
      title: "状态",
      dataIndex: "can_execute",
      key: "status",
      width: 120,
      render: (canExecute: boolean, record: NodePreviewItem) => (
        canExecute ? (
          <Tag color="success" icon={<CheckCircleOutlined />}>
            可执行
          </Tag>
        ) : (
          <Tag color="warning" icon={<ExclamationCircleOutlined />}>
            {record.skip_reason || "跳过"}
          </Tag>
        )
      ),
    },
  ];

  // 结果表格列
  const resultColumns = [
    {
      title: "节点名称",
      dataIndex: "node_name",
      key: "node_name",
      ellipsis: true,
    },
    {
      title: "路径",
      dataIndex: "node_path",
      key: "node_path",
      ellipsis: true,
      render: (path: string) => (
        <Text type="secondary" style={{ fontSize: 12 }}>
          {path}
        </Text>
      ),
    },
    {
      title: "状态",
      dataIndex: "status",
      key: "status",
      width: 100,
      render: (status: string) => (
        <Tag color={statusColors[status]}>{statusLabels[status] || status}</Tag>
      ),
    },
    {
      title: "错误信息",
      dataIndex: "error",
      key: "error",
      ellipsis: true,
      render: (error: string, record: { reason?: string }) => (
        <Text type="danger" style={{ fontSize: 12 }}>
          {error || record.reason || "-"}
        </Text>
      ),
    },
  ];

  // 渲染配置步骤
  const renderConfigStep = () => (
    <Space direction="vertical" style={{ width: "100%" }} size="large">
      <div>
        <Text strong>目标节点</Text>
        <Paragraph>
          <Tag color="blue">{nodeName}</Tag>
          <Text type="secondary">(ID: {nodeId})</Text>
        </Paragraph>
      </div>

      <div>
        <Text strong>选择工作流</Text>
        <Select
          style={{ width: "100%", marginTop: 8 }}
          placeholder="请选择要执行的工作流"
          value={workflowKey}
          onChange={setWorkflowKey}
          loading={workflowsLoading}
          options={workflows?.map((w) => ({
            label: w.name,
            value: w.workflow_key,
            description: w.description,
          }))}
          optionRender={(option) => (
            <div>
              <div>{option.data.label}</div>
              {option.data.description && (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {option.data.description}
                </Text>
              )}
            </div>
          )}
        />
      </div>

      <div>
        <Space>
          <Switch
            checked={includeDescendants}
            onChange={setIncludeDescendants}
          />
          <Text>包含子孙节点</Text>
        </Space>
      </div>

      <div>
        <Space>
          <Switch checked={skipNoSource} onChange={setSkipNoSource} />
          <Text>跳过无源文档节点</Text>
        </Space>
      </div>

      <div>
        <Space>
          <Switch checked={skipNoOutput} onChange={setSkipNoOutput} />
          <Text>跳过无产出文档的节点</Text>
        </Space>
      </div>

      <div>
        <Space align="start">
          <Switch
            checked={skipNameContains}
            onChange={setSkipNameContains}
            style={{ marginTop: 4 }}
          />
          <div>
            <Text>跳过节点名包含</Text>
            <Input
              placeholder="输入要跳过的文本"
              value={skipNamePattern}
              onChange={(e) => setSkipNamePattern(e.target.value)}
              disabled={!skipNameContains}
              style={{ width: 200, marginLeft: 8 }}
              size="small"
            />
          </div>
        </Space>
      </div>
    </Space>
  );

  // 渲染预览步骤
  const renderPreviewStep = () => {
    if (!previewData) {
      return <Spin />;
    }

    return (
      <Space direction="vertical" style={{ width: "100%" }} size="middle">
        <Alert
          type="info"
          showIcon
          message={
            <Space split={<span style={{ color: "#d9d9d9" }}>|</span>}>
              <span>工作流：<strong>{previewData.workflow_name}</strong></span>
              <span>总节点数：<strong>{previewData.total_nodes}</strong></span>
              <span>
                可执行：<Text type="success">{previewData.can_execute}</Text>
              </span>
              <span>
                将跳过：<Text type="warning">{previewData.will_skip}</Text>
              </span>
            </Space>
          }
        />

        <Table
          dataSource={previewData.nodes}
          columns={previewColumns}
          rowKey="node_id"
          size="small"
          pagination={{ pageSize: 10, showSizeChanger: true }}
          scroll={{ y: 300 }}
        />
      </Space>
    );
  };

  // 渲染执行步骤
  const renderExecuteStep = () => {
    if (!batchStatus) {
      return (
        <div style={{ textAlign: "center", padding: 48 }}>
          <Spin size="large" />
          <Paragraph style={{ marginTop: 16 }}>正在启动批量工作流...</Paragraph>
        </div>
      );
    }

    const { status, total_nodes, success_count, failed_count, skipped_count, progress } = batchStatus;
    const completed = success_count + failed_count + skipped_count;

    return (
      <Space direction="vertical" style={{ width: "100%" }} size="large">
        <div style={{ textAlign: "center" }}>
          <Progress
            type="circle"
            percent={Math.round(progress)}
            status={status === "failed" ? "exception" : undefined}
            format={() => (
              <span>
                {completed}/{total_nodes}
              </span>
            )}
          />
          <div style={{ marginTop: 16 }}>
            <Tag color={statusColors[status]} icon={<LoadingOutlined />}>
              {statusLabels[status]}
            </Tag>
          </div>
        </div>

        <Descriptions size="small" bordered column={2}>
          <Descriptions.Item label="成功">
            <Text type="success">{success_count}</Text>
          </Descriptions.Item>
          <Descriptions.Item label="失败">
            <Text type="danger">{failed_count}</Text>
          </Descriptions.Item>
          <Descriptions.Item label="跳过">
            <Text type="warning">{skipped_count}</Text>
          </Descriptions.Item>
          <Descriptions.Item label="进度">{progress.toFixed(1)}%</Descriptions.Item>
        </Descriptions>
      </Space>
    );
  };

  // 渲染完成步骤
  const renderCompleteStep = () => {
    if (!batchStatus) {
      return <Spin />;
    }

    const { status, success_count, failed_count, skipped_count, details } = batchStatus;
    const isSuccess = status === "completed" && failed_count === 0;
    const nodeResults = details?.node_results || [];

    return (
      <Space direction="vertical" style={{ width: "100%" }} size="middle">
        <Result
          status={isSuccess ? "success" : failed_count > 0 ? "warning" : "info"}
          title={isSuccess ? "批量工作流执行完成" : "批量工作流执行完成（部分失败）"}
          subTitle={
            <Space split={<span style={{ color: "#d9d9d9" }}>|</span>}>
              <span>
                成功：<Text type="success">{success_count}</Text>
              </span>
              <span>
                失败：<Text type="danger">{failed_count}</Text>
              </span>
              <span>
                跳过：<Text type="warning">{skipped_count}</Text>
              </span>
            </Space>
          }
        />

        {nodeResults.length > 0 && (
          <Table
            dataSource={nodeResults}
            columns={resultColumns}
            rowKey="node_id"
            size="small"
            pagination={{ pageSize: 10, showSizeChanger: true }}
            scroll={{ y: 250 }}
          />
        )}
      </Space>
    );
  };

  // 渲染步骤内容
  const renderStepContent = () => {
    switch (currentStep) {
      case 0:
        return renderConfigStep();
      case 1:
        return renderPreviewStep();
      case 2:
        return renderExecuteStep();
      case 3:
        return renderCompleteStep();
      default:
        return null;
    }
  };

  // 渲染底部按钮
  const renderFooter = () => {
    const buttons: React.ReactNode[] = [];

    // 关闭按钮
    if (currentStep === 3) {
      buttons.push(
        <Button key="close" type="primary" onClick={handleClose}>
          关闭
        </Button>
      );
    } else {
      buttons.push(
        <Button key="cancel" onClick={handleClose} disabled={currentStep === 2}>
          取消
        </Button>
      );
    }

    // 上一步按钮
    if (currentStep === 1) {
      buttons.unshift(
        <Button key="prev" onClick={() => setCurrentStep(0)}>
          上一步
        </Button>
      );
    }

    // 预览按钮
    if (currentStep === 0) {
      buttons.push(
        <Button
          key="preview"
          type="primary"
          icon={<ThunderboltOutlined />}
          onClick={handlePreview}
          loading={previewLoading}
          disabled={!workflowKey}
        >
          预览
        </Button>
      );
    }

    // 执行按钮
    if (currentStep === 1) {
      buttons.push(
        <Button
          key="execute"
          type="primary"
          icon={<ThunderboltOutlined />}
          onClick={handleExecute}
          loading={executeLoading}
          disabled={!previewData || previewData.can_execute === 0}
        >
          开始执行 ({previewData?.can_execute || 0} 个节点)
        </Button>
      );
    }

    return buttons;
  };

  return (
    <Modal
      title={
        <Space>
          <ThunderboltOutlined />
          批量执行工作流
        </Space>
      }
      open={open}
      onCancel={handleClose}
      footer={renderFooter()}
      width={800}
      maskClosable={currentStep !== 2}
      closable={currentStep !== 2}
      destroyOnHidden
    >
      <Steps
        current={currentStep}
        items={STEPS}
        style={{ marginBottom: 24 }}
        size="small"
      />
      {renderStepContent()}
    </Modal>
  );
}
