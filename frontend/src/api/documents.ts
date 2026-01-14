import { buildQuery, http } from "./http";
export {
  DOCUMENT_TYPES,
  DOCUMENT_TYPE_DEFINITIONS,
  DOCUMENT_TYPE_OPTIONS,
} from "../generated/documentTypes";
export type { DocumentType } from "../generated/documentTypes";

export type MetadataOperator =
  | "eq"
  | "like"
  | "in"
  | "gt"
  | "gte"
  | "lt"
  | "lte"
  | "any"
  | "all";

export interface MetadataFilterClause {
  key: string;
  operator: MetadataOperator;
  values: string[];
}

// Content formats
export const CONTENT_FORMATS = {
  HTML: "html",
  YAML: "yaml",
} as const;

export type ContentFormat = typeof CONTENT_FORMATS[keyof typeof CONTENT_FORMATS];

// Document content structure
export interface DocumentContent {
  format: ContentFormat;
  data: string;
}

export interface Document {
  id: number;
  title: string;
  version_number?: number | null;
  type?: string;
  position: number;
  content?: Record<string, unknown>;
  created_by: string;
  updated_by: string;
  created_at: string;
  updated_at: string;
  deleted_at?: string | null;
  metadata: Record<string, unknown>;
}


export interface DocumentCreatePayload {
  title: string;
  type?: string;
  position?: number;
  metadata?: Record<string, unknown>;
  content?: Record<string, unknown>;
}

export interface DocumentUpdatePayload {
  title?: string;
  type?: string;
  position?: number;
  metadata?: Record<string, unknown>;
  content?: Record<string, unknown>;
}

export interface DocumentsPage {
  page: number;
  size: number;
  total: number;
  items: Document[];
}

export interface DocumentListParams {
  page?: number;
  size?: number;
  query?: string;
  type?: string;
  id?: number[];
  include_deleted?: boolean;
  include_descendants?: boolean;
  metadataClauses?: MetadataFilterClause[];
}

export interface DocumentReorderPayload {
  ordered_ids: number[];
  type?: string | null;
}

export interface DocumentTrashParams {
  page?: number;
  size?: number;
  query?: string;
}

export interface DocumentTrashPage {
  page: number;
  size: number;
  total: number;
  items: Document[];
}

export interface DocumentVersion {
  document_id: number;
  version_number: number;
  title: string;
  type?: string;
  metadata?: Record<string, unknown>;
  content?: Record<string, unknown>;
  created_by: string;
  created_at: string;
  change_message?: string | null;
}

export interface DocumentVersionsPage {
  page: number;
  size: number;
  total: number;
  versions?: DocumentVersion[];
  items?: DocumentVersion[];
}

export interface DocumentBinding {
  node_id: number;
  node_name: string;
  node_path: string;
  created_at: string;
  created_by: string;
}

export interface DocumentBindingStatus {
  total_bindings: number;
  node_ids: number[];
}

export interface DocumentBatchBindPayload {
  node_ids: number[];
}

function buildDocumentQuery(params?: DocumentListParams): string {
  if (!params) return "";
  const flatParams: Record<string, unknown> = {};
  const { metadataClauses, ...rest } = params;
  Object.assign(flatParams, rest);

  if (metadataClauses && metadataClauses.length > 0) {
    const metadataParams = buildMetadataQueryParams(metadataClauses);
    Object.entries(metadataParams).forEach(([key, value]) => {
      if (flatParams[key]) {
        const prev = flatParams[key];
        if (Array.isArray(prev)) {
          flatParams[key] = Array.isArray(value) ? [...prev, ...value] : [...prev, value];
        } else {
          flatParams[key] = Array.isArray(value) ? [prev, ...value] : [prev, value];
        }
      } else {
        flatParams[key] = value;
      }
    });
  }
  return buildQuery(flatParams);
}

function buildMetadataQueryParams(
  clauses: MetadataFilterClause[],
): Record<string, string | string[]> {
  const output: Record<string, string | string[]> = {};

  clauses.forEach((clause) => {
    const base = `metadata.${clause.key}`;
    let key = base;
    let values = clause.values;

    switch (clause.operator) {
      case "eq":
        key = base;
        break;
      case "like":
        key = `${base}[like]`;
        values = [normalizeLikeValue(values[0])];
        break;
      case "in":
        key = `${base}[in]`;
        values = [values.join(",")];
        break;
      case "any":
        key = `${base}[any]`;
        values = [values.join(",")];
        break;
      case "all":
        key = `${base}[all]`;
        appendValue(output, key, values);
        return;
      case "gt":
      case "gte":
      case "lt":
      case "lte":
        key = `${base}[${clause.operator}]`;
        values = [values[0]];
        break;
      default:
        key = base;
    }

    appendValue(output, key, values);
  });

  return output;
}

function appendValue(
  target: Record<string, string | string[]>,
  key: string,
  values: string[],
) {
  if (values.length === 0) {
    return;
  }
  const nextValue = values.length === 1 ? values[0] : values;
  if (Object.prototype.hasOwnProperty.call(target, key)) {
    const existing = target[key];
    if (Array.isArray(existing)) {
      target[key] = existing.concat(values);
    } else {
      target[key] = [existing, ...values];
    }
  } else {
    target[key] = nextValue;
  }
}

function normalizeLikeValue(input: string): string {
  if (input.includes("%") || input.includes("_")) {
    return input;
  }
  return `%${input}%`;
}

export async function getDocuments(params?: DocumentListParams): Promise<DocumentsPage> {
  const query = buildDocumentQuery(params);
  return http<DocumentsPage>(`/api/v1/documents${query}`);
}

export async function getDocumentDetail(docId: number): Promise<Document> {
  return http<Document>(`/api/v1/documents/${docId}`);
}

export async function getNodeDocuments(
  nodeId: number,
  params?: DocumentListParams,
): Promise<DocumentsPage> {
  const query = buildDocumentQuery(params);
  return http<DocumentsPage>(`/api/v1/nodes/${nodeId}/subtree-documents${query}`);
}

export async function createDocument(payload: DocumentCreatePayload): Promise<Document> {
  return http<Document>("/api/v1/documents", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function updateDocument(docId: number, payload: DocumentUpdatePayload): Promise<Document> {
  return http<Document>(`/api/v1/documents/${docId}`, {
    method: "PUT",
    body: JSON.stringify(payload),
  });
}

export async function reorderDocuments(payload: DocumentReorderPayload): Promise<Document[]> {
  return http<Document[]>("/api/v1/documents/reorder", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function bindDocument(nodeId: number, docId: number): Promise<void> {
  await http<void>(`/api/v1/nodes/${nodeId}/bind/${docId}`, {
    method: "POST",
  });
}

export async function unbindDocument(nodeId: number, docId: number): Promise<void> {
  await http<void>(`/api/v1/nodes/${nodeId}/unbind/${docId}`, {
    method: "DELETE",
  });
}

export async function getDocumentBindings(docId: number): Promise<DocumentBinding[]> {
  return http<DocumentBinding[]>(`/api/v1/documents/${docId}/bindings`);
}

export async function getDocumentBindingStatus(docId: number): Promise<DocumentBindingStatus> {
  return http<DocumentBindingStatus>(`/api/v1/documents/${docId}/binding-status`);
}

export async function batchBindDocument(docId: number, payload: DocumentBatchBindPayload): Promise<DocumentBinding[]> {
  return http<DocumentBinding[]>(`/api/v1/documents/${docId}/batch-bind`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function deleteDocument(docId: number): Promise<void> {
  await http<void>(`/api/v1/documents/${docId}`, { method: "DELETE" });
}

export async function restoreDocument(docId: number): Promise<Document> {
  return http<Document>(`/api/v1/documents/${docId}/restore`, { method: "POST" });
}

export async function purgeDocument(docId: number): Promise<void> {
  await http<void>(`/api/v1/documents/${docId}/purge`, { method: "DELETE" });
}

export async function getDeletedDocuments(params?: DocumentTrashParams): Promise<DocumentTrashPage> {
  const query = buildQuery({
    page: params?.page,
    size: params?.size,
    query: params?.query,
  });
  return http<DocumentTrashPage>(`/api/v1/documents/trash${query}`);
}

export async function getDocumentVersions(
  docId: number,
  params?: { page?: number; size?: number },
): Promise<DocumentVersionsPage> {
  const query = buildQuery({
    page: params?.page,
    size: params?.size,
  });
  return http<DocumentVersionsPage>(`/api/v1/documents/${docId}/versions${query}`);
}

export async function restoreDocumentVersion(docId: number, versionNumber: number): Promise<Document> {
  return http<Document>(`/api/v1/documents/${docId}/versions/${versionNumber}/restore`, {
    method: "POST",
  });
}

// Document references
export interface DocumentReference {
  document_id: number;
  title: string;
  added_at: string;
}

export interface AddReferencePayload {
  document_id: number;
}

export interface ReferencingDocumentsResponse {
  referencing_documents: Document[];
  total: number;
}

export async function addDocumentReference(
  docId: number,
  payload: AddReferencePayload
): Promise<Document> {
  return http<Document>(`/api/v1/documents/${docId}/references`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export async function removeDocumentReference(
  docId: number,
  refDocId: number
): Promise<Document> {
  return http<Document>(`/api/v1/documents/${docId}/references/${refDocId}`, {
    method: "DELETE",
  });
}

export async function getReferencingDocuments(
  docId: number,
  params?: DocumentListParams
): Promise<ReferencingDocumentsResponse> {
  const query = buildDocumentQuery(params);
  return http<ReferencingDocumentsResponse>(
    `/api/v1/documents/${docId}/referencing${query}`
  );
}

// Source documents (workflow input)
export interface SourceDocInfo {
  id: number;
  title: string;
  type?: string | null;
}

export interface SourceDocument {
  node_id: number;
  document_id: number;
  relation_type: string;
  document?: SourceDocInfo;
}

export interface SourceRelation {
  node_id: number;
  document_id: number;
  relation_type: string;
}

export async function getNodeSourceDocuments(nodeId: number): Promise<SourceDocument[]> {
  return http<SourceDocument[]>(`/api/v1/nodes/${nodeId}/sources`);
}

export async function bindSourceDocument(nodeId: number, docId: number): Promise<SourceRelation> {
  return http<SourceRelation>(`/api/v1/nodes/${nodeId}/sources?document_id=${docId}`, {
    method: "POST",
  });
}

export async function unbindSourceDocument(nodeId: number, docId: number): Promise<void> {
  await http<void>(`/api/v1/nodes/${nodeId}/sources/${docId}`, {
    method: "DELETE",
  });
}

// Document copy
export interface DocumentCopyRequest {
  title?: string;
  node_id?: number;
}

export interface DocumentCopyResponse {
  new_document_id: number;
  title: string;
  message: string;
}

export async function copyDocument(
  docId: number,
  payload?: DocumentCopyRequest
): Promise<DocumentCopyResponse> {
  return http<DocumentCopyResponse>(`/api/v1/documents/${docId}/copy`, {
    method: "POST",
    body: JSON.stringify(payload || {}),
  });
}
