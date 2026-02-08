/**
 * 小红书卡片图片文档类型定义
 */

export interface ImageInfo {
  index: number;
  title: string;
  type: string;
  path: string;
}

export interface XiaohongshuCardImagesContent {
  images: ImageInfo[];
  source_doc_id: number | null;
}
