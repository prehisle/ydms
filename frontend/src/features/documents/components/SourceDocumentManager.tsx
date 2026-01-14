import { DownloadOutlined, PlusOutlined, DeleteOutlined, FileTextOutlined } from "@ant-design/icons";
import { Button, Empty, List, message, Popconfirm, Space, Spin, Typography } from "antd";
import { useEffect, useState } from "react";
import {
  bindSourceDocument,
  unbindSourceDocument,
  getNodeSourceDocuments,
  type Document,
  type SourceDocument,
} from "../../../api/documents";
import { DocumentTreeSelector } from "./DocumentTreeSelector";

const { Text, Link } = Typography;

interface SourceDocumentManagerProps {
  nodeId: number;
  canEdit?: boolean;
  onSourcesChanged?: () => void;
}

export function SourceDocumentManager({
  nodeId,
  canEdit = false,
  onSourcesChanged,
}: SourceDocumentManagerProps) {
  const [selectorOpen, setSelectorOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [sources, setSources] = useState<SourceDocument[]>([]);
  const [fetchError, setFetchError] = useState<string | null>(null);

  // 加载源文档列表
  const fetchSources = async () => {
    try {
      setLoading(true);
      setFetchError(null);
      const data = await getNodeSourceDocuments(nodeId);
      setSources(data);
    } catch (error: any) {
      console.error("Failed to fetch source documents:", error);
      setFetchError(error?.message || "加载源文档失败");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (nodeId) {
      fetchSources();
    }
  }, [nodeId]);

  const excludeDocIds = sources.map((s) => s.document_id);

  const handleAddSource = async (selectedDoc: Document) => {
    try {
      setLoading(true);
      await bindSourceDocument(nodeId, selectedDoc.id);
      message.success(`已关联源文档：${selectedDoc.title}`);
      setSelectorOpen(false);
      await fetchSources();
      onSourcesChanged?.();
    } catch (error: any) {
      console.error("Failed to add source document:", error);
      const errorMsg = error?.message || "关联源文档失败";
      message.error(errorMsg);
    } finally {
      setLoading(false);
    }
  };

  // 批量添加源文档（多选模式）
  const handleAddSources = async (docs: Document[]) => {
    if (docs.length === 0) return;

    setLoading(true);
    let successCount = 0;
    let failCount = 0;
    const failedDocs: string[] = [];

    for (const doc of docs) {
      try {
        await bindSourceDocument(nodeId, doc.id);
        successCount++;
      } catch (error: any) {
        console.error(`Failed to bind document ${doc.id}:`, error);
        failCount++;
        failedDocs.push(doc.title);
      }
    }

    if (failCount === 0) {
      message.success(`已关联 ${successCount} 个源文档`);
      setSelectorOpen(false);
    } else if (successCount === 0) {
      message.error(`关联失败：${failedDocs.join(", ")}`);
    } else {
      message.warning(`成功 ${successCount} 个，失败 ${failCount} 个`);
      // 部分失败时也关闭弹窗，因为已经有成功的
      setSelectorOpen(false);
    }

    await fetchSources();
    onSourcesChanged?.();
    setLoading(false);
  };

  const handleRemoveSource = async (docId: number) => {
    try {
      setLoading(true);
      await unbindSourceDocument(nodeId, docId);
      message.success("已解除源文档关联");
      await fetchSources();
      onSourcesChanged?.();
    } catch (error: any) {
      console.error("Failed to remove source document:", error);
      message.error("解除关联失败");
    } finally {
      setLoading(false);
    }
  };

  const handleDocumentClick = (docId: number) => {
    window.open(`/documents/${docId}`, "_blank");
  };

  if (fetchError) {
    return (
      <div style={{ padding: "20px", textAlign: "center" }}>
        <Text type="danger">{fetchError}</Text>
        <br />
        <Button size="small" onClick={fetchSources} style={{ marginTop: 8 }}>
          重试
        </Button>
      </div>
    );
  }

  return (
    <div>
      <Space direction="vertical" style={{ width: "100%" }} size="middle">
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <Space>
            <DownloadOutlined />
            <Typography.Text strong>源文档（工作流输入）({sources.length})</Typography.Text>
          </Space>
          {canEdit && (
            <Button
              size="small"
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => setSelectorOpen(true)}
              loading={loading}
            >
              关联源文档
            </Button>
          )}
        </div>

        <Spin spinning={loading}>
          {sources.length === 0 ? (
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description="暂无源文档"
              style={{ padding: "20px 0" }}
            />
          ) : (
            <List
              size="small"
              dataSource={sources}
              renderItem={(source) => (
                <List.Item
                  actions={
                    canEdit
                      ? [
                          <Popconfirm
                            key="delete"
                            title="确认解除关联？"
                            description="这只会解除关联关系，不会删除文档本身"
                            onConfirm={() => handleRemoveSource(source.document_id)}
                            okText="确认"
                            cancelText="取消"
                          >
                            <Button
                              type="text"
                              size="small"
                              danger
                              icon={<DeleteOutlined />}
                            />
                          </Popconfirm>,
                        ]
                      : undefined
                  }
                >
                  <List.Item.Meta
                    avatar={<FileTextOutlined style={{ fontSize: 16 }} />}
                    title={
                      <Link
                        onClick={() => handleDocumentClick(source.document_id)}
                        style={{ cursor: "pointer" }}
                      >
                        {source.document?.title || `文档 #${source.document_id}`}
                      </Link>
                    }
                    description={
                      source.document?.type && (
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          类型: {source.document.type}
                        </Text>
                      )
                    }
                  />
                </List.Item>
              )}
            />
          )}
        </Spin>
      </Space>

      <DocumentTreeSelector
        open={selectorOpen}
        onCancel={() => setSelectorOpen(false)}
        selectionMode="multiple"
        onSelectMultiple={handleAddSources}
        excludeDocIds={excludeDocIds}
        title="选择源文档（可多选）"
      />
    </div>
  );
}
