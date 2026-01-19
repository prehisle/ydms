package ndrclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"time"
)

// Client defines the contract for interacting with the upstream NDR service.
type Client interface {
	Ping(ctx context.Context) error
	CreateNode(ctx context.Context, meta RequestMeta, body NodeCreate) (Node, error)
	GetNode(ctx context.Context, meta RequestMeta, id int64, opts GetNodeOptions) (Node, error)
	GetNodeByPath(ctx context.Context, meta RequestMeta, path string, opts GetNodeOptions) (Node, error)
	HasChildren(ctx context.Context, meta RequestMeta, id int64) (bool, error)
	UpdateNode(ctx context.Context, meta RequestMeta, id int64, body NodeUpdate) (Node, error)
	DeleteNode(ctx context.Context, meta RequestMeta, id int64) error
	RestoreNode(ctx context.Context, meta RequestMeta, id int64) (Node, error)
	ListNodes(ctx context.Context, meta RequestMeta, params ListNodesParams) (NodesPage, error)
	ListChildren(ctx context.Context, meta RequestMeta, id int64, params ListChildrenParams) ([]Node, error)
	ReorderNodes(ctx context.Context, meta RequestMeta, payload NodeReorderPayload) ([]Node, error)
	PurgeNode(ctx context.Context, meta RequestMeta, id int64) error
	ListDocuments(ctx context.Context, meta RequestMeta, query url.Values) (DocumentsPage, error)
	ListNodeDocuments(ctx context.Context, meta RequestMeta, id int64, query url.Values) (DocumentsPage, error)
	ListNodeDocumentsByPath(ctx context.Context, meta RequestMeta, path string, query url.Values) (DocumentsPage, error)
	CreateDocument(ctx context.Context, meta RequestMeta, body DocumentCreate) (Document, error)
	GetDocument(ctx context.Context, meta RequestMeta, docID int64) (Document, error)
	ReorderDocuments(ctx context.Context, meta RequestMeta, payload DocumentReorderPayload) ([]Document, error)
	UpdateDocument(ctx context.Context, meta RequestMeta, docID int64, body DocumentUpdate) (Document, error)
	DeleteDocument(ctx context.Context, meta RequestMeta, docID int64) error
	RestoreDocument(ctx context.Context, meta RequestMeta, docID int64) (Document, error)
	PurgeDocument(ctx context.Context, meta RequestMeta, docID int64) error
	BindDocument(ctx context.Context, meta RequestMeta, nodeID, docID int64) error
	UnbindDocument(ctx context.Context, meta RequestMeta, nodeID, docID int64) error
	BindRelationship(ctx context.Context, meta RequestMeta, nodeID, docID int64) (Relationship, error)
	UnbindRelationship(ctx context.Context, meta RequestMeta, nodeID, docID int64) error
	ListRelationships(ctx context.Context, meta RequestMeta, nodeID, docID *int64) ([]Relationship, error)
	GetDocumentBindingStatus(ctx context.Context, meta RequestMeta, docID int64) (DocumentBindingStatus, error)
	GetDocumentBindings(ctx context.Context, meta RequestMeta, docID int64) ([]DocumentBinding, error)
	ListDocumentVersions(ctx context.Context, meta RequestMeta, docID int64, page, size int) (DocumentVersionsPage, error)
	GetDocumentVersion(ctx context.Context, meta RequestMeta, docID int64, versionNumber int) (DocumentVersion, error)
	GetDocumentVersionDiff(ctx context.Context, meta RequestMeta, docID int64, fromVersion, toVersion int) (DocumentVersionDiff, error)
	RestoreDocumentVersion(ctx context.Context, meta RequestMeta, docID int64, versionNumber int) (Document, error)

	// Source document methods (workflow input)
	BindSourceDocument(ctx context.Context, meta RequestMeta, nodeID, docID int64) (SourceRelation, error)
	UnbindSourceDocument(ctx context.Context, meta RequestMeta, nodeID, docID int64) error
	ListSourceDocuments(ctx context.Context, meta RequestMeta, nodeID int64) ([]SourceDocument, error)

	// Asset methods
	InitMultipartUpload(ctx context.Context, meta RequestMeta, req AssetInitRequest) (AssetInitResponse, error)
	GetAssetPartURLs(ctx context.Context, meta RequestMeta, assetID int64, partNumbers []int) (AssetPartURLsResponse, error)
	CompleteMultipartUpload(ctx context.Context, meta RequestMeta, assetID int64, parts []AssetCompletedPart) (Asset, error)
	AbortMultipartUpload(ctx context.Context, meta RequestMeta, assetID int64) error
	GetAsset(ctx context.Context, meta RequestMeta, assetID int64) (Asset, error)
	GetAssetDownloadURL(ctx context.Context, meta RequestMeta, assetID int64) (AssetDownloadURLResponse, error)
	DeleteAsset(ctx context.Context, meta RequestMeta, assetID int64) error
}

// NDRConfig describes the minimal configuration required by the client.
type NDRConfig struct {
	BaseURL string
	APIKey  string
	Debug   bool
}

// RequestMeta contains per-request metadata forwarded to NDR.
type RequestMeta struct {
	APIKey    string
	UserID    string
	RequestID string
	AdminKey  string
}

// Error represents an HTTP error returned by the NDR service.
type Error struct {
	StatusCode int
	Status     string
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("ndr request failed: %s", e.Status)
}

type httpClient struct {
	baseURL    *url.URL
	apiKey     string
	httpClient *http.Client
	debug      bool
}

// NewClient returns an HTTP backed NDR client.
func NewClient(cfg NDRConfig) Client {
	var parsed *url.URL
	if cfg.BaseURL != "" {
		parsed, _ = url.Parse(cfg.BaseURL)
	}
	return &httpClient{
		baseURL:    parsed,
		apiKey:     cfg.APIKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		debug:      cfg.Debug,
	}
}

func (c *httpClient) Ping(ctx context.Context) error {
	req, err := c.newRequest(ctx, http.MethodGet, "/ready", RequestMeta{}, nil)
	if err != nil {
		return err
	}
	_, err = c.do(req, nil)
	return err
}

func (c *httpClient) CreateNode(ctx context.Context, meta RequestMeta, body NodeCreate) (Node, error) {
	req, err := c.newRequest(ctx, http.MethodPost, "/api/v1/nodes", meta, body)
	if err != nil {
		return Node{}, err
	}
	var resp Node
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) GetNode(ctx context.Context, meta RequestMeta, id int64, opts GetNodeOptions) (Node, error) {
	endpoint := fmt.Sprintf("/api/v1/nodes/%d", id)
	query := url.Values{}
	if opts.IncludeDeleted != nil {
		query.Set("include_deleted", fmt.Sprintf("%t", *opts.IncludeDeleted))
	}
	req, err := c.newRequestWithQuery(ctx, http.MethodGet, endpoint, meta, nil, query)
	if err != nil {
		return Node{}, err
	}
	var resp Node
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) GetNodeByPath(ctx context.Context, meta RequestMeta, nodePath string, opts GetNodeOptions) (Node, error) {
	query := url.Values{}
	query.Set("path", nodePath)
	if opts.IncludeDeleted != nil {
		query.Set("include_deleted", fmt.Sprintf("%t", *opts.IncludeDeleted))
	}
	req, err := c.newRequestWithQuery(ctx, http.MethodGet, "/api/v1/nodes/by-path", meta, nil, query)
	if err != nil {
		return Node{}, err
	}
	var resp Node
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) HasChildren(ctx context.Context, meta RequestMeta, id int64) (bool, error) {
	children, err := c.ListChildren(ctx, meta, id, ListChildrenParams{})
	if err != nil {
		return false, err
	}
	return len(children) > 0, nil
}

func (c *httpClient) UpdateNode(ctx context.Context, meta RequestMeta, id int64, body NodeUpdate) (Node, error) {
	endpoint := fmt.Sprintf("/api/v1/nodes/%d", id)
	req, err := c.newRequest(ctx, http.MethodPut, endpoint, meta, body)
	if err != nil {
		return Node{}, err
	}
	var resp Node
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) DeleteNode(ctx context.Context, meta RequestMeta, id int64) error {
	endpoint := fmt.Sprintf("/api/v1/nodes/%d", id)
	req, err := c.newRequest(ctx, http.MethodDelete, endpoint, meta, nil)
	if err != nil {
		return err
	}
	_, err = c.do(req, nil)
	return err
}

func (c *httpClient) RestoreNode(ctx context.Context, meta RequestMeta, id int64) (Node, error) {
	endpoint := fmt.Sprintf("/api/v1/nodes/%d/restore", id)
	req, err := c.newRequest(ctx, http.MethodPost, endpoint, meta, nil)
	if err != nil {
		return Node{}, err
	}
	var resp Node
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) ListNodes(ctx context.Context, meta RequestMeta, params ListNodesParams) (NodesPage, error) {
	query := url.Values{}
	if params.Page > 0 {
		query.Set("page", fmt.Sprintf("%d", params.Page))
	}
	if params.Size > 0 {
		query.Set("size", fmt.Sprintf("%d", params.Size))
	}
	if params.IncludeDeleted != nil {
		query.Set("include_deleted", fmt.Sprintf("%t", *params.IncludeDeleted))
	}
	req, err := c.newRequestWithQuery(ctx, http.MethodGet, "/api/v1/nodes", meta, nil, query)
	if err != nil {
		return NodesPage{}, err
	}
	var resp NodesPage
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) ListChildren(ctx context.Context, meta RequestMeta, id int64, params ListChildrenParams) ([]Node, error) {
	endpoint := fmt.Sprintf("/api/v1/nodes/%d/children", id)
	query := url.Values{}
	if params.Depth > 0 {
		query.Set("depth", fmt.Sprintf("%d", params.Depth))
	}
	req, err := c.newRequestWithQuery(ctx, http.MethodGet, endpoint, meta, nil, query)
	if err != nil {
		return nil, err
	}
	var resp []Node
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) ReorderNodes(ctx context.Context, meta RequestMeta, payload NodeReorderPayload) ([]Node, error) {
	req, err := c.newRequest(ctx, http.MethodPost, "/api/v1/nodes/reorder", meta, payload)
	if err != nil {
		return nil, err
	}
	var resp []Node
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) PurgeNode(ctx context.Context, meta RequestMeta, id int64) error {
	endpoint := fmt.Sprintf("/api/v1/nodes/%d/purge", id)
	req, err := c.newRequest(ctx, http.MethodDelete, endpoint, meta, nil)
	if err != nil {
		return err
	}
	_, err = c.do(req, nil)
	return err
}

func (c *httpClient) ListDocuments(ctx context.Context, meta RequestMeta, query url.Values) (DocumentsPage, error) {
	req, err := c.newRequestWithQuery(ctx, http.MethodGet, "/api/v1/documents", meta, nil, query)
	if err != nil {
		return DocumentsPage{}, err
	}
	var resp DocumentsPage
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) ListNodeDocuments(ctx context.Context, meta RequestMeta, id int64, query url.Values) (DocumentsPage, error) {
	endpoint := fmt.Sprintf("/api/v1/nodes/%d/subtree-documents", id)
	req, err := c.newRequestWithQuery(ctx, http.MethodGet, endpoint, meta, nil, query)
	if err != nil {
		return DocumentsPage{}, err
	}
	var resp DocumentsPage
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) ListNodeDocumentsByPath(ctx context.Context, meta RequestMeta, nodePath string, query url.Values) (DocumentsPage, error) {
	if query == nil {
		query = url.Values{}
	}
	query.Set("path", nodePath)
	req, err := c.newRequestWithQuery(ctx, http.MethodGet, "/api/v1/nodes/by-path/subtree-documents", meta, nil, query)
	if err != nil {
		return DocumentsPage{}, err
	}
	var resp DocumentsPage
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) CreateDocument(ctx context.Context, meta RequestMeta, body DocumentCreate) (Document, error) {
	req, err := c.newRequest(ctx, http.MethodPost, "/api/v1/documents", meta, body)
	if err != nil {
		return Document{}, err
	}
	var resp Document
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) GetDocument(ctx context.Context, meta RequestMeta, docID int64) (Document, error) {
	endpoint := fmt.Sprintf("/api/v1/documents/%d", docID)
	req, err := c.newRequest(ctx, http.MethodGet, endpoint, meta, nil)
	if err != nil {
		return Document{}, err
	}
	var resp Document
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) ReorderDocuments(ctx context.Context, meta RequestMeta, payload DocumentReorderPayload) ([]Document, error) {
	req, err := c.newRequest(ctx, http.MethodPost, "/api/v1/documents/reorder", meta, payload)
	if err != nil {
		return nil, err
	}
	var resp []Document
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) UpdateDocument(ctx context.Context, meta RequestMeta, docID int64, body DocumentUpdate) (Document, error) {
	endpoint := fmt.Sprintf("/api/v1/documents/%d", docID)
	req, err := c.newRequest(ctx, http.MethodPut, endpoint, meta, body)
	if err != nil {
		return Document{}, err
	}
	var resp Document
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) DeleteDocument(ctx context.Context, meta RequestMeta, docID int64) error {
	endpoint := fmt.Sprintf("/api/v1/documents/%d", docID)
	req, err := c.newRequest(ctx, http.MethodDelete, endpoint, meta, nil)
	if err != nil {
		return err
	}
	_, err = c.do(req, nil)
	return err
}

func (c *httpClient) BindDocument(ctx context.Context, meta RequestMeta, nodeID, docID int64) error {
	endpoint := fmt.Sprintf("/api/v1/nodes/%d/bind/%d", nodeID, docID)
	req, err := c.newRequest(ctx, http.MethodPost, endpoint, meta, nil)
	if err != nil {
		return err
	}
	_, err = c.do(req, nil)
	return err
}

func (c *httpClient) UnbindDocument(ctx context.Context, meta RequestMeta, nodeID, docID int64) error {
	endpoint := fmt.Sprintf("/api/v1/nodes/%d/unbind/%d", nodeID, docID)
	req, err := c.newRequest(ctx, http.MethodDelete, endpoint, meta, nil)
	if err != nil {
		return err
	}
	_, err = c.do(req, nil)
	return err
}

func (c *httpClient) RestoreDocument(ctx context.Context, meta RequestMeta, docID int64) (Document, error) {
	endpoint := fmt.Sprintf("/api/v1/documents/%d/restore", docID)
	req, err := c.newRequest(ctx, http.MethodPost, endpoint, meta, nil)
	if err != nil {
		return Document{}, err
	}
	var resp Document
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) PurgeDocument(ctx context.Context, meta RequestMeta, docID int64) error {
	endpoint := fmt.Sprintf("/api/v1/documents/%d/purge", docID)
	req, err := c.newRequest(ctx, http.MethodDelete, endpoint, meta, nil)
	if err != nil {
		return err
	}
	_, err = c.do(req, nil)
	return err
}

func (c *httpClient) BindRelationship(ctx context.Context, meta RequestMeta, nodeID, docID int64) (Relationship, error) {
	query := url.Values{}
	query.Set("node_id", fmt.Sprintf("%d", nodeID))
	query.Set("document_id", fmt.Sprintf("%d", docID))
	req, err := c.newRequestWithQuery(ctx, http.MethodPost, "/api/v1/relationships", meta, nil, query)
	if err != nil {
		return Relationship{}, err
	}
	var resp Relationship
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) UnbindRelationship(ctx context.Context, meta RequestMeta, nodeID, docID int64) error {
	query := url.Values{}
	query.Set("node_id", fmt.Sprintf("%d", nodeID))
	query.Set("document_id", fmt.Sprintf("%d", docID))
	req, err := c.newRequestWithQuery(ctx, http.MethodDelete, "/api/v1/relationships", meta, nil, query)
	if err != nil {
		return err
	}
	_, err = c.do(req, nil)
	return err
}

func (c *httpClient) ListRelationships(ctx context.Context, meta RequestMeta, nodeID, docID *int64) ([]Relationship, error) {
	query := url.Values{}
	if nodeID != nil {
		query.Set("node_id", fmt.Sprintf("%d", *nodeID))
	}
	if docID != nil {
		query.Set("document_id", fmt.Sprintf("%d", *docID))
	}
	req, err := c.newRequestWithQuery(ctx, http.MethodGet, "/api/v1/relationships", meta, nil, query)
	if err != nil {
		return nil, err
	}
	var resp []Relationship
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) GetDocumentBindingStatus(ctx context.Context, meta RequestMeta, docID int64) (DocumentBindingStatus, error) {
	endpoint := fmt.Sprintf("/api/v1/documents/%d/binding-status", docID)
	req, err := c.newRequest(ctx, http.MethodGet, endpoint, meta, nil)
	if err != nil {
		return DocumentBindingStatus{}, err
	}
	var resp DocumentBindingStatus
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) GetDocumentBindings(ctx context.Context, meta RequestMeta, docID int64) ([]DocumentBinding, error) {
	endpoint := fmt.Sprintf("/api/v1/documents/%d/bindings", docID)
	req, err := c.newRequest(ctx, http.MethodGet, endpoint, meta, nil)
	if err != nil {
		return nil, err
	}
	var resp []DocumentBinding
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) ListDocumentVersions(ctx context.Context, meta RequestMeta, docID int64, page, size int) (DocumentVersionsPage, error) {
	endpoint := fmt.Sprintf("/api/v1/documents/%d/versions", docID)
	query := url.Values{}
	if page > 0 {
		query.Set("page", fmt.Sprintf("%d", page))
	}
	if size > 0 {
		query.Set("size", fmt.Sprintf("%d", size))
	}
	req, err := c.newRequestWithQuery(ctx, http.MethodGet, endpoint, meta, nil, query)
	if err != nil {
		return DocumentVersionsPage{}, err
	}
	var resp DocumentVersionsPage
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) GetDocumentVersion(ctx context.Context, meta RequestMeta, docID int64, versionNumber int) (DocumentVersion, error) {
	endpoint := fmt.Sprintf("/api/v1/documents/%d/versions/%d", docID, versionNumber)
	req, err := c.newRequest(ctx, http.MethodGet, endpoint, meta, nil)
	if err != nil {
		return DocumentVersion{}, err
	}
	var resp DocumentVersion
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) GetDocumentVersionDiff(ctx context.Context, meta RequestMeta, docID int64, fromVersion, toVersion int) (DocumentVersionDiff, error) {
	endpoint := fmt.Sprintf("/api/v1/documents/%d/versions/%d/diff", docID, fromVersion)
	query := url.Values{}
	query.Set("to", fmt.Sprintf("%d", toVersion))
	req, err := c.newRequestWithQuery(ctx, http.MethodGet, endpoint, meta, nil, query)
	if err != nil {
		return DocumentVersionDiff{}, err
	}
	var resp DocumentVersionDiff
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) RestoreDocumentVersion(ctx context.Context, meta RequestMeta, docID int64, versionNumber int) (Document, error) {
	endpoint := fmt.Sprintf("/api/v1/documents/%d/versions/%d/restore", docID, versionNumber)
	req, err := c.newRequest(ctx, http.MethodPost, endpoint, meta, nil)
	if err != nil {
		return Document{}, err
	}
	var resp Document
	_, err = c.do(req, &resp)
	return resp, err
}

// BindSourceDocument binds a document as a source document to a node.
func (c *httpClient) BindSourceDocument(ctx context.Context, meta RequestMeta, nodeID, docID int64) (SourceRelation, error) {
	endpoint := fmt.Sprintf("/api/v1/nodes/%d/sources", nodeID)
	query := url.Values{}
	query.Set("document_id", fmt.Sprintf("%d", docID))
	req, err := c.newRequestWithQuery(ctx, http.MethodPost, endpoint, meta, nil, query)
	if err != nil {
		return SourceRelation{}, err
	}
	var resp SourceRelation
	_, err = c.do(req, &resp)
	return resp, err
}

// UnbindSourceDocument removes a source document from a node.
func (c *httpClient) UnbindSourceDocument(ctx context.Context, meta RequestMeta, nodeID, docID int64) error {
	endpoint := fmt.Sprintf("/api/v1/nodes/%d/sources/%d", nodeID, docID)
	req, err := c.newRequest(ctx, http.MethodDelete, endpoint, meta, nil)
	if err != nil {
		return err
	}
	_, err = c.do(req, nil)
	return err
}

// ListSourceDocuments lists all source documents for a node.
func (c *httpClient) ListSourceDocuments(ctx context.Context, meta RequestMeta, nodeID int64) ([]SourceDocument, error) {
	endpoint := fmt.Sprintf("/api/v1/nodes/%d/sources", nodeID)
	req, err := c.newRequest(ctx, http.MethodGet, endpoint, meta, nil)
	if err != nil {
		return nil, err
	}
	var resp []SourceDocument
	_, err = c.do(req, &resp)
	return resp, err
}

func (c *httpClient) newRequest(ctx context.Context, method, endpoint string, meta RequestMeta, body any) (*http.Request, error) {
	return c.newRequestWithQuery(ctx, method, endpoint, meta, body, nil)
}

func (c *httpClient) newRequestWithQuery(ctx context.Context, method, endpoint string, meta RequestMeta, body any, query url.Values) (*http.Request, error) {
	if c.baseURL == nil {
		return nil, fmt.Errorf("ndr base url is not configured")
	}
	var payload io.Reader
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(bodyBytes)
	}
	fullURL := *c.baseURL
	fullURL.Path = path.Join(c.baseURL.Path, endpoint)
	if query != nil {
		fullURL.RawQuery = query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL.String(), payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	apiKey := meta.APIKey
	if apiKey == "" {
		apiKey = c.apiKey
	}
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	if meta.UserID != "" {
		req.Header.Set("x-user-id", meta.UserID)
	}
	if meta.RequestID != "" {
		req.Header.Set("x-request-id", meta.RequestID)
	}
	if meta.AdminKey != "" {
		req.Header.Set("x-admin-key", meta.AdminKey)
	}
	if c.debug {
		log.Printf("[ndr] request %s %s meta=%+v body=%s", method, fullURL.String(), meta, truncateForLog(bodyBytes))
	}
	return req, nil
}

func (c *httpClient) do(req *http.Request, out any) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, err
	}

	if c.debug {
		log.Printf("[ndr] response %s %s status=%d body=%s", req.Method, req.URL.Path, resp.StatusCode, truncateForLog(respBody))
	}

	if resp.StatusCode >= 400 {
		return resp, &Error{StatusCode: resp.StatusCode, Status: resp.Status}
	}
	if out != nil {
		if len(respBody) == 0 {
			return resp, nil
		}
		if err := json.Unmarshal(respBody, out); err != nil {
			return resp, err
		}
	}
	return resp, nil
}

func truncateForLog(data []byte) string {
	if len(data) == 0 {
		return "<empty>"
	}
	const limit = 2048
	if len(data) <= limit {
		return string(data)
	}
	return fmt.Sprintf("%s...(truncated %d bytes)", string(data[:limit]), len(data)-limit)
}
