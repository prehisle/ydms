import { Space, Typography, Tag, Tooltip, Button } from "antd";
import {
  FolderOutlined,
  CheckCircleOutlined,
  MenuUnfoldOutlined,
  MenuFoldOutlined,
} from "@ant-design/icons";
import type { Category } from "../api/categories";

export interface StatusBarProps {
  selectedCategory: Category | null;
  selectedCount: number;
  totalCategories: number;
  includeDescendants: boolean;
  userRole?: string;
  headerCollapsed: boolean;
  onToggleHeader: () => void;
}

export function StatusBar({
  selectedCategory,
  selectedCount,
  totalCategories,
  includeDescendants,
  userRole,
  headerCollapsed,
  onToggleHeader,
}: StatusBarProps) {
  const getRoleLabel = (role: string) => {
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

  return (
    <div
      style={{
        height: "32px",
        background: "#f5f5f5",
        borderTop: "1px solid #d9d9d9",
        display: "flex",
        alignItems: "center",
        padding: "0 16px",
        fontSize: "12px",
        color: "#595959",
        justifyContent: "space-between",
      }}
    >
      {/* 左侧：折叠按钮 + 选中节点信息 */}
      <Space size={16}>
        <Tooltip title={headerCollapsed ? "展开顶部栏" : "收起顶部栏"}>
          <Button
            type="text"
            size="small"
            icon={headerCollapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
            onClick={onToggleHeader}
            style={{ fontSize: "14px" }}
          />
        </Tooltip>
        {selectedCount === 0 ? (
          <Typography.Text type="secondary" style={{ fontSize: "12px" }}>
            未选择节点
          </Typography.Text>
        ) : selectedCount === 1 && selectedCategory ? (
          <Space size={8}>
            <FolderOutlined />
            <Typography.Text strong style={{ fontSize: "12px" }}>
              {selectedCategory.name}
            </Typography.Text>
            <Typography.Text type="secondary" style={{ fontSize: "12px" }}>
              (ID: {selectedCategory.id})
            </Typography.Text>
            <Tooltip title={`完整路径: ${selectedCategory.path}`}>
              <Typography.Text code style={{ fontSize: "11px", cursor: "help" }}>
                {selectedCategory.path}
              </Typography.Text>
            </Tooltip>
            {selectedCategory.deleted_at && <Tag color="red">已删除</Tag>}
          </Space>
        ) : (
          <Space size={8}>
            <CheckCircleOutlined />
            <Typography.Text style={{ fontSize: "12px" }}>
              已选择 <strong>{selectedCount}</strong> 个节点
            </Typography.Text>
          </Space>
        )}
      </Space>

      {/* 中间：全局状态 */}
      <Space size={16} style={{ flex: 1, justifyContent: "center" }}>
        <Typography.Text type="secondary" style={{ fontSize: "12px" }}>
          共 <strong>{totalCategories}</strong> 个节点
        </Typography.Text>
        <Typography.Text type="secondary" style={{ fontSize: "12px" }}>
          {includeDescendants ? "包含子节点" : "仅当前节点"}
        </Typography.Text>
      </Space>

      {/* 右侧：用户角色 */}
      <Space size={12}>
        {userRole && (
          <Space size={8}>
            <Typography.Text type="secondary" style={{ fontSize: "12px" }}>
              当前角色:
            </Typography.Text>
            <Tag color={userRole === "super_admin" ? "red" : userRole === "course_admin" ? "blue" : "green"}>
              {getRoleLabel(userRole)}
            </Tag>
          </Space>
        )}
      </Space>
    </div>
  );
}
