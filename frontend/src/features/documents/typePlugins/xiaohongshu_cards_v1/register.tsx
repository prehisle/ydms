import { Divider, Space, Typography } from "antd";
import type { FC } from "react";

import { registerYamlPreview } from "../../previewRegistry";
import type { YamlPreviewRenderResult } from "../../previewRegistry";

const { Title, Text } = Typography;

registerYamlPreview("xiaohongshu_cards_v1", {
  render(content) {
    return renderXiaohongshuCardsContent(content);
  },
});

function renderXiaohongshuCardsContent(content: string): YamlPreviewRenderResult {
  if (!content || !content.trim()) {
    return { type: "empty" };
  }

  // 小红书卡片是完整的 HTML 文档，直接渲染
  return {
    type: "component",
    node: <XiaohongshuCardsPreview html={content} />,
  };
}

const XiaohongshuCardsPreview: FC<{ html: string }> = ({ html }) => {
  // 统计卡片数量
  const cardCount = (html.match(/class="card/g) || []).length;

  return (
    <div style={{ padding: "24px", overflow: "auto", height: "100%" }}>
      <Space align="baseline" style={{ display: "flex", justifyContent: "space-between" }}>
        <Title level={3} style={{ marginBottom: 8 }}>
          小红书卡片
        </Title>
        <Text type="secondary">{cardCount} 张卡片</Text>
      </Space>
      <Divider />

      {/* 使用 iframe 渲染完整的 HTML 文档，保持样式隔离 */}
      <iframe
        srcDoc={html}
        style={{
          width: "100%",
          height: "calc(100vh - 200px)",
          border: "1px solid #d9d9d9",
          borderRadius: 8,
          background: "#0F0F0F",
        }}
        title="小红书卡片预览"
        sandbox="allow-same-origin allow-scripts"
      />
    </div>
  );
};
