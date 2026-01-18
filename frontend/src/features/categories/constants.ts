/** 节点类型选项 */
export const CATEGORY_TYPE_OPTIONS = [
  { value: "course", label: "课程" },
  { value: "chapter", label: "章" },
  { value: "section", label: "节" },
  { value: "topic", label: "知识点" },
] as const;

/** 节点类型映射（value -> label） */
export const CATEGORY_TYPE_LABELS: Record<string, string> = {
  course: "课程",
  chapter: "章",
  section: "节",
  topic: "知识点",
};

/** 获取节点类型显示标签 */
export function getCategoryTypeLabel(type: string | null | undefined): string {
  if (!type) return "";
  return CATEGORY_TYPE_LABELS[type] || type;
}
