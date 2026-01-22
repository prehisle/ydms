import { useState, useEffect, useCallback, useRef, type ReactNode } from "react";
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
import { useQuery, useQueryClient } from "@tanstack/react-query";

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

// 单个节点的执行结果
interface NodeExecutionResult {
  nodeId: number;
  nodeName: string;
  batchId: string;
  status: BatchWorkflowStatusResponse;
}

interface BatchWorkflowModalProps {
  open: boolean;
  nodeIds: number[];
  nodeNames: string[];
  onClose: () => void;
  onSuccess?: () => void;
}

export function BatchWorkflowModal({
  open,
  nodeIds,
  nodeNames,
  onClose,
  onSuccess,
}: BatchWorkflowModalProps) {
  const queryClient = useQueryClient();

  // 第一个节点 ID（用于获取工作流列表）
  const primaryNodeId = nodeIds[0];
  const nodeCount = nodeIds.length;

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

  // 预览状态
  const [previewData, setPreviewData] = useState<BatchWorkflowPreviewResponse | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);

  // 串行执行状态
  const [currentNodeIndex, setCurrentNodeIndex] = useState(0); // 当前处理的节点索引
  const [currentBatchId, setCurrentBatchId] = useState<string | null>(null);
  const [executeLoading, setExecuteLoading] = useState(false);
  const [executionResults, setExecutionResults] = useState<NodeExecutionResult[]>([]);
  const isExecutingRef = useRef(false); // 防止重复执行

  // 获取可用工作流列表（使用第一个节点）
  const { data: workflows, isLoading: workflowsLoading } = useQuery({
    queryKey: ["node-workflows", primaryNodeId],
    queryFn: () => listNodeWorkflows(primaryNodeId),
    enabled: open && primaryNodeId != null,
  });

  // 轮询当前批次状态
  const { data: batchStatus } = useQuery({
    queryKey: ["batch-workflow-status", currentBatchId],
    queryFn: () => getBatchWorkflowStatus(currentBatchId!),
    enabled: currentStep === 2 && currentBatchId !== null,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (status === "completed" || status === "failed" || status === "cancelled") {
        return false; // 停止轮询
      }
      return 2000; // 每 2 秒轮询
    },
  });

  // 执行下一个节点的函数
  const executeNextNode = useCallback(async (
    nodeIndex: number,
    results: NodeExecutionResult[]
  ) => {
    if (nodeIndex >= nodeIds.length || !workflowKey) {
      // 所有节点执行完毕
      setExecutionResults(results);
      setCurrentStep(3);
      if (onSuccess) {
        onSuccess();
      }
      isExecutingRef.current = false;
      return;
    }

    const nodeId = nodeIds[nodeIndex];
    const nodeName = nodeNames[nodeIndex];

    try {
      const result = await executeBatchWorkflow(nodeId, {
        workflow_key: workflowKey,
        include_descendants: includeDescendants,
        skip_no_source: skipNoSource,
        skip_no_output: skipNoOutput,
        skip_name_contains: skipNameContains && skipNamePattern ? skipNamePattern : undefined,
      });

      setCurrentNodeIndex(nodeIndex);
      setCurrentBatchId(result.batch_id);

      // 保存当前节点的 batch_id，等待轮询完成后继续
    } catch (error) {
      message.error(`节点 "${nodeName}" 执行失败: ${(error as Error).message}`);
      // 即使失败也继续下一个节点
      const failedResult: NodeExecutionResult = {
        nodeId,
        nodeName,
        batchId: "",
        status: {
          batch_id: "",
          workflow_key: workflowKey,
          root_node_id: nodeId,
          status: "failed",
          total_nodes: 0,
          success_count: 0,
          failed_count: 1,
          skipped_count: 0,
          progress: 100,
          error_message: (error as Error).message,
          created_at: new Date().toISOString(),
        },
      };
      executeNextNode(nodeIndex + 1, [...results, failedResult]);
    }
  }, [nodeIds, nodeNames, workflowKey, includeDescendants, skipNoSource, skipNoOutput, skipNameContains, skipNamePattern, onSuccess]);

  // 当前批次完成时，处理下一个节点
  useEffect(() => {
    if (!batchStatus || currentStep !== 2 || !isExecutingRef.current) return;

    const { status } = batchStatus;
    if (status === "completed" || status === "failed" || status === "cancelled") {
      // 当前批次完成，保存结果并处理下一个节点
      const currentResult: NodeExecutionResult = {
        nodeId: nodeIds[currentNodeIndex],
        nodeName: nodeNames[currentNodeIndex],
        batchId: currentBatchId || "",
        status: batchStatus,
      };

      const newResults = [...executionResults, currentResult];
      setExecutionResults(newResults);

      // 清除当前批次状态缓存
      queryClient.removeQueries({ queryKey: ["batch-workflow-status", currentBatchId] });

      // 处理下一个节点
      const nextIndex = currentNodeIndex + 1;
      if (nextIndex < nodeIds.length) {
        setCurrentBatchId(null);
        executeNextNode(nextIndex, newResults);
      } else {
        // 所有节点执行完毕
        setCurrentStep(3);
        if (onSuccess) {
          onSuccess();
        }
        isExecutingRef.current = false;
      }
    }
  }, [batchStatus, currentStep, currentNodeIndex, currentBatchId, nodeIds, nodeNames, executionResults, executeNextNode, onSuccess, queryClient]);

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
    setCurrentNodeIndex(0);
    setCurrentBatchId(null);
    setExecutionResults([]);
    isExecutingRef.current = false;
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

      // 对所有节点分别预览并合并结果
      const results = await Promise.all(
        nodeIds.map(nodeId => previewBatchWorkflow(nodeId, request))
      );

      // 合并预览结果
      const mergedNodes: NodePreviewItem[] = [];
      let totalNodes = 0;
      let canExecuteCount = 0;
      let willSkipCount = 0;

      for (const result of results) {
        mergedNodes.push(...result.nodes);
        totalNodes += result.total_nodes;
        canExecuteCount += result.can_execute;
        willSkipCount += result.will_skip;
      }

      const mergedResult: BatchWorkflowPreviewResponse = {
        root_node_id: nodeIds[0],
        workflow_key: workflowKey,
        workflow_name: results[0]?.workflow_name || "",
        total_nodes: totalNodes,
        can_execute: canExecuteCount,
        will_skip: willSkipCount,
        nodes: mergedNodes,
      };

      setPreviewData(mergedResult);
      setCurrentStep(1);
    } catch (error) {
      message.error(`预览失败: ${(error as Error).message}`);
    } finally {
      setPreviewLoading(false);
    }
  }, [nodeIds, workflowKey, includeDescendants, skipNoSource, skipNoOutput, skipNameContains, skipNamePattern]);

  // 开始执行（串行）
  const handleExecute = useCallback(async () => {
    if (!workflowKey || isExecutingRef.current) {
      return;
    }

    setExecuteLoading(true);
    isExecutingRef.current = true;

    // 保存当前选项为默认值
    saveDefaults({
      workflowKey,
      includeDescendants,
      skipNoSource,
      skipNoOutput,
      skipNameContains,
      skipNamePattern,
    });

    // 进入执行步骤
    setCurrentStep(2);
    setCurrentNodeIndex(0);
    setExecutionResults([]);

    // 开始执行第一个节点
    executeNextNode(0, []);

    setExecuteLoading(false);
  }, [workflowKey, includeDescendants, skipNoSource, skipNoOutput, skipNameContains, skipNamePattern, executeNextNode]);

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
        <Text strong>目标节点 ({nodeCount} 个)</Text>
        <Paragraph>
          {nodeNames.map((name, index) => (
            <Tag key={nodeIds[index]} color="blue" style={{ marginBottom: 4 }}>
              {name}
            </Tag>
          ))}
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
    const currentNodeName = nodeNames[currentNodeIndex] || "";
    const completedNodes = executionResults.length;

    // 如果当前批次还没有状态，显示启动中
    if (!batchStatus) {
      return (
        <div style={{ textAlign: "center", padding: 48 }}>
          <Spin size="large" />
          <Paragraph style={{ marginTop: 16 }}>
            正在启动节点 {currentNodeIndex + 1}/{nodeCount}：{currentNodeName}
          </Paragraph>
        </div>
      );
    }

    const { status, total_nodes, success_count, failed_count, skipped_count, progress } = batchStatus;
    const completed = success_count + failed_count + skipped_count;

    return (
      <Space direction="vertical" style={{ width: "100%" }} size="large">
        {/* 总体进度 */}
        <Alert
          type="info"
          showIcon
          message={
            <Space>
              <span>
                正在处理节点 <strong>{currentNodeIndex + 1}/{nodeCount}</strong>：
              </span>
              <Tag color="blue">{currentNodeName}</Tag>
              {completedNodes > 0 && (
                <Text type="secondary">（已完成 {completedNodes} 个节点）</Text>
              )}
            </Space>
          }
        />

        {/* 当前批次进度 */}
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
    // 汇总所有节点的执行结果
    let totalSuccess = 0;
    let totalFailed = 0;
    let totalSkipped = 0;
    const allNodeResults: Array<Record<string, unknown>> = [];

    for (const result of executionResults) {
      totalSuccess += result.status.success_count;
      totalFailed += result.status.failed_count;
      totalSkipped += result.status.skipped_count;

      // 收集详细结果
      const nodeResults = result.status.details?.node_results || [];
      allNodeResults.push(...(nodeResults as Array<Record<string, unknown>>));
    }

    const allSuccess = totalFailed === 0;

    return (
      <Space direction="vertical" style={{ width: "100%" }} size="middle">
        <Result
          status={allSuccess ? "success" : totalFailed > 0 ? "warning" : "info"}
          title={allSuccess ? "批量工作流执行完成" : "批量工作流执行完成（部分失败）"}
          subTitle={
            <Space direction="vertical" size="small">
              <Text>共处理 {nodeCount} 个根节点</Text>
              <Space split={<span style={{ color: "#d9d9d9" }}>|</span>}>
                <span>
                  成功：<Text type="success">{totalSuccess}</Text>
                </span>
                <span>
                  失败：<Text type="danger">{totalFailed}</Text>
                </span>
                <span>
                  跳过：<Text type="warning">{totalSkipped}</Text>
                </span>
              </Space>
            </Space>
          }
        />

        {/* 每个根节点的执行摘要 */}
        {executionResults.length > 1 && (
          <div>
            <Text strong style={{ marginBottom: 8, display: "block" }}>各节点执行情况：</Text>
            <Space direction="vertical" style={{ width: "100%" }} size="small">
              {executionResults.map((result, index) => (
                <div
                  key={result.nodeId}
                  style={{
                    padding: 8,
                    border: "1px solid #f0f0f0",
                    borderRadius: 4,
                    display: "flex",
                    justifyContent: "space-between",
                    alignItems: "center",
                  }}
                >
                  <Space>
                    <Text>{index + 1}.</Text>
                    <Text strong>{result.nodeName}</Text>
                  </Space>
                  <Space size="small">
                    <Tag color="success">{result.status.success_count} 成功</Tag>
                    {result.status.failed_count > 0 && (
                      <Tag color="error">{result.status.failed_count} 失败</Tag>
                    )}
                    {result.status.skipped_count > 0 && (
                      <Tag color="warning">{result.status.skipped_count} 跳过</Tag>
                    )}
                  </Space>
                </div>
              ))}
            </Space>
          </div>
        )}

        {/* 详细结果表格 */}
        {allNodeResults.length > 0 && (
          <Table
            dataSource={allNodeResults}
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
    const buttons: ReactNode[] = [];

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
