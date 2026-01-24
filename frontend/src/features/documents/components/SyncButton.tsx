import { type FC, useState, useEffect, useRef } from "react";
import {
  Button,
  Tooltip,
  Modal,
  Space,
  Typography,
  Tag,
  Alert,
  Spin,
  message,
} from "antd";
import {
  SyncOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  ExclamationCircleOutlined,
  DatabaseOutlined,
} from "@ant-design/icons";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";

import {
  getSyncStatus,
  triggerSync,
  type SyncStatusResponse,
} from "../../../api/sync";

const { Text, Paragraph } = Typography;

interface SyncButtonProps {
  documentId: number;
  disabled?: boolean;
}

const statusColors: Record<string, string> = {
  pending: "processing",
  success: "success",
  failed: "error",
  skipped: "warning",
};

const statusLabels: Record<string, string> = {
  pending: "同步中",
  success: "已同步",
  failed: "同步失败",
  skipped: "已跳过",
};

export const SyncButton: FC<SyncButtonProps> = ({ documentId, disabled }) => {
  const queryClient = useQueryClient();
  const [detailModalOpen, setDetailModalOpen] = useState(false);
  const pollCountRef = useRef(0);
  const maxPollCount = 30; // 最多轮询 30 次（60 秒）

  const {
    data: syncStatus,
    isLoading: loadingStatus,
    refetch: refetchStatus,
  } = useQuery({
    queryKey: ["sync-status", documentId],
    queryFn: () => getSyncStatus(documentId),
    staleTime: 30 * 1000,
    refetchInterval: (query) => {
      const status = query.state.data;
      if (status?.last_sync?.status === "pending") {
        pollCountRef.current += 1;
        // 超过最大轮询次数后停止轮询
        if (pollCountRef.current >= maxPollCount) {
          return false;
        }
        return 2000;
      }
      // 状态不是 pending 时重置计数器
      pollCountRef.current = 0;
      return false;
    },
  });

  const syncMutation = useMutation({
    mutationFn: () => triggerSync(documentId),
    onSuccess: (response) => {
      if (response.status === "pending" && response.message?.includes("already in progress")) {
        message.info("同步任务正在进行中");
      } else {
        message.success("同步任务已提交");
      }
      refetchStatus();
    },
    onError: (error: Error) => {
      message.error(`同步失败: ${error.message}`);
    },
  });

  useEffect(() => {
    if (syncStatus?.last_sync?.status === "success") {
      queryClient.invalidateQueries({ queryKey: ["document-detail", documentId] });
    }
  }, [syncStatus?.last_sync?.status, documentId, queryClient]);

  const hasSyncTarget = syncStatus?.sync_enabled;
  const lastStatus = syncStatus?.last_sync?.status;
  const isPending = lastStatus === "pending" || syncMutation.isPending;

  const getStatusIcon = () => {
    if (isPending) {
      return <SyncOutlined spin />;
    }
    switch (lastStatus) {
      case "success":
        return <CheckCircleOutlined style={{ color: "#52c41a" }} />;
      case "failed":
        return <CloseCircleOutlined style={{ color: "#ff4d4f" }} />;
      case "skipped":
        return <ExclamationCircleOutlined style={{ color: "#faad14" }} />;
      default:
        return <DatabaseOutlined />;
    }
  };

  const getTooltipContent = () => {
    if (!hasSyncTarget) {
      return "未配置同步目标 (metadata.sync_target)";
    }
    if (isPending) {
      return "正在同步...";
    }
    const target = syncStatus?.sync_target;
    if (target) {
      // 简化配置：只有 record_id
      if (!target.table && !target.field) {
        return `同步到 ${target.connection || "jkt"} (id=${target.record_id})`;
      }
      // 完整配置：有 table 和 field
      return `同步到 ${target.connection || "rkt"}.${target.table}.${target.field}`;
    }
    return "同步到 MySQL";
  };

  const formatTime = (isoString?: string) => {
    if (!isoString) return "-";
    return new Date(isoString).toLocaleString("zh-CN");
  };

  return (
    <>
      <Space.Compact>
        <Tooltip title={getTooltipContent()}>
          <Button
            icon={getStatusIcon()}
            onClick={() => {
              if (hasSyncTarget) {
                syncMutation.mutate();
              } else {
                setDetailModalOpen(true);
              }
            }}
            disabled={disabled || isPending || loadingStatus}
            loading={syncMutation.isPending}
          >
            同步
          </Button>
        </Tooltip>

        {hasSyncTarget && lastStatus && (
          <Button
            type="default"
            size="middle"
            onClick={() => setDetailModalOpen(true)}
            style={{ padding: "0 8px" }}
          >
            <Tag color={statusColors[lastStatus]} style={{ margin: 0 }}>
              {statusLabels[lastStatus]}
            </Tag>
          </Button>
        )}
      </Space.Compact>

      <Modal
        title="MySQL 同步状态"
        open={detailModalOpen}
        onCancel={() => setDetailModalOpen(false)}
        footer={
          <Space>
            <Button onClick={() => refetchStatus()}>刷新</Button>
            <Button
              type="primary"
              onClick={() => {
                syncMutation.mutate();
                setDetailModalOpen(false);
              }}
              disabled={!hasSyncTarget || isPending}
              loading={syncMutation.isPending}
            >
              立即同步
            </Button>
          </Space>
        }
      >
        {loadingStatus ? (
          <div style={{ textAlign: "center", padding: 24 }}>
            <Spin />
          </div>
        ) : (
          <Space direction="vertical" style={{ width: "100%" }} size="middle">
            <div>
              <Text strong>同步状态：</Text>
              {hasSyncTarget ? (
                <Tag color="success">已配置</Tag>
              ) : (
                <Tag color="default">未配置</Tag>
              )}
            </div>

            {syncStatus?.sync_target && (
              <div>
                <Text strong>同步目标：</Text>
                <Paragraph code style={{ margin: "4px 0" }}>
                  {syncStatus.sync_target.table && syncStatus.sync_target.field
                    ? `${syncStatus.sync_target.connection || "rkt"}.${syncStatus.sync_target.table}.${syncStatus.sync_target.field} (id=${syncStatus.sync_target.record_id})`
                    : `${syncStatus.sync_target.connection || "jkt"} (id=${syncStatus.sync_target.record_id})`
                  }
                </Paragraph>
              </div>
            )}

            {!hasSyncTarget && (
              <Alert
                type="info"
                message="未配置同步目标"
                description={
                  <span>
                    请在文档 metadata 中添加 sync_target 配置：
                    <pre style={{ marginTop: 8, fontSize: 12 }}>
{`// 简化配置（推荐）
{
  "sync_target": {
    "record_id": 12345
  }
}

// 完整配置（默认处理器）
{
  "sync_target": {
    "table": "表名",
    "record_id": 记录ID,
    "field": "字段名",
    "connection": "rkt"
  }
}`}
                    </pre>
                  </span>
                }
                showIcon
              />
            )}

            {syncStatus?.last_sync && (
              <>
                <div>
                  <Text strong>最后同步状态：</Text>
                  <Tag color={statusColors[syncStatus.last_sync.status]}>
                    {statusLabels[syncStatus.last_sync.status]}
                  </Tag>
                </div>

                <div>
                  <Text strong>同步版本：</Text>
                  <Text>v{syncStatus.last_sync.version}</Text>
                </div>

                {syncStatus.last_sync.synced_at && (
                  <div>
                    <Text strong>同步时间：</Text>
                    <Text>{formatTime(syncStatus.last_sync.synced_at)}</Text>
                  </div>
                )}

                {syncStatus.last_sync.error && (
                  <Alert
                    type="error"
                    message="同步错误"
                    description={syncStatus.last_sync.error}
                    showIcon
                  />
                )}
              </>
            )}
          </Space>
        )}
      </Modal>
    </>
  );
};
