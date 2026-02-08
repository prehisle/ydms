import { Divider, Space, Table, Typography } from "antd";
import type { FC } from "react";

import { registerYamlPreview } from "../../previewRegistry";
import type { YamlPreviewRenderResult } from "../../previewRegistry";
import type { ImageInfo, XiaohongshuCardImagesContent } from "./types";

const { Title, Text } = Typography;

registerYamlPreview("xiaohongshu_card_images_v1", {
  render(content) {
    return renderXiaohongshuCardImagesContent(content);
  },
});

function renderXiaohongshuCardImagesContent(content: string): YamlPreviewRenderResult {
  if (!content || !content.trim()) {
    return { type: "empty" };
  }

  try {
    const data: XiaohongshuCardImagesContent = JSON.parse(content);
    return {
      type: "component",
      node: <XiaohongshuCardImagesPreview data={data} />,
    };
  } catch {
    return {
      type: "error",
      message: "JSON 解析失败",
    };
  }
}

const columns = [
  {
    title: "序号",
    dataIndex: "index",
    key: "index",
    width: 80,
  },
  {
    title: "标题",
    dataIndex: "title",
    key: "title",
  },
  {
    title: "类型",
    dataIndex: "type",
    key: "type",
    width: 120,
  },
  {
    title: "路径",
    dataIndex: "path",
    key: "path",
    ellipsis: true,
  },
];

const XiaohongshuCardImagesPreview: FC<{ data: XiaohongshuCardImagesContent }> = ({ data }) => {
  const { images, source_doc_id } = data;

  return (
    <div style={{ padding: "24px", overflow: "auto", height: "100%" }}>
      <Space align="baseline" style={{ display: "flex", justifyContent: "space-between" }}>
        <Title level={3} style={{ marginBottom: 8 }}>
          小红书卡片图片
        </Title>
        <Text type="secondary">{images.length} 张图片</Text>
      </Space>
      {source_doc_id && (
        <Text type="secondary">源文档 ID: {source_doc_id}</Text>
      )}
      <Divider />

      <Table<ImageInfo>
        dataSource={images}
        columns={columns}
        rowKey="index"
        pagination={false}
        size="small"
      />
    </div>
  );
};
