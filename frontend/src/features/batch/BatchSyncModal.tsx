import { useState, useEffect, useCallback } from "react";
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
import { useQuery } from "@tanstack/react-query";

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

interface BatchSyncModalProps {
  open: boolean;
  nodeId: number;
  nodeName: string;
  onClose: () => void;
  onSuccess?: () => void;
}

export function BatchSyncModal({
  open,
  nodeId,
  nodeName,
  onClose,
  onSuccess,
}: BatchSyncModalProps) {
  // 步骤状态
  const [currentStep, setCurrentStep] = useState(0);

  // 配置状态
  const [includeDescendants, setIncludeDescendants] = useState(true);

  // 预览和执行状态
  const [previewData, setPreviewData] = useState<BatchSyncPreviewResponse | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [batchId, setBatchId] = useState<string | null>(null);
  const [executeLoading, setExecuteLoading] = useState(false);

  // 轮询批次状态
  const { data: batchStatus } = useQuery({
    queryKey: ["batch-sync-status", batchId],
    queryFn: () => getBatchSyncStatus(batchId!),
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

  // 重置状态
  const resetState = useCallback(() => {
    setCurrentStep(0);
    setIncludeDescendants(true);
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
    setPreviewLoading(true);
    try {
      const result = await previewBatchSync(nodeId, {
        include_descendants: includeDescendants,
      });
      setPreviewData(result);
      setCurrentStep(1);
    } catch (error) {
      message.error(`预览失败: ${(error as Error).message}`);
    } finally {
      setPreviewLoading(false);
    }
  }, [nodeId, includeDescendants]);

  // 执行
  const handleExecute = useCallback(async () => {
    setExecuteLoading(true);
    try {
      const result = await executeBatchSync(nodeId, {
        include_descendants: includeDescendants,
        concurrency: 3,
      });
      setBatchId(result.batch_id);
      setCurrentStep(2);
      message.success("批量同步已启动");
    } catch (error) {
      message.error(`执行失败: ${(error as Error).message}`);
    } finally {
      setExecuteLoading(false);
    }
  }, [nodeId, includeDescendants]);

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
        <Text strong>目标节点</Text>
        <Paragraph>
          <Tag color="blue">{nodeName}</Tag>
          <Text type="secondary">(ID: {nodeId})</Text>
        </Paragraph>
      </div>

      <Alert
        type="info"
        showIcon
        message="同步说明"
        description="批量同步将把节点下所有配置了 sync_target 的文档同步到外部 MySQL 数据库。"
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
    if (!batchStatus) {
      return (
        <div style={{ textAlign: "center", padding: 48 }}>
          <Spin size="large" />
          <Paragraph style={{ marginTop: 16 }}>正在启动批量同步...</Paragraph>
        </div>
      );
    }

    const { status, total_documents, success_count, failed_count, skipped_count, progress } = batchStatus;
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
    if (!batchStatus) {
      return <Spin />;
    }

    const { status, success_count, failed_count, skipped_count, details } = batchStatus;
    const isSuccess = status === "completed" && failed_count === 0;
    const docResults = details?.document_results || [];

    return (
      <Space direction="vertical" style={{ width: "100%" }} size="middle">
        <Result
          status={isSuccess ? "success" : failed_count > 0 ? "warning" : "info"}
          title={isSuccess ? "批量同步执行完成" : "批量同步执行完成（部分失败）"}
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

        {docResults.length > 0 && (
          <Table
            dataSource={docResults}
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
