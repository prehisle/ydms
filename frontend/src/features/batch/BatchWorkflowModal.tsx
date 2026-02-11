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
  previewBatchSync,
  executeBatchSync,
  getBatchSyncStatus,
  type BatchWorkflowPreviewRequest,
  type BatchWorkflowPreviewResponse,
  type NodePreviewItem,
  type BatchWorkflowStatusResponse,
  type BatchSyncPreviewResponse,
  type DocumentPreviewItem,
  type BatchSyncStatusResponse,
} from "../../api/batchOperations";
import { listNodeWorkflows } from "../../api/workflows";

const { Text, Paragraph } = Typography;

// localStorage 键名
const STORAGE_KEY = "ydms_batch_workflow_defaults";

// 单个工作流的配置
interface WorkflowConfig {
  includeDescendants: boolean;
  skipNoSource: boolean;
  skipNoOutput: boolean;
  skipNameContains: boolean;
  skipNamePatterns: string[];
  skipDocTypes: string[];
  customParams: Record<string, unknown>;  // 工作流自定义参数
}

// 默认选项类型（按工作流分别保存）
interface BatchWorkflowDefaults {
  lastWorkflowKey?: string;
  configs: Record<string, WorkflowConfig>;
}

// 默认配置
const defaultConfig: WorkflowConfig = {
  includeDescendants: true,
  skipNoSource: true,
  skipNoOutput: true,
  skipNameContains: false,
  skipNamePatterns: [],
  skipDocTypes: [],
  customParams: {},
};

// 从 localStorage 加载默认选项
function loadDefaults(): BatchWorkflowDefaults {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) {
      const parsed = JSON.parse(stored);
      return {
        lastWorkflowKey: parsed.lastWorkflowKey,
        configs: parsed.configs || {},
      };
    }
  } catch {
    // 忽略解析错误
  }
  return {
    configs: {},
  };
}

// 获取指定工作流的配置（兼容旧格式 skipNamePattern: string → skipNamePatterns: string[]）
function getWorkflowConfig(workflowKey: string): WorkflowConfig {
  const defaults = loadDefaults();
  const raw = defaults.configs[workflowKey];
  if (!raw) return { ...defaultConfig };
  // 兼容旧格式
  if (!raw.skipNamePatterns && (raw as unknown as { skipNamePattern?: string }).skipNamePattern) {
    raw.skipNamePatterns = [(raw as unknown as { skipNamePattern: string }).skipNamePattern];
  }
  if (!raw.skipNamePatterns) {
    raw.skipNamePatterns = [];
  }
  return raw;
}

// 保存指定工作流的配置
function saveWorkflowConfig(workflowKey: string, config: WorkflowConfig): void {
  try {
    const defaults = loadDefaults();
    defaults.lastWorkflowKey = workflowKey;
    defaults.configs[workflowKey] = config;
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
  const [workflowKey, setWorkflowKey] = useState<string | undefined>(defaults.lastWorkflowKey);
  const [includeDescendants, setIncludeDescendants] = useState(defaultConfig.includeDescendants);
  const [skipNoSource, setSkipNoSource] = useState(defaultConfig.skipNoSource);
  const [skipNoOutput, setSkipNoOutput] = useState(defaultConfig.skipNoOutput);
  const [skipNameContains, setSkipNameContains] = useState(defaultConfig.skipNameContains);
  const [skipNamePatterns, setSkipNamePatterns] = useState<string[]>(defaultConfig.skipNamePatterns);
  const [skipDocTypes, setSkipDocTypes] = useState<string[]>(defaultConfig.skipDocTypes);
  const [customParams, setCustomParams] = useState<Record<string, unknown>>(defaultConfig.customParams);

  // 判断是否为同步模式
  const isSyncMode = workflowKey === "sync_to_mysql";

  // 预览状态（支持工作流和同步两种模式）
  const [previewData, setPreviewData] = useState<BatchWorkflowPreviewResponse | null>(null);
  const [syncPreviewData, setSyncPreviewData] = useState<BatchSyncPreviewResponse | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);

  // 串行执行状态
  const [currentNodeIndex, setCurrentNodeIndex] = useState(0); // 当前处理的节点索引
  const [currentBatchId, setCurrentBatchId] = useState<string | null>(null);
  const [executeLoading, setExecuteLoading] = useState(false);
  const [executionResults, setExecutionResults] = useState<NodeExecutionResult[]>([]);
  const isExecutingRef = useRef(false); // 防止重复执行

  // 当 workflowKey 变化时，加载该工作流的配置
  useEffect(() => {
    if (workflowKey) {
      const config = getWorkflowConfig(workflowKey);
      setIncludeDescendants(config.includeDescendants);
      setSkipNoSource(config.skipNoSource);
      setSkipNoOutput(config.skipNoOutput);
      setSkipNameContains(config.skipNameContains);
      setSkipNamePatterns(config.skipNamePatterns);
      setSkipDocTypes(config.skipDocTypes);
      setCustomParams(config.customParams || {});
    }
  }, [workflowKey]);

  // 获取可用工作流列表（使用第一个节点）
  const { data: workflows, isLoading: workflowsLoading } = useQuery({
    queryKey: ["node-workflows", primaryNodeId],
    queryFn: () => listNodeWorkflows(primaryNodeId),
    enabled: open && primaryNodeId != null,
  });

  // 轮询当前批次状态（根据模式选择不同的 API）
  const { data: batchStatus } = useQuery<BatchWorkflowStatusResponse | BatchSyncStatusResponse>({
    queryKey: [isSyncMode ? "batch-sync-status" : "batch-workflow-status", currentBatchId],
    queryFn: async () => {
      if (isSyncMode) {
        return getBatchSyncStatus(currentBatchId!);
      }
      return getBatchWorkflowStatus(currentBatchId!);
    },
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
    results: NodeExecutionResult[],
    syncMode: boolean
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
      let batchId: string;
      if (syncMode) {
        // 同步模式：调用批量同步 API
        const result = await executeBatchSync(nodeId, {
          include_descendants: includeDescendants,
          skip_doc_types: skipDocTypes.length > 0 ? skipDocTypes : undefined,
        });
        batchId = result.batch_id;
      } else {
        // 工作流模式：调用批量工作流 API
        const result = await executeBatchWorkflow(nodeId, {
          workflow_key: workflowKey,
          include_descendants: includeDescendants,
          skip_no_source: skipNoSource,
          skip_no_output: skipNoOutput,
          skip_name_contains: skipNameContains && skipNamePatterns.length > 0 ? skipNamePatterns.join(",") : undefined,
          skip_doc_types: skipDocTypes.length > 0 ? skipDocTypes : undefined,
          parameters: Object.keys(customParams).length > 0 ? customParams : undefined,
        });
        batchId = result.batch_id;
      }

      setCurrentNodeIndex(nodeIndex);
      setCurrentBatchId(batchId);

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
      executeNextNode(nodeIndex + 1, [...results, failedResult], syncMode);
    }
  }, [nodeIds, nodeNames, workflowKey, includeDescendants, skipNoSource, skipNoOutput, skipNameContains, skipNamePatterns, skipDocTypes, customParams, onSuccess]);

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
        status: batchStatus as BatchWorkflowStatusResponse,
      };

      const newResults = [...executionResults, currentResult];
      setExecutionResults(newResults);

      // 清除当前批次状态缓存
      queryClient.removeQueries({ queryKey: [isSyncMode ? "batch-sync-status" : "batch-workflow-status", currentBatchId] });

      // 处理下一个节点
      const nextIndex = currentNodeIndex + 1;
      if (nextIndex < nodeIds.length) {
        setCurrentBatchId(null);
        executeNextNode(nextIndex, newResults, isSyncMode);
      } else {
        // 所有节点执行完毕
        setCurrentStep(3);
        if (onSuccess) {
          onSuccess();
        }
        isExecutingRef.current = false;
      }
    }
  }, [batchStatus, currentStep, currentNodeIndex, currentBatchId, nodeIds, nodeNames, executionResults, executeNextNode, onSuccess, queryClient, isSyncMode]);

  // 重置状态（使用保存的默认值）
  const resetState = useCallback(() => {
    const savedDefaults = loadDefaults();
    setCurrentStep(0);
    setWorkflowKey(savedDefaults.lastWorkflowKey);
    // 配置会在 workflowKey 变化时通过 useEffect 加载
    setPreviewData(null);
    setSyncPreviewData(null);
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
      if (isSyncMode) {
        // 同步模式：调用批量同步预览 API
        const results = await Promise.all(
          nodeIds.map(nodeId => previewBatchSync(nodeId, {
            include_descendants: includeDescendants,
            skip_doc_types: skipDocTypes.length > 0 ? skipDocTypes : undefined,
          }))
        );

        // 合并预览结果
        const mergedDocs: DocumentPreviewItem[] = [];
        let totalDocs = 0;
        let canSyncCount = 0;
        let willSkipCount = 0;

        for (const result of results) {
          mergedDocs.push(...result.documents);
          totalDocs += result.total_documents;
          canSyncCount += result.can_sync;
          willSkipCount += result.will_skip;
        }

        const mergedResult: BatchSyncPreviewResponse = {
          root_node_id: nodeIds[0],
          total_documents: totalDocs,
          can_sync: canSyncCount,
          will_skip: willSkipCount,
          documents: mergedDocs,
        };

        setSyncPreviewData(mergedResult);
        setPreviewData(null);
      } else {
        // 工作流模式：调用批量工作流预览 API
        const request: BatchWorkflowPreviewRequest = {
          workflow_key: workflowKey,
          include_descendants: includeDescendants,
          skip_no_source: skipNoSource,
          skip_no_output: skipNoOutput,
          skip_name_contains: skipNameContains && skipNamePatterns.length > 0 ? skipNamePatterns.join(",") : undefined,
          skip_doc_types: skipDocTypes.length > 0 ? skipDocTypes : undefined,
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
        setSyncPreviewData(null);
      }
      setCurrentStep(1);
    } catch (error) {
      message.error(`预览失败: ${(error as Error).message}`);
    } finally {
      setPreviewLoading(false);
    }
  }, [nodeIds, workflowKey, includeDescendants, skipNoSource, skipNoOutput, skipNameContains, skipNamePatterns, skipDocTypes, isSyncMode]);

  // 开始执行（串行）
  const handleExecute = useCallback(async () => {
    if (!workflowKey || isExecutingRef.current) {
      return;
    }

    setExecuteLoading(true);
    isExecutingRef.current = true;

    // 保存当前选项为默认值
    saveWorkflowConfig(workflowKey, {
      includeDescendants,
      skipNoSource,
      skipNoOutput,
      skipNameContains,
      skipNamePatterns,
      skipDocTypes,
      customParams,
    });

    // 进入执行步骤
    setCurrentStep(2);
    setCurrentNodeIndex(0);
    setExecutionResults([]);

    // 开始执行第一个节点
    executeNextNode(0, [], isSyncMode);

    setExecuteLoading(false);
  }, [workflowKey, includeDescendants, skipNoSource, skipNoOutput, skipNameContains, skipNamePatterns, skipDocTypes, customParams, executeNextNode, isSyncMode]);

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

      {/* 工作流模式特有选项 */}
      {!isSyncMode && (
        <>
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
                <Select
                  mode="tags"
                  tokenSeparators={[","]}
                  placeholder="输入关键词后回车"
                  value={skipNamePatterns}
                  onChange={setSkipNamePatterns}
                  disabled={!skipNameContains}
                  style={{ width: 300, marginLeft: 8 }}
                  size="small"
                />
              </div>
            </Space>
          </div>
        </>
      )}

      {/* 跳过文档类型选项 */}
      <div>
        <Text strong>跳过文档类型</Text>
        <Select
          mode="multiple"
          style={{ width: "100%", marginTop: 8 }}
          placeholder="选择要跳过的文档类型"
          value={skipDocTypes}
          onChange={setSkipDocTypes}
          options={[
            { label: "默写练习 (dictation_v1)", value: "dictation_v1" },
            { label: "Markdown (markdown_v1)", value: "markdown_v1" },
            { label: "综合选择题 (comprehensive_choice_v1)", value: "comprehensive_choice_v1" },
            { label: "案例分析 (case_analysis_v1)", value: "case_analysis_v1" },
            { label: "论文题 (essay_v1)", value: "essay_v1" },
            { label: "知识点概览 (knowledge_overview_v1)", value: "knowledge_overview_v1" },
          ]}
        />
      </div>

      {/* 工作流自定义参数 */}
      {(() => {
        const selectedWorkflow = workflows?.find(w => w.workflow_key === workflowKey);
        const schema = selectedWorkflow?.parameter_schema as {
          properties?: Record<string, { type: string; title?: string; description?: string; default?: unknown }>;
        } | undefined;

        if (!schema?.properties || Object.keys(schema.properties).length === 0) {
          return null;
        }

        // 过滤掉 PDMS 标准参数和工作流内部参数，只显示用户需要配置的自定义参数
        const HIDDEN_PARAMS = [
          // PDMS 标准参数（由系统自动传递）
          "run_id", "node_id", "workflow_key", "source_doc_ids",
          "callback_url", "pdms_base_url", "api_key",
          // 工作流内部参数（由系统自动处理或有合理默认值）
          "doc_id", "image_doc_id", "image_paths", "source_doc_id",
          "skip_screenshot", "skip_publish", "publish_title", "publish_caption",
          "topic", "target_docs",
        ];
        const customProperties = Object.entries(schema.properties).filter(
          ([key]) => !HIDDEN_PARAMS.includes(key)
        );

        if (customProperties.length === 0) {
          return null;
        }

        return (
          <div>
            <Text strong>工作流参数</Text>
            <div style={{ marginTop: 8 }}>
              {customProperties.map(([key, prop]) => (
                <div key={key} style={{ marginBottom: 12 }}>
                  <Text style={{ display: "block", marginBottom: 4 }}>{prop.title || key}</Text>
                  <Input
                    placeholder={prop.description || `请输入${prop.title || key}`}
                    value={(customParams[key] as string) ?? (prop.default as string) ?? ""}
                    onChange={(e) => setCustomParams(prev => ({ ...prev, [key]: e.target.value }))}
                  />
                  {prop.description && (
                    <Text type="secondary" style={{ fontSize: 12, display: "block", marginTop: 2 }}>
                      {prop.description}
                    </Text>
                  )}
                </div>
              ))}
            </div>
          </div>
        );
      })()}
    </Space>
  );

  // 同步模式预览表格列
  const syncPreviewColumns = [
    {
      title: "文档名称",
      dataIndex: "document_name",
      key: "document_name",
      ellipsis: true,
    },
    {
      title: "文档类型",
      dataIndex: "document_type",
      key: "document_type",
      width: 150,
      render: (type: string) => (
        <Text type="secondary" style={{ fontSize: 12 }}>
          {type}
        </Text>
      ),
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
      dataIndex: "can_sync",
      key: "status",
      width: 120,
      render: (canSync: boolean, record: DocumentPreviewItem) => (
        canSync ? (
          <Tag color="success" icon={<CheckCircleOutlined />}>
            可同步
          </Tag>
        ) : (
          <Tag color="warning" icon={<ExclamationCircleOutlined />}>
            {record.skip_reason || "跳过"}
          </Tag>
        )
      ),
    },
  ];

  // 渲染预览步骤
  const renderPreviewStep = () => {
    // 同步模式
    if (isSyncMode) {
      if (!syncPreviewData) {
        return <Spin />;
      }

      return (
        <Space direction="vertical" style={{ width: "100%" }} size="middle">
          <Alert
            type="info"
            showIcon
            message={
              <Space split={<span style={{ color: "#d9d9d9" }}>|</span>}>
                <span>操作：<strong>同步到 MySQL</strong></span>
                <span>总文档数：<strong>{syncPreviewData.total_documents}</strong></span>
                <span>
                  可同步：<Text type="success">{syncPreviewData.can_sync}</Text>
                </span>
                <span>
                  将跳过：<Text type="warning">{syncPreviewData.will_skip}</Text>
                </span>
              </Space>
            }
          />

          <Table
            dataSource={syncPreviewData.documents}
            columns={syncPreviewColumns}
            rowKey="document_id"
            size="small"
            pagination={{ pageSize: 10, showSizeChanger: true }}
            scroll={{ y: 300 }}
          />
        </Space>
      );
    }

    // 工作流模式
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

    const { status, success_count, failed_count, skipped_count, progress } = batchStatus;
    // 同步模式使用 total_documents，工作流模式使用 total_nodes
    const totalItems = 'total_documents' in batchStatus ? batchStatus.total_documents : ('total_nodes' in batchStatus ? batchStatus.total_nodes : 0);
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
                {completed}/{totalItems}
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
      const canExecuteCount = isSyncMode
        ? (syncPreviewData?.can_sync || 0)
        : (previewData?.can_execute || 0);
      const isDisabled = isSyncMode
        ? (!syncPreviewData || syncPreviewData.can_sync === 0)
        : (!previewData || previewData.can_execute === 0);
      const buttonText = isSyncMode
        ? `开始同步 (${canExecuteCount} 个文档)`
        : `开始执行 (${canExecuteCount} 个节点)`;

      buttons.push(
        <Button
          key="execute"
          type="primary"
          icon={<ThunderboltOutlined />}
          onClick={handleExecute}
          loading={executeLoading}
          disabled={isDisabled}
        >
          {buttonText}
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
