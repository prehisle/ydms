import { Avatar, Dropdown, Layout, Menu, Space, message } from "antd";
import type { MenuProps } from "antd";
import { KeyOutlined, LogoutOutlined, UserOutlined } from "@ant-design/icons";
import { useMemo, type CSSProperties } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "../../contexts/AuthContext";
import { useUIContext } from "../../contexts/UIContext";

const HEADER_BASE_STYLE: CSSProperties = {
  background: "#fff",
  padding: "0 24px",
  borderBottom: "1px solid #f0f0f0",
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  transition: "all 0.3s ease",
};

const BRAND_STYLE: CSSProperties = {
  fontSize: "18px",
  fontWeight: 600,
  whiteSpace: "nowrap",
};

const MENU_STYLE: CSSProperties = {
  flex: 1,
  minWidth: 0,
  borderBottom: "none",
};

export interface NavBarProps {
  collapsed?: boolean;
}

/**
 * 顶部导航栏组件
 * 包含品牌标识、主导航菜单、用户下拉菜单
 */
export function NavBar({ collapsed = false }: NavBarProps): JSX.Element {
  const { user, logout } = useAuth();
  const { handleOpenChangePassword } = useUIContext();
  const location = useLocation();
  const navigate = useNavigate();

  // 根据当前路径确定激活的导航项
  const activeKey = useMemo(() => {
    if (location.pathname.startsWith("/system")) return "system";
    return "documents";
  }, [location.pathname]);

  // 系统管理仅对超级管理员可见
  const canAccessSystem = user?.role === "super_admin";

  // 构建导航菜单项
  const navItems = useMemo<MenuProps["items"]>(() => {
    const items: MenuProps["items"] = [
      { key: "documents", label: <Link to="/documents">节点与文档管理</Link> },
    ];
    if (canAccessSystem) {
      items.push({ key: "system", label: <Link to="/system">系统管理</Link> });
    }
    return items;
  }, [canAccessSystem]);

  // 退出登录处理
  const handleLogout = async (): Promise<void> => {
    try {
      await logout();
      message.success("已退出登录");
      navigate("/login");
    } catch {
      message.error("退出登录失败");
    }
  };

  // 获取角色显示标签
  const getRoleLabel = (role: string): string => {
    switch (role) {
      case "super_admin":
        return "超级管理员";
      case "course_admin":
        return "课程管理员";
      case "proofreader":
        return "校对员";
      default:
        return role;
    }
  };

  // 用户下拉菜单
  const userMenuItems: MenuProps["items"] = [
    {
      key: "user-info",
      label: (
        <div>
          <div style={{ fontWeight: 500 }}>{user?.display_name || user?.username}</div>
          <div style={{ fontSize: "12px", color: "#999" }}>{getRoleLabel(user?.role || "")}</div>
        </div>
      ),
      disabled: true,
    },
    { type: "divider" },
    {
      key: "change-password",
      label: "修改密码",
      icon: <KeyOutlined />,
      onClick: () => handleOpenChangePassword(),
    },
    {
      key: "logout",
      label: "退出登录",
      icon: <LogoutOutlined />,
      onClick: () => {
        void handleLogout();
      },
    },
  ];

  const displayName = user?.display_name || user?.username || "用户";

  return (
    <Layout.Header
      style={{
        ...HEADER_BASE_STYLE,
        height: collapsed ? 0 : 64,
        minHeight: collapsed ? 0 : 64,
        lineHeight: collapsed ? 0 : "64px",
        overflow: "hidden",
        opacity: collapsed ? 0 : 1,
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 24, flex: 1, minWidth: 0 }}>
        <div style={BRAND_STYLE}>YDMS 资料管理系统</div>
        <Menu mode="horizontal" selectedKeys={[activeKey]} items={navItems} style={MENU_STYLE} />
      </div>
      <Dropdown menu={{ items: userMenuItems }} placement="bottomRight">
        <Space style={{ cursor: "pointer" }}>
          <Avatar icon={<UserOutlined />} size="small" />
          <span>{displayName}</span>
        </Space>
      </Dropdown>
    </Layout.Header>
  );
}
