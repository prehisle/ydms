import { createContext, useContext, useState, useCallback, ReactNode } from "react";

interface UIContextValue {
  // 分类相关弹窗
  trashModalOpen: boolean;
  showCreateModal: { open: boolean; parentId: number | null };
  showRenameModal: boolean;

  // 文档编辑器抽屉
  documentEditorState: {
    open: boolean;
    docId: number | null;
    nodeId: number | null;
    mode: "create" | "edit";
  };

  // 文档回收站抽屉
  documentTrashOpen: boolean;

  // 文档历史抽屉
  documentHistoryState: {
    open: boolean;
    docId: number | null;
    title?: string;
    docType?: string;
  };

  // 文档重排序弹窗
  reorderModal: boolean;

  // 用户相关弹窗
  changePasswordOpen: boolean;
  userManagementOpen: boolean;
  apiKeyManagementOpen: boolean;

  // 任务中心抽屉
  taskCenterOpen: boolean;

  // Actions - Category
  handleOpenTrash: () => void;
  handleCloseTrash: () => void;
  handleOpenCreateModal: (parentId: number | null) => void;
  handleCloseCreateModal: () => void;
  handleOpenRenameModal: () => void;
  handleCloseRenameModal: () => void;

  // Actions - Document Editor
  handleOpenDocumentEditor: (params: {
    mode: "create" | "edit";
    docId?: number;
    nodeId?: number;
  }) => void;
  handleCloseDocumentEditor: () => void;

  // Actions - Document Trash
  handleOpenDocumentTrash: () => void;
  handleCloseDocumentTrash: () => void;

  // Actions - Document History
  handleOpenDocumentHistory: (docId: number, title?: string, docType?: string) => void;
  handleCloseDocumentHistory: () => void;

  // Actions - Reorder Modal
  handleOpenReorderModal: () => void;
  handleCloseReorderModal: () => void;

  // Actions - User Modals
  handleOpenChangePassword: () => void;
  handleCloseChangePassword: () => void;
  handleOpenUserManagement: () => void;
  handleCloseUserManagement: () => void;
  handleOpenAPIKeyManagement: () => void;
  handleCloseAPIKeyManagement: () => void;

  // Actions - Task Center
  handleOpenTaskCenter: () => void;
  handleCloseTaskCenter: () => void;
}

const UIContext = createContext<UIContextValue | undefined>(undefined);

export const useUIContext = () => {
  const context = useContext(UIContext);
  if (!context) {
    throw new Error("useUIContext must be used within UIProvider");
  }
  return context;
};

interface UIProviderProps {
  children: ReactNode;
}

export const UIProvider = ({ children }: UIProviderProps) => {
  // 分类相关弹窗
  const [trashModalOpen, setTrashModalOpen] = useState(false);
  const [showCreateModal, setShowCreateModal] = useState<{
    open: boolean;
    parentId: number | null;
  }>({ open: false, parentId: null });
  const [showRenameModal, setShowRenameModal] = useState(false);

  // 文档编辑器抽屉
  const [documentEditorState, setDocumentEditorState] = useState<{
    open: boolean;
    docId: number | null;
    nodeId: number | null;
    mode: "create" | "edit";
  }>({ open: false, docId: null, nodeId: null, mode: "edit" });

  // 文档回收站抽屉
  const [documentTrashOpen, setDocumentTrashOpen] = useState(false);

  // 文档历史抽屉
  const [documentHistoryState, setDocumentHistoryState] = useState<{
    open: boolean;
    docId: number | null;
    title?: string;
    docType?: string;
  }>({ open: false, docId: null, title: undefined, docType: undefined });

  // 文档重排序弹窗
  const [reorderModal, setReorderModal] = useState(false);

  // 用户相关弹窗
  const [changePasswordOpen, setChangePasswordOpen] = useState(false);
  const [userManagementOpen, setUserManagementOpen] = useState(false);
  const [apiKeyManagementOpen, setAPIKeyManagementOpen] = useState(false);

  // 任务中心抽屉
  const [taskCenterOpen, setTaskCenterOpen] = useState(false);

  // Category Actions
  const handleOpenTrash = useCallback(() => {
    setTrashModalOpen(true);
  }, []);

  const handleCloseTrash = useCallback(() => {
    setTrashModalOpen(false);
  }, []);

  const handleOpenCreateModal = useCallback((parentId: number | null) => {
    setShowCreateModal({ open: true, parentId });
  }, []);

  const handleCloseCreateModal = useCallback(() => {
    setShowCreateModal({ open: false, parentId: null });
  }, []);

  const handleOpenRenameModal = useCallback(() => {
    setShowRenameModal(true);
  }, []);

  const handleCloseRenameModal = useCallback(() => {
    setShowRenameModal(false);
  }, []);

  // Document Editor Actions
  const handleOpenDocumentEditor = useCallback(
    (params: { mode: "create" | "edit"; docId?: number; nodeId?: number }) => {
      setDocumentEditorState({
        open: true,
        docId: params.docId ?? null,
        nodeId: params.nodeId ?? null,
        mode: params.mode,
      });
    },
    [],
  );

  const handleCloseDocumentEditor = useCallback(() => {
    setDocumentEditorState({ open: false, docId: null, nodeId: null, mode: "edit" });
  }, []);

  // Document Trash Actions
  const handleOpenDocumentTrash = useCallback(() => {
    setDocumentTrashOpen(true);
  }, []);

  const handleCloseDocumentTrash = useCallback(() => {
    setDocumentTrashOpen(false);
  }, []);

  // Document History Actions
  const handleOpenDocumentHistory = useCallback((docId: number, title?: string, docType?: string) => {
    setDocumentHistoryState({ open: true, docId, title, docType });
  }, []);

  const handleCloseDocumentHistory = useCallback(() => {
    setDocumentHistoryState({ open: false, docId: null, title: undefined, docType: undefined });
  }, []);

  // Reorder Modal Actions
  const handleOpenReorderModal = useCallback(() => {
    setReorderModal(true);
  }, []);

  const handleCloseReorderModal = useCallback(() => {
    setReorderModal(false);
  }, []);

  // User Modal Actions
  const handleOpenChangePassword = useCallback(() => {
    setChangePasswordOpen(true);
  }, []);

  const handleCloseChangePassword = useCallback(() => {
    setChangePasswordOpen(false);
  }, []);

  const handleOpenUserManagement = useCallback(() => {
    setUserManagementOpen(true);
  }, []);

  const handleCloseUserManagement = useCallback(() => {
    setUserManagementOpen(false);
  }, []);

  const handleOpenAPIKeyManagement = useCallback(() => {
    setAPIKeyManagementOpen(true);
  }, []);

  const handleCloseAPIKeyManagement = useCallback(() => {
    setAPIKeyManagementOpen(false);
  }, []);

  // Task Center Actions
  const handleOpenTaskCenter = useCallback(() => {
    setTaskCenterOpen(true);
  }, []);

  const handleCloseTaskCenter = useCallback(() => {
    setTaskCenterOpen(false);
  }, []);

  const value: UIContextValue = {
    trashModalOpen,
    showCreateModal,
    showRenameModal,
    documentEditorState,
    documentTrashOpen,
    documentHistoryState,
    reorderModal,
    changePasswordOpen,
    userManagementOpen,
    apiKeyManagementOpen,
    taskCenterOpen,
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
    handleOpenChangePassword,
    handleCloseChangePassword,
    handleOpenUserManagement,
    handleCloseUserManagement,
    handleOpenAPIKeyManagement,
    handleCloseAPIKeyManagement,
    handleOpenTaskCenter,
    handleCloseTaskCenter,
  };

  return <UIContext.Provider value={value}>{children}</UIContext.Provider>;
};
