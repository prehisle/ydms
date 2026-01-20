import { type FC, useMemo } from "react";
import DOMPurify from "dompurify";
import { Alert, Empty } from "antd";

interface HTMLPreviewProps {
  content: string;
  className?: string;
  contentClassName?: string;
  unstyled?: boolean;
  /** 额外注入的 CSS 文本，追加在内置样式之后 */
  styleCss?: string;
}

export const HTMLPreview: FC<HTMLPreviewProps> = ({ content, className, contentClassName, unstyled, styleCss }) => {
  const sanitizedHTML = useMemo(() => {
    if (!content || !content.trim()) {
      return null;
    }

    try {
      return DOMPurify.sanitize(content, {
        ALLOWED_TAGS: [
          'div', 'span', 'p', 'br', 'hr',
          'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
          'ul', 'ol', 'li', 'dl', 'dt', 'dd',
          'table', 'thead', 'tbody', 'tfoot', 'tr', 'th', 'td',
          'section', 'article', 'header', 'footer', 'main', 'aside',
          'strong', 'em', 'u', 's', 'code', 'pre', 'blockquote',
          'a', 'img',
          'svg', 'g', 'path', 'circle', 'ellipse', 'rect', 'line', 'polyline', 'polygon',
          'text', 'tspan', 'defs', 'linearGradient', 'radialGradient', 'stop', 'clipPath', 'mask',
        ],
        ALLOWED_ATTR: [
          'class', 'id', 'style',
          'href', 'target', 'rel',
          'src', 'alt', 'title',
          'width', 'height', 'viewBox', 'preserveAspectRatio',
          'fill', 'stroke', 'stroke-width', 'stroke-linecap', 'stroke-linejoin', 'stroke-dasharray',
          'font-size', 'font-weight', 'font-family', 'text-anchor', 'dominant-baseline', 'alignment-baseline',
          'd', 'cx', 'cy', 'r', 'x', 'y', 'x1', 'y1', 'x2', 'y2',
          'points', 'transform', 'opacity', 'stop-color', 'stop-opacity',
          'colspan', 'rowspan',
          'xmlns', 'xmlns:xlink', 'xlink:href',
        ],
        ALLOWED_URI_REGEXP: /^(?:(?:(?:f|ht)tps?|mailto|tel|callto|sms|cid|xmpp):|[^a-z]|[a-z+.\-]+(?:[^a-z+.\-:]|$))/i,
      });
    } catch (error) {
      console.error('HTML sanitization error:', error);
      return null;
    }
  }, [content]);

  if (!content || !content.trim()) {
    return (
      <div style={{ padding: '24px' }}>
        <Empty description="暂无内容，请在左侧编辑器中输入HTML代码" />
      </div>
    );
  }

  if (!sanitizedHTML) {
    return (
      <div style={{ padding: '24px' }}>
        <Alert
          type="error"
          message="HTML解析错误"
          description="无法渲染HTML内容，请检查代码格式"
        />
      </div>
    );
  }

  const wrapperStyle = unstyled
    ? undefined
    : {
        padding: "24px",
        height: "100%",
        overflow: "auto" as const,
        backgroundColor: "#fff",
      };

  const wrapperClassName = ["html-preview-wrapper", className].filter(Boolean).join(" ");
  const innerClassName = ["html-preview-content", contentClassName].filter(Boolean).join(" ");

  return (
    <div className={wrapperClassName} style={wrapperStyle}>
      <div
        className={innerClassName}
        dangerouslySetInnerHTML={{ __html: sanitizedHTML }}
        style={unstyled ? undefined : { lineHeight: 1.8, fontSize: "14px" }}
      />
      <style>{`
        .html-preview-content h1 {
          font-size: 28px;
          font-weight: 600;
          margin: 20px 0 16px;
          line-height: 1.4;
        }
        .html-preview-content h2 {
          font-size: 24px;
          font-weight: 600;
          margin: 18px 0 14px;
          line-height: 1.4;
        }
        .html-preview-content h3 {
          font-size: 20px;
          font-weight: 600;
          margin: 16px 0 12px;
          line-height: 1.4;
        }
        .html-preview-content p {
          margin: 12px 0;
        }
        .html-preview-content ul, .html-preview-content ol {
          margin: 12px 0;
          padding-left: 24px;
        }
        .html-preview-content li {
          margin: 6px 0;
        }
        .html-preview-content code {
          background: #f5f5f5;
          padding: 2px 6px;
          border-radius: 3px;
          font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
          font-size: 13px;
        }
        .html-preview-content pre {
          background: #f5f5f5;
          padding: 12px;
          border-radius: 4px;
          overflow-x: auto;
        }
        .html-preview-content pre code {
          background: none;
          padding: 0;
        }
        .html-preview-content blockquote {
          border-left: 4px solid #1890ff;
          padding-left: 16px;
          margin: 16px 0;
          color: #666;
        }
        .html-preview-content table {
          border-collapse: collapse;
          width: 100%;
          margin: 16px 0;
        }
        .html-preview-content th, .html-preview-content td {
          border: 1px solid #d9d9d9;
          padding: 8px 12px;
          text-align: left;
        }
        .html-preview-content th {
          background: #fafafa;
          font-weight: 600;
        }
        .html-preview-content a {
          color: #1890ff;
          text-decoration: none;
        }
        .html-preview-content a:hover {
          text-decoration: underline;
        }
        ${styleCss ?? ""}
      `}</style>
    </div>
  );
};
