import { type FC, useMemo, useRef, useEffect, useState } from "react";
import { Alert, Empty } from "antd";

interface HTMLPreviewProps {
  content: string;
  className?: string;
  /** @deprecated 在 iframe 模式下不再使用 */
  contentClassName?: string;
  /** @deprecated 在 iframe 模式下不再使用 */
  unstyled?: boolean;
  /** 额外注入的 CSS 文本，会被注入到 iframe 内部 */
  styleCss?: string;
}

/** 默认的基础样式，注入到 iframe 内部 */
const DEFAULT_STYLES = `
  * {
    box-sizing: border-box;
  }
  html, body {
    margin: 0;
    padding: 0;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
    font-size: 14px;
    line-height: 1.8;
    color: #333;
    background: #fff;
  }
  body {
    padding: 24px;
  }
  h1 {
    font-size: 28px;
    font-weight: 600;
    margin: 20px 0 16px;
    line-height: 1.4;
  }
  h2 {
    font-size: 24px;
    font-weight: 600;
    margin: 18px 0 14px;
    line-height: 1.4;
  }
  h3 {
    font-size: 20px;
    font-weight: 600;
    margin: 16px 0 12px;
    line-height: 1.4;
  }
  h4, h5, h6 {
    font-weight: 600;
    margin: 14px 0 10px;
    line-height: 1.4;
  }
  p {
    margin: 12px 0;
  }
  ul, ol {
    margin: 12px 0;
    padding-left: 24px;
  }
  li {
    margin: 6px 0;
  }
  code {
    background: #f5f5f5;
    padding: 2px 6px;
    border-radius: 3px;
    font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
    font-size: 13px;
  }
  pre {
    background: #f5f5f5;
    padding: 12px;
    border-radius: 4px;
    overflow-x: auto;
  }
  pre code {
    background: none;
    padding: 0;
  }
  blockquote {
    border-left: 4px solid #1890ff;
    padding-left: 16px;
    margin: 16px 0;
    color: #666;
  }
  table {
    border-collapse: collapse;
    width: 100%;
    margin: 16px 0;
  }
  th, td {
    border: 1px solid #d9d9d9;
    padding: 8px 12px;
    text-align: left;
  }
  th {
    background: #fafafa;
    font-weight: 600;
  }
  a {
    color: #1890ff;
    text-decoration: none;
  }
  a:hover {
    text-decoration: underline;
  }
  img {
    max-width: 100%;
    height: auto;
  }
`;

export const HTMLPreview: FC<HTMLPreviewProps> = ({ content, className, styleCss }) => {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const [error, setError] = useState<string | null>(null);

  // 构建完整的 HTML 文档
  const fullHTML = useMemo(() => {
    if (!content || !content.trim()) {
      return null;
    }

    try {
      // 检查内容是否已经是完整的 HTML 文档
      const isFullDocument = /<html[\s>]/i.test(content) || /<!doctype/i.test(content);

      if (isFullDocument) {
        // 如果是完整文档，注入额外样式到 head 中
        if (styleCss) {
          const styleTag = `<style>${styleCss}</style>`;
          if (/<\/head>/i.test(content)) {
            return content.replace(/<\/head>/i, `${styleTag}</head>`);
          }
          // 如果没有 head 标签，在 html 标签后添加
          if (/<html[^>]*>/i.test(content)) {
            return content.replace(/(<html[^>]*>)/i, `$1<head>${styleTag}</head>`);
          }
        }
        return content;
      }

      // 如果只是 HTML 片段，包装成完整文档
      return `<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <style>${DEFAULT_STYLES}</style>
  ${styleCss ? `<style>${styleCss}</style>` : ""}
</head>
<body>
${content}
</body>
</html>`;
    } catch (err) {
      console.error("HTML processing error:", err);
      setError("HTML 处理错误");
      return null;
    }
  }, [content, styleCss]);

  // 当 iframe 加载后，监听内部链接点击
  useEffect(() => {
    const iframe = iframeRef.current;
    if (!iframe) return;

    const handleLoad = () => {
      try {
        const iframeDoc = iframe.contentDocument || iframe.contentWindow?.document;
        if (!iframeDoc) return;

        // 拦截链接点击，在新标签页打开外部链接
        iframeDoc.addEventListener("click", (e) => {
          const target = e.target as HTMLElement;
          const anchor = target.closest("a");
          if (anchor && anchor.href) {
            e.preventDefault();
            window.open(anchor.href, "_blank", "noopener,noreferrer");
          }
        });
      } catch (err) {
        // 跨域情况下无法访问 iframe 内容，忽略
      }
    };

    iframe.addEventListener("load", handleLoad);
    return () => iframe.removeEventListener("load", handleLoad);
  }, [fullHTML]);

  if (!content || !content.trim()) {
    return (
      <div style={{ padding: "24px" }}>
        <Empty description="暂无内容，请在左侧编辑器中输入HTML代码" />
      </div>
    );
  }

  if (error || !fullHTML) {
    return (
      <div style={{ padding: "24px" }}>
        <Alert
          type="error"
          message="HTML解析错误"
          description={error || "无法渲染HTML内容，请检查代码格式"}
        />
      </div>
    );
  }

  const wrapperClassName = ["html-preview-wrapper", className].filter(Boolean).join(" ");

  return (
    <div
      className={wrapperClassName}
      style={{
        height: "100%",
        width: "100%",
        overflow: "hidden",
        backgroundColor: "#fff",
      }}
    >
      <iframe
        ref={iframeRef}
        srcDoc={fullHTML}
        sandbox="allow-same-origin"
        style={{
          width: "100%",
          height: "100%",
          border: "none",
          display: "block",
        }}
        title="HTML Preview"
      />
    </div>
  );
};
