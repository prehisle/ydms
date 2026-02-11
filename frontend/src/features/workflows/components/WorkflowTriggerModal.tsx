import { Alert, DatePicker, Form, Input, Modal, Space, Tag, Typography } from "antd";
import dayjs from "dayjs";
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
      // Convert dayjs objects to ISO 8601 strings
      const params = { ...values };
      if (params.schedule_at) {
        params.schedule_at = params.schedule_at.format("YYYY-MM-DDTHH:mm:ssZ");
      }
      await onTrigger(workflow.workflow_key, params);
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
  const renderFormFields = () => {
    const schema = workflow.parameter_schema as {
      properties?: Record<string, { type: string; title?: string; description?: string; default?: unknown }>;
      required?: string[];
    };

    if (!schema || !schema.properties) {
      return null;
    }

    // 过滤掉 PDMS 标准参数和工作流内部参数
    const HIDDEN_PARAMS = [
      "run_id", "node_id", "workflow_key", "source_doc_ids",
      "callback_url", "pdms_base_url", "api_key",
      "doc_id", "image_doc_id", "image_paths", "source_doc_id",
      "skip_screenshot", "skip_publish", "publish_title", "publish_caption",
      "target_docs",
    ];

    const required = schema.required || [];

    return Object.entries(schema.properties)
      .filter(([key]) => !HIDDEN_PARAMS.includes(key))
      .map(([key, prop]) => {
      const isRequired = required.includes(key);

      // schedule_at: 渲染为日期时间选择器
      if (key === "schedule_at") {
        return (
          <Form.Item
            key={key}
            name={key}
            label={prop.title || "定时发布"}
            extra={prop.description || "支持 1 小时至 14 天内。不填则立即发布。"}
            rules={[
              {
                validator: (_, value) => {
                  if (!value) return Promise.resolve();
                  if (value.isBefore(dayjs().add(1, "hour"))) {
                    return Promise.reject("定时发布时间必须在 1 小时后");
                  }
                  if (value.isAfter(dayjs().add(14, "day"))) {
                    return Promise.reject("定时发布时间不能超过 14 天");
                  }
                  return Promise.resolve();
                },
              },
            ]}
          >
            <DatePicker
              showTime
              format="YYYY-MM-DD HH:mm"
              placeholder="不填则立即发布"
              style={{ width: "100%" }}
              disabledDate={(current) =>
                current && (current.isBefore(dayjs(), "day") || current.isAfter(dayjs().add(14, "day"), "day"))
              }
            />
          </Form.Item>
        );
      }

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
