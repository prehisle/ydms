import {
  Button,
  Drawer,
  Layout,
  Popconfirm,
  Space,
  Spin,
  Tag,
  Tooltip,
  Typography,
  message,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ComponentProps,
  type CSSProperties,
  type ReactNode,
} from "react";
import { useOutletContext } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import {
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  HistoryOutlined,
  HolderOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  RollbackOutlined,
} from "@ant-design/icons";
import type { Category } from "./api/categories";
import { useAuth } from "./contexts/AuthContext";
import { useCategoryContext } from "./contexts/CategoryContext";
import { useDocumentContext } from "./contexts/DocumentContext";
import { useUIContext } from "./contexts/UIContext";
import { UserManagementDrawer } from "./features/users/UserManagementDrawer";
import { APIKeyManagementDrawer } from "./features/apikeys/APIKeyManagementDrawer";
import type { MainLayoutOutletContext } from "./components/Layout";
import { Document, DocumentTrashPage, DocumentVersionsPage, copyDocument } from "./api/documents";
import { CategoryTreePanel } from "./features/categories/components/CategoryTreePanel";
import type { CategoryTreePanelProps } from "./features/categories/components/CategoryTreePanel";
import { CategoryBreadcrumb } from "./features/categories/components/CategoryBreadcrumb";
import type { CategoryBreadcrumbProps } from "./features/categories/components/CategoryBreadcrumb";
import { CategoryTrashModal } from "./features/categories/components/CategoryTrashModal";
import { CategoryDeletePreviewModal } from "./features/categories/components/CategoryDeletePreviewModal";
import { CategoryFormModal } from "./features/categories/components/CategoryFormModal";
import { DocumentPanel } from "./features/documents/components/DocumentPanel";
import { DOCUMENT_TYPES } from "./features/documents/constants";
import { DocumentHistoryDrawer } from "./features/documents/components/DocumentHistoryDrawer";
import { DocumentTrashDrawer } from "./features/documents/components/DocumentTrashDrawer";
import { DocumentReorderModal } from "./features/documents/components/DocumentReorderModal";
import { SourceDocumentManager } from "./features/documents/components/SourceDocumentManager";
import { WorkflowManager } from "./features/workflows";
import { BatchWorkflowModal, BatchSyncModal } from "./features/batch";
import { StatusBar } from "./components/StatusBar";
import { useDocumentDrag } from "./features/documents/hooks/useDocumentDrag";
import { useTreeSiderState } from "./features/categories/hooks/useTreeSiderState";
import type { TreeSiderState } from "./features/categories/hooks/useTreeSiderState";

const dragDebugEnabled =
  (import.meta.env.VITE_DEBUG_DRAG ?? "").toString().toLowerCase() === "1";
const menuDebugEnabled =
  (import.meta.env.VITE_DEBUG_MENU ?? "").toString().toLowerCase() === "1";

const DocumentEditorLazy = lazy(() =>
  import("./features/documents/components/DocumentEditor").then((module) => ({
    default: module.DocumentEditor,
  })),
);

const DocumentEditorFallback = () => (
  <div style={{ display: "flex", justifyContent: "center", alignItems: "center", height: "100%" }}>
    <Space direction="vertical" align="center" size="middle">
      <Spin size="large" />
      <Typography.Text type="secondary">加载编辑器...</Typography.Text>
    </Space>
  </div>
);

const { Sider, Content, Footer } = Layout;
const TREE_MIN_WIDTH = 240;
const TREE_MAX_WIDTH = 600;
const CONTENT_MIN_WIDTH = 320;
const TREE_WIDTH_STORAGE_KEY = "ydms_tree_width";
const TREE_COLLAPSED_STORAGE_KEY = "ydms_tree_collapsed";
const TREE_COLLAPSED_WIDTH = 48;
const TREE_DEFAULT_WIDTH = 360;
const APP_LAYOUT_STYLE: CSSProperties = {
  height: "100%",
  minHeight: 0,
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
};
const CONTENT_STYLE: CSSProperties = { padding: "24px", overflow: "auto" };
const DOCUMENT_STACK_STYLE: CSSProperties = { width: "100%" };
const SIDER_BASE_STYLE: CSSProperties = {
  background: "#fff",
  borderRight: "1px solid #f0f0f0",
  display: "flex",
  flexDirection: "column",
  position: "relative",
  overflow: "hidden",
  minHeight: 0,
  height: "100%",
};
const TREE_CONTAINER_STYLE: CSSProperties = {
  flex: 1,
  minHeight: 0,
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
};
const RESIZER_WRAPPER_STYLE: CSSProperties = {
  position: "absolute",
  top: 0,
  right: -4,
  width: 8,
  height: "100%",
  cursor: "col-resize",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  zIndex: 10,
  touchAction: "none",
};
const RESIZER_BAR_BASE_STYLE: CSSProperties = {
  width: 2,
  height: "60%",
  borderRadius: 2,
  transition: "background-color 0.2s ease",
};

/**
 * 文档管理页面组件
 * 包含分类树、文档列表、文档编辑器等核心功能
 */
export const DocumentsPage = () => {
  const { headerCollapsed, onToggleHeader } = useOutletContext<MainLayoutOutletContext>();
  const { user } = useAuth();
  const [messageApi, contextHolder] = message.useMessage();
  const queryClient = useQueryClient();
  const treeSiderState = useTreeSiderState({
    widthStorageKey: TREE_WIDTH_STORAGE_KEY,
    collapsedStorageKey: TREE_COLLAPSED_STORAGE_KEY,
    defaultWidth: TREE_DEFAULT_WIDTH,
    minWidth: TREE_MIN_WIDTH,
    maxWidth: TREE_MAX_WIDTH,
    contentMinWidth: CONTENT_MIN_WIDTH,
  });

  // Category Context
  const {
    categoriesList,
    isLoading,
    isFetching,
    error,
    lookups,
    selectedIds,
    selectionParentId,
    lastSelectedId,
    selectedNodeId,
    categoryForm,
    isMutating,
    setMutating,
    createMutation,
    updateMutation,
    deleteMutation,
    bulkDeleteMutation,
    purgeMutation,
    trashQuery,
    trashItems,
    isTrashInitialLoading,
    selectedTrashRowKeys,
    setSelectedTrashRowKeys,
    isTrashProcessing,
    deletePreview,
    openDeletePreview,
    closeDeletePreview,
    setDeletePreviewLoading,
    handleSelectionChange,
    handleRefreshTree,
    invalidateCategoryQueries,
    handleTrashBulkRestore,
    handleTrashBulkPurge,
  } = useCategoryContext();

  // Document Context
  const {
    documents,
    isDocumentsLoading,
    isDocumentsFetching,
    documentsError,
    documentListPage,
    documentListSize,
    documentListTotal,
    documentFilterForm,
    includeDescendants,
    documentTrashParams,
    documentTrashQuery,
    documentHistoryParams,
    documentHistoryDocId,
    documentHistoryQuery,
    deleteDocumentMutation,
    restoreDocumentMutation,
    purgeDocumentMutation,
    documentReorderMutation,
    restoreDocumentVersionMutation,
    deletingDocId,
    restoringDocId,
    purgingDocId,
    restoringVersionNumber,
    handleDocumentSearch,
    handleDocumentReset,
    handleIncludeDescendantsChange,
    handleDocumentListPageChange,
    handleDocumentTrashSearch,
    handleDocumentTrashPageChange,
    handleDocumentHistoryPageChange,
    handleRefreshDocumentTrash,
    setDocumentHistoryDocId,
  } = useDocumentContext();

  // UI Context
  const {
    trashModalOpen,
    showCreateModal,
    showRenameModal,
    documentEditorState,
    documentTrashOpen,
    documentHistoryState,
    reorderModal,
    userManagementOpen,
    apiKeyManagementOpen,
    handleOpenTrash,
    handleCloseTrash,
    handleOpenCreateModal,
    handleCloseCreateModal,
    handleOpenRenameModal,
    handleCloseRenameModal,
    handleOpenDocumentEditor,
    handleCloseDocumentEditor,
    handleOpenDocumentTrash,
    handleCloseDocumentTrash,
    handleOpenDocumentHistory,
    handleCloseDocumentHistory,
    handleOpenReorderModal,
    handleCloseReorderModal,
    handleOpenUserManagement,
    handleCloseUserManagement,
    handleOpenAPIKeyManagement,
    handleCloseAPIKeyManagement,
  } = useUIContext();

  // 刷新分类和文档查询
  const invalidateAllQueries = useCallback(async () => {
    await Promise.all([
      invalidateCategoryQueries(),
      queryClient.invalidateQueries({ queryKey: ["node-documents"] }),
    ]);
  }, [invalidateCategoryQueries, queryClient]);

  // 文档拖拽功能
  const { handleDocumentDragStart, handleDocumentDragEnd, handleDropOnNode } = useDocumentDrag({
    selectedNodeId,
    messageApi,
    onInvalidateQueries: invalidateAllQueries,
  });

  useEffect(() => {
    document.title = "资料目录管理";
  }, []);

  // Sync rename modal form values
  useEffect(() => {
    if (showRenameModal && selectedIds.length === 1) {
      const current = lookups.byId.get(selectedIds[0]);
      if (current) {
        categoryForm.setFieldsValue({ name: current.name, type: current.type });
      }
    }
  }, [showRenameModal, selectedIds, lookups, categoryForm]);

  const handleRequestCreate = useCallback(
    (parentId: number | null) => {
      categoryForm.resetFields();
      handleOpenCreateModal(parentId);
    },
    [categoryForm, handleOpenCreateModal],
  );

  const handleRequestRename = useCallback(() => {
    if (selectedIds.length !== 1) {
      return;
    }
    const current = lookups.byId.get(selectedIds[0]);
    if (!current) {
      return;
    }
    categoryForm.setFieldsValue({ name: current.name, type: current.type });
    handleOpenRenameModal();
  }, [categoryForm, lookups, selectedIds, handleOpenRenameModal]);

  const handleRequestDelete = useCallback(
    (ids: number[]) => {
      if (ids.length === 0) {
        return;
      }
      openDeletePreview("soft", ids);
    },
    [openDeletePreview],
  );

  const handleOpenTrashWithRefresh = useCallback(async () => {
    setSelectedTrashRowKeys([]);
    handleOpenTrash();
    await trashQuery.refetch();
  }, [setSelectedTrashRowKeys, handleOpenTrash, trashQuery]);

  const handleConfirmDelete = useCallback((adminPassword?: string) => {
    if (!deletePreview.visible || deletePreview.ids.length === 0) {
      return;
    }
    const targetIds = deletePreview.ids;
    setDeletePreviewLoading(true);
    const handleError = (err: unknown, fallback: string) => {
      const msg = err instanceof Error ? err.message : fallback;
      messageApi.error(msg);
      setDeletePreviewLoading(false);
    };
    if (deletePreview.mode === "soft") {
      if (targetIds.length === 1) {
        const targetId = targetIds[0];
        deleteMutation.mutate(
          { id: targetId, payload: adminPassword ? { admin_password: adminPassword } : undefined },
          {
            onSuccess: () => {
              closeDeletePreview();
            },
            onError: (err) => handleError(err, "删除失败，请重试"),
          },
        );
      } else {
        bulkDeleteMutation.mutate(
          { ids: targetIds },
          {
            onSuccess: () => {
              closeDeletePreview();
            },
            onError: (err) => handleError(err, "批量删除失败，请重试"),
          },
        );
      }
    } else {
      const targetId = targetIds[0];
      purgeMutation.mutate(targetId, {
        onSuccess: () => {
          closeDeletePreview();
        },
        onError: (err) => handleError(err, "彻底删除失败，请重试"),
      });
    }
  }, [
    bulkDeleteMutation,
    closeDeletePreview,
    deleteMutation,
    deletePreview,
    messageApi,
    purgeMutation,
    setDeletePreviewLoading,
  ]);

  const handleOpenAddDocument = useCallback(
    (nodeId: number) => {
      handleOpenDocumentEditor({ mode: "create", nodeId });
    },
    [handleOpenDocumentEditor],
  );

  const handleToolbarAddDocument = useCallback(() => {
    if (selectedNodeId == null) {
      messageApi.warning("请选择一个目录节点后再添加文档");
      return;
    }
    handleOpenAddDocument(selectedNodeId);
  }, [handleOpenAddDocument, messageApi, selectedNodeId]);

  const handleEditDocument = useCallback(
    (doc: Document) => {
      handleOpenDocumentEditor({
        mode: "edit",
        docId: doc.id,
        nodeId: selectedNodeId ?? undefined,
      });
    },
    [selectedNodeId, handleOpenDocumentEditor],
  );

  const handleSoftDeleteDocument = useCallback(
    async (doc: Document) => {
      try {
        // 1. 获取文档的绑定状态
        const { getDocumentBindingStatus, unbindDocument } = await import("./api/documents");
        const bindingStatus = await getDocumentBindingStatus(doc.id);

        if (bindingStatus.total_bindings > 1) {
          // 2. 文档关联到多个目录，显示选择对话框
          const { Modal } = await import("antd");
          Modal.confirm({
            title: "该文档关联到多个目录",
            content: (
              <>
                <p>
                  文档 <strong>"{doc.title}"</strong> 关联到 <strong>{bindingStatus.total_bindings}</strong> 个目录。
                </p>
                <p>请选择操作：</p>
                <ul>
                  <li>
                    <strong>仅从当前目录移除</strong>：文档在其他目录中仍然可见
                  </li>
                  <li>
                    <strong>从所有目录删除</strong>：文档将被移入回收站，所有目录中都不可见
                  </li>
                </ul>
              </>
            ),
            okText: "仅从当前目录移除",
            cancelText: "从所有目录删除",
            okType: "default",
            cancelButtonProps: { danger: true },
            onOk: async () => {
              // 仅解绑当前节点
              if (selectedNodeId == null) {
                messageApi.error("无法确定当前目录");
                return;
              }
              try {
                await unbindDocument(selectedNodeId, doc.id);
                messageApi.success("已从当前目录移除");
                // 刷新文档列表和分类树
                await invalidateAllQueries();
              } catch (error) {
                messageApi.error("移除失败：" + (error as Error).message);
              }
            },
            onCancel: () => {
              // 删除文档
              deleteDocumentMutation.mutate(doc.id);
            },
          });
        } else {
          // 3. 只有一个绑定，直接软删除
          deleteDocumentMutation.mutate(doc.id);
        }
      } catch (error) {
        messageApi.error("检查文档关系失败：" + (error as Error).message);
      }
    },
    [deleteDocumentMutation, selectedNodeId, messageApi, invalidateAllQueries],
  );

  const handleOpenDocHistoryWrapper = useCallback(
    (doc: Document) => {
      setDocumentHistoryDocId(doc.id);
      handleOpenDocumentHistory(doc.id, doc.title, doc.type);
    },
    [handleOpenDocumentHistory, setDocumentHistoryDocId],
  );

  // 文档复制状态
  const [copyingDocId, setCopyingDocId] = useState<number | null>(null);

  // 批量操作弹窗状态
  const [batchWorkflowModal, setBatchWorkflowModal] = useState<{
    open: boolean;
    nodeId: number;
    nodeName: string;
  }>({ open: false, nodeId: 0, nodeName: "" });

  const [batchSyncModal, setBatchSyncModal] = useState<{
    open: boolean;
    nodeId: number;
    nodeName: string;
  }>({ open: false, nodeId: 0, nodeName: "" });

  const handleOpenBatchWorkflow = useCallback((nodeId: number, nodeName: string) => {
    setBatchWorkflowModal({ open: true, nodeId, nodeName });
  }, []);

  const handleCloseBatchWorkflow = useCallback(() => {
    setBatchWorkflowModal((prev) => ({ ...prev, open: false }));
  }, []);

  const handleOpenBatchSync = useCallback((nodeId: number, nodeName: string) => {
    setBatchSyncModal({ open: true, nodeId, nodeName });
  }, []);

  const handleCloseBatchSync = useCallback(() => {
    setBatchSyncModal((prev) => ({ ...prev, open: false }));
  }, []);

  const handleCopyDocument = useCallback(
    async (doc: Document) => {
      setCopyingDocId(doc.id);
      try {
        // 传递当前选中的节点 ID，确保副本出现在当前列表中
        const result = await copyDocument(doc.id, {
          node_id: selectedNodeId ?? undefined,
        });
        messageApi.success(`文档已复制：${result.title}`);
        await queryClient.invalidateQueries({ queryKey: ["node-documents"] });
      } catch (error) {
        const msg = error instanceof Error ? error.message : "复制文档失败";
        messageApi.error(msg);
      } finally {
        setCopyingDocId(null);
      }
    },
    [messageApi, queryClient, selectedNodeId],
  );

  const handleCloseDocumentHistoryWrapper = useCallback(() => {
    setDocumentHistoryDocId(null);
    handleCloseDocumentHistory();
  }, [handleCloseDocumentHistory, setDocumentHistoryDocId]);

  const handleOpenReorderModalWrapper = useCallback(() => {
    if (selectedNodeId == null || documents.length <= 1) {
      return;
    }
    handleOpenReorderModal();
  }, [documents.length, selectedNodeId, handleOpenReorderModal]);

	  const handleReorderConfirm = useCallback(
	    (orderedIds: number[]) => {
	      if (selectedNodeId == null) {
	        return;
	      }
	      documentReorderMutation.mutate({
	        ordered_ids: orderedIds,
	      });
	    },
	    [documentReorderMutation, selectedNodeId],
	  );

  const handleBreadcrumbNavigate = useCallback(
    (nodeId: number) => {
      // 选中点击的分类节点
      handleSelectionChange({
        selectedIds: [nodeId],
        selectionParentId: lookups.byId.get(nodeId)?.parent_id ?? null,
        lastSelectedId: nodeId,
      });
    },
    [handleSelectionChange, lookups],
  );

  const handleOpenDocumentTrashWithRefresh = useCallback(async () => {
    handleOpenDocumentTrash();
    // 打开回收站时自动刷新数据
    await documentTrashQuery.refetch();
  }, [handleOpenDocumentTrash, documentTrashQuery]);

  // Sync document editor close with deletion
  useEffect(() => {
    if (deletingDocId && documentEditorState.open && documentEditorState.docId === deletingDocId) {
      handleCloseDocumentEditor();
    }
  }, [deletingDocId, documentEditorState, handleCloseDocumentEditor]);

  // Close reorder modal on success
  useEffect(() => {
    if (documentReorderMutation.isSuccess) {
      handleCloseReorderModal();
    }
  }, [documentReorderMutation.isSuccess, handleCloseReorderModal]);

  const documentColumns = useMemo<ColumnsType<Document>>(
    () => [
      {
        title: "",
        key: "drag-handle",
        width: 40,
        align: "center",
        render: (_: unknown, record: Document) => (
          <span
            className="drag-handle"
            draggable
            onDragStart={(e) => handleDocumentDragStart(e, record)}
            onDragEnd={handleDocumentDragEnd}
            onClick={(e) => e.stopPropagation()}
            onDoubleClick={(e) => e.stopPropagation()}
            style={{ cursor: "grab", color: "#999", display: "inline-flex", alignItems: "center" }}
            aria-label="拖拽文档"
          >
            <HolderOutlined />
          </span>
        ),
      },
      {
        title: "ID",
        dataIndex: "id",
        key: "id",
        width: 80,
      },
      {
        title: "名称",
        dataIndex: "title",
        key: "title",
        render: (value: string) => <Typography.Text>{value}</Typography.Text>,
      },
      {
        title: "类型",
        dataIndex: "type",
        key: "type",
        render: (value: string | undefined) => <Typography.Text>{value || "-"}</Typography.Text>,
      },
      {
        title: "标签",
        key: "tags",
        render: (_: unknown, record: Document) => {
          const rawTags = record.metadata?.tags as unknown;
          if (!Array.isArray(rawTags) || rawTags.length === 0) {
            return <Typography.Text type="secondary">-</Typography.Text>;
          }
          const normalized = rawTags
            .map((item) => (typeof item === "string" ? item : String(item ?? "")))
            .map((item) => item.trim())
            .filter((item) => item.length > 0);
          if (normalized.length === 0) {
            return <Typography.Text type="secondary">-</Typography.Text>;
          }
          const displayTags = normalized.slice(0, 3);
          const remaining = normalized.length - displayTags.length;
          return (
            <Space size={[4, 4]} wrap>
              {displayTags.map((tag) => (
                <Tag key={tag}>{tag}</Tag>
              ))}
              {remaining > 0 ? <Tag>+{remaining}</Tag> : null}
            </Space>
          );
        },
      },
      {
        title: "版本",
        dataIndex: "version_number",
        key: "version_number",
        width: 100,
        render: (value: number | undefined | null) => <Typography.Text>{value ?? "-"}</Typography.Text>,
      },
      {
        title: "位置",
        dataIndex: "position",
        key: "position",
        width: 80,
        render: (value: number) => <Typography.Text>{value}</Typography.Text>,
      },
      {
        title: "更新时间",
        dataIndex: "updated_at",
        key: "updated_at",
        render: (value: string) => new Date(value).toLocaleString(),
      },
      {
        title: "操作",
        key: "actions",
        width: 240,
        render: (_: unknown, record: Document) => {
          const deleting = deletingDocId === record.id;
          const copying = copyingDocId === record.id;
          return (
            <Space>
              <Tooltip title="编辑文档">
                <Button
                  icon={<EditOutlined />}
                  type="text"
                  shape="circle"
                  onClick={() => handleEditDocument(record)}
                  aria-label="编辑文档"
                />
              </Tooltip>
              {user?.role !== "proofreader" && (
                <>
                  <Tooltip title="复制文档">
                    <Button
                      icon={<CopyOutlined />}
                      type="text"
                      shape="circle"
                      loading={copying}
                      onClick={() => handleCopyDocument(record)}
                      aria-label="复制文档"
                    />
                  </Tooltip>
                  <Tooltip title="删除文档">
                    <span style={{ display: "inline-flex" }}>
                      <Button
                        icon={<DeleteOutlined />}
                        type="text"
                        danger
                        shape="circle"
                        loading={deleteDocumentMutation.isPending && deleting}
                        onClick={() => handleSoftDeleteDocument(record)}
                        aria-label="删除文档"
                      />
                    </span>
                  </Tooltip>
                </>
              )}
              <Tooltip title="历史版本">
                <Button
                  icon={<HistoryOutlined />}
                  type="text"
                  shape="circle"
                  onClick={() => handleOpenDocHistoryWrapper(record)}
                  aria-label="查看历史版本"
                />
              </Tooltip>
            </Space>
          );
        },
      },
    ],
    [
      user,
      deleteDocumentMutation.isPending,
      deletingDocId,
      copyingDocId,
      handleEditDocument,
      handleCopyDocument,
      handleOpenDocHistoryWrapper,
      handleSoftDeleteDocument,
      handleDocumentDragStart,
      handleDocumentDragEnd,
    ],
  );

  const trashTableColumns = useMemo(
    () =>
      buildTrashColumns(
        false,
        false,
        (id: number) => {
          /* handled by mutation */
        },
        (id: number) => {
          /* handled by mutation */
        },
      ),
    [],
  );

  const treePanelProps: CategoryTreePanelProps = {
    categories: categoriesList,
    lookups,
    isLoading,
    isFetching,
    error,
    isMutating,
    selectedIds,
    selectionParentId,
    lastSelectedId,
    selectedNodeId,
    includeDescendants,
    createLoading: createMutation.isPending,
    trashIsFetching: trashQuery.isFetching,
    messageApi,
    dragDebugEnabled,
    menuDebugEnabled,
    canManageCategories: user?.role !== "proofreader",
    canCreateRoot: user?.role === "super_admin",
    onSelectionChange: handleSelectionChange,
    onRequestCreate: handleRequestCreate,
    onRequestRename: handleRequestRename,
    onRequestDelete: handleRequestDelete,
    onOpenTrash: handleOpenTrashWithRefresh,
    onOpenAddDocument: handleOpenAddDocument,
    onIncludeDescendantsChange: handleIncludeDescendantsChange,
    onRefresh: handleRefreshTree,
    onInvalidateQueries: invalidateCategoryQueries,
    setIsMutating: setMutating,
    onDocumentDrop: handleDropOnNode,
    onOpenBatchWorkflow: handleOpenBatchWorkflow,
    onOpenBatchSync: handleOpenBatchSync,
  };

  const breadcrumbProps: CategoryBreadcrumbProps = {
    selectedNodeId,
    lookups,
    onNavigate: handleBreadcrumbNavigate,
  };

  const documentPanelProps: ComponentProps<typeof DocumentPanel> = {
    filterForm: documentFilterForm,
    documentTypes: DOCUMENT_TYPES,
    selectedNodeId,
    documents,
    columns: documentColumns,
    isLoading: isDocumentsLoading,
    isFetching: isDocumentsFetching,
    error: documentsError,
    canCreateDocument: user?.role !== "proofreader",
    pagination: {
      current: documentListPage,
      pageSize: documentListSize,
      total: documentListTotal,
      onChange: handleDocumentListPageChange,
    },
    onSearch: handleDocumentSearch,
    onReset: handleDocumentReset,
    onAddDocument: handleToolbarAddDocument,
    onReorderDocuments: handleOpenReorderModalWrapper,
    onOpenTrash: handleOpenDocumentTrashWithRefresh,
    onDocumentDragStart: handleDocumentDragStart,
    onDocumentDragEnd: handleDocumentDragEnd,
    onRowDoubleClick: handleEditDocument,
  };

  return (
    <Layout style={APP_LAYOUT_STYLE}>
      {contextHolder}
      <Layout ref={treeSiderState.layoutRef} style={{ flex: 1, minHeight: 0, overflow: "hidden" }}>
        <TreeSider state={treeSiderState} collapsedWidth={TREE_COLLAPSED_WIDTH} baseStyle={SIDER_BASE_STYLE}>
          <CategoryTreePanel {...treePanelProps} />
        </TreeSider>
        <Content style={CONTENT_STYLE}>
          <DocumentContentSection
            breadcrumb={breadcrumbProps}
            panel={documentPanelProps}
            sourceManager={{
              nodeId: selectedNodeId,
              canEdit: user?.role !== "proofreader",
            }}
          />
        </Content>
      </Layout>
      <Footer style={{ padding: 0 }}>
        <StatusBar
          selectedCategory={selectedIds.length === 1 ? lookups.byId.get(selectedIds[0]) ?? null : null}
          selectedCount={selectedIds.length}
          totalCategories={categoriesList?.length ?? 0}
          includeDescendants={includeDescendants}
          userRole={user?.role}
          headerCollapsed={headerCollapsed}
          onToggleHeader={onToggleHeader}
        />
      </Footer>
      <CategoryTrashModal
        open={trashModalOpen}
        loading={trashQuery.isFetching || isTrashProcessing}
        isInitialLoading={isTrashInitialLoading}
        error={trashQuery.error}
        trashItems={trashItems}
        columns={trashTableColumns}
        selectedRowKeys={selectedTrashRowKeys}
        onSelectedRowKeysChange={(keys) => setSelectedTrashRowKeys(keys)}
        onRefresh={() => {
          void trashQuery.refetch();
        }}
        onClose={handleCloseTrash}
        onBulkRestore={handleTrashBulkRestore}
        onBulkPurge={handleTrashBulkPurge}
        isMutating={isTrashProcessing}
        onClearSelection={() => setSelectedTrashRowKeys([])}
      />
      <Drawer
        open={documentEditorState.open}
        width="100%"
        closable={false}
        maskClosable={false}
        destroyOnClose
        onClose={handleCloseDocumentEditor}
        styles={{ body: { padding: 0, height: "100%", display: "flex" } }}
      >
        {documentEditorState.open &&
        (documentEditorState.mode === "create" || documentEditorState.docId != null) ? (
          <div style={{ flex: 1, minWidth: 0, minHeight: 0 }}>
            <Suspense fallback={<DocumentEditorFallback />}>
              <DocumentEditorLazy
                mode={documentEditorState.mode}
                docId={
                  documentEditorState.mode === "edit" ? documentEditorState.docId ?? undefined : undefined
                }
                nodeId={documentEditorState.nodeId ?? undefined}
                onClose={handleCloseDocumentEditor}
              />
            </Suspense>
          </div>
        ) : null}
      </Drawer>
      <DocumentReorderModal
        open={reorderModal}
        documents={documents}
        loading={documentReorderMutation.isPending}
        onCancel={handleCloseReorderModal}
        onConfirm={handleReorderConfirm}
      />
      <DocumentTrashDrawer
        open={documentTrashOpen}
        loading={documentTrashQuery.isLoading || documentTrashQuery.isFetching}
        data={documentTrashQuery.data as DocumentTrashPage | undefined}
        error={documentTrashQuery.error}
        onClose={handleCloseDocumentTrash}
        onRefresh={handleRefreshDocumentTrash}
        onSearch={handleDocumentTrashSearch}
        onPageChange={handleDocumentTrashPageChange}
        onRestore={(doc) => restoreDocumentMutation.mutate(doc.id)}
        onPurge={(doc) => purgeDocumentMutation.mutate(doc.id)}
        restoreLoadingId={restoringDocId}
        purgeLoadingId={purgingDocId}
        queryValue={documentTrashParams.query}
      />
      <DocumentHistoryDrawer
        open={documentHistoryState.open}
        documentTitle={documentHistoryState.title}
        documentType={documentHistoryState.docType}
        loading={documentHistoryQuery.isLoading || documentHistoryQuery.isFetching}
        data={(documentHistoryQuery.data ?? undefined) as DocumentVersionsPage | undefined}
        error={documentHistoryQuery.error}
        onClose={handleCloseDocumentHistoryWrapper}
        onPageChange={handleDocumentHistoryPageChange}
        onRestore={(version) => {
          if (documentHistoryState.docId == null) return;
          restoreDocumentVersionMutation.mutate({
            docId: documentHistoryState.docId,
            version: version.version_number,
          });
        }}
        restoreLoadingVersion={restoringVersionNumber}
      />
      <CategoryDeletePreviewModal
        open={deletePreview.visible}
        mode={deletePreview.mode}
        loading={deletePreview.loading}
        result={deletePreview.result}
        confirmLoading={
          deletePreview.loading ||
          deleteMutation.isPending ||
          bulkDeleteMutation.isPending ||
          purgeMutation.isPending
        }
        onCancel={closeDeletePreview}
        onConfirm={handleConfirmDelete}
      />
      <CategoryFormModal
        open={showCreateModal.open}
        title={showCreateModal.parentId ? "新建子目录" : "新建根目录"}
        confirmLoading={createMutation.isPending}
        form={categoryForm}
        onCancel={handleCloseCreateModal}
        onSubmit={() => {
          categoryForm
            .validateFields()
            .then((values) => {
              setMutating(true);
              createMutation.mutate(
                {
                  name: values.name.trim(),
                  parent_id: showCreateModal.parentId,
                  type: values.type || null,
                },
                {
                  onSuccess: () => {
                    handleCloseCreateModal();
                  },
                  onError: () => {
                    setMutating(false);
                  },
                },
              );
            })
            .catch(() => undefined);
        }}
      />
      <CategoryFormModal
        open={showRenameModal}
        title="编辑目录"
        confirmLoading={updateMutation.isPending}
        form={categoryForm}
        onCancel={handleCloseRenameModal}
        onSubmit={() => {
          categoryForm
            .validateFields()
            .then((values) => {
              if (selectedIds.length !== 1) return;
              setMutating(true);
              updateMutation.mutate(
                {
                  id: selectedIds[0],
                  payload: { name: values.name.trim(), type: values.type },
                },
                {
                  onSuccess: () => {
                    handleCloseRenameModal();
                  },
                  onError: () => {
                    setMutating(false);
                  },
                },
              );
            })
            .catch(() => undefined);
        }}
      />
      <UserManagementDrawer open={userManagementOpen} onClose={handleCloseUserManagement} />
      <APIKeyManagementDrawer open={apiKeyManagementOpen} onClose={handleCloseAPIKeyManagement} />
      <BatchWorkflowModal
        open={batchWorkflowModal.open}
        nodeId={batchWorkflowModal.nodeId}
        nodeName={batchWorkflowModal.nodeName}
        onClose={handleCloseBatchWorkflow}
        onSuccess={invalidateAllQueries}
      />
      <BatchSyncModal
        open={batchSyncModal.open}
        nodeId={batchSyncModal.nodeId}
        nodeName={batchSyncModal.nodeName}
        onClose={handleCloseBatchSync}
        onSuccess={invalidateAllQueries}
      />
    </Layout>
  );
};

interface TreeSiderProps {
  state: TreeSiderState;
  collapsedWidth: number;
  baseStyle: CSSProperties;
  children: ReactNode;
}

const TreeSider = ({ state, collapsedWidth, baseStyle, children }: TreeSiderProps) => {
  const {
    treeWidth,
    treeCollapsed,
    toggleCollapsed,
    toggleButtonStyle,
    siderDynamicStyle,
    handleResizeStart,
    handleResizeTouchStart,
    isResizing,
  } = state;

  return (
    <Sider
      className="documents-tree-sider"
      width={treeWidth}
      collapsedWidth={collapsedWidth}
      collapsed={treeCollapsed}
      trigger={null}
      style={{
        ...baseStyle,
        padding: treeCollapsed ? "0" : "16px",
        ...siderDynamicStyle,
      }}
    >
      <Tooltip title={treeCollapsed ? "展开目录树" : "折叠目录树"}>
        <Button
          icon={treeCollapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
          shape="circle"
          type="text"
          aria-label={treeCollapsed ? "展开目录树" : "折叠目录树"}
          onClick={toggleCollapsed}
          style={{ ...toggleButtonStyle, flexShrink: 0 }}
        />
      </Tooltip>
      {!treeCollapsed && (
        <>
          <div style={TREE_CONTAINER_STYLE}>{children}</div>
          <div
            role="separator"
            aria-orientation="vertical"
            aria-label="调整目录树宽度"
            onMouseDown={handleResizeStart}
            onTouchStart={handleResizeTouchStart}
            style={RESIZER_WRAPPER_STYLE}
          >
            <div
              style={{
                ...RESIZER_BAR_BASE_STYLE,
                background: isResizing ? "#1677ff" : "transparent",
              }}
            />
          </div>
        </>
      )}
    </Sider>
  );
};

interface DocumentContentSectionProps {
  breadcrumb: CategoryBreadcrumbProps;
  panel: ComponentProps<typeof DocumentPanel>;
  sourceManager?: {
    nodeId: number | null;
    canEdit: boolean;
  };
}

const DocumentContentSection = ({ breadcrumb, panel, sourceManager }: DocumentContentSectionProps) => {
  // 当源文档变化时，通过 key 强制刷新 WorkflowManager
  const [workflowRefreshKey, setWorkflowRefreshKey] = useState(0);

  const handleSourcesChanged = useCallback(() => {
    setWorkflowRefreshKey(prev => prev + 1);
  }, []);

  return (
    <Space direction="vertical" size="large" style={DOCUMENT_STACK_STYLE}>
      <CategoryBreadcrumb {...breadcrumb} />
      {sourceManager?.nodeId != null && (
        <>
          <SourceDocumentManager
            nodeId={sourceManager.nodeId}
            canEdit={sourceManager.canEdit}
            onSourcesChanged={handleSourcesChanged}
          />
          <WorkflowManager
            key={workflowRefreshKey}
            nodeId={sourceManager.nodeId}
            canEdit={sourceManager.canEdit}
          />
        </>
      )}
      <DocumentPanel {...panel} />
    </Space>
  );
};

// Helper function preserved from original
function buildTrashColumns(
  restoreLoading: boolean,
  purgeLoading: boolean,
  onRestore: (id: number) => void,
  onPurge: (id: number) => void,
): ColumnsType<Category> {
  return [
    {
      title: "名称",
      dataIndex: "name",
      key: "name",
      render: (value: string) => (
        <Space>
          <Typography.Text>{value}</Typography.Text>
          <Tag color="red">已删除</Tag>
        </Space>
      ),
    },
    {
      title: "路径",
      dataIndex: "path",
      key: "path",
      render: (value: string) => <Typography.Text code>{value}</Typography.Text>,
    },
    {
      title: "删除时间",
      dataIndex: "deleted_at",
      key: "deleted_at",
      render: (value?: string | null) => (value ? new Date(value).toLocaleString() : "-"),
    },
    {
      title: "操作",
      key: "actions",
      render: (_, record) => (
        <Space>
          <Tooltip title="恢复">
            <span style={{ display: "inline-flex" }}>
              <Button
                icon={<RollbackOutlined />}
                type="text"
                shape="circle"
                onClick={() => onRestore(record.id)}
                disabled={restoreLoading}
                loading={restoreLoading}
                aria-label="恢复目录"
              />
            </span>
          </Tooltip>
          <Popconfirm
            title="彻底删除后无法恢复，确认操作？"
            okText="删除"
            cancelText="取消"
            onConfirm={() => onPurge(record.id)}
            disabled={purgeLoading}
          >
            <Tooltip title="彻底删除">
              <span style={{ display: "inline-flex" }}>
                <Button
                  icon={<DeleteOutlined />}
                  danger
                  type="text"
                  shape="circle"
                  disabled={purgeLoading}
                  loading={purgeLoading}
                  aria-label="彻底删除"
                />
              </span>
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ];
}
