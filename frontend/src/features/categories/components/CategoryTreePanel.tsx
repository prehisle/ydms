import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import type {
  CSSProperties,
  Key,
  MouseEvent as ReactMouseEvent,
  DragEvent as ReactDragEvent,
  ReactNode,
} from "react";

import { Alert, Empty, Menu, Modal, Spin, Tag, Tree, Typography } from "antd";
import type { MenuProps, TreeProps, GetRef } from "antd";
import type { MessageInstance } from "antd/es/message/interface";

type TreeRef = GetRef<typeof Tree>;
import {
  ClearOutlined,
  CopyOutlined,
  DeleteOutlined,
  FileAddOutlined,
  ScissorOutlined,
  SnippetsOutlined,
  PlusSquareOutlined,
  EditOutlined,
  LinkOutlined,
  ThunderboltOutlined,
  SyncOutlined,
} from "@ant-design/icons";

import type { Category } from "../../../api/categories";
import {
  bindSourceDocument,
  getNodeDocuments,
  getNodeSourceDocuments,
} from "../../../api/documents";
import { CategoryTreeToolbar } from "./CategoryTreeToolbar";
import type { CategoryLookups, ParentKey, TreeDataNode } from "../types";
import { buildFilteredTree, getParentId } from "../utils";
import { useCategoryClipboard } from "../hooks/useCategoryClipboard";
import { useCategoryContextMenu } from "../hooks/useCategoryContextMenu";
import { useTreePaste } from "../hooks/useTreePaste";
import { useTreeDrag } from "../hooks/useTreeDrag";

type AntTreeProps = TreeProps<TreeDataNode>;
type TreeRightClickInfo = Parameters<NonNullable<AntTreeProps["onRightClick"]>>[0];
type TreeSelectInfo = Parameters<NonNullable<AntTreeProps["onSelect"]>>[1];

/** 文档剪贴板状态，用于跨节点复制/粘贴文档ID */
interface DocClipboard {
  sourceNodeId: number;
  sourceNodeName: string;
  docIds: number[];
  copiedAt: number;
}

const PANEL_ROOT_STYLE: CSSProperties = {
  flex: 1,
  display: "flex",
  flexDirection: "column",
  height: "100%",
  minHeight: 0,
  overflow: "hidden",
};

const PANEL_BODY_STYLE: CSSProperties = {
  flex: 1,
  minHeight: 0,
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
};

const PANEL_SCROLL_STYLE: CSSProperties = {
  flex: 1,
  minHeight: 0,
  // Tree virtual scroll will manage its own scroll container; avoid double scrollbars.
  overflow: "hidden",
};

const PANEL_PLACEHOLDER_STYLE: CSSProperties = {
  flex: 1,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
};

export interface CategoryTreePanelProps {
  categories: Category[] | undefined;
  lookups: CategoryLookups;
  isLoading: boolean;
  isFetching: boolean;
  error: unknown;
  isMutating: boolean;
  selectedIds: number[];
  selectionParentId: ParentKey | undefined;
  lastSelectedId: number | null;
  selectedNodeId: number | null;
  includeDescendants: boolean;
  createLoading: boolean;
  trashIsFetching: boolean;
  messageApi: MessageInstance;
  dragDebugEnabled: boolean;
  menuDebugEnabled: boolean;
  canManageCategories?: boolean; // 是否有权限管理分类（创建、编辑、删除）
  canCreateRoot?: boolean; // 是否可以创建根节点
  onSelectionChange: (payload: {
    selectedIds: number[];
    selectionParentId: ParentKey | undefined;
    lastSelectedId: number | null;
  }) => void;
  onRequestCreate: (parentId: number | null) => void;
  onRequestRename: () => void;
  onRequestDelete: (ids: number[]) => void;
  onOpenTrash: () => void;
  onOpenAddDocument: (nodeId: number) => void;
  onIncludeDescendantsChange: (value: boolean) => void;
  onRefresh: () => void;
  onInvalidateQueries: () => Promise<void>;
  setIsMutating: (value: boolean) => void;
  onDocumentDrop?: (targetNodeId: number, dragData: any) => void;
  onOpenBatchWorkflow?: (nodeIds: number[], nodeNames: string[]) => void;
  onOpenBatchSync?: (nodeIds: number[], nodeNames: string[]) => void;
  /** 外部跳转时需要滚动到的节点 ID，设置后会自动展开路径并滚动 */
  scrollToNodeId?: number | null;
  /** 滚动完成后的回调，用于清除 scrollToNodeId */
  onScrollToNodeComplete?: () => void;
}

export function CategoryTreePanel({
  categories,
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
  createLoading,
  trashIsFetching,
  messageApi,
  dragDebugEnabled,
  menuDebugEnabled,
  canManageCategories = true,
  canCreateRoot = true,
  onSelectionChange,
  onRequestCreate,
  onRequestRename,
  onRequestDelete,
  onOpenTrash,
  onOpenAddDocument,
  onIncludeDescendantsChange,
  onRefresh,
  onInvalidateQueries,
  setIsMutating,
  onDocumentDrop,
  onOpenBatchWorkflow,
  onOpenBatchSync,
  scrollToNodeId,
  onScrollToNodeComplete,
}: CategoryTreePanelProps) {
  const [searchValue, setSearchValue] = useState("");
  const [expandedKeys, setExpandedKeys] = useState<string[]>([]);
  const [autoExpandParent, setAutoExpandParent] = useState(true);
  const treeRef = useRef<TreeRef>(null);
  const [dropTargetNodeId, setDropTargetNodeId] = useState<number | null>(null);
  const [treeContainerEl, setTreeContainerEl] = useState<HTMLDivElement | null>(null);
  const [treeHeight, setTreeHeight] = useState(0);
  const [docClipboard, setDocClipboard] = useState<DocClipboard | null>(null);
  const {
    clipboard,
    clipboardSourceSet,
    copySelection: handleCopySelection,
    cutSelection: handleCutSelection,
    clearClipboard,
    resetClipboard,
  } = useCategoryClipboard({ messageApi, selectedIds, selectionParentId });

  const {
    contextMenu,
    openContextMenu,
    closeContextMenu,
    suppressNativeContextMenu,
    menuContainerRef,
  } = useCategoryContextMenu({ menuDebugEnabled, lookups });

  const effectiveCategories = categories ?? [];
  const filteredTree = useMemo(
    () => buildFilteredTree(effectiveCategories, null, searchValue.trim()),
    [effectiveCategories, searchValue],
  );
  const treeData = filteredTree.nodes;
  const defaultExpandedKeys = useMemo(
    () => getDefaultExpandedKeys(effectiveCategories, 1),
    [effectiveCategories],
  );

  useLayoutEffect(() => {
    if (!treeContainerEl) return;

    const update = () => {
      const next = Math.max(0, Math.floor(treeContainerEl.clientHeight));
      setTreeHeight((prev) => (prev === next ? prev : next));
    };

    update();

    if (typeof ResizeObserver === "undefined") {
      window.addEventListener("resize", update);
      return () => window.removeEventListener("resize", update);
    }

    const ro = new ResizeObserver(() => update());
    ro.observe(treeContainerEl);
    return () => ro.disconnect();
  }, [treeContainerEl]);

  useEffect(() => {
    if (searchValue.trim()) {
      setExpandedKeys(Array.from(filteredTree.matchedKeys));
      setAutoExpandParent(true);
    }
  }, [filteredTree.matchedKeys, searchValue]);

  useEffect(() => {
    if (!searchValue.trim() && effectiveCategories.length > 0) {
      setExpandedKeys((prev) =>
        prev.length === 0 ? defaultExpandedKeys : prev,
      );
    }
  }, [effectiveCategories, searchValue, defaultExpandedKeys]);

  // 外部跳转（如 URL 参数）时：自动展开到目标节点的父级路径并滚动
  useEffect(() => {
    if (!scrollToNodeId) return;
    if (searchValue.trim()) return;
    if (lookups.byId.size === 0) return;

    // 展开父级路径
    const nextExpanded = new Set<string>();
    const visited = new Set<number>();
    let current = lookups.byId.get(scrollToNodeId);
    while (current?.parent_id != null) {
      const parentId = current.parent_id;
      if (visited.has(parentId)) break;
      visited.add(parentId);
      nextExpanded.add(String(parentId));
      current = lookups.byId.get(parentId);
    }

    if (nextExpanded.size > 0) {
      setExpandedKeys((prev) => {
        const merged = new Set<string>(prev);
        nextExpanded.forEach((k) => merged.add(k));
        return Array.from(merged);
      });
      setAutoExpandParent(true);
    }

    // 延迟滚动，等待展开动画完成
    const key = String(scrollToNodeId);
    const timer = setTimeout(() => {
      treeRef.current?.scrollTo?.({ key, align: "auto" });
      onScrollToNodeComplete?.();
    }, 300);
    return () => clearTimeout(timer);
  }, [scrollToNodeId, searchValue, lookups.byId, onScrollToNodeComplete]);


  useEffect(() => {
    if (!categories || categories.length === 0) {
      if (selectedIds.length > 0 || selectionParentId !== undefined || lastSelectedId !== null) {
        onSelectionChange({ selectedIds: [], selectionParentId: undefined, lastSelectedId: null });
      }
      return;
    }
    const existing = new Set<number>();
    const flatten = (nodes?: Category[]) => {
      if (!nodes) return;
      nodes.forEach((node) => {
        existing.add(node.id);
        if (node.children && node.children.length > 0) {
          flatten(node.children);
        }
      });
    };
    flatten(categories);
    const nextIds = selectedIds.filter((id) => existing.has(id));
    if (
      nextIds.length !== selectedIds.length ||
      (nextIds.length > 0 && selectionParentId !== (lookups.byId.get(nextIds[0])?.parent_id ?? null))
    ) {
      const first = nextIds[0];
      const parentId = first != null ? lookups.byId.get(first)?.parent_id ?? null : undefined;
      onSelectionChange({
        selectedIds: nextIds,
        selectionParentId: parentId ?? (nextIds.length === 0 ? undefined : parentId),
        lastSelectedId: nextIds.length > 0 ? nextIds[nextIds.length - 1] : null,
      });
    }
  }, [categories, lookups.byId, selectedIds, selectionParentId, lastSelectedId, onSelectionChange]);

  const isDescendantOrSelf = useCallback(
    (nodeId: number | null, sourceSet: Set<number>) => {
      if (nodeId == null) {
        return false;
      }
      const visited = new Set<number>();
      let current = lookups.byId.get(nodeId);
      while (current) {
        if (sourceSet.has(current.id)) {
          return true;
        }
        const parentId = current.parent_id;
        if (parentId == null || visited.has(parentId)) {
          break;
        }
        visited.add(parentId);
        current = lookups.byId.get(parentId);
      }
      return false;
    },
    [lookups.byId],
  );

  const handleSearchChange = useCallback((value: string) => {
    setSearchValue(value);
  }, []);

  const handleSearchSubmit = useCallback((value: string) => {
    setSearchValue(value);
  }, []);

  const handleCreateRootClick = useCallback(() => {
    onRequestCreate(null);
  }, [onRequestCreate]);

  const focusNodeSelection = useCallback(
    (nodeId: number) => {
      const node = lookups.byId.get(nodeId);
      const parentId = node?.parent_id ?? null;
      onSelectionChange({
        selectedIds: [nodeId],
        selectionParentId: parentId,
        lastSelectedId: nodeId,
      });
    },
    [lookups.byId, onSelectionChange],
  );

  const handleCreateChild = useCallback(
    (parentId: number) => {
      focusNodeSelection(parentId);
      onRequestCreate(parentId);
    },
    [focusNodeSelection, onRequestCreate],
  );

  const handleRenameNode = useCallback(
    (nodeId: number) => {
      focusNodeSelection(nodeId);
      onRequestRename();
    },
    [focusNodeSelection, onRequestRename],
  );

  const handleDeleteSelection = useCallback(
    (ids: number[]) => {
      if (ids.length === 0) {
        messageApi.warning("请先选择需要删除的目录");
        return;
      }
      onRequestDelete(ids);
    },
    [messageApi, onRequestDelete],
  );

  /** 复制节点子树下所有文档的 ID */
  const handleCopySubtreeDocIds = useCallback(
    async (nodeId: number, nodeName: string) => {
      const messageKey = "copy-subtree-doc-ids";
      messageApi.open({
        type: "loading",
        content: "正在获取子树文档...",
        key: messageKey,
        duration: 0,
      });

      try {
        const pageSize = 100; // API 最大支持 100
        const allIds: number[] = [];
        const seen = new Set<number>();
        let page = 1;

        while (true) {
          const result = await getNodeDocuments(nodeId, {
            page,
            size: pageSize,
            include_descendants: true,
          });

          for (const doc of result.items ?? []) {
            if (!seen.has(doc.id)) {
              seen.add(doc.id);
              allIds.push(doc.id);
            }
          }

          const fetchedCount = result.page * result.size;
          if ((result.items?.length ?? 0) === 0 || fetchedCount >= result.total) {
            break;
          }
          page += 1;
        }

        if (allIds.length === 0) {
          messageApi.info({ content: "该节点子树下没有文档", key: messageKey });
          return;
        }

        setDocClipboard({
          sourceNodeId: nodeId,
          sourceNodeName: nodeName,
          docIds: allIds,
          copiedAt: Date.now(),
        });

        messageApi.success({
          content: `已复制 ${allIds.length} 个文档ID（来自「${nodeName}」）`,
          key: messageKey,
        });
      } catch (error) {
        messageApi.error({
          content: "复制失败：" + (error as Error).message,
          key: messageKey,
        });
      }
    },
    [messageApi],
  );

  /** 将剪贴板中的文档 ID 批量关联为当前节点及其子孙节点的源文档 */
  const handlePasteAsSourceDocs = useCallback(
    async (nodeId: number) => {
      const messageKey = "paste-source-docs";

      if (!docClipboard || docClipboard.docIds.length === 0) {
        messageApi.warning("文档剪贴板为空，请先使用「复制子树文档ID」");
        return;
      }

      // 收集目标节点及其所有子孙节点
      const collectDescendantIds = (id: number): number[] => {
        const result = [id];
        const children = lookups.parentToChildren.get(id) ?? [];
        for (const child of children) {
          result.push(...collectDescendantIds(child.id));
        }
        return result;
      };
      const targetNodeIds = collectDescendantIds(nodeId);
      const targetNodeName = lookups.byId.get(nodeId)?.name ?? String(nodeId);

      // 计算总操作数
      const totalOperations = targetNodeIds.length * docClipboard.docIds.length;

      // 数量较大时二次确认
      if (totalOperations > 10) {
        const confirmed = await new Promise<boolean>((resolve) => {
          Modal.confirm({
            title: "确认关联源文档",
            content: `即将为「${targetNodeName}」及其 ${targetNodeIds.length - 1} 个子孙节点关联 ${docClipboard.docIds.length} 个源文档（共 ${totalOperations} 次操作），是否继续？`,
            okText: "继续",
            cancelText: "取消",
            onOk: () => resolve(true),
            onCancel: () => resolve(false),
          });
        });

        if (!confirmed) {
          messageApi.info({ content: "已取消", key: messageKey });
          return;
        }
      }

      setIsMutating(true);
      messageApi.open({
        type: "loading",
        content: `正在为 ${targetNodeIds.length} 个节点关联源文档...`,
        key: messageKey,
        duration: 0,
      });

      try {
        let successCount = 0;
        let skippedCount = 0;
        let failedCount = 0;

        // 对每个目标节点执行绑定
        for (const targetId of targetNodeIds) {
          // 获取该节点已有的源文档进行去重
          const existingSources = await getNodeSourceDocuments(targetId);
          const existingIds = new Set(existingSources.map((s) => s.document_id));
          const toBindIds = docClipboard.docIds.filter((id) => !existingIds.has(id));

          // 批量绑定
          for (const docId of toBindIds) {
            try {
              await bindSourceDocument(targetId, docId);
              successCount++;
            } catch (e: unknown) {
              const errMsg = (e as Error).message ?? "";
              if (errMsg.includes("already exists") || errMsg.includes("已存在")) {
                skippedCount++;
              } else {
                failedCount++;
              }
            }
          }

          // 跳过已存在的
          skippedCount += existingIds.size;
        }

        // 刷新查询
        await onInvalidateQueries();

        // 显示结果
        const parts: string[] = [`已为 ${targetNodeIds.length} 个节点关联 ${successCount} 个源文档`];
        if (skippedCount > 0) {
          parts.push(`${skippedCount} 个已存在`);
        }
        if (failedCount > 0) {
          parts.push(`${failedCount} 个失败`);
        }

        if (failedCount > 0) {
          messageApi.warning({ content: parts.join("，"), key: messageKey });
        } else {
          messageApi.success({ content: parts.join("，"), key: messageKey });
        }
      } catch (error) {
        messageApi.error({
          content: "关联失败：" + (error as Error).message,
          key: messageKey,
        });
      } finally {
        setIsMutating(false);
      }
    },
    [docClipboard, lookups, messageApi, onInvalidateQueries, setIsMutating],
  );

  const { handlePasteAsChild, handlePasteBefore, handlePasteAfter } = useTreePaste({
    clipboard,
    clipboardSourceSet,
    isMutating,
    setIsMutating,
    messageApi,
    isDescendantOrSelf,
    onInvalidateQueries,
    onSelectionChange,
    resetClipboard,
    lookups,
  });

  const handleNodeDragOver = useCallback((event: ReactDragEvent<HTMLSpanElement>, nodeId: number) => {
    // 检查是否是文档拖放（使用 Array.from 处理 DOMStringList 兼容性）
    const isDocumentDrag = Array.from(event.dataTransfer.types).includes("application/json");
    if (!isDocumentDrag) {
      // 节点拖拽：不阻止冒泡，让事件传递给 Tree 组件
      return;
    }
    // 文档拖放：阻止默认行为和冒泡，显示拖放目标
    event.preventDefault();
    event.stopPropagation();
    event.dataTransfer.dropEffect = "copy";
    setDropTargetNodeId(nodeId);
  }, []);

  const handleNodeDragLeave = useCallback((event: ReactDragEvent<HTMLSpanElement>) => {
    // 检查是否是文档拖放（使用 Array.from 处理 DOMStringList 兼容性）
    const isDocumentDrag = Array.from(event.dataTransfer.types).includes("application/json");
    if (!isDocumentDrag) {
      // 节点拖拽：不阻止冒泡，让事件传递给 Tree 组件
      return;
    }
    // 文档拖放：清理高亮状态
    event.preventDefault();
    event.stopPropagation();
    setDropTargetNodeId(null);
  }, []);

  const handleNodeDrop = useCallback(
    (event: ReactDragEvent<HTMLSpanElement>, nodeId: number) => {
      // 先检查是否是文档拖放
      const data = event.dataTransfer.getData("application/json");
      if (!data) {
        // 不是文档拖放，让事件继续冒泡给 Tree 组件处理节点拖拽
        return;
      }

      // 有 JSON 数据，视为文档拖放尝试，阻止冒泡
      event.preventDefault();
      event.stopPropagation();
      setDropTargetNodeId(null);

      try {
        const dragData = JSON.parse(data);
        if (dragData.type !== "document") {
          // 不是文档类型，忽略但不报错
          return;
        }

        if (onDocumentDrop) {
          onDocumentDrop(nodeId, dragData);
        }
      } catch (error) {
        messageApi.error("拖放失败：数据格式错误");
      }
    },
    [messageApi, onDocumentDrop],
  );

  const renderTreeTitle = useCallback(
    (node: TreeDataNode) => {
      const nodeId = Number(node.key);
      const category = lookups.byId.get(nodeId);
      const docCount = category?.subtree_doc_count ?? 0;
      const isCutNode = clipboard?.mode === "cut" && clipboardSourceSet.has(nodeId);
      const isDropTarget = dropTargetNodeId === nodeId;

      return (
        <span
          style={{
            opacity: isCutNode ? 0.5 : 1,
            backgroundColor: isDropTarget ? "#e6f7ff" : "transparent",
            padding: "2px 8px",
            borderRadius: 4,
            display: "inline-flex",
            alignItems: "center",
            width: "100%",
          }}
          onDragOver={(e) => handleNodeDragOver(e, nodeId)}
          onDragLeave={handleNodeDragLeave}
          onDrop={(e) => handleNodeDrop(e, nodeId)}
        >
          <span style={{ flex: 1 }}>{node.title as string}</span>
          {docCount > 0 && (
            <Tag
              color="blue"
              style={{
                marginLeft: 8,
                fontSize: 11,
                lineHeight: "16px",
                padding: "0 4px",
              }}
            >
              {docCount}
            </Tag>
          )}
        </span>
      );
    },
    [clipboard, clipboardSourceSet, dropTargetNodeId, lookups.byId, handleNodeDragOver, handleNodeDragLeave, handleNodeDrop],
  );

  const handleTreeRightClick = useCallback<NonNullable<AntTreeProps["onRightClick"]>>(
    (info) => {
      const { event, node } = info as TreeRightClickInfo;
      const mouseEvent = event as ReactMouseEvent;
      mouseEvent.preventDefault();
      mouseEvent.stopPropagation();
      if (typeof mouseEvent.nativeEvent?.preventDefault === "function") {
        mouseEvent.nativeEvent.preventDefault();
      }
      const nodeId = Number(node.key);
      const parentId = getParentId(node);
      const normalizedParent = parentId ?? null;

      if (!selectedIds.includes(nodeId)) {
        onSelectionChange({
          selectedIds: [nodeId],
          selectionParentId: normalizedParent,
          lastSelectedId: nodeId,
        });
      } else if (selectionParentId !== normalizedParent) {
        onSelectionChange({
          selectedIds,
          selectionParentId: normalizedParent,
          lastSelectedId,
        });
      }

      openContextMenu({
        nodeId,
        parentId: normalizedParent,
        x: mouseEvent.clientX,
        y: mouseEvent.clientY,
      });
    },
    [selectedIds, selectionParentId, lastSelectedId, onSelectionChange, openContextMenu],
  );

  const contextMenuItems = useMemo<MenuProps["items"]>(() => {
    if (!contextMenu.open || contextMenu.nodeId == null) {
      if (!clipboard) {
        return [];
      }
      return [
        {
          key: "clear-clipboard",
          icon: <ClearOutlined />,
          label: "清空剪贴板",
          onClick: () => {
            closeContextMenu("action:clear-clipboard-fallback");
            clearClipboard();
          },
        },
      ];
    }

    const nodeId = contextMenu.nodeId;
    const nodeParentId = contextMenu.parentId;
    const selectionIncludesNode = selectedIds.includes(nodeId);
    const resolvedSelectionParent =
      selectionIncludesNode && selectionParentId !== undefined
        ? selectionParentId
        : contextMenu.parentId;
    const resolvedSelectionIds = selectionIncludesNode ? selectedIds : [nodeId];
    const resolvedSelectionAvailable =
      resolvedSelectionParent !== undefined && resolvedSelectionIds.length > 0;
    const contextCanCopy = !isMutating && resolvedSelectionAvailable;

    const items: MenuProps["items"] = [];

    if (canManageCategories) {
      items.push({
        key: "add-document",
        icon: <FileAddOutlined />,
        label: "添加文档",
        onClick: () => {
          closeContextMenu("action:add-document");
          onOpenAddDocument(nodeId);
        },
      });
    }

    // 批量操作菜单项 - 仅管理员可用
    const targetNode = lookups.byId.get(nodeId);
    if (canManageCategories && targetNode) {
      items.push({ type: "divider" });
      if (onOpenBatchWorkflow) {
        items.push({
          key: "batch-workflow",
          icon: <ThunderboltOutlined />,
          label: "批量执行工作流",
          disabled: isMutating,
          onClick: () => {
            closeContextMenu("action:batch-workflow");
            // 收集所有选中的节点（如果当前节点在选中列表中则使用选中列表，否则只使用当前节点）
            const targetIds = selectionIncludesNode ? resolvedSelectionIds : [nodeId];
            const targetNames = targetIds.map(id => lookups.byId.get(id)?.name || `节点 ${id}`);

            // 检测父子节点重合
            const hasOverlap = (): boolean => {
              const idSet = new Set(targetIds);
              for (const id of targetIds) {
                const visited = new Set<number>();
                let current = lookups.byId.get(id);
                while (current?.parent_id != null) {
                  const parentId = current.parent_id;
                  if (idSet.has(parentId)) return true;
                  if (visited.has(parentId)) break; // cycle 防护
                  visited.add(parentId);
                  current = lookups.byId.get(parentId);
                }
              }
              return false;
            };

            if (hasOverlap()) {
              messageApi.warning("选中的节点存在包含关系（父子重合），请调整选择后重试");
              return;
            }

            onOpenBatchWorkflow(targetIds, targetNames);
          },
        });
      }
      if (onOpenBatchSync) {
        items.push({
          key: "batch-sync",
          icon: <SyncOutlined />,
          label: "批量同步文档",
          disabled: isMutating,
          onClick: () => {
            closeContextMenu("action:batch-sync");
            // 支持多选：如果有选中的节点，使用选中的节点；否则使用右键点击的节点
            const targetIds = selectedIds.length > 0 ? selectedIds : [nodeId];
            const targetNames = selectedIds.length > 0
              ? selectedIds.map(id => lookups.byId.get(id)?.name || `节点${id}`)
              : [targetNode.name];

            if (targetIds.length === 0) {
              messageApi.warning("请先选择要同步的节点");
              return;
            }

            onOpenBatchSync(targetIds, targetNames);
          },
        });
      }
      items.push({ type: "divider" });
    }

    // 复制节点路径 - 所有用户都可以使用
    if (targetNode?.path) {
      items.push({
        key: "copy-path",
        icon: <LinkOutlined />,
        label: "复制节点路径",
        onClick: () => {
          closeContextMenu("action:copy-path");
          navigator.clipboard.writeText(targetNode.path).then(
            () => messageApi.success(`已复制路径: ${targetNode.path}`),
            () => messageApi.error("复制失败")
          );
        },
      });
    }

    // 复制节点ID - 所有用户都可以使用
    if (targetNode) {
      items.push({
        key: "copy-id",
        icon: <CopyOutlined />,
        label: "复制节点ID",
        onClick: () => {
          closeContextMenu("action:copy-id");
          navigator.clipboard.writeText(String(targetNode.id)).then(
            () => messageApi.success(`已复制ID: ${targetNode.id}`),
            () => messageApi.error("复制失败")
          );
        },
      });

      // 复制子树文档ID - 所有用户都可以使用
      items.push({
        key: "copy-subtree-doc-ids",
        icon: <CopyOutlined />,
        label: "复制子树文档ID",
        onClick: () => {
          closeContextMenu("action:copy-subtree-doc-ids");
          handleCopySubtreeDocIds(nodeId, targetNode.name);
        },
      });
    }

    if (resolvedSelectionAvailable) {
      if (canManageCategories && resolvedSelectionIds.length === 1) {
        const targetId = resolvedSelectionIds[0];
        items.push(
          {
            key: "create-child",
            icon: <PlusSquareOutlined />,
            label: "新建子目录",
            disabled: isMutating,
            onClick: () => {
              closeContextMenu("action:create-child");
              handleCreateChild(targetId);
            },
          },
          {
            key: "rename-node",
            icon: <EditOutlined />,
            label: "编辑目录",
            disabled: isMutating,
            onClick: () => {
              closeContextMenu("action:rename-node");
              handleRenameNode(targetId);
            },
          },
        );
      }
      if (canManageCategories) {
        items.push({
          key: "copy-selection",
          icon: <CopyOutlined />,
          label: "复制所选",
          disabled: !contextCanCopy,
          onClick: () => {
            closeContextMenu("action:copy-selection");
            handleCopySelection({
              ids: resolvedSelectionIds,
              parentId: resolvedSelectionParent ?? null,
            });
          },
        });
        items.push(
          {
            key: "cut-selection",
            icon: <ScissorOutlined />,
            label: "剪切所选",
            disabled: !contextCanCopy,
            onClick: () => {
              closeContextMenu("action:cut-selection");
              handleCutSelection({
                ids: resolvedSelectionIds,
                parentId: resolvedSelectionParent ?? null,
              });
            },
          },
          {
            key: "delete-node",
            icon: <DeleteOutlined />,
            label: "删除目录",
            disabled: resolvedSelectionIds.length === 0 || isMutating,
            onClick: () => {
              closeContextMenu("action:delete-node");
              handleDeleteSelection(resolvedSelectionIds);
            },
          },
        );
      }
    }

    if (clipboard && canManageCategories) {
      items.push({ type: "divider" });
      items.push(
        {
          key: "paste-child",
          icon: clipboard.mode === "cut" ? <ScissorOutlined /> : <SnippetsOutlined />,
          label: clipboard.mode === "cut" ? "剪切到该节点" : "粘贴为子节点",
          disabled: isMutating || isDescendantOrSelf(nodeId, clipboardSourceSet),
          onClick: () => {
            closeContextMenu("action:paste-child");
            handlePasteAsChild(nodeId);
          },
        },
        {
          key: "paste-before",
          icon: <SnippetsOutlined />,
          label: clipboard.mode === "cut" ? "剪切到此前" : "粘贴到此前",
          disabled:
            isMutating ||
            clipboardSourceSet.has(nodeId) ||
            isDescendantOrSelf(nodeParentId, clipboardSourceSet),
          onClick: () => {
            closeContextMenu("action:paste-before");
            handlePasteBefore(nodeId);
          },
        },
        {
          key: "paste-after",
          icon: <SnippetsOutlined />,
          label: clipboard.mode === "cut" ? "剪切到此后" : "粘贴到此后",
          disabled:
            isMutating ||
            clipboardSourceSet.has(nodeId) ||
            isDescendantOrSelf(nodeParentId, clipboardSourceSet),
          onClick: () => {
            closeContextMenu("action:paste-after");
            handlePasteAfter(nodeId);
          },
        },
      );
      // 粘贴为源文档 - 仅在有文档剪贴板时显示
      if (docClipboard && docClipboard.docIds.length > 0) {
        items.push({
          key: "paste-source-docs",
          icon: <LinkOutlined />,
          label: `粘贴为源文档（${docClipboard.docIds.length}个）`,
          disabled: isMutating,
          onClick: () => {
            closeContextMenu("action:paste-source-docs");
            handlePasteAsSourceDocs(nodeId);
          },
        });
      }
      items.push({ type: "divider" });
      items.push({
        key: "clear-clipboard",
        icon: <ClearOutlined />,
        label: "清空剪贴板",
        onClick: () => {
          closeContextMenu("action:clear-clipboard");
          clearClipboard();
          setDocClipboard(null); // 同时清空文档剪贴板
        },
      });
    } else if (canManageCategories && docClipboard && docClipboard.docIds.length > 0) {
      // 没有目录剪贴板，但有文档剪贴板时也显示粘贴为源文档
      items.push({ type: "divider" });
      items.push({
        key: "paste-source-docs",
        icon: <LinkOutlined />,
        label: `粘贴为源文档（${docClipboard.docIds.length}个）`,
        disabled: isMutating,
        onClick: () => {
          closeContextMenu("action:paste-source-docs");
          handlePasteAsSourceDocs(nodeId);
        },
      });
      items.push({
        key: "clear-doc-clipboard",
        icon: <ClearOutlined />,
        label: "清空文档剪贴板",
        onClick: () => {
          closeContextMenu("action:clear-doc-clipboard");
          setDocClipboard(null);
          messageApi.success("已清空文档剪贴板");
        },
      });
    }

    if (menuDebugEnabled) {
      // eslint-disable-next-line no-console
      console.log("[menu-debug] context menu items", {
        nodeId,
        selectionIncludesNode,
        resolvedSelectionIds,
        resolvedSelectionParent,
        count: items.length,
        keys: items.map((item) => {
          if (!item) {
            return null;
          }
          if ("key" in item && item.key) {
            return item.key;
          }
          return item.type ?? null;
        }),
      });
    }

    return items;
  }, [
    canManageCategories,
    clipboard,
    clipboardSourceSet,
    clearClipboard,
    closeContextMenu,
    docClipboard,
    handleCopySelection,
    handleCopySubtreeDocIds,
    handleCutSelection,
    handleCreateChild,
    handleRenameNode,
    handlePasteAfter,
    handlePasteAsChild,
    handlePasteAsSourceDocs,
    handlePasteBefore,
    handleDeleteSelection,
    isDescendantOrSelf,
    isMutating,
    menuDebugEnabled,
    messageApi,
    lookups,
    onOpenAddDocument,
    onOpenBatchWorkflow,
    onOpenBatchSync,
    selectedIds,
    selectionParentId,
    contextMenu,
  ]);

  const contextMenuVisible = contextMenu.open && (contextMenuItems?.length ?? 0) > 0;

  const { handleDrop } = useTreeDrag({
    lookups,
    messageApi,
    closeContextMenu,
    dragDebugEnabled,
    menuDebugEnabled,
    setIsMutating,
    onRefresh,
    onInvalidateQueries,
  });
  const handleSelect = useCallback<NonNullable<AntTreeProps["onSelect"]>>(
    (_keys, info) => {
      const typedInfo = info as TreeSelectInfo;
      const clickedId = Number(typedInfo.node.key);
      const parentId = getParentId(typedInfo.node);
      const normalizedParent = parentId ?? null;
      const nativeEvent = typedInfo.nativeEvent as MouseEvent | undefined;
      const isShift = nativeEvent?.shiftKey ?? false;
      const isMeta = nativeEvent ? nativeEvent.metaKey || nativeEvent.ctrlKey : false;

      if (typedInfo.selected) {
        if (
          selectionParentId !== undefined &&
          selectionParentId !== normalizedParent &&
          (selectedIds.length > 1 || isMeta || isShift)
        ) {
          messageApi.warning("仅支持同一父节点下的多选");
          return;
        }

        if (
          selectionParentId !== undefined &&
          selectionParentId !== normalizedParent &&
          !isMeta &&
          !isShift
        ) {
          onSelectionChange({
            selectedIds: [clickedId],
            selectionParentId: normalizedParent,
            lastSelectedId: clickedId,
          });
          return;
        }

        let nextIds = selectedIds;
        if (!isMeta && !isShift) {
          nextIds = [clickedId];
        } else if (isShift && lastSelectedId != null && selectionParentId !== undefined) {
          const siblings = lookups.parentToChildren.get(normalizedParent) ?? [];
          const order = siblings.map((node) => node.id);
          const lastIndex = order.indexOf(lastSelectedId);
          const currentIndex = order.indexOf(clickedId);
          if (lastIndex !== -1 && currentIndex !== -1) {
            const [start, end] = [
              Math.min(lastIndex, currentIndex),
              Math.max(lastIndex, currentIndex),
            ];
            const rangeSet = new Set<number>(nextIds);
            for (let i = start; i <= end; i += 1) {
              rangeSet.add(order[i]);
            }
            nextIds = Array.from(rangeSet).sort((a, b) => order.indexOf(a) - order.indexOf(b));
          }
        } else if (isMeta) {
          const set = new Set<number>(nextIds);
          if (set.has(clickedId)) {
            set.delete(clickedId);
          } else {
            set.add(clickedId);
          }
          nextIds = Array.from(set);
        }
        onSelectionChange({
          selectedIds: nextIds,
          selectionParentId: normalizedParent,
          lastSelectedId: clickedId,
        });
      } else {
        const set = new Set<number>(selectedIds);
        set.delete(clickedId);
        const nextIds = Array.from(set);
        const nextParent = nextIds.length === 0 ? undefined : normalizedParent;
        onSelectionChange({
          selectedIds: nextIds,
          selectionParentId: nextParent,
          lastSelectedId,
        });
      }
    },
    [
      lastSelectedId,
      lookups.parentToChildren,
      messageApi,
      onSelectionChange,
      selectedIds,
      selectionParentId,
    ],
  );

  let bodyContent: ReactNode;
  if (isLoading) {
    bodyContent = (
      <div style={PANEL_PLACEHOLDER_STYLE}>
        <Spin />
      </div>
    );
  } else if (error) {
    bodyContent = (
      <div style={PANEL_PLACEHOLDER_STYLE}>
        <Alert type="error" message="目录树加载失败" description={(error as Error).message} />
      </div>
    );
  } else if (treeData.length === 0) {
    bodyContent = (
      <div style={PANEL_PLACEHOLDER_STYLE}>
        <Empty description="暂无目录" />
      </div>
    );
  } else {
    // 虚拟滚动需要有效的容器高度，否则退回普通滚动模式
    const enableVirtual = treeHeight > 0;
    const scrollStyle: CSSProperties = {
      ...PANEL_SCROLL_STYLE,
      overflow: enableVirtual ? "hidden" : "auto",
    };
    bodyContent = (
      <div
        ref={setTreeContainerEl}
        style={scrollStyle}
        onContextMenuCapture={suppressNativeContextMenu}
      >
        <Tree<TreeDataNode>
          ref={treeRef}
          blockNode
          draggable={{ icon: false }}
          showLine={{ showLeafIcon: false }}
          multiple
          treeData={treeData}
          titleRender={renderTreeTitle}
          onDrop={handleDrop}
          selectedKeys={selectedIds.map(String)}
          onSelect={handleSelect}
          onRightClick={handleTreeRightClick}
          expandedKeys={expandedKeys}
          autoExpandParent={autoExpandParent}
          onExpand={(keys) => {
            setExpandedKeys(keys.map(String));
            setAutoExpandParent(false);
          }}
          virtual={enableVirtual}
          height={enableVirtual ? treeHeight : undefined}
          itemHeight={28}
          style={{ userSelect: "none" }}
        />
      </div>
    );
  }

  return (
    <div style={PANEL_ROOT_STYLE}>
      {contextMenuVisible ? (
        <div
          ref={menuContainerRef}
          style={{
            position: "fixed",
            top: contextMenu.y,
            left: contextMenu.x,
            zIndex: 1050,
            background: "#fff",
            border: "1px solid #d9d9d9",
            borderRadius: 6,
            boxShadow: "0 6px 18px rgba(0,0,0,0.15)",
            minWidth: 160,
            overflow: "hidden",
          }}
        >
          <Menu
            selectable={false}
            items={contextMenuItems}
            onClick={({ domEvent }) => {
              domEvent.stopPropagation();
              domEvent.preventDefault();
            }}
          />
        </div>
      ) : null}
      <CategoryTreeToolbar
        searchValue={searchValue}
        onSearchChange={handleSearchChange}
        onSearchSubmit={handleSearchSubmit}
        onRefresh={onRefresh}
        onCreateRoot={handleCreateRootClick}
        onOpenTrash={onOpenTrash}
        includeDescendants={includeDescendants}
        onIncludeDescendantsChange={onIncludeDescendantsChange}
        isRefreshing={isFetching || isMutating}
        createLoading={createLoading}
        trashIsFetching={trashIsFetching}
        selectedNodeId={selectedNodeId}
        canCreate={canCreateRoot}
      />
      {clipboard ? (
        <Tag color={clipboard.mode === "cut" ? "orange" : "blue"} style={{ marginBottom: 16 }}>
          剪贴板：{clipboard.mode === "cut" ? "剪切" : "复制"} {clipboard.sourceIds.length} 项
        </Tag>
      ) : null}
      {docClipboard ? (
        <Tag color="green" style={{ marginBottom: 16, marginLeft: clipboard ? 8 : 0 }}>
          文档剪贴板：{docClipboard.docIds.length} 个（来自「{docClipboard.sourceNodeName}」）
        </Tag>
      ) : null}
      <div style={PANEL_BODY_STYLE}>{bodyContent}</div>
    </div>
  );
}

function getDefaultExpandedKeys(nodes: Category[], maxDepth: number): string[] {
  if (!Array.isArray(nodes) || nodes.length === 0 || maxDepth <= 0) {
    return [];
  }
  const collected = new Set<string>();
  const traverse = (items: Category[] | undefined, depth: number) => {
    if (!items || depth >= maxDepth) {
      return;
    }
    items.forEach((item) => {
      collected.add(item.id.toString());
      if (item.children && item.children.length > 0) {
        traverse(item.children, depth + 1);
      }
    });
  };
  traverse(nodes, 0);
  return Array.from(collected);
}
