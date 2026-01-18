import { Layout, message } from "antd";
import type { MessageInstance } from "antd/es/message/interface";
import { useCallback, type CSSProperties } from "react";
import { Outlet } from "react-router-dom";
import { CategoryProvider, useCategoryContext } from "../../contexts/CategoryContext";
import { DocumentProvider } from "../../contexts/DocumentContext";
import { UIProvider, useUIContext } from "../../contexts/UIContext";
import { ChangePasswordModal } from "../../features/auth";
import { usePersistentBoolean } from "../../hooks/usePersistentBoolean";
import { NavBar } from "./NavBar";

/**
 * Outlet context 类型定义
 * 子路由组件可以通过 useOutletContext 获取这些值
 */
export interface MainLayoutOutletContext {
  /** 顶部导航栏是否折叠 */
  headerCollapsed: boolean;
  /** 切换顶部导航栏折叠状态 */
  onToggleHeader: () => void;
}

const CONTENT_STYLE: CSSProperties = {
  flex: 1,
  minHeight: 0,
  overflow: "hidden",
};

/**
 * 主布局内容组件
 * 负责渲染导航栏和子路由内容
 */
interface MainLayoutContentProps {
  messageApi: MessageInstance;
  headerCollapsed: boolean;
  onToggleHeader: () => void;
}

function MainLayoutContent({
  messageApi,
  headerCollapsed,
  onToggleHeader,
}: MainLayoutContentProps): JSX.Element {
  const { selectedNodeId } = useCategoryContext();
  const { changePasswordOpen, handleCloseChangePassword } = useUIContext();

  return (
    <DocumentProvider messageApi={messageApi} selectedNodeId={selectedNodeId}>
      <Layout style={{ minHeight: "100vh", height: "100vh", overflow: "hidden" }}>
        <NavBar collapsed={headerCollapsed} />
        <Layout.Content style={CONTENT_STYLE}>
          <Outlet context={{ headerCollapsed, onToggleHeader } satisfies MainLayoutOutletContext} />
        </Layout.Content>
      </Layout>
      <ChangePasswordModal open={changePasswordOpen} onClose={handleCloseChangePassword} />
    </DocumentProvider>
  );
}

/**
 * 主布局组件
 * 提供全局状态 Providers 和布局结构
 *
 * 布局结构：
 * - UIProvider: 全局 UI 状态（弹窗、抽屉）
 * - CategoryProvider: 分类树状态
 * - DocumentProvider: 文档列表状态
 * - NavBar: 顶部导航栏
 * - Outlet: 子路由内容区域
 */
export function MainLayout(): JSX.Element {
  const [messageApi, contextHolder] = message.useMessage();
  const [headerCollapsed, setHeaderCollapsed] = usePersistentBoolean("ydms_header_collapsed", false);

  const handleToggleHeader = useCallback(() => {
    setHeaderCollapsed((prev) => !prev);
  }, [setHeaderCollapsed]);

  return (
    <>
      {contextHolder}
      <UIProvider>
        <CategoryProvider messageApi={messageApi}>
          <MainLayoutContent
            messageApi={messageApi}
            headerCollapsed={headerCollapsed}
            onToggleHeader={handleToggleHeader}
          />
        </CategoryProvider>
      </UIProvider>
    </>
  );
}
