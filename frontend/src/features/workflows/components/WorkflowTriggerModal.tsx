import { Alert, Form, Input, Modal, Space, Tag, Typography } from "antd";
import { useState } from "react";
import type { WorkflowDefinition } from "../../../api/workflows";
import type { SourceDocument } from "../../../api/documents";

const { Text } = Typography;

interface WorkflowTriggerModalProps {
  open: boolean;
  workflow: WorkflowDefinition;
  sources: SourceDocument[];
  onCancel: () => void;
  onTrigger: (workflowKey: string, parameters: Record<string, unknown>) => Promise<void>;
}

export function WorkflowTriggerModal({
  open,
  workflow,
  sources,
  onCancel,
  onTrigger,
}: WorkflowTriggerModalProps) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);

  const handleOk = async () => {
    try {
      const values = await form.validateFields();
      setLoading(true);
      await onTrigger(workflow.workflow_key, values);
      form.resetFields();
    } catch (error) {
      // 表单验证失败或触发失败
      console.error("Trigger workflow failed:", error);
    } finally {
      setLoading(false);
    }
  };

  const handleCancel = () => {
    form.resetFields();
    onCancel();
  };

  // 根据 parameter_schema 渲染表单字段
  // 简化实现：目前只支持 string 类型的参数
  const renderFormFields = () => {
    const schema = workflow.parameter_schema as {
      properties?: Record<string, { type: string; title?: string; description?: string; default?: unknown }>;
      required?: string[];
    };

    if (!schema || !schema.properties) {
      return null;
    }

    const required = schema.required || [];

    return Object.entries(schema.properties).map(([key, prop]) => {
      const isRequired = required.includes(key);

      return (
        <Form.Item
          key={key}
          name={key}
          label={prop.title || key}
          rules={isRequired ? [{ required: true, message: `请输入${prop.title || key}` }] : []}
          extra={prop.description}
          initialValue={prop.default}
        >
          <Input placeholder={prop.description || `请输入${prop.title || key}`} />
        </Form.Item>
      );
    });
  };

  return (
    <Modal
      title={`运行工作流: ${workflow.name}`}
      open={open}
      onOk={handleOk}
      onCancel={handleCancel}
      confirmLoading={loading}
      okText="运行"
      cancelText="取消"
      width={500}
    >
      <Space direction="vertical" style={{ width: "100%" }} size="middle">
        {/* 工作流描述 */}
        {workflow.description && (
          <Text type="secondary">{workflow.description}</Text>
        )}

        {/* 源文档列表 */}
        <div>
          <Text strong style={{ display: "block", marginBottom: 8 }}>
            将读取以下源文档:
          </Text>
          {sources.length === 0 ? (
            <Alert
              type="warning"
              message="未关联源文档"
              description="请先关联源文档，工作流需要源文档作为输入"
              showIcon
            />
          ) : (
            <Space wrap>
              {sources.map((source) => (
                <Tag key={source.document_id}>{source.document?.title || `文档 ${source.document_id}`}</Tag>
              ))}
            </Space>
          )}
        </div>

        {/* 参数表单 */}
        <Form form={form} layout="vertical">
          {renderFormFields()}
        </Form>
      </Space>
    </Modal>
  );
}
