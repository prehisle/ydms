import { http } from "./http";

export interface Category {
  id: number;
  name: string;
  slug: string;
  path: string;
  type?: string | null;
  parent_id: number | null;
  position: number;
  subtree_doc_count: number;
  created_at: string;
  updated_at: string;
  deleted_at?: string | null;
  children?: Category[];
}

export interface CategoryCreatePayload {
  name: string;
  parent_id?: number | null;
  type?: string | null;
}

export interface CategoryUpdatePayload {
  name?: string;
  type?: string | null;
}

export interface CategoryMovePayload {
  new_parent_id?: number | null;
}

export interface CategoryReorderPayload {
  parent_id?: number | null;
  ordered_ids: number[];
}

export interface CategoryRepositionPayload {
  new_parent_id?: number | null;
  ordered_ids: number[];
}

export interface CategoryRepositionResult {
  category: Category;
  siblings: Category[];
}

export interface CategoryBulkIDsPayload {
  ids: number[];
}

export interface CategoryBulkCopyPayload {
  source_ids: number[];
  target_parent_id?: number | null;
  insert_before_id?: number | null;
  insert_after_id?: number | null;
}

export interface CategoryBulkMovePayload {
  source_ids: number[];
  target_parent_id?: number | null;
  insert_before_id?: number | null;
  insert_after_id?: number | null;
}

export interface CategoryDependencySummary {
  id: number;
  name: string;
  path: string;
  has_children: boolean;
  document_count: number;
  include_descendants: boolean;
  warnings?: string[];
}

export interface CategoryBulkCheckPayload {
  ids: number[];
  include_descendants?: boolean;
}

export interface CategoryBulkCheckResponse {
  items: CategoryDependencySummary[];
}

export async function getCategoryTree(
  includeDeleted = false,
): Promise<Category[]> {
  const query = includeDeleted ? "?include_deleted=true" : "";
  return http<Category[]>(`/api/v1/categories/tree${query}`);
}

export async function createCategory(payload: CategoryCreatePayload) {
  return http<Category>("/api/v1/categories", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function updateCategory(
  id: number,
  payload: CategoryUpdatePayload,
) {
  return http<Category>(`/api/v1/categories/${id}`, {
    method: "PATCH",
    body: JSON.stringify(payload),
  });
}

export interface CategoryDeletePayload {
  admin_password?: string;
}

export async function deleteCategory(id: number, payload?: CategoryDeletePayload) {
  return http<void>(`/api/v1/categories/${id}`, {
    method: "DELETE",
    body: payload ? JSON.stringify(payload) : undefined,
  });
}

export async function moveCategory(
  id: number,
  payload: CategoryMovePayload,
): Promise<Category> {
  return http<Category>(`/api/v1/categories/${id}/move`, {
    method: "PATCH",
    body: JSON.stringify(payload),
  });
}

export async function reorderCategories(payload: CategoryReorderPayload) {
  return http<Category[]>("/api/v1/categories/reorder", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function repositionCategory(
  id: number,
  payload: CategoryRepositionPayload,
): Promise<CategoryRepositionResult> {
  return http<CategoryRepositionResult>(
    `/api/v1/categories/${id}/reposition`,
    {
      method: "PATCH",
      body: JSON.stringify(payload),
    },
  );
}

export async function restoreCategory(id: number) {
  return http<Category>(`/api/v1/categories/${id}/restore`, {
    method: "POST",
  });
}

export async function getDeletedCategories(): Promise<Category[]> {
  return http<Category[]>("/api/v1/categories/trash");
}

export async function purgeCategory(id: number) {
  return http<void>(`/api/v1/categories/${id}/purge`, {
    method: "DELETE",
  });
}

export async function bulkRestoreCategories(payload: CategoryBulkIDsPayload) {
  return http<Category[]>("/api/v1/categories/bulk/restore", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function bulkDeleteCategories(payload: CategoryBulkIDsPayload) {
  return http<{ deleted_ids: number[] }>("/api/v1/categories/bulk/delete", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function bulkPurgeCategories(payload: CategoryBulkIDsPayload) {
  return http<{ purged_ids: number[] }>("/api/v1/categories/bulk/purge", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function bulkCopyCategories(payload: CategoryBulkCopyPayload) {
  return http<{ items: Category[] }>("/api/v1/categories/bulk/copy", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function bulkMoveCategories(payload: CategoryBulkMovePayload) {
  return http<{ items: Category[] }>("/api/v1/categories/bulk/move", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function bulkCheckCategories(
  payload: CategoryBulkCheckPayload,
): Promise<CategoryBulkCheckResponse> {
  return http<CategoryBulkCheckResponse>("/api/v1/categories/bulk/check", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}
