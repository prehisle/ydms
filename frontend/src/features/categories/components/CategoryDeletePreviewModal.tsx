import { useState, type FC } from "react";

import { Card, Input, Modal, Space, Spin, Typography, Alert } from "antd";

import type { CategoryBulkCheckResponse } from "../../../api/categories";

interface CategoryDeletePreviewModalProps {
  open: boolean;
  mode: "soft" | "purge";
  loading: boolean;
  result: CategoryBulkCheckResponse | null;
  confirmLoading: boolean;
  onCancel: () => void;
  onConfirm: (adminPassword?: string) => void;
}

export const CategoryDeletePreviewModal: FC<CategoryDeletePreviewModalProps> = ({
  open,
  mode,
  loading,
  result,
  confirmLoading,
  onCancel,
  onConfirm,
}) => {
  const [adminPassword, setAdminPassword] = useState("");

  const hasChildren = result?.items.some((item) => item.has_children) ?? false;

  const handleConfirm = () => {
    onConfirm(hasChildren ? adminPassword : undefined);
    setAdminPassword("");
  };

  const handleCancel = () => {
    setAdminPassword("");
    onCancel();
  };

  return (
    <Modal
      title={mode === "purge" ? "彻底删除确认" : "删除确认"}
      open={open}
      confirmLoading={confirmLoading}
      onCancel={handleCancel}
      onOk={handleConfirm}
      okText={mode === "purge" ? "彻底删除" : "删除"}
      okButtonProps={{ disabled: hasChildren && !adminPassword }}
      cancelButtonProps={{ disabled: confirmLoading }}
      width={520}
    >
      {loading ? (
        <div style={{ display: "flex", justifyContent: "center", padding: "24px 0" }}>
          <Spin />
        </div>
      ) : result ? (
        <Space direction="vertical" style={{ width: "100%" }}>
          {result.items.map((item) => (
            <Card key={item.id} size="small" bordered={false} style={{ border: "1px solid #f0f0f0" }}>
              <Typography.Text strong>{item.name}</Typography.Text>
              <Typography.Paragraph type="secondary" style={{ marginBottom: 8 }}>
                {item.path}
              </Typography.Paragraph>
              <Typography.Text>关联文档：{item.document_count}</Typography.Text>
              {item.has_children && (
                <Typography.Paragraph type="warning" style={{ margin: "8px 0 0" }}>
                  ⚠️ 包含子目录
                </Typography.Paragraph>
              )}
              {item.warnings && item.warnings.length > 0 ? (
                <ul style={{ paddingLeft: 16, margin: "8px 0 0" }}>
                  {item.warnings.map((warn, idx) => (
                    <li key={idx}>{warn}</li>
                  ))}
                </ul>
              ) : (
                <Typography.Paragraph type="secondary" style={{ margin: "8px 0 0" }}>
                  无关联风险
                </Typography.Paragraph>
              )}
            </Card>
          ))}
          {hasChildren && (
            <Alert
              type="warning"
              message="强制删除需要管理员密码"
              description={
                <div style={{ marginTop: 8 }}>
                  <Typography.Text type="secondary">
                    选中的目录包含子目录，将递归删除所有子目录和文档。请输入您的登录密码确认：
                  </Typography.Text>
                  <Input.Password
                    placeholder="请输入管理员密码"
                    value={adminPassword}
                    onChange={(e) => setAdminPassword(e.target.value)}
                    style={{ marginTop: 8 }}
                  />
                </div>
              }
            />
          )}
        </Space>
      ) : (
        <Typography.Paragraph type="secondary">正在加载删除校验信息...</Typography.Paragraph>
      )}
    </Modal>
  );
};
