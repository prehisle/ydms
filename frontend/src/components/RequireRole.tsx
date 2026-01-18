import { Navigate } from "react-router-dom";
import { useAuth } from "../contexts/AuthContext";

export interface RequireRoleProps {
  /** 允许访问的角色列表 */
  allowedRoles: string[];
  /** 子组件 */
  children: React.ReactNode;
  /** 无权限时重定向的路径，默认 /documents */
  redirectTo?: string;
}

/**
 * 角色权限守卫组件
 * 用于保护需要特定角色才能访问的路由
 *
 * @example
 * <RequireRole allowedRoles={["super_admin"]}>
 *   <SystemDashboard />
 * </RequireRole>
 */
export function RequireRole({
  allowedRoles,
  children,
  redirectTo = "/documents",
}: RequireRoleProps): JSX.Element {
  const { user, loading } = useAuth();

  // 认证加载中，显示空白
  if (loading) {
    return <></>;
  }

  // 未登录，重定向到登录页
  if (!user) {
    return <Navigate to="/login" replace />;
  }

  // 检查用户角色
  if (!allowedRoles.includes(user.role)) {
    return <Navigate to={redirectTo} replace />;
  }

  return <>{children}</>;
}
