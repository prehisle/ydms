import { type FC, type CSSProperties, useState, useEffect, useCallback, useMemo, useRef } from "react";
import { useNavigate, useSearchParams, useParams } from "react-router-dom";
import Editor, { type OnMount } from "@monaco-editor/react";
import type { editor } from "monaco-editor";
import {
  Button,
  Input,
  InputNumber,
  Select,
  Space,
  message,
  Spin,
  Typography,
  Layout,
  Result,
  Progress,
  Tag,
  Popover,
  Tooltip,
} from "antd";
import {
  SaveOutlined,
  CloseOutlined,
  FullscreenOutlined,
  FullscreenExitOutlined,
  PlusOutlined,
  MinusCircleOutlined,
  LinkOutlined,
  CopyOutlined,
  FolderOutlined,
} from "@ant-design/icons";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { useAuth } from "../../../contexts/AuthContext";
import { useCategoryContext } from "../../../contexts/CategoryContext";
import {
  DOCUMENT_TYPES,
  DOCUMENT_TYPE_MAP,
  DOCUMENT_TYPE_KEYS,
} from "../constants";
import { DOCUMENT_TYPE_THEMES } from "../../../generated/documentTypeThemes";
import overviewStylesCss from "../typePlugins/overviewStyles.css?inline";
import { parseFrontMatterHtml } from "../typePlugins/shared";
import { getDocumentTemplate } from "../templates";
import { HTMLPreview } from "./HTMLPreview";
import { YAMLPreview } from "./YAMLPreview";
import { useDocumentTagCache } from "../hooks/useDocumentTagCache";
import {
  bindDocument,
  createDocument,
  getDocumentDetail,
  getDocumentBindings,
  updateDocument,
  type Document,
  type DocumentCreatePayload,
  type DocumentUpdatePayload,
  type DocumentBinding,
} from "../../../api/documents";
import type { MetadataValueType } from "../types";
import { DocumentReferenceModal } from "./DocumentReferenceModal";
import { AIProcessingButton } from "./AIProcessingButton";
import { SyncButton } from "./SyncButton";
import {
  useFileUpload,
  formatUploadLink,
} from "../../../hooks/useFileUpload";

const { Header, Content } = Layout;
const { Title, Text } = Typography;

function getDocumentTypeDefinition(type: string) {
  return DOCUMENT_TYPE_MAP[type as keyof typeof DOCUMENT_TYPE_MAP];
}

function getDocumentContentFormat(type: string): "html" | "yaml" {
  const definition = getDocumentTypeDefinition(type);
  return definition?.contentFormat === "html" ? "html" : "yaml";
}

interface DocumentEditorProps {
  mode?: "create" | "edit";
  docId?: number;
  nodeId?: number;
  onClose?: () => void;
}

/** 编辑/预览区域布局模式 */
type PaneMode = "split" | "editor" | "preview";

/** HTML 预览样式类型 */
type HtmlPreviewStyle = "default" | "yjxt" | `theme:${string}`;

interface MetadataEntry {
  id: string;
  key: string;
  type: MetadataValueType;
  value: string | string[];
}

function generateMetadataEntryId() {
  return `meta-${Math.random().toString(36).slice(2, 10)}-${Date.now().toString(36)}`;
}

function createEmptyEntry(): MetadataEntry {
  return {
    id: generateMetadataEntryId(),
    key: "",
    type: "string",
    value: "",
  };
}

function buildMetadataEntriesFromObject(source: Record<string, unknown>): MetadataEntry[] {
  return Object.entries(source).map(([key, value]) => {
    if (typeof value === "boolean") {
      return {
        id: generateMetadataEntryId(),
        key,
        type: "boolean",
        value: value ? "true" : "false",
      };
    }
    if (typeof value === "number") {
      return {
        id: generateMetadataEntryId(),
        key,
        type: "number",
        value: value.toString(),
      };
    }
    if (Array.isArray(value) && value.every((item) => typeof item === "string")) {
      return {
        id: generateMetadataEntryId(),
        key,
        type: "string[]",
        value: value as string[],
      };
    }
    return {
      id: generateMetadataEntryId(),
      key,
      type: "string",
      value: typeof value === "string" ? value : JSON.stringify(value),
    };
  });
}

function getDefaultValueForType(
  type: MetadataValueType,
  previous?: string | string[],
): string | string[] {
  switch (type) {
    case "number": {
      if (typeof previous === "string" && previous.trim() && !Number.isNaN(Number(previous))) {
        return previous;
      }
      return "";
    }
    case "boolean": {
      if (previous === "true" || previous === "false") {
        return previous;
      }
      return "true";
    }
    case "string[]": {
      if (Array.isArray(previous)) {
        return previous;
      }
      if (typeof previous === "string" && previous.trim().length > 0) {
        return [previous.trim()];
      }
      return [];
    }
    case "string":
    default: {
      if (typeof previous === "string") {
        return previous;
      }
      return "";
    }
  }
}

function extractDocumentContent(content?: Record<string, unknown> | null): string {
  if (!content) {
    return "";
  }
  const maybeData = (content as { data?: unknown }).data;
  if (typeof maybeData === "string") {
    return maybeData;
  }
  const maybePreview = (content as { preview?: unknown }).preview;
  if (typeof maybePreview === "string") {
    return maybePreview;
  }
  return "";
}

export const DocumentEditor: FC<DocumentEditorProps> = ({ mode, docId: docIdProp, nodeId: nodeIdProp, onClose }) => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { docId: docIdParam } = useParams<{ docId: string }>();
  const queryClient = useQueryClient();
  const { user } = useAuth();
  const { lookups } = useCategoryContext();
  const { getTags, upsert } = useDocumentTagCache(user?.id ?? null);

  const parsedDocIdFromRoute = docIdParam ? Number.parseInt(docIdParam, 10) : undefined;
  const effectiveDocId = docIdProp ?? parsedDocIdFromRoute;
  const resolvedMode: "create" | "edit" = mode ?? (effectiveDocId ? "edit" : "create");
  const isEditMode = resolvedMode === "edit";

  const nodeIdQuery = searchParams.get("nodeId");
  const parsedNodeIdFromQuery = nodeIdQuery ? Number.parseInt(nodeIdQuery, 10) : undefined;
  const effectiveNodeId = nodeIdProp ?? parsedNodeIdFromQuery;

  const defaultDocumentType = DOCUMENT_TYPE_KEYS[0] ?? "overview";
  const [title, setTitle] = useState("");
  const [documentType, setDocumentType] = useState<string>(defaultDocumentType);
  const [position, setPosition] = useState<number | undefined>();
  const [content, setContent] = useState("");
  const [metadataDifficulty, setMetadataDifficulty] = useState<number | null>(null);
  const [metadataTags, setMetadataTags] = useState<string[]>([]);
  const [metadataEntries, setMetadataEntries] = useState<MetadataEntry[]>([]);
  const [pendingReferences, setPendingReferences] = useState<Array<{ document_id: number; title: string }>>([]);
  const [referenceModalOpen, setReferenceModalOpen] = useState(false);

  // 编辑/预览区域布局模式
  const [paneMode, setPaneMode] = useState<PaneMode>("split");
  // HTML 预览样式
  const [htmlPreviewStyle, setHtmlPreviewStyle] = useState<HtmlPreviewStyle>("default");

  // 文件上传 hook
  const { uploadFile, uploading, progress } = useFileUpload();
  // Monaco 编辑器实例引用
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null);

  const cachedTagOptions = useMemo(
    () => getTags(documentType).map((tag) => ({ label: tag, value: tag })),
    [documentType, getTags],
  );

  const closeEditor = useCallback(() => {
    if (onClose) {
      onClose();
      return;
    }
    navigate(-1);
  }, [navigate, onClose]);

  const {
    data: existingDoc,
    isLoading: isLoadingDoc,
    error: loadError,
  } = useQuery<Document | null>({
    queryKey: ["document-detail", effectiveDocId],
    queryFn: async () => {
      if (!effectiveDocId) {
        return null;
      }
	return getDocumentDetail(effectiveDocId);
    },
    enabled: isEditMode && typeof effectiveDocId === "number",
  });

  // 获取文档绑定的节点路径信息
  const { data: documentBindings, isLoading: isLoadingBindings } = useQuery<DocumentBinding[]>({
    queryKey: ["document-bindings", effectiveDocId],
    queryFn: async () => {
      if (!effectiveDocId) {
        return [];
      }
      return getDocumentBindings(effectiveDocId);
    },
    enabled: isEditMode && typeof effectiveDocId === "number",
  });

  useEffect(() => {
    if (!isEditMode || !existingDoc) {
      return;
    }
    setTitle(existingDoc.title ?? "");
    const nextType =
      existingDoc.type && getDocumentTypeDefinition(existingDoc.type)
        ? existingDoc.type
        : defaultDocumentType;
    setDocumentType(nextType);
    setPosition(existingDoc.position);
    setContent(extractDocumentContent(existingDoc.content));
    const metadata = (existingDoc.metadata ?? {}) as Record<string, unknown>;
    const difficultyValue = metadata.difficulty;
    setMetadataDifficulty(typeof difficultyValue === "number" ? difficultyValue : null);
    const tagsValue = metadata.tags;
    if (Array.isArray(tagsValue)) {
      setMetadataTags(tagsValue.map((tag) => String(tag)).filter((tag) => tag.trim().length > 0));
    } else {
      setMetadataTags([]);
    }
    const restMetadata = { ...metadata };
    delete restMetadata.difficulty;
    delete restMetadata.tags;
    setMetadataEntries(buildMetadataEntriesFromObject(restMetadata));
  }, [isEditMode, existingDoc, defaultDocumentType]);

  useEffect(() => {
    if (isEditMode) {
      return;
    }
    const template = getDocumentTemplate(documentType);
    if (template) {
      setContent(template.data);
    } else {
      setContent("");
    }
  }, [isEditMode, documentType]);

  useEffect(() => {
    if (!isEditMode) {
      setMetadataDifficulty(null);
      setMetadataTags([]);
      setMetadataEntries([]);
    }
  }, [isEditMode]);

  useEffect(() => {
    if (!isEditMode || !loadError) {
      return;
    }
    const errorMessage = loadError instanceof Error ? loadError.message : "文档加载失败";
    message.error(errorMessage);
    closeEditor();
  }, [isEditMode, loadError, closeEditor]);

  useEffect(() => {
    if (metadataTags.length === 0) {
      return;
    }
    upsert(documentType, metadataTags);
  }, [documentType, metadataTags, upsert]);

  const editorLanguage = useMemo(() => {
    return getDocumentContentFormat(documentType);
  }, [documentType]);

  /** 当前文档是否为 HTML 格式 */
  const isHtmlDocument = editorLanguage === "html";

  /** HTML 预览内容（剥离 YAML front-matter） */
  const htmlPreviewContent = useMemo(() => {
    if (!isHtmlDocument) {
      return content;
    }
    return parseFrontMatterHtml(content).body;
  }, [content, isHtmlDocument]);

  /** HTML 预览样式选项 */
  const htmlPreviewStyleOptions = useMemo(() => {
    const themeOptions = DOCUMENT_TYPE_THEMES.knowledge_overview_v1.map((theme) => ({
      value: `theme:${theme.id}`,
      label: theme.label,
      title: "description" in theme ? theme.description : undefined,
    }));

    return [
      { value: "default", label: "默认（内置）" },
      { value: "yjxt", label: "YJXT 基础" },
      { label: "知识点概览主题", options: themeOptions },
    ];
  }, []);

  /** 当前选中的主题（仅当样式以 theme: 开头时有效） */
  const selectedOverviewTheme = useMemo(() => {
    if (!htmlPreviewStyle.startsWith("theme:")) {
      return undefined;
    }
    const themeId = htmlPreviewStyle.slice("theme:".length);
    const theme = DOCUMENT_TYPE_THEMES.knowledge_overview_v1.find((item) => item.id === themeId);
    if (!theme) {
      return undefined;
    }
    return { ...theme, className: `overview-theme-${theme.id}` };
  }, [htmlPreviewStyle]);

  /** HTML 预览注入的 CSS 文本 */
  const htmlPreviewInjectedCss = useMemo(() => {
    if (!isHtmlDocument) {
      return undefined;
    }
    if (htmlPreviewStyle === "yjxt") {
      return overviewStylesCss;
    }
    if (selectedOverviewTheme?.css) {
      return `${overviewStylesCss}\n${selectedOverviewTheme.css}`;
    }
    return undefined;
  }, [isHtmlDocument, htmlPreviewStyle, selectedOverviewTheme]);

  /** HTML 预览 wrapper className */
  const htmlPreviewWrapperClassName = useMemo(() => {
    if (!isHtmlDocument) {
      return undefined;
    }
    if (selectedOverviewTheme) {
      return ["overview-theme-wrapper", selectedOverviewTheme.className].filter(Boolean).join(" ");
    }
    return undefined;
  }, [isHtmlDocument, selectedOverviewTheme]);

  /** HTML 预览 content className */
  const htmlPreviewContentClassName = useMemo(() => {
    if (!isHtmlDocument) {
      return undefined;
    }
    if (htmlPreviewStyle === "yjxt") {
      return "yjxt-main-content";
    }
    return undefined;
  }, [isHtmlDocument, htmlPreviewStyle]);

  // 根据节点 ID 构建完整的人类可读路径（从分类树递归查找）
  const buildReadablePath = useCallback((nodeId: number): string => {
    const names: string[] = [];
    let currentId: number | null = nodeId;

    while (currentId !== null) {
      const category = lookups.byId.get(currentId);
      if (!category) break;
      names.unshift(category.name);
      currentId = category.parent_id;
    }

    return names.length > 0 ? names.join(" / ") : "";
  }, [lookups.byId]);

  // 格式化节点路径用于显示
  const formattedBindings = useMemo(() => {
    if (!documentBindings || documentBindings.length === 0) {
      return [];
    }
    return documentBindings
      .filter((binding) => binding.node_id)
      .sort((a, b) => a.node_path.localeCompare(b.node_path))
      .map((binding) => {
        // 优先使用分类树构建人类可读路径，回退到 node_name
        const readablePath = buildReadablePath(binding.node_id);
        return {
          nodeId: binding.node_id,
          displayPath: readablePath || binding.node_name || `节点 ${binding.node_id}`,
          rawPath: binding.node_path,
        };
      });
  }, [documentBindings, buildReadablePath]);

  // 节点路径显示组件
  const bindingsDisplay = useMemo(() => {
    if (!isEditMode) {
      return null;
    }
    if (isLoadingBindings) {
      return (
        <Space size={4} align="center">
          <Spin size="small" />
          <Text type="secondary" style={{ fontSize: 12 }}>
            加载路径...
          </Text>
        </Space>
      );
    }
    if (formattedBindings.length === 0) {
      return null;
    }

    const firstBinding = formattedBindings[0];
    const restCount = formattedBindings.length - 1;

    // 完整路径内容（用于 Popover 展示）
    const fullPathContent = (
      <div style={{ maxWidth: 500 }}>
        {formattedBindings.map((binding, index) => (
          <div
            key={binding.nodeId}
            style={{
              padding: "8px 0",
              borderBottom: index < formattedBindings.length - 1 ? "1px solid #f0f0f0" : "none",
            }}
          >
            <Space>
              <FolderOutlined style={{ color: "#1890ff" }} />
              <span style={{ wordBreak: "break-all" }}>{binding.displayPath}</span>
            </Space>
          </div>
        ))}
      </div>
    );

    return (
      <Popover
        content={fullPathContent}
        title={`所属目录${restCount > 0 ? ` (共 ${formattedBindings.length} 个)` : ""}`}
        placement="bottomLeft"
        trigger="hover"
      >
        <Tag
          style={{
            fontSize: 12,
            color: "#595959",
            background: "#fafafa",
            borderColor: "#d9d9d9",
            cursor: "pointer",
            maxWidth: 300,
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          }}
        >
          <FolderOutlined style={{ marginRight: 4 }} />
          {firstBinding.displayPath}
          {restCount > 0 && <span style={{ marginLeft: 4, color: "#1890ff" }}>+{restCount}</span>}
        </Tag>
      </Popover>
    );
  }, [isEditMode, isLoadingBindings, formattedBindings]);

  const handleTypeChange = useCallback((value: string) => {
    setDocumentType(value);
  }, []);

  const buildMetadataPayload = useCallback((): Record<string, any> => {
    const payload: Record<string, any> = {};

    // 处理引用字段
    if (isEditMode && existingDoc?.metadata?.references) {
      // 编辑模式：保留原文档中的 references 字段（由 DocumentReferenceManager 管理）
      payload.references = existingDoc.metadata.references;
    } else if (!isEditMode && pendingReferences.length > 0) {
      // 新建模式：使用待添加的引用
      payload.references = pendingReferences.map(ref => ({
        document_id: ref.document_id,
        title: ref.title,
        added_at: new Date().toISOString(),
      }));
    }

    if (metadataDifficulty != null) {
      payload.difficulty = metadataDifficulty;
    }
    if (metadataTags.length > 0) {
      const trimmedTags = Array.from(
        new Set(metadataTags.map((tag) => tag.trim()).filter((tag) => tag.length > 0)),
      );
      if (trimmedTags.length > 0) {
        payload.tags = trimmedTags;
      }
    }
    metadataEntries.forEach((entry) => {
      const key = entry.key.trim();
      if (!key || key === "difficulty" || key === "tags" || key === "references") {
        return;
      }
      switch (entry.type) {
        case "string": {
          const value = (entry.value as string).trim();
          if (value.length > 0) {
            payload[key] = value;
          }
          break;
        }
        case "number": {
          const raw = (entry.value as string).trim();
          if (!raw) {
            break;
          }
          const numeric = Number(raw);
          if (Number.isNaN(numeric)) {
            throw new Error(`元数据字段"${key}"的值必须是数字`);
          }
          payload[key] = numeric;
          break;
        }
        case "boolean": {
          const boolValue = entry.value as string;
          if (boolValue === "true" || boolValue === "false") {
            payload[key] = boolValue === "true";
          } else {
            throw new Error(`元数据字段"${key}"的布尔值只能为 true 或 false`);
          }
          break;
        }
        case "string[]": {
          const items = Array.isArray(entry.value) ? entry.value : [];
          const normalized = Array.from(
            new Set(items.map((item) => item.trim()).filter((item) => item.length > 0)),
          );
          if (normalized.length > 0) {
            payload[key] = normalized;
          }
          break;
        }
        default:
          break;
      }
    });
    return payload;
  }, [metadataDifficulty, metadataEntries, metadataTags, isEditMode, existingDoc, pendingReferences]);

  const renderMetadataValueControl = useCallback(
    (entry: MetadataEntry) => {
      switch (entry.type) {
        case "string":
          return (
            <Input
              placeholder="值"
              value={typeof entry.value === "string" ? entry.value : ""}
              onChange={(event) =>
                setMetadataEntries((prev) =>
                  prev.map((item) =>
                    item.id === entry.id ? { ...item, value: event.target.value } : item,
                  ),
                )
              }
              style={{ minWidth: 200 }}
            />
          );
        case "number":
          return (
            <Input
              placeholder="数值"
              value={typeof entry.value === "string" ? entry.value : ""}
              onChange={(event) =>
                setMetadataEntries((prev) =>
                  prev.map((item) =>
                    item.id === entry.id ? { ...item, value: event.target.value } : item,
                  ),
                )
              }
              style={{ minWidth: 160 }}
              inputMode="decimal"
            />
          );
        case "boolean":
          return (
            <Select
              value={entry.value as string}
              options={[
                { value: "true", label: "true" },
                { value: "false", label: "false" },
              ]}
              onChange={(value: string) =>
                setMetadataEntries((prev) =>
                  prev.map((item) => (item.id === entry.id ? { ...item, value } : item)),
                )
              }
              style={{ width: 120 }}
            />
          );
        case "string[]":
          return (
            <Select
              mode="tags"
              allowClear
              placeholder="输入多个值"
              value={Array.isArray(entry.value) ? entry.value : []}
              onChange={(values: string[]) =>
                setMetadataEntries((prev) =>
                  prev.map((item) =>
                    item.id === entry.id ? { ...item, value: values } : item,
                  ),
                )
              }
              style={{ minWidth: 220 }}
            />
          );
        default:
          return null;
      }
    },
    [setMetadataEntries],
  );

  const createMutation = useMutation({
    mutationFn: async () => {
      if (!title.trim()) {
        throw new Error("请输入文档标题");
      }
      if (effectiveNodeId == null) {
        throw new Error("缺少节点ID参数");
      }

      const template = getDocumentTemplate(documentType);
      const payload: DocumentCreatePayload = {
        title: title.trim(),
        type: documentType,
        position,
        content: template
          ? {
              format: template.format,
              data: content,
            }
          : undefined,
      };
      const metadataPayload = buildMetadataPayload();
      if (Object.keys(metadataPayload).length > 0) {
        payload.metadata = metadataPayload;
      }

      const doc = await createDocument(payload);
      await bindDocument(effectiveNodeId, doc.id);
      return doc;
    },
    onSuccess: async (_doc) => {
      message.success("文档创建成功");
      if (effectiveNodeId != null) {
        await queryClient.invalidateQueries({ queryKey: ["node-documents", effectiveNodeId] });
      }
      upsert(documentType, metadataTags);
      closeEditor();
    },
    onError: (error: Error) => {
      message.error(error.message || "创建失败");
    },
  });

  const updateMutation = useMutation({
    mutationFn: async () => {
      if (!title.trim()) {
        throw new Error("请输入文档标题");
      }
      if (!effectiveDocId) {
        throw new Error("文档ID不存在");
      }

      const template = getDocumentTemplate(documentType);
      const payload: DocumentUpdatePayload = {
        title: title.trim(),
        type: documentType,
        position,
        content: template
          ? {
              format: template.format,
              data: content,
            }
          : undefined,
        metadata: buildMetadataPayload(),
      };

      return updateDocument(effectiveDocId, payload);
    },
    onSuccess: async () => {
      message.success("文档更新成功");
      if (effectiveDocId) {
        await queryClient.invalidateQueries({ queryKey: ["document-detail", effectiveDocId] });
      }
      if (effectiveNodeId != null) {
        await queryClient.invalidateQueries({ queryKey: ["node-documents", effectiveNodeId] });
      } else {
        await queryClient.invalidateQueries({ queryKey: ["node-documents"] });
      }
      upsert(documentType, metadataTags);
      // 保存后不关闭窗口，方便继续编辑或执行同步操作
    },
    onError: (error: Error) => {
      message.error(error.message || "更新失败");
    },
  });

  const handleSave = useCallback(() => {
    if (isEditMode) {
      updateMutation.mutate();
    } else {
      createMutation.mutate();
    }
  }, [isEditMode, createMutation, updateMutation]);

  const handleCancel = useCallback(() => {
    closeEditor();
  }, [closeEditor]);

  // 处理编辑器挂载
  const handleEditorMount: OnMount = useCallback((editorInstance) => {
    editorRef.current = editorInstance;
    console.log("[paste-debug] Editor mounted");
  }, []);

  // 使用 window 级别的捕获阶段监听粘贴事件
  // Monaco Editor 会在内部 textarea 上处理 paste 并 stopPropagation，
  // 所以必须在 capture 阶段拦截，并用 hasTextFocus() 判断焦点
  useEffect(() => {
    const handlePaste = async (e: ClipboardEvent) => {
      const editorInstance = editorRef.current;
      if (!editorInstance) return;

      // 只在编辑器有焦点时处理
      if (!editorInstance.hasTextFocus()) return;

      console.log("[paste-debug] Paste event triggered");
      const items = e.clipboardData?.items;
      console.log("[paste-debug] Clipboard items:", items?.length);
      if (!items) return;

      for (const item of Array.from(items)) {
        console.log("[paste-debug] Item:", item.kind, item.type);
        if (item.kind === "file") {
          const file = item.getAsFile();
          if (!file) continue;

          // 只有找到文件才阻止默认行为，不影响正常文本粘贴
          e.preventDefault();
          e.stopPropagation();

          const placeholder = `[上传中: ${file.name}...]`;
          const selection = editorInstance.getSelection();

          if (selection) {
            editorInstance.executeEdits("paste-upload", [{
              range: selection,
              text: placeholder,
              forceMoveMarkers: true,
            }]);
          }

          try {
            const result = await uploadFile(file);
            const model = editorInstance.getModel();
            if (model) {
              const currentContent = model.getValue();
              const newContent = currentContent.replace(placeholder, formatUploadLink(result));
              model.setValue(newContent);
              message.success(`文件 "${file.name}" 上传成功`);
            }
          } catch (err) {
            const model = editorInstance.getModel();
            if (model) {
              const currentContent = model.getValue();
              const errorText = `[上传失败: ${file.name}]`;
              model.setValue(currentContent.replace(placeholder, errorText));
            }
            const errorMessage = err instanceof Error ? err.message : "上传失败";
            message.error(errorMessage);
          }
          break;
        }
      }
    };

    // 在 capture 阶段监听，确保在 Monaco 处理之前捕获到事件
    window.addEventListener("paste", handlePaste as unknown as EventListener, true);

    return () => {
      window.removeEventListener("paste", handlePaste as unknown as EventListener, true);
    };
  }, [uploadFile]);

  // paneMode 变化时通知 Monaco 重新布局（仅在编辑器可见时）
  useEffect(() => {
    if (paneMode === "preview") {
      return; // 编辑器隐藏时不触发布局
    }
    const rafId = requestAnimationFrame(() => {
      editorRef.current?.layout();
    });
    return () => cancelAnimationFrame(rafId);
  }, [paneMode]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "s") {
        e.preventDefault();
        handleSave();
      }
      if (e.key === "Escape") {
        // 如果事件已被其他处理器（如 Modal、Select 等）处理，不再继续
        if (e.defaultPrevented) {
          return;
        }
        // 如果有弹窗打开，让弹窗先处理 Esc（Antd Modal 自带 Esc 关闭）
        if (referenceModalOpen) {
          return;
        }
        // 若处于最大化状态，先恢复 split 布局
        if (paneMode !== "split") {
          e.preventDefault();
          setPaneMode("split");
          return;
        }
        handleCancel();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [handleSave, handleCancel, paneMode, referenceModalOpen]);

  const isEmbedded = typeof onClose === "function";
  const containerHeight = isEmbedded ? "100%" : "100vh";
  const contentHeight = isEmbedded ? "calc(100% - 64px)" : "calc(100vh - 64px)";

  if (isEditMode && isLoadingDoc) {
    return (
      <div style={{ display: "flex", justifyContent: "center", alignItems: "center", height: containerHeight }}>
        <Space direction="vertical" align="center" size="middle">
          <Spin size="large" />
          <Typography.Text type="secondary">加载中...</Typography.Text>
        </Space>
      </div>
    );
  }

  if (isEditMode && !isLoadingDoc && !existingDoc && !loadError) {
    return (
      <Result
        status="warning"
        title="未找到文档"
        subTitle="文档可能已被删除或暂不可用"
        extra={
          <Button type="primary" onClick={handleCancel}>
            返回
          </Button>
        }
      />
    );
  }

  return (
    <Layout style={{ height: containerHeight, overflow: "hidden" }}>
      <Header
        style={{
          background: "#fff",
          borderBottom: "1px solid #f0f0f0",
          padding: "0 24px",
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          height: "64px",
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: "16px", flex: 1 }}>
          <Title level={5} style={{ margin: 0 }}>
            {isEditMode ? "编辑文档" : "新建文档"}
          </Title>
          {bindingsDisplay}

          <Input
            placeholder="请输入文档标题"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            style={{ width: "300px" }}
          />

          <Select
            value={documentType}
            onChange={handleTypeChange}
            options={DOCUMENT_TYPES}
            style={{ width: "160px" }}
            disabled={isEditMode}
          />

          <Select
            mode="tags"
            allowClear
            placeholder="标签"
            value={metadataTags}
            options={cachedTagOptions}
            onChange={(values) =>
              setMetadataTags(
                Array.from(new Set(values.map((val) => val.trim()).filter((val) => val.length > 0))),
              )
            }
            style={{ minWidth: "220px" }}
          />
        </div>

        <Space>
          {isEditMode && effectiveDocId && (
            <Button
              icon={<CopyOutlined />}
              onClick={() => {
                const docPath = `@doc:${effectiveDocId}`;
                navigator.clipboard.writeText(docPath).then(
                  () => message.success(`已复制文档路径: ${docPath}`),
                  () => message.error("复制失败")
                );
              }}
            >
              复制路径
            </Button>
          )}
          {isEditMode && effectiveDocId && (
            <AIProcessingButton
              documentId={effectiveDocId}
              documentTitle={title}
            />
          )}
          {isEditMode && effectiveDocId && (
            <SyncButton documentId={effectiveDocId} />
          )}
          <Button
            icon={<LinkOutlined />}
            onClick={() => setReferenceModalOpen(true)}
          >
            引用
            {(() => {
              const referenceCount = isEditMode
                ? (existingDoc?.metadata?.references as any[])?.length || 0
                : pendingReferences.length;
              return referenceCount > 0 ? ` (${referenceCount})` : "";
            })()}
          </Button>
          <Button
            type="primary"
            icon={<SaveOutlined />}
            onClick={handleSave}
            loading={createMutation.isPending || updateMutation.isPending}
          >
            保存 (Ctrl+S)
          </Button>
          <Button icon={<CloseOutlined />} onClick={handleCancel}>
            取消 (Esc)
          </Button>
          <Button icon={<FullscreenExitOutlined />} onClick={handleCancel} type="text" />
        </Space>
      </Header>

      <Content style={{ display: "flex", height: contentHeight }}>
        <div
          style={{
            flex: 1,
            minWidth: 0,
            borderRight: paneMode === "split" ? "1px solid #f0f0f0" : "none",
            display: paneMode === "preview" ? "none" : "flex",
            flexDirection: "column",
          }}
        >
          <div
            style={{
              padding: "8px 16px",
              background: "#fafafa",
              borderBottom: "1px solid #f0f0f0",
              fontWeight: 500,
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
            }}
          >
            <span>源码编辑</span>
            <Tooltip title={paneMode === "editor" ? "退出最大化" : "最大化编辑"}>
              <Button
                type="text"
                size="small"
                icon={paneMode === "editor" ? <FullscreenExitOutlined /> : <FullscreenOutlined />}
                onClick={() => setPaneMode((prev) => (prev === "editor" ? "split" : "editor"))}
              />
            </Tooltip>
          </div>
          <div
            style={{
              padding: "12px 16px",
              borderBottom: "1px solid #f0f0f0",
            }}
          >
            <Space direction="vertical" style={{ width: "100%" }} size="middle">
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                <Typography.Text strong>额外元数据</Typography.Text>
                <Button
                  type="dashed"
                  icon={<PlusOutlined />}
                  onClick={() => setMetadataEntries((prev) => [...prev, createEmptyEntry()])}
                >
                  添加字段
                </Button>
              </div>
              <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
                可为文档增加自定义键值对，支持字符串、数字、布尔值和字符串数组类型。
              </Typography.Paragraph>
              {metadataEntries.length === 0 ? (
                <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
                  尚未添加额外元数据。
                </Typography.Paragraph>
              ) : (
                <Space direction="vertical" size="middle" style={{ width: "100%" }}>
                  {metadataEntries.map((entry) => (
                    <Space
                      key={entry.id}
                      wrap
                      align="start"
                      style={{
                        width: "100%",
                        padding: "12px",
                        border: "1px solid #f0f0f0",
                        borderRadius: 6,
                        background: "#fafafa",
                      }}
                    >
                      <Input
                        placeholder="字段名"
                        value={entry.key}
                        onChange={(event) =>
                          setMetadataEntries((prev) =>
                            prev.map((item) =>
                              item.id === entry.id ? { ...item, key: event.target.value } : item,
                            ),
                          )
                        }
                        style={{ minWidth: 160 }}
                      />
                      <Select
                        value={entry.type}
                        options={[
                          { value: "string", label: "字符串" },
                          { value: "number", label: "数字" },
                          { value: "boolean", label: "布尔" },
                          { value: "string[]", label: "字符串数组" },
                        ]}
                        onChange={(value: MetadataValueType) =>
                          setMetadataEntries((prev) =>
                            prev.map((item) =>
                              item.id === entry.id
                                ? {
                                    ...item,
                                    type: value,
                                    value: getDefaultValueForType(value, item.value),
                                  }
                                : item,
                            ),
                          )
                        }
                        style={{ width: 140 }}
                      />
                      {renderMetadataValueControl(entry)}
                      <Button
                        icon={<MinusCircleOutlined />}
                        onClick={() =>
                          setMetadataEntries((prev) => prev.filter((item) => item.id !== entry.id))
                        }
                      />
                    </Space>
                  ))}
                </Space>
              )}
            </Space>
          </div>

          <div style={{ flex: 1, overflow: "hidden", position: "relative" }}>
            <Editor
              language={editorLanguage}
              value={content}
              onChange={(value) => setContent(value || "")}
              onMount={handleEditorMount}
              theme="vs-dark"
              options={{
                minimap: { enabled: true },
                fontSize: 14,
                lineNumbers: "on",
                wordWrap: "on",
                automaticLayout: true,
                scrollBeyondLastLine: false,
                tabSize: 2,
                insertSpaces: true,
              }}
            />
            {/* 上传进度指示器 */}
            {uploading && progress && (
              <div
                style={{
                  position: "absolute",
                  bottom: 20,
                  right: 20,
                  zIndex: 1000,
                  background: "rgba(0, 0, 0, 0.75)",
             borderRadius: 8,
                  padding: 12,
                }}
              >
                <Progress
                  type="circle"
                  percent={progress.percent}
                  size={60}
                  status="active"
                  strokeColor="#1890ff"
                />
              </div>
            )}
          </div>
        </div>

        <div
          style={{
            flex: 1,
            minWidth: 0,
            display: paneMode === "editor" ? "none" : "flex",
            flexDirection: "column",
            background: "#fff",
          }}
        >
          <div
            style={{
              padding: "8px 16px",
              background: "#fafafa",
              borderBottom: "1px solid #f0f0f0",
              fontWeight: 500,
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
            }}
          >
            <span>实时预览</span>
            <Space size={12} align="center">
              {isHtmlDocument && (
                <Space size={4} align="center">
                  <Typography.Text type="secondary">样式：</Typography.Text>
                  <Select
                    size="small"
                    value={htmlPreviewStyle}
                    options={htmlPreviewStyleOptions}
                    onChange={(value) => setHtmlPreviewStyle(value as HtmlPreviewStyle)}
                    style={{ minWidth: 160 }}
                    dropdownMatchSelectWidth={false}
                  />
                </Space>
              )}
              <Tooltip title={paneMode === "preview" ? "退出最大化" : "最大化预览"}>
                <Button
                  type="text"
                  size="small"
                  icon={paneMode === "preview" ? <FullscreenExitOutlined /> : <FullscreenOutlined />}
                  onClick={() => setPaneMode((prev) => (prev === "preview" ? "split" : "preview"))}
                />
              </Tooltip>
            </Space>
          </div>
          <div style={{ flex: 1, overflow: "auto" }}>
            {isHtmlDocument ? (
              <HTMLPreview
                content={htmlPreviewContent}
                className={htmlPreviewWrapperClassName}
                contentClassName={htmlPreviewContentClassName}
                styleCss={htmlPreviewInjectedCss}
              />
            ) : (
              <YAMLPreview content={content} documentType={documentType} />
            )}
          </div>
        </div>
      </Content>

      <DocumentReferenceModal
        open={referenceModalOpen}
        onCancel={() => setReferenceModalOpen(false)}
        document={isEditMode && existingDoc ? existingDoc : undefined}
        onDocumentUpdated={(updatedDoc) => {
          queryClient.setQueryData(["document-detail", effectiveDocId], updatedDoc);
        }}
        pendingReferences={isEditMode ? undefined : pendingReferences}
        onPendingAdd={(doc) => {
          setPendingReferences((prev) => [...prev, { document_id: doc.id, title: doc.title }]);
        }}
        onPendingRemove={(docId) => {
          setPendingReferences((prev) => prev.filter((ref) => ref.document_id !== docId));
        }}
      />
    </Layout>
  );
};
