import { Form } from "antd";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createContext,
  useContext,
  useState,
  useCallback,
  useMemo,
  ReactNode,
} from "react";
import type { MessageInstance } from "antd/es/message/interface";
import { Category, getCategoryTree } from "../api/categories";
import { useTreeActions } from "../features/categories/hooks/useTreeActions";
import { useTrashQuery } from "../features/categories/hooks/useTrashQuery";
import { useDeletePreview } from "../features/categories/hooks/useDeletePreview";
import { buildLookups } from "../features/categories/utils";
import type { ParentKey } from "../features/categories/types";

interface CategoryContextValue {
  // 分类树数据
  categoriesList: Category[];
  isLoading: boolean;
  isFetching: boolean;
  error: Error | null;
  lookups: ReturnType<typeof buildLookups>;

  // 选择状态
  selectedIds: number[];
  selectionParentId: number | null | undefined;
  lastSelectedId: number | null;
  selectedNodeId: number | null;

  // 表单
  categoryForm: ReturnType<typeof Form.useForm<{ name: string; type?: string | null }>>[0];

  // 变更操作状态
  isMutating: boolean;
  setMutating: (value: boolean) => void;

  // CRUD mutations
  createMutation: ReturnType<typeof useTreeActions>["createMutation"];
  updateMutation: ReturnType<typeof useTreeActions>["updateMutation"];
  deleteMutation: ReturnType<typeof useTreeActions>["deleteMutation"];
  bulkDeleteMutation: ReturnType<typeof useTreeActions>["bulkDeleteMutation"];
  restoreMutation: ReturnType<typeof useTreeActions>["restoreMutation"];
  purgeMutation: ReturnType<typeof useTreeActions>["purgeMutation"];

  // 回收站
  trashQuery: ReturnType<typeof useTrashQuery>["trashQuery"];
  trashItems: ReturnType<typeof useTrashQuery>["trashItems"];
  isTrashInitialLoading: boolean;
  selectedTrashRowKeys: ReturnType<typeof useTrashQuery>["selectedRowKeys"];
  setSelectedTrashRowKeys: ReturnType<typeof useTrashQuery>["setSelectedRowKeys"];
  isTrashProcessing: boolean;

  // 删除预览
  deletePreview: ReturnType<typeof useDeletePreview>["deletePreview"];
  openDeletePreview: ReturnType<typeof useDeletePreview>["openPreview"];
  closeDeletePreview: ReturnType<typeof useDeletePreview>["closePreview"];
  setDeletePreviewLoading: ReturnType<typeof useDeletePreview>["setLoading"];

  // Actions
  handleSelectionChange: (params: {
    selectedIds: number[];
    selectionParentId: ParentKey | undefined;
    lastSelectedId: number | null;
  }) => void;
  handleRefreshTree: () => void;
  invalidateCategoryQueries: () => Promise<void>;
  handleTrashBulkRestore: () => Promise<void>;
  handleTrashBulkPurge: () => Promise<void>;
}

const CategoryContext = createContext<CategoryContextValue | undefined>(undefined);

export const useCategoryContext = () => {
  const context = useContext(CategoryContext);
  if (!context) {
    throw new Error("useCategoryContext must be used within CategoryProvider");
  }
  return context;
};

interface CategoryProviderProps {
  messageApi: MessageInstance;
  children: ReactNode;
}

export const CategoryProvider = ({ messageApi, children }: CategoryProviderProps) => {
  const queryClient = useQueryClient();

  // 分类树查询
  const { data, isLoading, isFetching, error, refetch } = useQuery({
    queryKey: ["categories-tree"],
    queryFn: () => getCategoryTree(),
  });

  // 选择状态
  const [selectedIds, setSelectedIds] = useState<number[]>([]);
  const [selectionParentId, setSelectionParentId] = useState<number | null | undefined>(undefined);
  const [lastSelectedId, setLastSelectedId] = useState<number | null>(null);

  // 表单
  const [categoryForm] = Form.useForm<{ name: string; type?: string | null }>();

  // Mutations
  const {
    createMutation,
    updateMutation,
    deleteMutation,
    bulkDeleteMutation,
    restoreMutation,
    purgeMutation,
    setMutating,
    isMutating,
  } = useTreeActions(messageApi);

  // 回收站
  const {
    trashQuery,
    trashItems,
    isInitialLoading: isTrashInitialLoading,
    selectedRowKeys: selectedTrashRowKeys,
    setSelectedRowKeys: setSelectedTrashRowKeys,
    handleBulkRestore,
    handleBulkPurge,
    isProcessing: isTrashProcessing,
  } = useTrashQuery(messageApi);

  // 删除预览
  const {
    deletePreview,
    openPreview: openDeletePreview,
    closePreview: closeDeletePreview,
    setLoading: setDeletePreviewLoading,
  } = useDeletePreview(messageApi);

  const selectedNodeId = selectedIds.length === 1 ? selectedIds[0] : null;
  const categoriesList = data ?? [];
  const lookups = useMemo(() => buildLookups(data ?? []), [data]);

  const handleSelectionChange = useCallback(
    ({
      selectedIds: nextIds,
      selectionParentId: nextParent,
      lastSelectedId: nextLast,
    }: {
      selectedIds: number[];
      selectionParentId: ParentKey | undefined;
      lastSelectedId: number | null;
    }) => {
      setSelectedIds(nextIds);
      setSelectionParentId(nextParent);
      setLastSelectedId(nextLast);
    },
    [],
  );

  const handleRefreshTree = useCallback(() => refetch(), [refetch]);

  const invalidateCategoryQueries = useCallback(async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["categories-tree"] }),
      queryClient.invalidateQueries({ queryKey: ["categories-trash"] }),
    ]);
  }, [queryClient]);

  const handleTrashBulkRestore = useCallback(async () => {
    await handleBulkRestore();
    await queryClient.invalidateQueries({ queryKey: ["categories-tree"] });
  }, [handleBulkRestore, queryClient]);

  const handleTrashBulkPurge = useCallback(async () => {
    await handleBulkPurge();
    await queryClient.invalidateQueries({ queryKey: ["categories-tree"] });
  }, [handleBulkPurge, queryClient]);

  const value: CategoryContextValue = {
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
    restoreMutation,
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
  };

  return (
    <CategoryContext.Provider value={value}>
      {children}
    </CategoryContext.Provider>
  );
};
