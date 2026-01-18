import { type FC, type DragEvent, useEffect, useMemo } from "react";

import {
  Alert,
  Button,
  Card,
  Empty,
  Form,
  Input,
  Select,
  Space,
  Spin,
  Table,
  Tooltip,
  Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import type { FormInstance } from "antd/es/form";
import {
  PlusOutlined,
  SearchOutlined,
  SortAscendingOutlined,
  MinusCircleOutlined,
  UploadOutlined,
} from "@ant-design/icons";

import type { Document, MetadataOperator } from "../../../api/documents";
import { useAuth } from "../../../contexts/AuthContext";
import { useDocumentTagCache } from "../hooks/useDocumentTagCache";
import type { DocumentFilterFormValues, MetadataFilterFormValue } from "../types";

interface DocumentPanelProps {
  filterForm: FormInstance<DocumentFilterFormValues>;
  documentTypes: { label: string; value: string }[];
  selectedNodeId: number | null;
  documents: Document[];
  columns: ColumnsType<Document>;
  isLoading: boolean;
  isFetching: boolean;
  error: unknown;
  canCreateDocument?: boolean; // 是否有权限创建文档
  pagination?: {
    current: number;
    pageSize: number;
    total: number;
    onChange: (page: number, pageSize?: number) => void;
  };
  onSearch: (values: DocumentFilterFormValues) => void;
  onReset: () => void;
  onAddDocument: () => void;
  onReorderDocuments: () => void;
  onOpenTrash: () => void;
  onDocumentDragStart?: (event: DragEvent<HTMLElement>, document: Document) => void;
  onDocumentDragEnd?: (event: DragEvent<HTMLElement>) => void;
  onRowDoubleClick?: (document: Document) => void;
}

export const DocumentPanel: FC<DocumentPanelProps> = ({
  filterForm,
  documentTypes,
  selectedNodeId,
  documents,
  columns,
  isLoading,
  isFetching,
  error,
  canCreateDocument = true,
  pagination,
  onSearch,
  onReset,
  onAddDocument,
  onReorderDocuments,
  onOpenTrash,
  onDocumentDragStart,
  onDocumentDragEnd,
  onRowDoubleClick,
}) => {
  const { user } = useAuth();
  const { getTags, upsert } = useDocumentTagCache(user?.id ?? null);
  const selectedType = Form.useWatch("type", filterForm);

  const cachedTagOptions = useMemo(() => {
    const typeValue = typeof selectedType === "string" ? selectedType : undefined;
    return getTags(typeValue).map((tag) => ({ label: tag, value: tag }));
  }, [getTags, selectedType]);

  useEffect(() => {
    if (documents.length === 0) {
      return;
    }
    const tagsByType = new Map<string, Set<string>>();
    documents.forEach((doc) => {
      if (typeof doc.type !== "string") {
        return;
      }
      const raw = doc.metadata?.tags as unknown;
      if (!Array.isArray(raw) || raw.length === 0) {
        return;
      }
      const bucket = tagsByType.get(doc.type) ?? new Set<string>();
      raw.forEach((item) => {
        const normalized =
          typeof item === "string" ? item.trim() : String(item ?? "").trim();
        if (normalized.length > 0) {
          bucket.add(normalized);
        }
      });
      if (bucket.size > 0) {
        tagsByType.set(doc.type, bucket);
      }
    });
    tagsByType.forEach((tagSet, docType) => {
      upsert(docType, Array.from(tagSet));
    });
  }, [documents, upsert]);

  return (
    <Space direction="vertical" size="large" style={{ width: "100%" }}>
      <Card>
        <Form
          layout="inline"
          form={filterForm}
          onFinish={onSearch}
          style={{ gap: 16, flexWrap: "wrap" }}
        >
          <Form.Item name="docId" label="文档 ID">
            <Input
              placeholder="例如 123"
              allowClear
              style={{ width: 160 }}
              onKeyPress={(e) => {
                // Allow only numbers, backspace, delete, and navigation keys
                if (
                  !/[0-9]/.test(e.key) &&
                  !["Backspace", "Delete", "ArrowLeft", "ArrowRight", "Tab"].includes(e.key)
                ) {
                  e.preventDefault();
                }
              }}
            />
          </Form.Item>
          <Form.Item name="query" label="关键字">
            <Input placeholder="标题 / 内容" allowClear style={{ width: 200 }} />
          </Form.Item>
          <Form.Item name="type" label="文档类型">
            <Select
              allowClear
              style={{ width: 160 }}
              placeholder="选择类型"
              options={documentTypes}
            />
          </Form.Item>
          <Form.Item name="tags" label="标签">
            <Select
              mode="tags"
              allowClear
              style={{ minWidth: 200 }}
              placeholder="输入或选择标签"
              options={cachedTagOptions}
            />
          </Form.Item>
          <Form.Item style={{ width: "100%", marginBottom: 0 }}>
            <MetadataFiltersField filterForm={filterForm} onReset={onReset} />
          </Form.Item>
        </Form>
      </Card>
      <Card
        title={
          <Space>
            <UploadOutlined />
            <span>产出文档</span>
          </Space>
        }
        extra={
          <Space>
            <Tooltip title="文档回收站">
              <Button onClick={onOpenTrash}>
                回收站
              </Button>
            </Tooltip>
            <Tooltip title="调整排序">
              <Button
                icon={<SortAscendingOutlined />}
                onClick={onReorderDocuments}
                disabled={selectedNodeId == null || documents.length <= 1}
                aria-label="调整文档排序"
              />
            </Tooltip>
            {canCreateDocument && (
              <Tooltip title="新增文档">
                <Button
                  type="primary"
                  icon={<PlusOutlined />}
                  onClick={onAddDocument}
                  disabled={selectedNodeId == null}
                  aria-label="新增文档"
                />
              </Tooltip>
            )}
          </Space>
        }
      >
        {selectedNodeId == null ? (
          <Typography.Paragraph type="secondary">
            请选择单个目录节点以查看文档。
          </Typography.Paragraph>
        ) : isLoading ? (
          <div style={{ display: "flex", justifyContent: "center", padding: "48px 0" }}>
            <Spin />
          </div>
        ) : error ? (
          <Alert
            type="error"
            message="文档加载失败"
            description={(error as Error).message}
          />
        ) : documents.length === 0 ? (
          <Empty description="暂无文档" />
        ) : (
          <Table
            rowKey="id"
            dataSource={documents}
            columns={columns}
            pagination={
              pagination
                ? {
                    current: pagination.current,
                    pageSize: pagination.pageSize,
                    total: pagination.total,
                    showTotal: (total) => `共 ${total} 条`,
                    showSizeChanger: true,
                    pageSizeOptions: [10, 20, 50, 100],
                    position: ["bottomCenter"],
                    onChange: pagination.onChange,
                  }
                : false
            }
            loading={isFetching}
            onRow={(record) => ({
              onDoubleClick: () => {
                if (onRowDoubleClick) {
                  onRowDoubleClick(record);
                }
              },
            })}
          />
        )}
      </Card>
    </Space>
  );
};

const metadataOperatorOptions: { value: MetadataOperator; label: string }[] = [
  { value: "eq", label: "等于" },
  { value: "like", label: "模糊包含" },
  { value: "in", label: "集合 (IN)" },
  { value: "gt", label: "大于" },
  { value: "gte", label: "大于等于" },
  { value: "lt", label: "小于" },
  { value: "lte", label: "小于等于" },
  { value: "any", label: "数组含任一" },
  { value: "all", label: "数组含全部" },
];

const MetadataFiltersField: FC<{
  filterForm: FormInstance<DocumentFilterFormValues>;
  onReset: () => void;
}> = ({ filterForm, onReset }) => (
  <Form.List name="metadataFilters">
    {(fields, { add, remove }) => {
      const metadataFilters = filterForm.getFieldValue("metadataFilters") ?? [];
      return (
        <Space direction="vertical" size="middle" style={{ width: "100%" }}>
          <Space align="center" wrap>
            <Typography.Text strong>元数据条件</Typography.Text>
            <Button
              type="dashed"
              icon={<PlusOutlined />}
              onClick={() => add({ operator: "eq", value: "" })}
            >
              添加条件
            </Button>
            <Space>
              <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>
                筛选
              </Button>
              <Button onClick={onReset}>重置</Button>
            </Space>
          </Space>
          {fields.length === 0 ? null : (
            <Space direction="vertical" size="small" style={{ width: "100%" }}>
              {fields.map((field) => {
                const index = typeof field.name === "number" ? field.name : Number(field.name);
                const currentFilter: MetadataFilterFormValue = metadataFilters?.[index] ?? {};
                const currentOperator: MetadataOperator = currentFilter?.operator ?? "eq";
                return (
                  <Space
                    key={field.key}
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
                    <Form.Item
                      {...field}
                      name={[field.name, "key"]}
                      fieldKey={[field.fieldKey ?? field.name, "key"]}
                      rules={[{ required: true, message: "请输入元数据键" }]}
                      style={{ marginBottom: 0 }}
                    >
                      <Input placeholder="键" allowClear style={{ width: 160 }} />
                    </Form.Item>
                    <Form.Item
                      {...field}
                      name={[field.name, "operator"]}
                      fieldKey={[field.fieldKey ?? field.name, "operator"]}
                      initialValue="eq"
                      style={{ marginBottom: 0 }}
                    >
                      <Select
                        style={{ width: 140 }}
                        options={metadataOperatorOptions}
                        onChange={(value: MetadataOperator) =>
                          handleOperatorChange(filterForm, field.name, value)
                        }
                      />
                    </Form.Item>
                    <Form.Item
                      {...field}
                      name={[field.name, "value"]}
                      fieldKey={[field.fieldKey ?? field.name, "value"]}
                      style={{ marginBottom: 0 }}
                    >
                      {renderMetadataFilterValueInput(currentOperator)}
                    </Form.Item>
                    <Button
                      icon={<MinusCircleOutlined />}
                      onClick={() => remove(field.name)}
                    />
                  </Space>
                );
              })}
            </Space>
          )}
        </Space>
      );
    }}
  </Form.List>
);

function handleOperatorChange(
  form: FormInstance<DocumentFilterFormValues>,
  index: number,
  nextOperator: MetadataOperator,
) {
  const currentFilters: MetadataFilterFormValue[] = form.getFieldValue("metadataFilters") ?? [];
  const nextFilters = [...currentFilters];
  const previous = nextFilters[index]?.value;
  nextFilters[index] = {
    ...(nextFilters[index] ?? {}),
    operator: nextOperator,
    value: getDefaultValueForOperator(nextOperator, previous),
  };
  form.setFieldsValue({ metadataFilters: nextFilters });
}

function getDefaultValueForOperator(
  operator: MetadataOperator,
  previous?: string | string[],
): string | string[] {
  switch (operator) {
    case "in":
    case "any":
    case "all":
      if (Array.isArray(previous)) {
        return previous;
      }
      if (typeof previous === "string" && previous.trim()) {
        return [previous.trim()];
      }
      return [];
    case "gt":
    case "gte":
    case "lt":
    case "lte":
      return typeof previous === "string" ? previous : "";
    case "like":
    case "eq":
    default:
      return typeof previous === "string" ? previous : "";
  }
}

function renderMetadataFilterValueInput(operator: MetadataOperator) {
  switch (operator) {
    case "in":
    case "any":
    case "all":
      return <Select mode="tags" style={{ minWidth: 220 }} allowClear placeholder="多个值" />;
    case "gt":
    case "gte":
    case "lt":
    case "lte":
      return <Input placeholder="数值" allowClear style={{ width: 160 }} inputMode="decimal" />;
    case "like":
      return <Input placeholder="模糊匹配值" allowClear style={{ width: 200 }} />;
    case "eq":
    default:
      return <Input placeholder="值" allowClear style={{ width: 200 }} />;
  }
}
