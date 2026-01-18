import { lazy, Suspense } from "react";
import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { Spin, Space, Typography } from "antd";
import { DocumentsPage } from "./App";
import { AuthProvider } from "./contexts/AuthContext";
import { PrivateRoute } from "./components/PrivateRoute";
import { RequireRole } from "./components/RequireRole";
import { LoginPage } from "./features/auth";
import { MainLayout } from "./components/Layout";
import { TaskListPage, TaskDetailPage } from "./features/tasks";
import { SystemDashboard, SystemUsersPage, SystemApiKeysPage } from "./features/system";

const DocumentEditor = lazy(() =>
  import("./features/documents/components/DocumentEditor").then((module) => ({
    default: module.DocumentEditor,
  }))
);

const LoadingFallback = () => (
  <div
    style={{
      display: "flex",
      justifyContent: "center",
      alignItems: "center",
      height: "100vh",
    }}
  >
    <Space direction="vertical" align="center" size="middle">
      <Spin size="large" />
      <Typography.Text type="secondary">加载中...</Typography.Text>
    </Space>
  </div>
);

export const AppRoutes = () => {
  return (
    <BrowserRouter>
      <AuthProvider>
        <Routes>
          {/* 公开路由 */}
          <Route path="/login" element={<LoginPage />} />

          {/* 受保护的路由 - 使用 MainLayout 作为布局容器 */}
          <Route
            path="/"
            element={
              <PrivateRoute>
                <MainLayout />
              </PrivateRoute>
            }
          >
            {/* 默认重定向到文档管理 */}
            <Route index element={<Navigate to="/documents" replace />} />

            {/* 节点与文档管理 */}
            <Route path="documents" element={<DocumentsPage />} />
            <Route
              path="documents/new"
              element={
                <Suspense fallback={<LoadingFallback />}>
                  <DocumentEditor />
                </Suspense>
              }
            />
            <Route
              path="documents/:docId/edit"
              element={
                <Suspense fallback={<LoadingFallback />}>
                  <DocumentEditor />
                </Suspense>
              }
            />

            {/* 任务中心 */}
            <Route path="tasks" element={<TaskListPage />} />
            <Route path="tasks/:id" element={<TaskDetailPage />} />

            {/* 系统管理 - 仅超级管理员可访问 */}
            <Route
              path="system"
              element={
                <RequireRole allowedRoles={["super_admin"]}>
                  <SystemDashboard />
                </RequireRole>
              }
            />
            <Route
              path="system/users"
              element={
                <RequireRole allowedRoles={["super_admin"]}>
                  <SystemUsersPage />
                </RequireRole>
              }
            />
            <Route
              path="system/api-keys"
              element={
                <RequireRole allowedRoles={["super_admin"]}>
                  <SystemApiKeysPage />
                </RequireRole>
              }
            />

            {/* 未匹配路由重定向 */}
            <Route path="*" element={<Navigate to="/documents" replace />} />
          </Route>
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  );
};
