import {
  PlayCircleOutlined,
  HistoryOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  LoadingOutlined,
  ClockCircleOutlined,
  FileTextOutlined,
} from "@ant-design/icons";
import {
  Button,
  Collapse,
  Empty,
  List,
  message,
  Modal,
  Space,
  Spin,
  Tag,
  Tooltip,
  Typography,
} from "antd";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  listNodeWorkflowRuns,
  triggerNodeWorkflow,
  type WorkflowRun,
} from "../../../api/workflows";
import {
  getNodeSourceDocuments,
  getNodeDocuments,
  type SourceDocument,
  type Document,
} from "../../../api/documents";

const { Text } = Typography;

interface WorkflowManagerProps {
  nodeId: number;
  canEdit?: boolean;
}

// 状态标签颜色映射
const statusColors: Record<string, string> = {
  pending: "default",
  running: "processing",
  success: "success",
  failed: "error",
  cancelled: "warning",
};

// 状态标签文本映射
const statusLabels: Record<string, string> = {
  pending: "等待中",
  running: "运行中",
  success: "成功",
  failed: "失败",
  cancelled: "已取消",
};

// 状态图标映射
const statusIcons: Record<string, React.ReactNode> = {
  pending: <ClockCircleOutlined />,
  running: <LoadingOutlined spin />,
  success: <CheckCircleOutlined />,
  failed: <CloseCircleOutlined />,
  cancelled: <CloseCircleOutlined />,
};

export function WorkflowManager({ nodeId, canEdit = false }: WorkflowManagerProps) {
  // 原始数据状态
  const [nodeDocs, setNodeDocs] = useState<Document[]>([]);
  const [sources, setSources] = useState<SourceDocument[]>([]);
  const [runs, setRuns] = useState<WorkflowRun[]>([]);

  // 加载状态
  const [loading, setLoading] = useState(false);
  const [runsLoading, setRunsLoading] = useState(false);
  const [triggerLoading, setTriggerLoading] = useState(false);
  const [historyModalOpen, setHistoryModalOpen] = useState(false);

  // 派生数据：排除源文档后的产出文档列表（同步计算，无竞态）
  const targetDocs = useMemo(() => {
    const sourceDocIds = new Set(sources.map(s => s.document_id));
    return nodeDocs.filter(doc => !sourceDocIds.has(doc.id));
  }, [nodeDocs, sources]);

  // 加载节点数据（源文档 + 节点文档 + 运行历史）
  useEffect(() => {
    if (!nodeId) return;

    let cancelled = false;

    const loadData = async () => {
      setLoading(true);
      try {
        // 并发加载，避免串行等待
        const [sourcesData, docsPage, runsData] = await Promise.all([
          getNodeSourceDocuments(nodeId),
          getNodeDocuments(nodeId, { include_descendants: false, size: 100 }),
          listNodeWorkflowRuns(nodeId, { limit: 10 }),
        ]);

        // 检查是否已取消（节点已切换）
        if (cancelled) return;

        setSources(sourcesData);
        setNodeDocs(docsPage.items);
        setRuns(runsData.runs || []);
      } catch (error) {
        if (!cancelled) {
          console.error("Failed to load workflow data:", error);
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    // 切换节点时先清空旧数据
    setSources([]);
    setNodeDocs([]);
    setRuns([]);

    loadData();

    return () => {
      cancelled = true;
    };
  }, [nodeId]);

  // 刷新运行历史
  const refreshRuns = useCallback(async () => {
    try {
      setRunsLoading(true);
      const data = await listNodeWorkflowRuns(nodeId, { limit: 10 });
      setRuns(data.runs || []);
    } catch (error) {
      console.error("Failed to refresh workflow runs:", error);
    } finally {
      setRunsLoading(false);
    }
  }, [nodeId]);

  // 运行工作流（直接触发统一工作流）
  const handleRunWorkflow = async () => {
    try {
      setTriggerLoading(true);
      const result = await triggerNodeWorkflow(nodeId, "generate_node_documents", {});
      message.success(result.message || "工作流已触发");
      refreshRuns(); // 刷新运行历史
    } catch (error: any) {
      message.error(error?.message || "触发工作流失败");
    } finally {
      setTriggerLoading(false);
    }
  };

  // 格式化时间
  const formatTime = (timeStr?: string) => {
    if (!timeStr) return "-";
    return new Date(timeStr).toLocaleString("zh-CN", {
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  // 检查是否可以运行工作流
  const canRunWorkflow = sources.length > 0 && targetDocs.length > 0;

  return (
    <>
      <Collapse
        defaultActiveKey={[]}
        items={[
          {
            key: "workflow",
            label: (
              <Space>
                <PlayCircleOutlined />
                <span>节点工作流</span>
              </Space>
            ),
            extra: (
              <Space>
                {canEdit && (
                  <Button
                    type="primary"
                    size="small"
                    icon={<PlayCircleOutlined />}
                    onClick={(e) => {
                      e.stopPropagation();
                      handleRunWorkflow();
                    }}
                    disabled={!canRunWorkflow}
                    loading={triggerLoading}
                  >
                    运行工作流
                  </Button>
                )}
                <Tooltip title="运行历史">
                  <Button
                    size="small"
                    icon={<HistoryOutlined />}
                    onClick={(e) => {
                      e.stopPropagation();
                      setHistoryModalOpen(true);
                    }}
                  >
                    历史
                  </Button>
                </Tooltip>
              </Space>
            ),
            children: (
              <Spin spinning={loading}>
                <Space direction="vertical" style={{ width: "100%" }} size="small">
                  {/* 状态提示 */}
                  {sources.length === 0 && (
                    <Text type="warning" style={{ fontSize: 12 }}>
                      提示：请先关联源文档，工作流将读取源文档内容生成产出
                    </Text>
                  )}
                  {sources.length > 0 && targetDocs.length === 0 && (
                    <Text type="warning" style={{ fontSize: 12 }}>
                      提示：请先创建文档，工作流将根据文档类型生成内容
                    </Text>
                  )}

                  {/* 待生成文档列表 */}
                  {targetDocs.length > 0 ? (
                    <>
                      <Text strong style={{ fontSize: 12 }}>
                        待生成文档：{targetDocs.length} 个
                      </Text>
                      <List
                        size="small"
                        dataSource={targetDocs}
                        renderItem={(doc) => (
                          <List.Item style={{ padding: "4px 0" }}>
                            <Space size="small">
                              <FileTextOutlined style={{ color: "#1890ff" }} />
                              <Text style={{ fontSize: 12 }}>{doc.title}</Text>
                              <Tag style={{ fontSize: 11 }}>{doc.type}</Tag>
                            </Space>
                          </List.Item>
                        )}
                      />
                    </>
                  ) : (
                    <Empty
                      image={Empty.PRESENTED_IMAGE_SIMPLE}
                      description="暂无待生成文档"
                      style={{ padding: "12px 0" }}
                    />
                  )}

                  {/* 最近运行 */}
                  {runs.length > 0 && (
                    <>
                      <Text strong style={{ fontSize: 12, marginTop: 8 }}>
                        最近运行
                      </Text>
                      <List
                        size="small"
                        loading={runsLoading}
                        dataSource={runs.slice(0, 3)}
                        renderItem={(run) => (
                          <List.Item style={{ padding: "4px 0" }}>
                            <Space size="small">
                              <Tag
                                icon={statusIcons[run.status]}
                                color={statusColors[run.status]}
                              >
                                {statusLabels[run.status]}
                              </Tag>
                              <Text type="secondary" style={{ fontSize: 12 }}>
                                {formatTime(run.created_at)}
                              </Text>
                            </Space>
                          </List.Item>
                        )}
                      />
                    </>
                  )}
                </Space>
              </Spin>
            ),
          },
        ]}
      />

      {/* 历史记录弹窗 */}
      <Modal
        title="工作流运行历史"
        open={historyModalOpen}
        onCancel={() => setHistoryModalOpen(false)}
        footer={null}
        width={600}
      >
        <List
          size="small"
          loading={runsLoading}
          dataSource={runs}
          locale={{ emptyText: "暂无运行记录" }}
          renderItem={(run) => (
            <List.Item>
              <List.Item.Meta
                title={
                  <Space>
                    <Tag
                      icon={statusIcons[run.status]}
                      color={statusColors[run.status]}
                    >
                      {statusLabels[run.status]}
                    </Tag>
                    <Text>{run.workflow_key}</Text>
                  </Space>
                }
                description={
                  <Space direction="vertical" size={0}>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      触发时间: {formatTime(run.created_at)}
                      {run.finished_at && ` | 完成时间: ${formatTime(run.finished_at)}`}
                    </Text>
                    {run.error_message && (
                      <Text type="danger" style={{ fontSize: 12 }}>
                        错误: {run.error_message}
                      </Text>
                    )}
                  </Space>
                }
              />
            </List.Item>
          )}
        />
      </Modal>
    </>
  );
}
