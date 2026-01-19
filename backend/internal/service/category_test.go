package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/yjxt/ydms/backend/internal/cache"
	"github.com/yjxt/ydms/backend/internal/ndrclient"
)

type fakeNDR struct {
	pingErr      error
	createdNodes []ndrclient.NodeCreate
	updatedNodes []struct {
		ID   int64
		Body ndrclient.NodeUpdate
	}
	deletedNodes  []int64
	restoredNodes []int64
	purgedNodes   []int64
	listResponse  ndrclient.NodesPage
	listResponses map[int]ndrclient.NodesPage
	getNodes      map[int64]ndrclient.Node
	createResp    ndrclient.Node
	updateResp    ndrclient.Node
	restoreResp   ndrclient.Node
	reorderResp   []ndrclient.Node
	reorderErr    error
	reorderInput  *ndrclient.NodeReorderPayload
	listErr       error
	createErr     error
	updateErr     error
	deleteErr     error
	getErr        error
	restoreErr    error
	purgeErr      error

	// Document-related fields
	createdDocs []ndrclient.DocumentCreate
	updatedDocs []struct {
		ID   int64
		Body ndrclient.DocumentUpdate
	}
	createDocResp      ndrclient.Document
	updateDocResp      ndrclient.Document
	createDocErr       error
	updateDocErr       error
	docsListResp       ndrclient.DocumentsPage
	nodeDocsResp       []ndrclient.Document
	docsListErr        error
	nodeDocsErr        error
	docVersionsResp    ndrclient.DocumentVersionsPage
	docVersionsErr     error
	deleteDocErr       error
	restoreDocErr      error
	restoreDocResp     ndrclient.Document
	purgeDocErr        error
	getDocResp         ndrclient.Document
	getDocErr          error
	deletedDocIDs      []int64
	restoredDocIDs     []int64
	purgedDocIDs       []int64
	docBindings        map[int64]map[int64]struct{}
	reorderDocPayloads []ndrclient.DocumentReorderPayload
	reorderDocResp     []ndrclient.Document
	reorderDocErr      error
}

func newFakeNDR() *fakeNDR {
	return &fakeNDR{
		getNodes:      make(map[int64]ndrclient.Node),
		listResponses: make(map[int]ndrclient.NodesPage),
		docBindings:   make(map[int64]map[int64]struct{}),
	}
}

func (f *fakeNDR) Ping(context.Context) error {
	return f.pingErr
}

func (f *fakeNDR) CreateNode(_ context.Context, _ ndrclient.RequestMeta, body ndrclient.NodeCreate) (ndrclient.Node, error) {
	f.createdNodes = append(f.createdNodes, body)
	return f.createResp, f.createErr
}

func (f *fakeNDR) GetNode(_ context.Context, _ ndrclient.RequestMeta, id int64, _ ndrclient.GetNodeOptions) (ndrclient.Node, error) {
	if f.getErr != nil {
		return ndrclient.Node{}, f.getErr
	}
	node, ok := f.getNodes[id]
	if !ok {
		return ndrclient.Node{}, errors.New("not found")
	}
	return node, nil
}

func (f *fakeNDR) UpdateNode(_ context.Context, _ ndrclient.RequestMeta, id int64, body ndrclient.NodeUpdate) (ndrclient.Node, error) {
	f.updatedNodes = append(f.updatedNodes, struct {
		ID   int64
		Body ndrclient.NodeUpdate
	}{ID: id, Body: body})
	return f.updateResp, f.updateErr
}

func (f *fakeNDR) DeleteNode(_ context.Context, _ ndrclient.RequestMeta, id int64) error {
	f.deletedNodes = append(f.deletedNodes, id)
	return f.deleteErr
}

func (f *fakeNDR) RestoreNode(_ context.Context, _ ndrclient.RequestMeta, id int64) (ndrclient.Node, error) {
	f.restoredNodes = append(f.restoredNodes, id)
	return f.restoreResp, f.restoreErr
}

func (f *fakeNDR) ListNodes(_ context.Context, _ ndrclient.RequestMeta, params ndrclient.ListNodesParams) (ndrclient.NodesPage, error) {
	if f.listErr != nil {
		return ndrclient.NodesPage{}, f.listErr
	}
	if len(f.listResponses) > 0 {
		if page, ok := f.listResponses[params.Page]; ok {
			return page, nil
		}
		return ndrclient.NodesPage{
			Page:  params.Page,
			Size:  params.Size,
			Total: 0,
			Items: []ndrclient.Node{},
		}, nil
	}
	return f.listResponse, f.listErr
}

func (f *fakeNDR) ListChildren(context.Context, ndrclient.RequestMeta, int64, ndrclient.ListChildrenParams) ([]ndrclient.Node, error) {
	return nil, nil
}

func (f *fakeNDR) HasChildren(_ context.Context, _ ndrclient.RequestMeta, id int64) (bool, error) {
	for _, node := range f.getNodes {
		if node.ParentID != nil && *node.ParentID == id {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeNDR) ReorderNodes(_ context.Context, _ ndrclient.RequestMeta, payload ndrclient.NodeReorderPayload) ([]ndrclient.Node, error) {
	f.reorderInput = &payload
	return f.reorderResp, f.reorderErr
}

func (f *fakeNDR) PurgeNode(_ context.Context, _ ndrclient.RequestMeta, id int64) error {
	f.purgedNodes = append(f.purgedNodes, id)
	return f.purgeErr
}

func (f *fakeNDR) ListDocuments(context.Context, ndrclient.RequestMeta, url.Values) (ndrclient.DocumentsPage, error) {
	return f.docsListResp, f.docsListErr
}

func (f *fakeNDR) ListNodeDocuments(context.Context, ndrclient.RequestMeta, int64, url.Values) (ndrclient.DocumentsPage, error) {
	if f.nodeDocsErr != nil {
		return ndrclient.DocumentsPage{}, f.nodeDocsErr
	}
	return ndrclient.DocumentsPage{
		Page:  1,
		Size:  100,
		Total: len(f.nodeDocsResp),
		Items: f.nodeDocsResp,
	}, nil
}

func (f *fakeNDR) CreateDocument(_ context.Context, _ ndrclient.RequestMeta, body ndrclient.DocumentCreate) (ndrclient.Document, error) {
	f.createdDocs = append(f.createdDocs, body)
	return f.createDocResp, f.createDocErr
}

func (f *fakeNDR) ReorderDocuments(_ context.Context, _ ndrclient.RequestMeta, payload ndrclient.DocumentReorderPayload) ([]ndrclient.Document, error) {
	f.reorderDocPayloads = append(f.reorderDocPayloads, payload)
	if f.reorderDocErr != nil {
		return nil, f.reorderDocErr
	}
	return f.reorderDocResp, nil
}

func (f *fakeNDR) BindDocument(_ context.Context, _ ndrclient.RequestMeta, nodeID, docID int64) error {
	if f.docBindings == nil {
		f.docBindings = make(map[int64]map[int64]struct{})
	}
	nodeBindings, ok := f.docBindings[docID]
	if !ok {
		nodeBindings = make(map[int64]struct{})
		f.docBindings[docID] = nodeBindings
	}
	nodeBindings[nodeID] = struct{}{}
	return nil
}

func (f *fakeNDR) UpdateDocument(_ context.Context, _ ndrclient.RequestMeta, id int64, body ndrclient.DocumentUpdate) (ndrclient.Document, error) {
	f.updatedDocs = append(f.updatedDocs, struct {
		ID   int64
		Body ndrclient.DocumentUpdate
	}{ID: id, Body: body})
	return f.updateDocResp, f.updateDocErr
}

func (f *fakeNDR) GetDocument(_ context.Context, _ ndrclient.RequestMeta, id int64) (ndrclient.Document, error) {
	if f.getDocErr != nil {
		return ndrclient.Document{}, f.getDocErr
	}
	if f.getDocResp.ID == 0 {
		return ndrclient.Document{ID: id}, nil
	}
	return f.getDocResp, nil
}

func (f *fakeNDR) DeleteDocument(_ context.Context, _ ndrclient.RequestMeta, id int64) error {
	f.deletedDocIDs = append(f.deletedDocIDs, id)
	return f.deleteDocErr
}

func (f *fakeNDR) RestoreDocument(_ context.Context, meta ndrclient.RequestMeta, docID int64) (ndrclient.Document, error) {
	f.restoredDocIDs = append(f.restoredDocIDs, docID)
	if f.restoreDocErr != nil {
		return ndrclient.Document{}, f.restoreDocErr
	}
	if f.restoreDocResp.ID != 0 {
		return f.restoreDocResp, nil
	}
	return ndrclient.Document{ID: docID}, nil
}

func (f *fakeNDR) PurgeDocument(_ context.Context, _ ndrclient.RequestMeta, docID int64) error {
	f.purgedDocIDs = append(f.purgedDocIDs, docID)
	return f.purgeDocErr
}

func (f *fakeNDR) UnbindDocument(_ context.Context, _ ndrclient.RequestMeta, nodeID, docID int64) error {
	if f.docBindings != nil {
		if nodeBindings, ok := f.docBindings[docID]; ok {
			delete(nodeBindings, nodeID)
			if len(nodeBindings) == 0 {
				delete(f.docBindings, docID)
			}
		}
	}
	return nil
}

func (f *fakeNDR) BindRelationship(_ context.Context, meta ndrclient.RequestMeta, nodeID, docID int64) (ndrclient.Relationship, error) {
	return ndrclient.Relationship{
		NodeID:     nodeID,
		DocumentID: docID,
		CreatedBy:  meta.UserID,
	}, nil
}

func (f *fakeNDR) UnbindRelationship(_ context.Context, _ ndrclient.RequestMeta, nodeID, docID int64) error {
	return nil
}

func (f *fakeNDR) ListRelationships(_ context.Context, meta ndrclient.RequestMeta, nodeID, docID *int64) ([]ndrclient.Relationship, error) {
	return []ndrclient.Relationship{}, nil
}

func (f *fakeNDR) GetDocumentBindingStatus(_ context.Context, _ ndrclient.RequestMeta, docID int64) (ndrclient.DocumentBindingStatus, error) {
	var nodeIDs []int64
	if f.docBindings != nil {
		if bindings, ok := f.docBindings[docID]; ok {
			nodeIDs = make([]int64, 0, len(bindings))
			for nodeID := range bindings {
				nodeIDs = append(nodeIDs, nodeID)
			}
		}
	}
	return ndrclient.DocumentBindingStatus{
		TotalBindings: len(nodeIDs),
		NodeIDs:       nodeIDs,
	}, nil
}

func (f *fakeNDR) GetDocumentBindings(_ context.Context, _ ndrclient.RequestMeta, docID int64) ([]ndrclient.DocumentBinding, error) {
	var bindings []ndrclient.DocumentBinding
	if f.docBindings != nil {
		if nodeBindings, ok := f.docBindings[docID]; ok {
			for nodeID := range nodeBindings {
				bindings = append(bindings, ndrclient.DocumentBinding{
					NodeID:   nodeID,
					NodeName: fmt.Sprintf("node-%d", nodeID),
					NodePath: fmt.Sprintf("path.to.node_%d", nodeID),
				})
			}
		}
	}
	return bindings, nil
}

func (f *fakeNDR) ListDocumentVersions(_ context.Context, _ ndrclient.RequestMeta, docID int64, page, size int) (ndrclient.DocumentVersionsPage, error) {
	if f.docVersionsResp.Page == 0 && f.docVersionsResp.Total == 0 && len(f.docVersionsResp.Versions) == 0 && len(f.docVersionsResp.Items) == 0 {
		return ndrclient.DocumentVersionsPage{
			Page:     page,
			Size:     size,
			Total:    0,
			Versions: []ndrclient.DocumentVersion{},
		}, nil
	}
	return f.docVersionsResp, f.docVersionsErr
}

func (f *fakeNDR) GetDocumentVersion(_ context.Context, _ ndrclient.RequestMeta, docID int64, versionNumber int) (ndrclient.DocumentVersion, error) {
	return ndrclient.DocumentVersion{
		DocumentID:    docID,
		VersionNumber: versionNumber,
	}, nil
}

func (f *fakeNDR) GetDocumentVersionDiff(_ context.Context, _ ndrclient.RequestMeta, docID int64, fromVersion, toVersion int) (ndrclient.DocumentVersionDiff, error) {
	return ndrclient.DocumentVersionDiff{
		FromVersion: fromVersion,
		ToVersion:   toVersion,
	}, nil
}

func (f *fakeNDR) RestoreDocumentVersion(_ context.Context, meta ndrclient.RequestMeta, docID int64, versionNumber int) (ndrclient.Document, error) {
	return ndrclient.Document{ID: docID}, nil
}

func (f *fakeNDR) GetNodeByPath(_ context.Context, _ ndrclient.RequestMeta, path string, _ ndrclient.GetNodeOptions) (ndrclient.Node, error) {
	// 通过路径查找节点
	for _, node := range f.getNodes {
		if node.Path == path {
			return node, nil
		}
	}
	return ndrclient.Node{}, errors.New("node not found by path")
}

func (f *fakeNDR) ListNodeDocumentsByPath(_ context.Context, _ ndrclient.RequestMeta, path string, query url.Values) (ndrclient.DocumentsPage, error) {
	// 简单实现：返回模拟的文档列表
	if f.nodeDocsErr != nil {
		return ndrclient.DocumentsPage{}, f.nodeDocsErr
	}
	return ndrclient.DocumentsPage{
		Page:  1,
		Size:  100,
		Total: len(f.nodeDocsResp),
		Items: f.nodeDocsResp,
	}, nil
}

// Asset methods (stub implementations for interface compliance)
func (f *fakeNDR) InitMultipartUpload(_ context.Context, _ ndrclient.RequestMeta, _ ndrclient.AssetInitRequest) (ndrclient.AssetInitResponse, error) {
	return ndrclient.AssetInitResponse{}, nil
}

func (f *fakeNDR) GetAssetPartURLs(_ context.Context, _ ndrclient.RequestMeta, _ int64, _ []int) (ndrclient.AssetPartURLsResponse, error) {
	return ndrclient.AssetPartURLsResponse{}, nil
}

func (f *fakeNDR) CompleteMultipartUpload(_ context.Context, _ ndrclient.RequestMeta, _ int64, _ []ndrclient.AssetCompletedPart) (ndrclient.Asset, error) {
	return ndrclient.Asset{}, nil
}

func (f *fakeNDR) AbortMultipartUpload(_ context.Context, _ ndrclient.RequestMeta, _ int64) error {
	return nil
}

func (f *fakeNDR) GetAsset(_ context.Context, _ ndrclient.RequestMeta, _ int64) (ndrclient.Asset, error) {
	return ndrclient.Asset{}, nil
}

func (f *fakeNDR) GetAssetDownloadURL(_ context.Context, _ ndrclient.RequestMeta, _ int64) (ndrclient.AssetDownloadURLResponse, error) {
	return ndrclient.AssetDownloadURLResponse{}, nil
}

func (f *fakeNDR) DeleteAsset(_ context.Context, _ ndrclient.RequestMeta, _ int64) error {
	return nil
}

// Source document methods (stub implementations for interface compliance)
func (f *fakeNDR) BindSourceDocument(_ context.Context, _ ndrclient.RequestMeta, nodeID, docID int64) (ndrclient.SourceRelation, error) {
	return ndrclient.SourceRelation{NodeID: nodeID, DocumentID: docID, RelationType: "source"}, nil
}

func (f *fakeNDR) UnbindSourceDocument(_ context.Context, _ ndrclient.RequestMeta, _, _ int64) error {
	return nil
}

func (f *fakeNDR) ListSourceDocuments(_ context.Context, _ ndrclient.RequestMeta, _ int64) ([]ndrclient.SourceDocument, error) {
	return []ndrclient.SourceDocument{}, nil
}

func TestCreateCategory(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	fake.createResp = sampleNode(1, "Math", "/math", nil, 1, now, now)
	svc := NewService(cache.NewNoop(), fake, nil)

	cat, err := svc.CreateCategory(context.Background(), RequestMeta{}, CategoryCreateRequest{Name: "Math"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cat.Name != "Math" || cat.Slug != "math" || cat.Path != "/math" {
		t.Fatalf("unexpected category %+v", cat)
	}
	if len(fake.createdNodes) != 1 {
		t.Fatalf("expected one create call, got %d", len(fake.createdNodes))
	}
}

func TestCreateCategoryRequiresName(t *testing.T) {
	fake := newFakeNDR()
	svc := NewService(cache.NewNoop(), fake, nil)

	if _, err := svc.CreateCategory(context.Background(), RequestMeta{}, CategoryCreateRequest{Name: "  "}); err == nil {
		t.Fatalf("expected error for empty name")
	}
}

func TestUpdateCategory(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	fake.updateResp = sampleNode(2, "Science", "/science", nil, 1, now, now)
	svc := NewService(cache.NewNoop(), fake, nil)

	newName := "Science"
	cat, err := svc.UpdateCategory(context.Background(), RequestMeta{}, 2, CategoryUpdateRequest{Name: &newName})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat.Slug != "science" {
		t.Fatalf("expected slug science, got %s", cat.Slug)
	}
	if len(fake.updatedNodes) != 1 {
		t.Fatalf("expected update call")
	}
}

func TestGetCategory(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	node := sampleNode(3, "History", "/history", nil, 1, now, now)
	fake.getNodes[3] = node
	svc := NewService(cache.NewNoop(), fake, nil)

	cat, err := svc.GetCategory(context.Background(), RequestMeta{}, 3, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat.Name != "History" {
		t.Fatalf("unexpected category name %s", cat.Name)
	}
}

func TestDeleteCategory(t *testing.T) {
	fake := newFakeNDR()
	svc := NewService(cache.NewNoop(), fake, nil)

	if err := svc.DeleteCategory(context.Background(), RequestMeta{}, 9, CategoryDeleteRequest{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.deletedNodes) != 1 || fake.deletedNodes[0] != 9 {
		t.Fatalf("expected delete call for id 9")
	}
}

func TestDeleteCategoryWithChildren(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	parent := sampleNode(100, "Parent", "/parent", nil, 1, now, now)
	child := sampleNode(101, "Child", "/parent/child", ptr[int64](100), 1, now, now)
	fake.getNodes[100] = parent
	fake.getNodes[101] = child
	svc := NewService(cache.NewNoop(), fake, nil)

	if err := svc.DeleteCategory(context.Background(), RequestMeta{}, 100, CategoryDeleteRequest{}); err == nil {
		t.Fatalf("expected error when deleting parent with children")
	}
	if len(fake.deletedNodes) != 0 {
		t.Fatalf("expected no delete call")
	}
}

func TestRestoreCategory(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	fake.restoreResp = sampleNode(5, "Physics", "/physics", nil, 1, now, now)
	svc := NewService(cache.NewNoop(), fake, nil)

	cat, err := svc.RestoreCategory(context.Background(), RequestMeta{}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat.Name != "Physics" {
		t.Fatalf("unexpected name %s", cat.Name)
	}
}

func TestMoveCategory(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	parent := sampleNode(10, "Root", "/root", nil, 1, now, now)
	child := sampleNode(11, "Child", "/child", ptr[int64](10), 2, now, now)
	fake.getNodes[10] = parent
	fake.updateResp = child
	svc := NewService(cache.NewNoop(), fake, nil)

	cat, err := svc.MoveCategory(context.Background(), RequestMeta{}, 11, MoveCategoryRequest{NewParentID: ptr[int64](10), ParentSpecified: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat.ParentID == nil || *cat.ParentID != 10 {
		t.Fatalf("expected parent id 10")
	}
}

func TestGetCategoryTree(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	fake.listResponse = ndrclient.NodesPage{
		Items: []ndrclient.Node{
			sampleNode(1, "Root", "/root", nil, 1, now, now),
			sampleNode(2, "Child", "/root/child", ptr[int64](1), 1, now, now),
			sampleNode(3, "Other", "/other", nil, 2, now, now),
		},
	}
	svc := NewService(cache.NewNoop(), fake, nil)

	tree, err := svc.GetCategoryTree(context.Background(), RequestMeta{}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tree) != 2 {
		t.Fatalf("expected 2 root nodes, got %d", len(tree))
	}
	var rootNode *Category
	for _, root := range tree {
		if root.Name == "Root" {
			rootNode = root
			break
		}
	}
	if rootNode == nil {
		t.Fatalf("expected Root node in tree")
	}
	if len(rootNode.Children) != 1 || rootNode.Children[0].Name != "Child" {
		t.Fatalf("expected Root to have Child node")
	}
}

func TestGetCategoryTreePaginates(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	fake.listResponses = map[int]ndrclient.NodesPage{
		1: {
			Page:  1,
			Size:  2,
			Total: 3,
			Items: []ndrclient.Node{
				sampleNode(1, "Root", "/root", nil, 1, now, now),
				sampleNode(3, "Other", "/other", nil, 2, now, now),
			},
		},
		2: {
			Page:  2,
			Size:  2,
			Total: 3,
			Items: []ndrclient.Node{
				sampleNode(2, "Child", "/root/child", ptr[int64](1), 1, now, now),
			},
		},
	}
	svc := NewService(cache.NewNoop(), fake, nil)

	tree, err := svc.GetCategoryTree(context.Background(), RequestMeta{}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tree) != 2 {
		t.Fatalf("expected 2 root nodes, got %d", len(tree))
	}
	var rootNode *Category
	for _, node := range tree {
		if node.Name == "Root" {
			rootNode = node
			break
		}
	}
	if rootNode == nil {
		t.Fatalf("expected Root node present")
	}
	if len(rootNode.Children) != 1 || rootNode.Children[0].Name != "Child" {
		t.Fatalf("expected paginated child to be attached")
	}
}

func TestReorderCategories(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	fake.reorderResp = []ndrclient.Node{
		sampleNode(2, "Child B", "/root/child-b", ptr[int64](1), 2, now, now),
		sampleNode(3, "Child A", "/root/child-a", ptr[int64](1), 1, now, now),
	}
	svc := NewService(cache.NewNoop(), fake, nil)

	parentID := int64(1)
	res, err := svc.ReorderCategories(context.Background(), RequestMeta{}, CategoryReorderRequest{
		ParentID:   &parentID,
		OrderedIDs: []int64{2, 3},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.reorderInput == nil || len(fake.reorderInput.OrderedIDs) != 2 {
		t.Fatalf("expected reorder payload to be captured")
	}
	if len(res) != 2 || res[0].Position != 2 || res[1].Position != 1 {
		t.Fatalf("unexpected reorder result: %+v", res)
	}
}

func TestRepositionCategoryMoveAndReorder(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	newParent := sampleNode(10, "Target", "/target", nil, 1, now, now)
	fake.getNodes[10] = newParent
	originalNode := sampleNode(2, "Node", "/node", nil, 1, now, now)
	fake.getNodes[2] = originalNode
	movedNode := sampleNode(2, "Node", "/target/node", ptr[int64](10), 1, now, now)
	fake.updateResp = movedNode
	fake.reorderResp = []ndrclient.Node{
		movedNode,
		sampleNode(11, "Sibling", "/target/sibling", ptr[int64](10), 2, now, now),
	}
	svc := NewService(cache.NewNoop(), fake, nil)

	result, err := svc.RepositionCategory(context.Background(), RequestMeta{}, 2, CategoryRepositionRequest{
		NewParentID:     ptr[int64](10),
		OrderedIDs:      []int64{2, 11},
		ParentSpecified: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.reorderInput == nil || fake.reorderInput.ParentID == nil || *fake.reorderInput.ParentID != 10 {
		t.Fatalf("expected reorder to target parent 10, got %+v", fake.reorderInput)
	}
	if result.Category.ParentID == nil || *result.Category.ParentID != 10 {
		t.Fatalf("expected category parent to be 10, got %+v", result.Category.ParentID)
	}
	if len(result.Siblings) != 2 {
		t.Fatalf("expected 2 siblings, got %d", len(result.Siblings))
	}
}

func TestRepositionCategoryReorderOnly(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	current := sampleNode(20, "Node", "/root/node", ptr[int64](5), 1, now, now)
	fake.getNodes[20] = current
	fake.reorderResp = []ndrclient.Node{
		current,
		sampleNode(21, "B", "/root/b", ptr[int64](5), 2, now, now),
	}
	svc := NewService(cache.NewNoop(), fake, nil)

	result, err := svc.RepositionCategory(context.Background(), RequestMeta{}, 20, CategoryRepositionRequest{
		OrderedIDs: []int64{20, 21},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.reorderInput == nil || fake.reorderInput.ParentID == nil || *fake.reorderInput.ParentID != 5 {
		t.Fatalf("expected reorder parent 5, got %+v", fake.reorderInput)
	}
	if len(fake.updatedNodes) != 0 {
		t.Fatalf("did not expect move when parent unchanged")
	}
	if result.Category.ParentID == nil || *result.Category.ParentID != 5 {
		t.Fatalf("expected parent id 5, got %+v", result.Category.ParentID)
	}
}

func TestRepositionCategoryRequiresOrderedIDs(t *testing.T) {
	fake := newFakeNDR()
	svc := NewService(cache.NewNoop(), fake, nil)

	if _, err := svc.RepositionCategory(context.Background(), RequestMeta{}, 1, CategoryRepositionRequest{
		OrderedIDs: []int64{},
	}); err == nil {
		t.Fatalf("expected error for empty ordered ids")
	}
	if _, err := svc.RepositionCategory(context.Background(), RequestMeta{}, 1, CategoryRepositionRequest{
		OrderedIDs: []int64{99},
	}); err == nil {
		t.Fatalf("expected error when ordered ids missing target")
	}
}

func TestBulkRestoreCategories(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	restored := sampleNode(30, "X", "/x", nil, 1, now, now)
	fake.restoreResp = restored
	svc := NewService(cache.NewNoop(), fake, nil)

	items, err := svc.BulkRestoreCategories(context.Background(), RequestMeta{}, []int64{30})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 || items[0].ID != 30 {
		t.Fatalf("unexpected items %v", items)
	}
}

func TestBulkDeleteCategories(t *testing.T) {
	fake := newFakeNDR()
	svc := NewService(cache.NewNoop(), fake, nil)

	ids, err := svc.BulkDeleteCategories(context.Background(), RequestMeta{}, []int64{40, 41, 40})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 unique ids, got %v", ids)
	}
	if len(fake.deletedNodes) != 2 || fake.deletedNodes[0] != 40 || fake.deletedNodes[1] != 41 {
		t.Fatalf("unexpected deleted nodes %v", fake.deletedNodes)
	}
}

func TestBulkDeleteCategoriesWithChildren(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	parent := sampleNode(200, "Parent", "/parent", nil, 1, now, now)
	child := sampleNode(201, "Child", "/parent/child", ptr[int64](200), 1, now, now)
	fake.getNodes[200] = parent
	fake.getNodes[201] = child
	svc := NewService(cache.NewNoop(), fake, nil)

	if _, err := svc.BulkDeleteCategories(context.Background(), RequestMeta{}, []int64{200}); err == nil {
		t.Fatalf("expected error when deleting parent with children")
	}
	if len(fake.deletedNodes) != 0 {
		t.Fatalf("expected no delete calls, got %v", fake.deletedNodes)
	}
}

func TestBulkPurgeCategories(t *testing.T) {
	fake := newFakeNDR()
	svc := NewService(cache.NewNoop(), fake, nil)
	ids, err := svc.BulkPurgeCategories(context.Background(), RequestMeta{}, []int64{40, 41})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.purgedNodes) != 2 {
		t.Fatalf("expected purge calls, got %v", fake.purgedNodes)
	}
	if len(ids) != 2 {
		t.Fatalf("unexpected ids %v", ids)
	}
}

func TestRepositionCategoryMoveToRoot(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	parent := sampleNode(30, "Parent", "/parent", nil, 1, now, now)
	child := sampleNode(31, "Child", "/parent/child", ptr[int64](30), 1, now, now)
	fake.getNodes[30] = parent
	fake.getNodes[31] = child
	fake.updateResp = sampleNode(31, "Child", "/child", nil, 1, now, now)
	fake.reorderResp = []ndrclient.Node{
		fake.updateResp,
	}
	svc := NewService(cache.NewNoop(), fake, nil)

	result, err := svc.RepositionCategory(context.Background(), RequestMeta{}, 31, CategoryRepositionRequest{
		NewParentID:     nil,
		OrderedIDs:      []int64{31},
		ParentSpecified: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.reorderInput == nil || fake.reorderInput.ParentID != nil {
		t.Fatalf("expected reorder root parent, got %+v", fake.reorderInput)
	}
	if result.Category.ParentID != nil {
		t.Fatalf("expected parent to be nil, got %+v", result.Category.ParentID)
	}
}

func TestGetDeletedCategories(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	deletedAt := now.Add(-time.Hour)
	deletedNode := sampleNode(5, "Trash", "/trash", nil, 1, now, now)
	deletedNode.DeletedAt = &deletedAt
	fake.listResponse = ndrclient.NodesPage{Items: []ndrclient.Node{
		deletedNode,
		sampleNode(6, "Active", "/active", nil, 2, now, now),
	}}
	svc := NewService(cache.NewNoop(), fake, nil)

	items, err := svc.GetDeletedCategories(context.Background(), RequestMeta{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 deleted item, got %d", len(items))
	}
	if items[0].ID != 5 || items[0].DeletedAt == nil {
		t.Fatalf("unexpected item %+v", items[0])
	}
}

func TestGetDeletedCategoriesPaginates(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()

	del1 := sampleNode(10, "Trash A", "/trash-a", nil, 1, now, now)
	del2 := sampleNode(11, "Trash B", "/trash-b", nil, 2, now, now)
	active := sampleNode(12, "Active", "/active", nil, 3, now, now)
	ts := now.Add(-2 * time.Hour)
	del1.DeletedAt = &ts
	ts2 := now.Add(-time.Hour)
	del2.DeletedAt = &ts2

	fake.listResponses = map[int]ndrclient.NodesPage{
		1: {
			Page:  1,
			Size:  1,
			Total: 3,
			Items: []ndrclient.Node{del1},
		},
		2: {
			Page:  2,
			Size:  1,
			Total: 3,
			Items: []ndrclient.Node{active},
		},
		3: {
			Page:  3,
			Size:  1,
			Total: 3,
			Items: []ndrclient.Node{del2},
		},
	}

	svc := NewService(cache.NewNoop(), fake, nil)

	items, err := svc.GetDeletedCategories(context.Background(), RequestMeta{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 deleted items, got %d", len(items))
	}
	ids := []int64{items[0].ID, items[1].ID}
	if !(containsID(ids, 10) && containsID(ids, 11)) {
		t.Fatalf("expected ids 10 and 11, got %v", ids)
	}
}

func TestPurgeCategory(t *testing.T) {
	fake := newFakeNDR()
	svc := NewService(cache.NewNoop(), fake, nil)

	if err := svc.PurgeCategory(context.Background(), RequestMeta{}, 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.purgedNodes) != 1 || fake.purgedNodes[0] != 42 {
		t.Fatalf("expected purge call for id 42")
	}
}

func containsID(ids []int64, target int64) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func sampleNode(id int64, name string, path string, parentID *int64, position int, created, updated time.Time) ndrclient.Node {
	return ndrclient.Node{
		ID:        id,
		Name:      name,
		Slug:      slugify(name),
		Path:      path,
		ParentID:  parentID,
		Position:  position,
		CreatedAt: created,
		UpdatedAt: updated,
	}
}
