import { Form } from "antd";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  useMemo,
  ReactNode,
} from "react";
import type { MessageInstance } from "antd/es/message/interface";
import {
  Document,
  DocumentVersionsPage,
  DocumentListParams,
  DocumentReorderPayload,
  DocumentTrashParams,
  MetadataFilterClause,
  getNodeDocuments,
  getNodeSourceDocuments,
  deleteDocument,
  restoreDocument,
  purgeDocument,
  reorderDocuments,
  getDeletedDocuments,
  getDocumentVersions,
  restoreDocumentVersion,
} from "../api/documents";
import type {
  DocumentFilterFormValues,
  MetadataFilterFormValue,
} from "../features/documents/types";
import type { MetadataOperator } from "../api/documents";

interface DocumentContextValue {
  // 文档列表
  documents: Document[];
  isDocumentsLoading: boolean;
  isDocumentsFetching: boolean;
  documentsError: Error | null;
  documentListPage: number;
  documentListSize: number;
  documentListTotal: number;

  // 筛选器
  documentFilters: DocumentFilterFormValues;
  includeDescendants: boolean;
  documentFilterForm: ReturnType<typeof Form.useForm<DocumentFilterFormValues>>[0];

  // 回收站
  documentTrashParams: DocumentTrashParams;
  documentTrashQuery: ReturnType<typeof useQuery>;

  // 历史记录
  documentHistoryParams: { page: number; size: number };
  documentHistoryDocId: number | null;
  documentHistoryQuery: ReturnType<typeof useQuery>;

  // Mutations
  deleteDocumentMutation: ReturnType<typeof useMutation<void, Error, number>>;
  restoreDocumentMutation: ReturnType<typeof useMutation<Document, Error, number>>;
  purgeDocumentMutation: ReturnType<typeof useMutation<void, Error, number>>;
  documentReorderMutation: ReturnType<typeof useMutation<Document[], Error, DocumentReorderPayload>>;
  restoreDocumentVersionMutation: ReturnType<
    typeof useMutation<Document, Error, { docId: number; version: number }>
  >;

  // Loading states
  deletingDocId: number | null;
  restoringDocId: number | null;
  purgingDocId: number | null;
  restoringVersionNumber: number | null;

  // Actions
  handleDocumentSearch: (values: DocumentFilterFormValues) => void;
  handleDocumentReset: () => void;
  handleIncludeDescendantsChange: (value: boolean) => void;
  handleDocumentListPageChange: (page: number, pageSize?: number) => void;
  handleDocumentTrashSearch: (query?: string) => void;
  handleDocumentTrashPageChange: (page: number, pageSize?: number) => void;
  handleDocumentHistoryPageChange: (page: number, pageSize?: number) => void;
  handleRefreshDocumentTrash: () => void;
  setDocumentHistoryDocId: (docId: number | null) => void;
}

const DocumentContext = createContext<DocumentContextValue | undefined>(undefined);

export const useDocumentContext = () => {
  const context = useContext(DocumentContext);
  if (!context) {
    throw new Error("useDocumentContext must be used within DocumentProvider");
  }
  return context;
};

interface DocumentProviderProps {
  messageApi: MessageInstance;
  selectedNodeId: number | null;
  children: ReactNode;
}

export const DocumentProvider = ({
  messageApi,
  selectedNodeId,
  children,
}: DocumentProviderProps) => {
  const queryClient = useQueryClient();

  // 筛选器状态
  const [documentFilters, setDocumentFilters] = useState<DocumentFilterFormValues>({});
  const [includeDescendants, setIncludeDescendants] = useState(false);
  const [documentFilterForm] = Form.useForm<DocumentFilterFormValues>();

  // 文档列表分页参数
  const [documentListParams, setDocumentListParams] = useState({
    page: 1,
    size: 10,
  });

  // 回收站参数
const [documentTrashParams, setDocumentTrashParams] = useState<DocumentTrashParams>({
  page: 1,
  size: 20,
});

  // 历史记录状态
  const [documentHistoryParams, setDocumentHistoryParams] = useState({ page: 1, size: 10 });
  const [documentHistoryDocId, setDocumentHistoryDocId] = useState<number | null>(null);

  // 源文档查询（用于过滤产出文档）
  const sourceDocsQuery = useQuery({
    queryKey: ["node-source-docs", selectedNodeId],
    queryFn: async () => {
      if (selectedNodeId == null) return [];
      return getNodeSourceDocuments(selectedNodeId);
    },
    enabled: selectedNodeId != null,
    staleTime: 30_000,
  });

  // 文档列表查询
  const documentsQuery = useQuery({
    queryKey: ["node-documents", selectedNodeId, documentFilters, includeDescendants, documentListParams.page, documentListParams.size],
    queryFn: async () => {
      if (selectedNodeId == null) {
        return { page: 1, size: 10, total: 0, items: [] };
      }
      const params: DocumentListParams = {};
      if (documentFilters.query?.trim()) {
        params.query = documentFilters.query.trim();
      }
      if (documentFilters.type) {
        params.type = documentFilters.type;
      }
      if (documentFilters.docId) {
        const numericId = Number(documentFilters.docId);
        if (!Number.isNaN(numericId)) {
          params.id = [numericId];
        }
      }
      const metadataFilters = sanitizeMetadataFilters(documentFilters.metadataFilters);
      const tagClauses = buildTagClauses(documentFilters.tags);
      const combinedClauses: MetadataFilterClause[] = [];
      if (metadataFilters) {
        combinedClauses.push(
          ...metadataFilters.map(({ key, operator, values }) => ({ key, operator, values })),
        );
      }
      if (tagClauses) {
        combinedClauses.push(...tagClauses);
      }
      if (combinedClauses.length > 0) {
        params.metadataClauses = combinedClauses;
      }
      params.page = documentListParams.page;
      params.size = documentListParams.size;
      params.include_descendants = includeDescendants;

      const page = await getNodeDocuments(selectedNodeId, params);
      // Sort items by position
      const sortedItems = [...page.items].sort((a, b) => a.position - b.position);
      return { ...page, items: sortedItems };
    },
    enabled: selectedNodeId != null,
  });

  // 回收站查询
  const documentTrashQuery = useQuery({
    queryKey: ["documents-trash", documentTrashParams],
    queryFn: () => getDeletedDocuments(documentTrashParams),
    enabled: false,
    staleTime: 10_000,
  });

  const documentHistoryQuery = useQuery<DocumentVersionsPage | null>({
    queryKey: [
      "document-history",
      documentHistoryDocId,
      documentHistoryParams.page,
      documentHistoryParams.size,
    ],
    queryFn: async ({ queryKey }) => {
      const docId = queryKey[1] as number | null;
      const page = queryKey[2] as number;
      const size = queryKey[3] as number;
      if (docId == null) {
        return null;
      }
      return getDocumentVersions(docId, { page, size });
    },
    enabled: documentHistoryDocId != null,
  });

  // Mutations
  const deleteDocumentMutation = useMutation<void, Error, number>({
    mutationFn: async (docId) => {
      await deleteDocument(docId);
    },
    onSuccess: async () => {
      messageApi.success("文档已移入回收站");
      await queryClient.invalidateQueries({ queryKey: ["node-documents"] });
      await queryClient.invalidateQueries({ queryKey: ["documents-trash"] });
    },
    onError: (err) => {
      const msg = err instanceof Error ? err.message : "文档移入回收站失败";
      messageApi.error(msg);
    },
  });

  const restoreDocumentMutation = useMutation<Document, Error, number>({
    mutationFn: async (docId) => restoreDocument(docId),
    onSuccess: async () => {
      messageApi.success("文档已恢复");
      await queryClient.invalidateQueries({ queryKey: ["documents-trash"] });
      await queryClient.invalidateQueries({ queryKey: ["node-documents"] });
      // 显式刷新回收站列表
      await documentTrashQuery.refetch();
    },
    onError: (err) => {
      const msg = err instanceof Error ? err.message : "恢复文档失败";
      messageApi.error(msg);
    },
  });

  const purgeDocumentMutation = useMutation<void, Error, number>({
    mutationFn: async (docId) => {
      await purgeDocument(docId);
    },
    onSuccess: async () => {
      messageApi.success("文档已彻底删除");
      await queryClient.invalidateQueries({ queryKey: ["documents-trash"] });
      await queryClient.invalidateQueries({ queryKey: ["node-documents"] });
      // 显式刷新回收站列表
      await documentTrashQuery.refetch();
    },
    onError: (err) => {
      const msg = err instanceof Error ? err.message : "彻底删除失败";
      messageApi.error(msg);
    },
  });

  const documentReorderMutation = useMutation<Document[], Error, DocumentReorderPayload>({
    mutationFn: async (payload: DocumentReorderPayload) => {
      return reorderDocuments(payload);
    },
    onSuccess: async () => {
      messageApi.success("文档排序调整成功");
      if (selectedNodeId != null) {
        await queryClient.invalidateQueries({ queryKey: ["node-documents", selectedNodeId] });
      }
    },
    onError: (err: Error) => {
      const msg = err instanceof Error ? err.message : "排序调整失败";
      messageApi.error(msg);
    },
  });

  const restoreDocumentVersionMutation = useMutation<
    Document,
    Error,
    { docId: number; version: number }
  >({
    mutationFn: ({ docId, version }) => restoreDocumentVersion(docId, version),
    onSuccess: async (_doc, variables) => {
      messageApi.success(`已恢复至版本 v${variables.version}`);
      await queryClient.invalidateQueries({ queryKey: ["document-history", variables.docId] });
      await queryClient.invalidateQueries({ queryKey: ["node-documents"] });
    },
    onError: (err) => {
      const msg = err instanceof Error ? err.message : "版本回退失败";
      messageApi.error(msg);
    },
  });

  // 重置筛选器（当选中节点变化时）
  useEffect(() => {
    documentFilterForm.resetFields();
    setDocumentFilters({});
  }, [selectedNodeId, documentFilterForm]);

  const handleDocumentSearch = useCallback(
    (values: DocumentFilterFormValues) => {
      const trimmed: DocumentFilterFormValues = {
        ...values,
        docId: values.docId?.trim() || undefined,
        query: values.query?.trim() || undefined,
      };
      const sanitizedTags =
        values.tags
          ?.map((tag) => tag.trim())
          .filter((tag) => tag.length > 0) ?? [];
      if (sanitizedTags.length > 0) {
        trimmed.tags = sanitizedTags;
      } else {
        delete trimmed.tags;
      }
      const sanitizedMetadata = sanitizeMetadataFilters(values.metadataFilters);
      if (sanitizedMetadata) {
        trimmed.metadataFilters = sanitizedMetadata;
      } else {
        delete trimmed.metadataFilters;
      }
      setDocumentFilters(trimmed);
    },
    [],
  );

  const handleDocumentReset = useCallback(() => {
    documentFilterForm.resetFields();
    setDocumentFilters({});
  }, [documentFilterForm]);

  const handleIncludeDescendantsChange = useCallback((value: boolean) => {
    setIncludeDescendants(value);
    // Reset to page 1 when changing descendants filter
    setDocumentListParams((prev) => ({ ...prev, page: 1 }));
  }, []);

  const handleDocumentListPageChange = useCallback((page: number, pageSize?: number) => {
    setDocumentListParams((prev) => ({
      ...prev,
      page,
      size: pageSize ?? prev.size,
    }));
  }, []);

  const handleDocumentTrashSearch = useCallback((query?: string) => {
    setDocumentTrashParams((prev) => ({
      ...prev,
      page: 1,
      query,
    }));
  }, []);

  const handleDocumentTrashPageChange = useCallback((page: number, pageSize?: number) => {
    setDocumentTrashParams((prev) => ({
      ...prev,
      page,
      size: pageSize ?? prev.size,
    }));
  }, []);

  const handleDocumentHistoryPageChange = useCallback((page: number, pageSize?: number) => {
    setDocumentHistoryParams((prev) => ({
      ...prev,
      page,
      size: pageSize ?? prev.size,
    }));
  }, []);

  useEffect(() => {
    if (documentHistoryDocId == null) {
      return;
    }
    setDocumentHistoryParams((prev) => ({ ...prev, page: 1 }));
  }, [documentHistoryDocId]);

  const handleRefreshDocumentTrash = useCallback(() => {
    void documentTrashQuery.refetch();
  }, [documentTrashQuery]);

  const documentsPage = selectedNodeId == null
    ? { page: 1, size: 10, total: 0, items: [] }
    : documentsQuery.data ?? { page: 1, size: 10, total: 0, items: [] };

  // 过滤掉源文档，只保留产出文档
  const documents = useMemo(() => {
    const sourceDocIds = new Set(
      (sourceDocsQuery.data ?? []).map(s => s.document_id)
    );
    return documentsPage.items.filter(doc => !sourceDocIds.has(doc.id));
  }, [documentsPage.items, sourceDocsQuery.data]);
  const deletingDocId = deleteDocumentMutation.isPending
    ? deleteDocumentMutation.variables ?? null
    : null;
  const restoringDocId = restoreDocumentMutation.isPending
    ? restoreDocumentMutation.variables ?? null
    : null;
  const purgingDocId = purgeDocumentMutation.isPending
    ? purgeDocumentMutation.variables ?? null
    : null;
  const restoringVersionNumber = restoreDocumentVersionMutation.isPending
    ? restoreDocumentVersionMutation.variables?.version ?? null
    : null;

  const value: DocumentContextValue = {
    documents,
    isDocumentsLoading: documentsQuery.isLoading,
    isDocumentsFetching: documentsQuery.isFetching,
    documentsError: documentsQuery.error,
    documentListPage: documentsPage.page,
    documentListSize: documentsPage.size,
    documentListTotal: documentsPage.total,
    documentFilters,
    includeDescendants,
    documentFilterForm,
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
  };

  return <DocumentContext.Provider value={value}>{children}</DocumentContext.Provider>;
};

// Helper functions
interface SanitizedMetadataFilter extends MetadataFilterFormValue {
  key: string;
  operator: MetadataOperator;
  values: string[];
}

function sanitizeMetadataFilters(
  filters?: MetadataFilterFormValue[],
): SanitizedMetadataFilter[] | undefined {
  if (!filters || filters.length === 0) {
    return undefined;
  }
  const sanitized: SanitizedMetadataFilter[] = [];
  filters.forEach((filter) => {
    const key = filter.key?.trim();
    if (!key) {
      return;
    }
    const operator: MetadataOperator = filter.operator ?? "eq";
    const existingValues = Array.isArray((filter as SanitizedMetadataFilter).values)
      ? (filter as SanitizedMetadataFilter).values
      : undefined;
    switch (operator) {
      case "eq":
      case "like": {
        let value = typeof filter.value === "string" ? filter.value.trim() : "";
        if (!value && existingValues && existingValues.length > 0) {
          value = existingValues[0]?.trim() ?? "";
        }
        if (value) {
          sanitized.push({ key, operator, values: [value] });
        }
        break;
      }
      case "in":
      case "any":
      case "all": {
        const rawSource = Array.isArray(filter.value) ? filter.value : existingValues ?? [];
        const raw = rawSource.map((item) =>
          typeof item === "string" ? item : String(item ?? ""),
        );
        const values = Array.from(
          new Set(raw.map((item) => item.trim()).filter((item) => item.length > 0)),
        );
        if (values.length > 0) {
          sanitized.push({ key, operator, values });
        }
        break;
      }
      case "gt":
      case "gte":
      case "lt":
      case "lte": {
        let value = typeof filter.value === "string" ? filter.value.trim() : "";
        if (!value && existingValues && existingValues.length > 0) {
          value = existingValues[0]?.trim() ?? "";
        }
        if (value && !Number.isNaN(Number(value))) {
          sanitized.push({ key, operator, values: [value] });
        }
        break;
      }
      default:
        break;
    }
  });
  return sanitized.length > 0 ? sanitized : undefined;
}

function buildTagClauses(tags?: string[]): MetadataFilterClause[] | undefined {
  if (!tags || tags.length === 0) {
    return undefined;
  }
  const normalized = Array.from(
    new Set(tags.map((tag) => tag.trim()).filter((tag) => tag.length > 0)),
  );
  if (normalized.length === 0) {
    return undefined;
  }
  if (normalized.length === 1) {
    return [{ key: "tags", operator: "any", values: normalized }];
  }
  return [{ key: "tags", operator: "all", values: normalized }];
}
