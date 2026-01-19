import type { FC } from "react";
import ReactMarkdown, { defaultUrlTransform, type UrlTransform } from "react-markdown";
import remarkGfm from "remark-gfm";
import { Typography } from "antd";

import { registerYamlPreview } from "../../previewRegistry";
import type { YamlPreviewRenderResult } from "../../previewRegistry";

const { Title } = Typography;

const allowDataImageUrl: UrlTransform = (url, key, node) => {
  if (typeof url === "string") {
    const trimmed = url.trim();
    if (trimmed.toLowerCase().startsWith("data:image/")) {
      return trimmed;
    }
    return defaultUrlTransform(trimmed);
  }
  return defaultUrlTransform(url);
};

// 注册 markdown_v1 预览插件
registerYamlPreview("markdown_v1", {
  render(content) {
    return renderMarkdownV1Content(content);
  },
});

function renderMarkdownV1Content(content: string): YamlPreviewRenderResult {
  if (!content || !content.trim()) {
    return { type: "empty" };
  }

  return {
    type: "component",
    node: <MarkdownV1Preview content={content} />,
  };
}

const MarkdownV1Preview: FC<{ content: string }> = ({ content }) => (
  <div
    style={{
      padding: "24px",
      overflow: "auto",
      height: "100%",
      backgroundColor: "#ffffff",
    }}
  >
    <div
      style={{
        maxWidth: "900px",
        margin: "0 auto",
        fontSize: "16px",
        lineHeight: "1.6",
      }}
    >
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        urlTransform={allowDataImageUrl}
        components={{
          // 自定义标题样式
          h1: ({ children }) => (
            <Title level={1} style={{ marginTop: "24px", marginBottom: "16px" }}>
              {children}
            </Title>
          ),
          h2: ({ children }) => (
            <Title level={2} style={{ marginTop: "20px", marginBottom: "12px" }}>
              {children}
            </Title>
          ),
          h3: ({ children }) => (
            <Title level={3} style={{ marginTop: "16px", marginBottom: "8px" }}>
              {children}
            </Title>
          ),
          h4: ({ children }) => (
            <Title level={4} style={{ marginTop: "12px", marginBottom: "8px" }}>
              {children}
            </Title>
          ),
          h5: ({ children }) => (
            <Title level={5} style={{ marginTop: "8px", marginBottom: "8px" }}>
              {children}
            </Title>
          ),
          // 自定义代码块样式
          code: ({ children, className, ...props }) => {
            // 判断是否是行内代码（没有 className 通常表示行内代码）
            const isInline = !className || !className.startsWith("language-");

            if (isInline) {
              return (
                <code
                  style={{
                    backgroundColor: "#f5f5f5",
                    padding: "2px 6px",
                    borderRadius: "3px",
                    fontFamily: "monospace",
                    fontSize: "0.9em",
                  }}
                  {...props}
                >
                  {children}
                </code>
              );
            }
            return (
              <pre
                style={{
                  backgroundColor: "#f5f5f5",
                  padding: "16px",
                  borderRadius: "4px",
                  overflow: "auto",
                  marginTop: "16px",
                  marginBottom: "16px",
                }}
              >
                <code
                  className={className}
                  style={{
                    fontFamily: "monospace",
                    fontSize: "0.9em",
                  }}
                  {...props}
                >
                  {children}
                </code>
              </pre>
            );
          },
          // 自定义引用样式
          blockquote: ({ children }) => (
            <blockquote
              style={{
                borderLeft: "4px solid #d9d9d9",
                paddingLeft: "16px",
                marginLeft: 0,
                color: "#595959",
                fontStyle: "italic",
              }}
            >
              {children}
            </blockquote>
          ),
          // 自定义表格样式
          table: ({ children }) => (
            <div style={{ overflowX: "auto", marginTop: "16px", marginBottom: "16px" }}>
              <table
                style={{
                  width: "100%",
                  borderCollapse: "collapse",
                  border: "1px solid #e8e8e8",
                }}
              >
                {children}
              </table>
            </div>
          ),
          th: ({ children }) => (
            <th
              style={{
                backgroundColor: "#fafafa",
                padding: "12px",
                textAlign: "left",
                borderBottom: "2px solid #e8e8e8",
                fontWeight: 600,
              }}
            >
              {children}
            </th>
          ),
          td: ({ children }) => (
            <td
              style={{
                padding: "12px",
                borderBottom: "1px solid #e8e8e8",
              }}
            >
              {children}
            </td>
          ),
          // 自定义链接样式
          a: ({ children, href }) => (
            <a
              href={href}
              target="_blank"
              rel="noopener noreferrer"
              style={{
                color: "#1890ff",
                textDecoration: "none",
              }}
            >
              {children}
            </a>
          ),
          // 自定义图片样式
          img: ({ src, alt }) => (
            <img
              src={src}
              alt={alt}
              style={{
                maxWidth: "100%",
                height: "auto",
                marginTop: "16px",
                marginBottom: "16px",
                borderRadius: "4px",
              }}
            />
          ),
          // 自定义列表样式
          ul: ({ children }) => (
            <ul style={{ paddingLeft: "24px", marginTop: "8px", marginBottom: "8px" }}>
              {children}
            </ul>
          ),
          ol: ({ children, start }) => (
            <ol start={start} style={{ paddingLeft: "24px", marginTop: "8px", marginBottom: "8px", listStyleType: "decimal" }}>
              {children}
            </ol>
          ),
          li: ({ children }) => <li style={{ marginTop: "4px", marginBottom: "4px" }}>{children}</li>,
          // 自定义段落样式
          p: ({ children }) => <p style={{ marginTop: "12px", marginBottom: "12px" }}>{children}</p>,
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  </div>
);
