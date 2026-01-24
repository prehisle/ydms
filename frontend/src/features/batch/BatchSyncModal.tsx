import { useState, useEffect, useCallback, useRef, type ReactNode } from "react";
import {
  Modal,
  Steps,
  Button,
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
  message,
} from "antd";
import {
  SyncOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  ExclamationCircleOutlined,
  LoadingOutlined,
} from "@ant-design/icons";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import {
  previewBatchSync,
  executeBatchSync,
  getBatchSyncStatus,
  type BatchSyncPreviewResponse,
  type DocumentPreviewItem,
  type BatchSyncStatusResponse,
} from "../../api/batchOperations";

const { Text, Paragraph } = Typography;

// 步骤定义
const STEPS = [
  { title: "配置", description: "设置范围" },
  { title: "预览", description: "确认文档" },
  { title: "执行", description: "同步中" },
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
  status: BatchSyncStatusResponse;
}

interface BatchSyncModalProps {
  open: boolean;
  nodeIds: number[];
  nodeNames: string[];
  onClose: () => void;
  onSuccess?: () => void;
}

export function BatchSyncModal({
  open,
  nodeIds,
  nodeNames,
  onClose,
  onSuccess,
}: BatchSyncModalProps) {
  const queryClient = useQueryClient();

  // 节点数量
  const nodeCount = nodeIds.length;

  // 步骤状态
  const [currentStep, setCurrentStep] = useState(0);

  // 配置状态
  const [includeDescendants, setIncludeDescendants] = useState(true);

  // 预览状态
  const [previewData, setPreviewData] = useState<BatchSyncPreviewResponse | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);

  // 串行执行状态
  const [currentNodeIndex, setCurrentNodeIndex] = useState(0);
  const [currentBatchId, setCurrentBatchId] = useState<string | null>(null);
  const [executeLoading, setExecuteLoading] = useState(false);
  const [executionResults, setExecutionResults] = useState<NodeExecutionResult[]>([]);
  const isExecutingRef = useRef(false);

  // 轮询当前批次状态
  const { data: batchStatus } = useQuery({
    queryKey: ["batch-sync-status", currentBatchId],
    queryFn: () => getBatchSyncStatus(currentBatchId!),
    enabled: currentStep === 2 && currentBatchId !== null,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (status === "completed" || status === "failed" || status === "cancelled") {
        return false;
      }
      return 2000;
    },
  });

  // 执行下一个节点的函数
  const executeNextNode = useCallback(async (
    nodeIndex: number,
    results: NodeExecutionResult[]
  ) => {
    if (nodeIndex >= nodeIds.length) {
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
      const result = await executeBatchSync(nodeId, {
        include_descendants: includeDescendants,
        concurrency: 3,
      });

      setCurrentNodeIndex(nodeIndex);
      setCurrentBatchId(result.batch_id);
    } catch (error) {
      message.error(`节点 "${nodeName}" 执行失败: ${(error as Error).message}`);
      // 即使失败也继续下一个节点
      const failedResult: NodeExecutionResult = {
        nodeId,
        nodeName,
        batchId: "",
        status: {
          batch_id: "",
          root_node_id: nodeId,
          status: "failed",
          total_documents: 0,
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
  }, [nodeIds, nodeNames, includeDescendants, onSuccess]);

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
      queryClient.removeQueries({ queryKey: ["batch-sync-status", currentBatchId] });

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

  // 重置状态
  const resetState = useCallback(() => {
    setCurrentStep(0);
    setIncludeDescendants(true);
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
    setPreviewLoading(true);
    try {
      // 对所有节点分别预览并合并结果
      const results = await Promise.all(
        nodeIds.map(nodeId => previewBatchSync(nodeId, {
          include_descendants: includeDescendants,
        }))
      );

      // 合并预览结果
      const mergedDocuments: DocumentPreviewItem[] = [];
      let totalDocuments = 0;
      let canSyncCount = 0;
      let willSkipCount = 0;

      for (const result of results) {
        mergedDocuments.push(...result.documents);
        totalDocuments += result.total_documents;
        canSyncCount += result.can_sync;
        willSkipCount += result.will_skip;
      }

      const mergedResult: BatchSyncPreviewResponse = {
        root_node_id: nodeIds[0],
        total_documents: totalDocuments,
        can_sync: canSyncCount,
        will_skip: willSkipCount,
        documents: mergedDocuments,
      };

      setPreviewData(mergedResult);
      setCurrentStep(1);
    } catch (error) {
      message.error(`预览失败: ${(error as Error).message}`);
    } finally {
      setPreviewLoading(false);
    }
  }, [nodeIds, includeDescendants]);

  // 开始执行（串行）
  const handleExecute = useCallback(async () => {
    if (isExecutingRef.current) {
      return;
    }

    setExecuteLoading(true);
    isExecutingRef.current = true;

    // 进入执行步骤
    setCurrentStep(2);
    setCurrentNodeIndex(0);
    setExecutionResults([]);

    // 开始执行第一个节点
    executeNextNode(0, []);

    setExecuteLoading(false);
  }, [executeNextNode]);

  // 预览表格列
  const previewColumns = [
    {
      title: "文档名称",
      dataIndex: "document_name",
      key: "document_name",
      ellipsis: true,
    },
    {
      title: "类型",
      dataIndex: "document_type",
      key: "document_type",
      width: 120,
      render: (type: string) => <Tag>{type || "未知"}</Tag>,
    },
    {
      title: "节点路径",
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
      title: "同步目标",
      dataIndex: "sync_target",
      key: "sync_target",
      width: 180,
      render: (target: { table?: string; record_id: number } | undefined) => (
        target ? (
          <Text style={{ fontSize: 12 }}>
            {target.table || "默认表"}#{target.record_id}
          </Text>
        ) : (
          <Text type="secondary" style={{ fontSize: 12 }}>
            未配置
          </Text>
        )
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

  // 结果表格列
  const resultColumns = [
    {
      title: "文档名称",
      dataIndex: "document_name",
      key: "document_name",
      ellipsis: true,
    },
    {
      title: "类型",
      dataIndex: "document_type",
      key: "document_type",
      width: 100,
      render: (type: string) => <Tag>{type || "未知"}</Tag>,
    },
    {
      title: "节点路径",
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

      <Alert
        type="info"
        showIcon
        message="同步说明"
        description="批量同步将把节点下所有配置了 sync_target 的文档同步到外部 MySQL 数据库。源文档（工作流输入）将被自动排除。"
      />

      <div>
        <Space>
          <Switch
            checked={includeDescendants}
            onChange={setIncludeDescendants}
          />
          <Text>包含子孙节点</Text>
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
              <span>总文档数：<strong>{previewData.total_documents}</strong></span>
              <span>
                可同步：<Text type="success">{previewData.can_sync}</Text>
              </span>
              <span>
                将跳过：<Text type="warning">{previewData.will_skip}</Text>
              </span>
            </Space>
          }
        />

        <Table
          dataSource={previewData.documents}
          columns={previewColumns}
          rowKey="document_id"
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

    const { status, total_documents, success_count, failed_count, skipped_count, progress } = batchStatus;
    const completed = success_count + failed_count + skipped_count;

    return (
      <Space direction="vertical" style={{ width: "100%" }} size="large">
        {/* 总体进度 */}
        {nodeCount > 1 && (
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
        )}

        {/* 当前批次进度 */}
        <div style={{ textAlign: "center" }}>
          <Progress
            type="circle"
            percent={Math.round(progress)}
            status={status === "failed" ? "exception" : undefined}
            format={() => (
              <span>
                {completed}/{total_documents}
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
    const allDocResults: Array<Record<string, unknown>> = [];

    for (const result of executionResults) {
      totalSuccess += result.status.success_count;
      totalFailed += result.status.failed_count;
      totalSkipped += result.status.skipped_count;

      // 收集详细结果
      const docResults = result.status.details?.document_results || [];
      allDocResults.push(...(docResults as Array<Record<string, unknown>>));
    }

    const allSuccess = totalFailed === 0;

    return (
      <Space direction="vertical" style={{ width: "100%" }} size="middle">
        <Result
          status={allSuccess ? "success" : totalFailed > 0 ? "warning" : "info"}
          title={allSuccess ? "批量同步执行完成" : "批量同步执行完成（部分失败）"}
          subTitle={
            <Space direction="vertical" size="small">
              {nodeCount > 1 && <Text>共处理 {nodeCount} 个根节点</Text>}
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
        {allDocResults.length > 0 && (
          <Table
            dataSource={allDocResults}
            columns={resultColumns}
            rowKey="document_id"
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
          icon={<SyncOutlined />}
          onClick={handlePreview}
          loading={previewLoading}
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
          icon={<SyncOutlined />}
          onClick={handleExecute}
          loading={executeLoading}
          disabled={!previewData || previewData.can_sync === 0}
        >
          开始同步 ({previewData?.can_sync || 0} 个文档)
        </Button>
      );
    }

    return buttons;
  };

  return (
    <Modal
      title={
        <Space>
          <SyncOutlined />
          批量同步文档
        </Space>
      }
      open={open}
      onCancel={handleClose}
      footer={renderFooter()}
      width={900}
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
