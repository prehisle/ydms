package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yjxt/ydms/backend/internal/auth"
	"github.com/yjxt/ydms/backend/internal/cache"
	"github.com/yjxt/ydms/backend/internal/database"
	"github.com/yjxt/ydms/backend/internal/ndrclient"
	"github.com/yjxt/ydms/backend/internal/service"
)

// withTestUser 添加测试用户到请求上下文
func withTestUser(req *http.Request, user *database.User) *http.Request {
	if user == nil {
		user = &database.User{
			ID:       1,
			Username: "test-user",
			Role:     "super_admin",
		}
	}
	ctx := context.WithValue(req.Context(), auth.UserContextKey, user)
	return req.WithContext(ctx)
}

func TestCategoriesEndpoints(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{
		APIKey:   "test-key",
		UserID:   "tester",
		AdminKey: "admin",
	})
	router := NewRouter(handler)

	root := createCategory(t, router, `{"name":"Root"}`)
	if root.ID == 0 {
		t.Fatalf("expected root category id to be set")
	}
	childPayload := fmt.Sprintf(`{"name":"Child","parent_id":%d}`, root.ID)
	child := createCategory(t, router, childPayload)
	if child.ParentID == nil || *child.ParentID != root.ID {
		t.Fatalf("expected child parent_id to be %d, got %v", root.ID, child.ParentID)
	}

	treeReq := httptest.NewRequest(http.MethodGet, "/api/v1/categories/tree", nil)
	treeRec := httptest.NewRecorder()
	router.ServeHTTP(treeRec, treeReq)
	if treeRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for tree, got %d", treeRec.Code)
	}
	var tree []*service.Category
	if err := json.NewDecoder(treeRec.Body).Decode(&tree); err != nil {
		t.Fatalf("decode tree error: %v", err)
	}
	if len(tree) != 1 || len(tree[0].Children) != 1 {
		t.Fatalf("expected 1 root and 1 child, got %+v", tree)
	}

	reorderPayload := fmt.Sprintf(`{"parent_id":%d,"ordered_ids":[%d]}`, root.ID, child.ID)
	reorderReq := httptest.NewRequest(http.MethodPost, "/api/v1/categories/reorder", strings.NewReader(reorderPayload))
	reorderReq.Header.Set("Content-Type", "application/json")
	reorderRec := httptest.NewRecorder()
	router.ServeHTTP(reorderRec, reorderReq)
	if reorderRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for reorder, got %d", reorderRec.Code)
	}
	var reordered []service.Category
	if err := json.NewDecoder(reorderRec.Body).Decode(&reordered); err != nil {
		t.Fatalf("decode reorder error: %v", err)
	}
	if len(reordered) != 1 || reordered[0].ID != child.ID || reordered[0].Position != 1 {
		t.Fatalf("unexpected reorder response %+v", reordered)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/categories/%d", child.ID), nil)
	deleteReq = withTestUser(deleteReq, nil) // 添加测试用户
	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204 for delete, got %d", deleteRec.Code)
	}

	trashReq := httptest.NewRequest(http.MethodGet, "/api/v1/categories/trash", nil)
	trashRec := httptest.NewRecorder()
	router.ServeHTTP(trashRec, trashReq)
	if trashRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for trash, got %d", trashRec.Code)
	}
	var trashed []service.Category
	if err := json.NewDecoder(trashRec.Body).Decode(&trashed); err != nil {
		t.Fatalf("decode trash error: %v", err)
	}
	if len(trashed) != 1 || trashed[0].ID != child.ID || trashed[0].DeletedAt == nil {
		t.Fatalf("unexpected trash response %+v", trashed)
	}

	restoreReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/categories/%d/restore", child.ID), nil)
	restoreRec := httptest.NewRecorder()
	router.ServeHTTP(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for restore, got %d", restoreRec.Code)
	}
	var restored service.Category
	if err := json.NewDecoder(restoreRec.Body).Decode(&restored); err != nil {
		t.Fatalf("decode restore error: %v", err)
	}
	if restored.ID != child.ID || restored.DeletedAt != nil {
		t.Fatalf("unexpected restore response %+v", restored)
	}

	trashAfterRestoreReq := httptest.NewRequest(http.MethodGet, "/api/v1/categories/trash", nil)
	trashAfterRestoreRec := httptest.NewRecorder()
	router.ServeHTTP(trashAfterRestoreRec, trashAfterRestoreReq)
	if trashAfterRestoreRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for trash after restore, got %d", trashAfterRestoreRec.Code)
	}
	var trashAfterRestore []service.Category
	if err := json.NewDecoder(trashAfterRestoreRec.Body).Decode(&trashAfterRestore); err != nil {
		t.Fatalf("decode trash after restore error: %v", err)
	}
	if len(trashAfterRestore) != 0 {
		t.Fatalf("expected empty trash after restore, got %+v", trashAfterRestore)
	}

	deleteAgainReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/categories/%d", child.ID), nil)
	deleteAgainReq = withTestUser(deleteAgainReq, nil) // 添加测试用户
	deleteAgainRec := httptest.NewRecorder()
	router.ServeHTTP(deleteAgainRec, deleteAgainReq)
	if deleteAgainRec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204 for second delete, got %d", deleteAgainRec.Code)
	}

	purgeReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/categories/%d/purge", child.ID), nil)
	purgeRec := httptest.NewRecorder()
	router.ServeHTTP(purgeRec, purgeReq)
	if purgeRec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204 for purge, got %d", purgeRec.Code)
	}

	treeAfterPurgeReq := httptest.NewRequest(http.MethodGet, "/api/v1/categories/tree", nil)
	treeAfterPurgeRec := httptest.NewRecorder()
	router.ServeHTTP(treeAfterPurgeRec, treeAfterPurgeReq)
	if treeAfterPurgeRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for tree after purge, got %d", treeAfterPurgeRec.Code)
	}
	var treeAfterPurge []*service.Category
	if err := json.NewDecoder(treeAfterPurgeRec.Body).Decode(&treeAfterPurge); err != nil {
		t.Fatalf("decode tree after purge error: %v", err)
	}
	if len(treeAfterPurge) != 1 || len(treeAfterPurge[0].Children) != 0 {
		t.Fatalf("expected only root node after purge, got %+v", treeAfterPurge)
	}
}

func TestBulkCheckCategories(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	root := createCategory(t, router, `{"name":"Root"}`)
	childPayload := fmt.Sprintf(`{"name":"Child","parent_id":%d}`, root.ID)
	child := createCategory(t, router, childPayload)

	doc, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{Title: "Doc"})
	if err != nil {
		t.Fatalf("create document error: %v", err)
	}
	if err := ndr.BindDocument(context.Background(), ndrclient.RequestMeta{}, root.ID, doc.ID); err != nil {
		t.Fatalf("bind document error: %v", err)
	}

	reqBody := fmt.Sprintf(`{"ids":[%d,%d],"include_descendants":true}`, root.ID, child.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/categories/bulk/check", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp service.CategoryCheckResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response error: %v", err)
	}

	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.Items[0].DocumentCount == 0 {
		t.Fatalf("expected document count for root, got %#v", resp.Items[0])
	}
}

func TestBulkCopyCategoriesEndpoint(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	existing := createCategory(t, router, `{"name":"Topic"}`)
	parent := createCategory(t, router, `{"name":"Parent"}`)
	child := createCategory(t, router, fmt.Sprintf(`{"name":"Topic","parent_id":%d}`, parent.ID))
	createCategory(t, router, fmt.Sprintf(`{"name":"Leaf","parent_id":%d}`, child.ID))

	payload := fmt.Sprintf(`{"source_ids":[%d],"target_parent_id":null,"insert_before_id":%d}`, child.ID, existing.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/categories/bulk/copy", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	var resp struct {
		Items []service.Category `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response error: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	copied := resp.Items[0]
	if copied.ParentID != nil {
		t.Fatalf("expected copied parent nil, got %v", copied.ParentID)
	}
	if copied.Name == "Topic" {
		t.Fatalf("expected unique name different from Topic")
	}
	if len(copied.Children) != 1 || copied.Children[0].Name != "Leaf" {
		t.Fatalf("expected child Leaf, got %+v", copied.Children)
	}

	treeReq := httptest.NewRequest(http.MethodGet, "/api/v1/categories/tree", nil)
	treeRec := httptest.NewRecorder()
	router.ServeHTTP(treeRec, treeReq)
	if treeRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for tree, got %d", treeRec.Code)
	}
	var tree []*service.Category
	if err := json.NewDecoder(treeRec.Body).Decode(&tree); err != nil {
		t.Fatalf("decode tree error: %v", err)
	}
	if len(tree) < 2 {
		t.Fatalf("expected at least two root nodes, got %+v", tree)
	}
	if tree[0].ID != copied.ID || tree[1].ID != existing.ID {
		t.Fatalf("expected copied node before existing, got order %+v", []int64{tree[0].ID, tree[1].ID})
	}
}

func TestBulkMoveCategoriesEndpoint(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	sourceParent := createCategory(t, router, `{"name":"Source"}`)
	moveA := createCategory(t, router, fmt.Sprintf(`{"name":"MoveA","parent_id":%d}`, sourceParent.ID))
	moveB := createCategory(t, router, fmt.Sprintf(`{"name":"MoveB","parent_id":%d}`, sourceParent.ID))

	targetParent := createCategory(t, router, `{"name":"Target"}`)
	anchor := createCategory(t, router, fmt.Sprintf(`{"name":"Keep","parent_id":%d}`, targetParent.ID))

	payload := fmt.Sprintf(`{"source_ids":[%d,%d],"target_parent_id":%d,"insert_before_id":%d}`, moveA.ID, moveB.ID, targetParent.ID, anchor.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/categories/bulk/move", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Items []service.Category `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response error: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 moved items, got %d", len(resp.Items))
	}

	treeReq := httptest.NewRequest(http.MethodGet, "/api/v1/categories/tree", nil)
	treeRec := httptest.NewRecorder()
	router.ServeHTTP(treeRec, treeReq)
	if treeRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for tree, got %d", treeRec.Code)
	}
	var tree []*service.Category
	if err := json.NewDecoder(treeRec.Body).Decode(&tree); err != nil {
		t.Fatalf("decode tree error: %v", err)
	}

	var targetNode *service.Category
	var sourceNode *service.Category
	for _, node := range tree {
		if node.ID == targetParent.ID {
			targetNode = node
		}
		if node.ID == sourceParent.ID {
			sourceNode = node
		}
	}

	if targetNode == nil || len(targetNode.Children) != 3 {
		t.Fatalf("expected target to have 3 children, got %+v", targetNode)
	}
	if targetNode.Children[0].ID != moveA.ID || targetNode.Children[1].ID != moveB.ID || targetNode.Children[2].ID != anchor.ID {
		t.Fatalf("unexpected target children order: %+v", targetNode.Children)
	}
	if sourceNode == nil || len(sourceNode.Children) != 0 {
		t.Fatalf("expected source to be empty, got %+v", sourceNode)
	}
}

func TestCategoriesEndpoints_InvalidJSON(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/categories", strings.NewReader("{"))
	req = withTestUser(req, nil) // 添加测试用户
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for invalid json, got %d", rec.Code)
	}
}

func TestCategoriesEndpoints_InvalidCategoryID(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/categories/not-a-number", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for invalid id, got %d", rec.Code)
	}
}

func TestCategoriesEndpoints_MethodNotAllowed(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/categories", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405 for method not allowed, got %d", rec.Code)
	}
}

func TestCategoryRepositionEndpoint(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	root := createCategory(t, router, `{"name":"Root"}`)
	targetParent := createCategory(t, router, `{"name":"Target"}`)
	existing := createCategory(t, router, fmt.Sprintf(`{"name":"Existing","parent_id":%d}`, targetParent.ID))
	child := createCategory(t, router, fmt.Sprintf(`{"name":"Child","parent_id":%d}`, root.ID))

	payload := fmt.Sprintf(`{"new_parent_id":%d,"ordered_ids":[%d,%d]}`, targetParent.ID, existing.ID, child.ID)
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/categories/%d/reposition", child.ID), strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for reposition, got %d body=%s", rec.Code, rec.Body.String())
	}
	var result service.CategoryRepositionResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode reposition result error: %v", err)
	}
	if result.Category.ParentID == nil || *result.Category.ParentID != targetParent.ID {
		t.Fatalf("expected child parent to be %d, got %+v", targetParent.ID, result.Category.ParentID)
	}
	if len(result.Siblings) != 2 || result.Siblings[0].ID != existing.ID || result.Siblings[1].ID != child.ID {
		t.Fatalf("unexpected siblings response %+v", result.Siblings)
	}

	treeReq := httptest.NewRequest(http.MethodGet, "/api/v1/categories/tree", nil)
	treeRec := httptest.NewRecorder()
	router.ServeHTTP(treeRec, treeReq)
	if treeRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for tree, got %d", treeRec.Code)
	}
	var tree []*service.Category
	if err := json.NewDecoder(treeRec.Body).Decode(&tree); err != nil {
		t.Fatalf("decode tree error: %v", err)
	}
	var target *service.Category
	for _, node := range tree {
		if node.ID == targetParent.ID {
			target = node
			break
		}
	}
	if target == nil || len(target.Children) != 2 {
		t.Fatalf("expected target parent to have 2 children, got %+v", target)
	}
	if target.Children[1].ID != child.ID {
		t.Fatalf("expected child to be under target parent, got %+v", target.Children)
	}
}

func TestCategoryBulkEndpoints(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	root := createCategory(t, router, `{"name":"BulkRoot"}`)
	childA := createCategory(t, router, fmt.Sprintf(`{"name":"A","parent_id":%d}`, root.ID))
	childB := createCategory(t, router, fmt.Sprintf(`{"name":"B","parent_id":%d}`, root.ID))

	deleteReqA := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/categories/%d", childA.ID), nil)
	deleteReqA = withTestUser(deleteReqA, nil) // 添加测试用户
	deleteRecA := httptest.NewRecorder()
	router.ServeHTTP(deleteRecA, deleteReqA)
	if deleteRecA.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for delete A, got %d", deleteRecA.Code)
	}
	deleteReqB := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/categories/%d", childB.ID), nil)
	deleteReqB = withTestUser(deleteReqB, nil) // 添加测试用户
	deleteRecB := httptest.NewRecorder()
	router.ServeHTTP(deleteRecB, deleteReqB)
	if deleteRecB.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for delete B, got %d", deleteRecB.Code)
	}

	trashReq := httptest.NewRequest(http.MethodGet, "/api/v1/categories/trash", nil)
	trashRec := httptest.NewRecorder()
	router.ServeHTTP(trashRec, trashReq)
	if trashRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for trash, got %d", trashRec.Code)
	}
	var trashItems []service.Category
	if err := json.NewDecoder(trashRec.Body).Decode(&trashItems); err != nil {
		t.Fatalf("decode trash error: %v", err)
	}
	if len(trashItems) != 2 {
		t.Fatalf("expected 2 items in trash, got %d", len(trashItems))
	}

	restorePayload := fmt.Sprintf(`{"ids":[%d,%d]}`, childA.ID, childB.ID)
	restoreReq := httptest.NewRequest(http.MethodPost, "/api/v1/categories/bulk/restore", strings.NewReader(restorePayload))
	restoreReq.Header.Set("Content-Type", "application/json")
	restoreRec := httptest.NewRecorder()
	router.ServeHTTP(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for bulk restore, got %d", restoreRec.Code)
	}
	var restoreItems []service.Category
	if err := json.NewDecoder(restoreRec.Body).Decode(&restoreItems); err != nil {
		t.Fatalf("decode restore response error: %v", err)
	}
	if len(restoreItems) != 2 {
		t.Fatalf("expected 2 restored items, got %v", restoreItems)
	}

	deleteReqA = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/categories/%d", childA.ID), nil)
	deleteRecA = httptest.NewRecorder()
	router.ServeHTTP(deleteRecA, deleteReqA)
	deleteReqB = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/categories/%d", childB.ID), nil)
	deleteRecB = httptest.NewRecorder()
	router.ServeHTTP(deleteRecB, deleteReqB)

	purgePayload := fmt.Sprintf(`{"ids":[%d,%d]}`, childA.ID, childB.ID)
	purgeReq := httptest.NewRequest(http.MethodPost, "/api/v1/categories/bulk/purge", strings.NewReader(purgePayload))
	purgeReq.Header.Set("Content-Type", "application/json")
	purgeRec := httptest.NewRecorder()
	router.ServeHTTP(purgeRec, purgeReq)
	if purgeRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for bulk purge, got %d", purgeRec.Code)
	}
	var purgeResp struct {
		PurgedIDs []int64 `json:"purged_ids"`
	}
	if err := json.NewDecoder(purgeRec.Body).Decode(&purgeResp); err != nil {
		t.Fatalf("decode purge response error: %v", err)
	}
	if len(purgeResp.PurgedIDs) != 2 {
		t.Fatalf("expected 2 purged ids, got %v", purgeResp.PurgedIDs)
	}
}

func TestBulkDeleteCategoriesEndpoint(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	root := createCategory(t, router, `{"name":"Root"}`)
	childA := createCategory(t, router, fmt.Sprintf(`{"name":"ChildA","parent_id":%d}`, root.ID))
	childB := createCategory(t, router, fmt.Sprintf(`{"name":"ChildB","parent_id":%d}`, root.ID))

	payload := fmt.Sprintf(`{"ids":[%d,%d]}`, childA.ID, childB.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/categories/bulk/delete", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for bulk delete, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		DeletedIDs []int64 `json:"deleted_ids"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response error: %v", err)
	}
	if len(resp.DeletedIDs) != 2 {
		t.Fatalf("expected 2 deleted ids, got %v", resp.DeletedIDs)
	}

	trashReq := httptest.NewRequest(http.MethodGet, "/api/v1/categories/trash", nil)
	trashRec := httptest.NewRecorder()
	router.ServeHTTP(trashRec, trashReq)
	if trashRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for trash, got %d", trashRec.Code)
	}
	var trashItems []service.Category
	if err := json.NewDecoder(trashRec.Body).Decode(&trashItems); err != nil {
		t.Fatalf("decode trash response error: %v", err)
	}
	if len(trashItems) != 2 {
		t.Fatalf("expected 2 items in trash, got %d", len(trashItems))
	}

	parent := createCategory(t, router, `{"name":"Parent"}`)
	createCategory(t, router, fmt.Sprintf(`{"name":"Child","parent_id":%d}`, parent.ID))

	badPayload := fmt.Sprintf(`{"ids":[%d]}`, parent.ID)
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/categories/bulk/delete", strings.NewReader(badPayload))
	badReq.Header.Set("Content-Type", "application/json")
	badRec := httptest.NewRecorder()
	router.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502 when deleting parent with children, got %d body=%s", badRec.Code, badRec.Body.String())
	}
}

func createCategory(t *testing.T, router http.Handler, payload string) service.Category {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/categories", strings.NewReader(payload))
	req = withTestUser(req, nil) // 添加默认测试用户
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var cat service.Category
	if err := json.NewDecoder(rec.Body).Decode(&cat); err != nil {
		t.Fatalf("decode category error: %v", err)
	}
	return cat
}

type inMemoryNDR struct {
	nextID     int64
	nextDocID  int64
	nodes      map[int64]ndrclient.Node
	pathIndex  map[string]int64
	childOrder map[int64][]int64
	clock      int64
	documents  map[int64]ndrclient.Document
	bindings   map[int64]map[int64]struct{}
}

func newInMemoryNDR() *inMemoryNDR {
	return &inMemoryNDR{
		nextID:     1,
		nextDocID:  1,
		nodes:      make(map[int64]ndrclient.Node),
		pathIndex:  make(map[string]int64),
		childOrder: make(map[int64][]int64),
		documents:  make(map[int64]ndrclient.Document),
		bindings:   make(map[int64]map[int64]struct{}),
	}
}

func (f *inMemoryNDR) Ping(context.Context) error {
	return nil
}

func (f *inMemoryNDR) CreateNode(ctx context.Context, meta ndrclient.RequestMeta, body ndrclient.NodeCreate) (ndrclient.Node, error) {
	if body.Slug == nil || *body.Slug == "" {
		return ndrclient.Node{}, fmt.Errorf("slug is required")
	}
	id := f.nextID
	f.nextID++

	var parentID *int64
	if body.ParentPath != nil && *body.ParentPath != "" {
		parentPath := strings.TrimRight(*body.ParentPath, "/")
		if pid, ok := f.pathIndex[parentPath]; ok {
			parentID = &pid
		} else {
			return ndrclient.Node{}, fmt.Errorf("unknown parent path %s", parentPath)
		}
	}
	node := ndrclient.Node{
		ID:        id,
		Name:      body.Name,
		Slug:      *body.Slug,
		Path:      f.composePath(parentID, *body.Slug),
		ParentID:  parentID,
		Position:  f.appendChild(parentID, id),
		CreatedAt: f.tick(),
		UpdatedAt: f.tick(),
	}
	f.nodes[id] = node
	f.pathIndex[node.Path] = id
	return node, nil
}

func (f *inMemoryNDR) GetNode(ctx context.Context, meta ndrclient.RequestMeta, id int64, opts ndrclient.GetNodeOptions) (ndrclient.Node, error) {
	node, ok := f.nodes[id]
	if !ok {
		return ndrclient.Node{}, fmt.Errorf("node %d not found", id)
	}
	return node, nil
}

func (f *inMemoryNDR) UpdateNode(ctx context.Context, meta ndrclient.RequestMeta, id int64, body ndrclient.NodeUpdate) (ndrclient.Node, error) {
	node, ok := f.nodes[id]
	if !ok {
		return ndrclient.Node{}, fmt.Errorf("node %d not found", id)
	}
	if body.Name != nil && strings.TrimSpace(*body.Name) != "" {
		node.Name = *body.Name
	}
	if body.Slug != nil && strings.TrimSpace(*body.Slug) != "" {
		node.Slug = *body.Slug
	}
	if value, ok := body.ParentPath.Value(); ok {
		var parentID *int64
		if value != nil {
			parentPath := strings.TrimSpace(*value)
			if parentPath != "" {
				parentPath = strings.TrimRight(parentPath, "/")
				if pid, ok := f.pathIndex[parentPath]; ok {
					parentID = &pid
				} else {
					return ndrclient.Node{}, fmt.Errorf("unknown parent path %s", parentPath)
				}
			}
		}
		f.moveChild(node.ParentID, parentID, id)
		node.ParentID = parentID
	}
	oldPath := node.Path
	node.Path = f.composePath(node.ParentID, node.Slug)
	node.UpdatedAt = f.tick()
	f.nodes[id] = node
	delete(f.pathIndex, oldPath)
	f.pathIndex[node.Path] = id
	return node, nil
}

func (f *inMemoryNDR) DeleteNode(ctx context.Context, meta ndrclient.RequestMeta, id int64) error {
	node, ok := f.nodes[id]
	if !ok {
		return fmt.Errorf("node %d not found", id)
	}
	ts := f.tick()
	node.DeletedAt = &ts
	node.UpdatedAt = ts
	f.nodes[id] = node
	return nil
}

func (f *inMemoryNDR) RestoreNode(ctx context.Context, meta ndrclient.RequestMeta, id int64) (ndrclient.Node, error) {
	node, ok := f.nodes[id]
	if !ok {
		return ndrclient.Node{}, fmt.Errorf("node %d not found", id)
	}
	node.DeletedAt = nil
	node.UpdatedAt = f.tick()
	f.nodes[id] = node
	return node, nil
}

func (f *inMemoryNDR) ListNodes(ctx context.Context, meta ndrclient.RequestMeta, params ndrclient.ListNodesParams) (ndrclient.NodesPage, error) {
	includeDeleted := params.IncludeDeleted != nil && *params.IncludeDeleted
	items := make([]ndrclient.Node, 0, len(f.nodes))
	for _, node := range f.nodes {
		if node.DeletedAt != nil && !includeDeleted {
			continue
		}
		items = append(items, node)
	}
	return ndrclient.NodesPage{
		Page:  params.Page,
		Size:  params.Size,
		Total: len(items),
		Items: items,
	}, nil
}

func (f *inMemoryNDR) ListChildren(ctx context.Context, meta ndrclient.RequestMeta, id int64, params ndrclient.ListChildrenParams) ([]ndrclient.Node, error) {
	children := f.childOrder[id]
	result := make([]ndrclient.Node, 0, len(children))
	for _, childID := range children {
		if node, ok := f.nodes[childID]; ok {
			result = append(result, node)
		}
	}
	return result, nil
}

func (f *inMemoryNDR) HasChildren(ctx context.Context, meta ndrclient.RequestMeta, id int64) (bool, error) {
	kids, err := f.ListChildren(ctx, meta, id, ndrclient.ListChildrenParams{})
	if err != nil {
		return false, err
	}
	return len(kids) > 0, nil
}

func (f *inMemoryNDR) ReorderNodes(ctx context.Context, meta ndrclient.RequestMeta, payload ndrclient.NodeReorderPayload) ([]ndrclient.Node, error) {
	key := int64(0)
	if payload.ParentID != nil {
		key = *payload.ParentID
	}
	f.childOrder[key] = append([]int64(nil), payload.OrderedIDs...)
	result := make([]ndrclient.Node, 0, len(payload.OrderedIDs))
	for idx, id := range payload.OrderedIDs {
		node, ok := f.nodes[id]
		if !ok {
			return nil, fmt.Errorf("node %d not found", id)
		}
		node.Position = idx + 1
		node.UpdatedAt = f.tick()
		f.nodes[id] = node
		result = append(result, node)
	}
	return result, nil
}

func (f *inMemoryNDR) PurgeNode(ctx context.Context, meta ndrclient.RequestMeta, id int64) error {
	node, ok := f.nodes[id]
	if !ok {
		return fmt.Errorf("node %d not found", id)
	}
	key := int64(0)
	if node.ParentID != nil {
		key = *node.ParentID
	}
	f.removeChild(key, id)
	delete(f.pathIndex, node.Path)
	delete(f.nodes, id)
	return nil
}

func (f *inMemoryNDR) ListDocuments(ctx context.Context, meta ndrclient.RequestMeta, query url.Values) (ndrclient.DocumentsPage, error) {
	includeDeleted := false
	if query != nil {
		val := strings.ToLower(strings.TrimSpace(query.Get("include_deleted")))
		if val == "true" || val == "1" || val == "yes" {
			includeDeleted = true
		}
	}

	items := make([]ndrclient.Document, 0, len(f.documents))
	for _, doc := range f.documents {
		if !includeDeleted && doc.DeletedAt != nil {
			continue
		}
		items = append(items, doc)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return ndrclient.DocumentsPage{
		Page:  1,
		Size:  len(items),
		Total: len(items),
		Items: items,
	}, nil
}

func (f *inMemoryNDR) ListNodeDocuments(ctx context.Context, meta ndrclient.RequestMeta, nodeID int64, query url.Values) (ndrclient.DocumentsPage, error) {
	includeDescendants := true
	if query != nil {
		if val := strings.ToLower(strings.TrimSpace(query.Get("include_descendants"))); val != "" {
			includeDescendants = !(val == "false" || val == "0")
		}
	}

	targetIDs := []int64{nodeID}
	if includeDescendants {
		for idx := 0; idx < len(targetIDs); idx++ {
			current := targetIDs[idx]
			children := f.childOrder[current]
			targetIDs = append(targetIDs, children...)
		}
	}

	seen := make(map[int64]struct{})
	docs := []ndrclient.Document{}
	for _, nid := range targetIDs {
		binding := f.bindings[nid]
		if binding == nil {
			continue
		}
		for docID := range binding {
			if _, ok := seen[docID]; ok {
				continue
			}
			if doc, ok := f.documents[docID]; ok {
				docs = append(docs, doc)
				seen[docID] = struct{}{}
			}
		}
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].ID < docs[j].ID })

	// Apply pagination
	page := 1
	size := 100 // default
	if query != nil {
		if p := query.Get("page"); p != "" {
			if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
				page = parsed
			}
		}
		if s := query.Get("size"); s != "" {
			if parsed, err := strconv.Atoi(s); err == nil && parsed > 0 {
				size = parsed
			}
		}
	}

	total := len(docs)
	start := (page - 1) * size
	end := start + size
	if start >= total {
		docs = []ndrclient.Document{}
	} else if end > total {
		docs = docs[start:]
	} else {
		docs = docs[start:end]
	}

	return ndrclient.DocumentsPage{
		Page:  page,
		Size:  size,
		Total: total,
		Items: docs,
	}, nil
}

func (f *inMemoryNDR) CreateDocument(ctx context.Context, meta ndrclient.RequestMeta, body ndrclient.DocumentCreate) (ndrclient.Document, error) {
	id := f.nextDocID
	f.nextDocID++
	createdAt := f.tick()
	updatedAt := f.tick()
	metadata := map[string]any{}
	if body.Metadata != nil {
		for k, v := range body.Metadata {
			metadata[k] = v
		}
	}
	content := map[string]any{}
	if body.Content != nil {
		for k, v := range body.Content {
			content[k] = v
		}
	}
	position := 1
	if body.Position != nil {
		position = *body.Position
	}
	doc := ndrclient.Document{
		ID:        id,
		Title:     body.Title,
		Type:      body.Type,
		Position:  position,
		Metadata:  metadata,
		Content:   content,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		CreatedBy: "tester",
		UpdatedBy: "tester",
	}
	f.documents[id] = doc
	return doc, nil
}

func (f *inMemoryNDR) ReorderDocuments(ctx context.Context, meta ndrclient.RequestMeta, payload ndrclient.DocumentReorderPayload) ([]ndrclient.Document, error) {
	available := make([]ndrclient.Document, 0, len(f.documents))
	for _, doc := range f.documents {
		if doc.DeletedAt != nil {
			continue
		}
		if payload.ApplyTypeFilter {
			if payload.Type == nil {
				if doc.Type != nil {
					continue
				}
			} else {
				if doc.Type == nil || *doc.Type != *payload.Type {
					continue
				}
			}
		}
		available = append(available, doc)
	}

	sort.Slice(available, func(i, j int) bool {
		if available[i].Position == available[j].Position {
			return available[i].ID < available[j].ID
		}
		return available[i].Position < available[j].Position
	})

	docMap := make(map[int64]ndrclient.Document, len(available))
	for _, doc := range available {
		docMap[doc.ID] = doc
	}

	seen := make(map[int64]struct{}, len(payload.OrderedIDs))
	ordered := make([]ndrclient.Document, 0, len(available))
	for _, id := range payload.OrderedIDs {
		doc, ok := docMap[id]
		if !ok {
			return nil, &ndrclient.Error{StatusCode: http.StatusNotFound, Status: "404 Not Found"}
		}
		if _, exists := seen[id]; exists {
			return nil, &ndrclient.Error{StatusCode: http.StatusBadRequest, Status: "400 Bad Request"}
		}
		seen[id] = struct{}{}
		ordered = append(ordered, doc)
	}

	for _, doc := range available {
		if _, exists := seen[doc.ID]; exists {
			continue
		}
		ordered = append(ordered, doc)
	}

	for idx := range ordered {
		doc := ordered[idx]
		doc.Position = idx
		doc.UpdatedAt = f.tick()
		f.documents[doc.ID] = doc
		ordered[idx] = doc
	}

	return ordered, nil
}

func (f *inMemoryNDR) BindDocument(ctx context.Context, meta ndrclient.RequestMeta, nodeID, docID int64) error {
	if _, ok := f.documents[docID]; !ok {
		return fmt.Errorf("document %d not found", docID)
	}
	if f.bindings[nodeID] == nil {
		f.bindings[nodeID] = make(map[int64]struct{})
	}
	f.bindings[nodeID][docID] = struct{}{}
	return nil
}

func (f *inMemoryNDR) UpdateDocument(ctx context.Context, meta ndrclient.RequestMeta, docID int64, body ndrclient.DocumentUpdate) (ndrclient.Document, error) {
	doc, ok := f.documents[docID]
	if !ok {
		return ndrclient.Document{}, fmt.Errorf("document %d not found", docID)
	}
	if body.Title != nil {
		doc.Title = *body.Title
	}
	if body.Content != nil {
		doc.Content = body.Content
	}
	if body.Metadata != nil {
		doc.Metadata = body.Metadata
	}
	if body.Type != nil {
		doc.Type = body.Type
	}
	if body.Position != nil {
		doc.Position = *body.Position
	}
	doc.UpdatedAt = f.tick()
	f.documents[docID] = doc
	return doc, nil
}

func (f *inMemoryNDR) GetDocument(ctx context.Context, meta ndrclient.RequestMeta, docID int64) (ndrclient.Document, error) {
	doc, ok := f.documents[docID]
	if !ok {
		return ndrclient.Document{}, fmt.Errorf("document %d not found", docID)
	}
	return doc, nil
}

func (f *inMemoryNDR) DeleteDocument(ctx context.Context, meta ndrclient.RequestMeta, docID int64) error {
	doc, ok := f.documents[docID]
	if !ok {
		return fmt.Errorf("document %d not found", docID)
	}
	ts := f.tick()
	doc.DeletedAt = &ts
	doc.UpdatedAt = ts
	f.documents[docID] = doc
	return nil
}

func (f *inMemoryNDR) RestoreDocument(ctx context.Context, meta ndrclient.RequestMeta, docID int64) (ndrclient.Document, error) {
	doc, ok := f.documents[docID]
	if !ok {
		return ndrclient.Document{}, fmt.Errorf("document %d not found", docID)
	}
	doc.DeletedAt = nil
	doc.UpdatedAt = f.tick()
	f.documents[docID] = doc
	return doc, nil
}

func (f *inMemoryNDR) PurgeDocument(ctx context.Context, meta ndrclient.RequestMeta, docID int64) error {
	delete(f.documents, docID)
	return nil
}

func (f *inMemoryNDR) UnbindDocument(ctx context.Context, meta ndrclient.RequestMeta, nodeID, docID int64) error {
	if f.bindings[nodeID] != nil {
		delete(f.bindings[nodeID], docID)
	}
	return nil
}

func (f *inMemoryNDR) GetDocumentBindingStatus(ctx context.Context, meta ndrclient.RequestMeta, docID int64) (ndrclient.DocumentBindingStatus, error) {
	nodeIDs := make([]int64, 0)
	for nodeID, docs := range f.bindings {
		if _, ok := docs[docID]; ok {
			nodeIDs = append(nodeIDs, nodeID)
		}
	}
	sort.Slice(nodeIDs, func(i, j int) bool { return nodeIDs[i] < nodeIDs[j] })
	return ndrclient.DocumentBindingStatus{
		TotalBindings: len(nodeIDs),
		NodeIDs:       nodeIDs,
	}, nil
}

func (f *inMemoryNDR) BindRelationship(ctx context.Context, meta ndrclient.RequestMeta, nodeID, docID int64) (ndrclient.Relationship, error) {
	if f.bindings[nodeID] == nil {
		f.bindings[nodeID] = make(map[int64]struct{})
	}
	f.bindings[nodeID][docID] = struct{}{}
	return ndrclient.Relationship{
		NodeID:     nodeID,
		DocumentID: docID,
		CreatedBy:  meta.UserID,
	}, nil
}

func (f *inMemoryNDR) UnbindRelationship(ctx context.Context, meta ndrclient.RequestMeta, nodeID, docID int64) error {
	if f.bindings[nodeID] != nil {
		delete(f.bindings[nodeID], docID)
	}
	return nil
}

func (f *inMemoryNDR) ListRelationships(ctx context.Context, meta ndrclient.RequestMeta, nodeID, docID *int64) ([]ndrclient.Relationship, error) {
	var rels []ndrclient.Relationship

	if nodeID != nil {
		// 查询特定节点的所有绑定
		if bindings, ok := f.bindings[*nodeID]; ok {
			for dID := range bindings {
				rels = append(rels, ndrclient.Relationship{
					NodeID:     *nodeID,
					DocumentID: dID,
					CreatedBy:  meta.UserID,
				})
			}
		}
	} else if docID != nil {
		// 查询特定文档绑定的所有节点
		for nID, bindings := range f.bindings {
			if _, ok := bindings[*docID]; ok {
				rels = append(rels, ndrclient.Relationship{
					NodeID:     nID,
					DocumentID: *docID,
					CreatedBy:  meta.UserID,
				})
			}
		}
	} else {
		// 返回所有关系
		for nID, bindings := range f.bindings {
			for dID := range bindings {
				rels = append(rels, ndrclient.Relationship{
					NodeID:     nID,
					DocumentID: dID,
					CreatedBy:  meta.UserID,
				})
			}
		}
	}

	return rels, nil
}

func (f *inMemoryNDR) ListDocumentVersions(_ context.Context, _ ndrclient.RequestMeta, docID int64, page, size int) (ndrclient.DocumentVersionsPage, error) {
	return ndrclient.DocumentVersionsPage{
		Page:     page,
		Size:     size,
		Total:    0,
		Versions: []ndrclient.DocumentVersion{},
	}, nil
}

func (f *inMemoryNDR) GetDocumentVersion(_ context.Context, _ ndrclient.RequestMeta, docID int64, versionNumber int) (ndrclient.DocumentVersion, error) {
	return ndrclient.DocumentVersion{
		DocumentID:    docID,
		VersionNumber: versionNumber,
	}, nil
}

func (f *inMemoryNDR) GetDocumentVersionDiff(_ context.Context, _ ndrclient.RequestMeta, docID int64, fromVersion, toVersion int) (ndrclient.DocumentVersionDiff, error) {
	return ndrclient.DocumentVersionDiff{
		FromVersion: fromVersion,
		ToVersion:   toVersion,
	}, nil
}

func (f *inMemoryNDR) RestoreDocumentVersion(_ context.Context, meta ndrclient.RequestMeta, docID int64, versionNumber int) (ndrclient.Document, error) {
	if doc, ok := f.documents[docID]; ok {
		return doc, nil
	}
	return ndrclient.Document{ID: docID}, nil
}

func (f *inMemoryNDR) GetNodeByPath(_ context.Context, _ ndrclient.RequestMeta, path string, opts ndrclient.GetNodeOptions) (ndrclient.Node, error) {
	// 通过路径查找节点
	for _, node := range f.nodes {
		if node.Path == path {
			includeDeleted := opts.IncludeDeleted != nil && *opts.IncludeDeleted
			if node.DeletedAt != nil && !includeDeleted {
				continue
			}
			return node, nil
		}
	}
	return ndrclient.Node{}, fmt.Errorf("node not found by path: %s", path)
}

func (f *inMemoryNDR) ListNodeDocumentsByPath(ctx context.Context, meta ndrclient.RequestMeta, path string, query url.Values) (ndrclient.DocumentsPage, error) {
	// 先通过路径找到节点
	node, err := f.GetNodeByPath(ctx, meta, path, ndrclient.GetNodeOptions{})
	if err != nil {
		return ndrclient.DocumentsPage{}, err
	}
	// 然后调用 ListNodeDocuments
	return f.ListNodeDocuments(ctx, meta, node.ID, query)
}

func (f *inMemoryNDR) composePath(parentID *int64, slug string) string {
	if parentID == nil {
		return "/" + slug
	}
	parent, ok := f.nodes[*parentID]
	if !ok {
		return "/" + slug
	}
	return strings.TrimRight(parent.Path, "/") + "/" + slug
}

func (f *inMemoryNDR) appendChild(parentID *int64, id int64) int {
	key := int64(0)
	if parentID != nil {
		key = *parentID
	}
	f.childOrder[key] = append(f.childOrder[key], id)
	return len(f.childOrder[key])
}

func (f *inMemoryNDR) moveChild(oldParent, newParent *int64, id int64) {
	oldKey := int64(0)
	if oldParent != nil {
		oldKey = *oldParent
	}
	newKey := int64(0)
	if newParent != nil {
		newKey = *newParent
	}
	f.removeChild(oldKey, id)
	f.childOrder[newKey] = append(f.childOrder[newKey], id)
}

func (f *inMemoryNDR) removeChild(parentKey int64, id int64) {
	children := f.childOrder[parentKey]
	for idx, childID := range children {
		if childID == id {
			f.childOrder[parentKey] = append(children[:idx], children[idx+1:]...)
			break
		}
	}
}

func (f *inMemoryNDR) tick() time.Time {
	f.clock++
	return time.Unix(0, f.clock*int64(time.Millisecond)).UTC()
}

// Asset methods (stub implementations for interface compliance)
func (f *inMemoryNDR) InitMultipartUpload(_ context.Context, _ ndrclient.RequestMeta, _ ndrclient.AssetInitRequest) (ndrclient.AssetInitResponse, error) {
	return ndrclient.AssetInitResponse{}, nil
}

func (f *inMemoryNDR) GetAssetPartURLs(_ context.Context, _ ndrclient.RequestMeta, _ int64, _ []int) (ndrclient.AssetPartURLsResponse, error) {
	return ndrclient.AssetPartURLsResponse{}, nil
}

func (f *inMemoryNDR) CompleteMultipartUpload(_ context.Context, _ ndrclient.RequestMeta, _ int64, _ []ndrclient.AssetCompletedPart) (ndrclient.Asset, error) {
	return ndrclient.Asset{}, nil
}

func (f *inMemoryNDR) AbortMultipartUpload(_ context.Context, _ ndrclient.RequestMeta, _ int64) error {
	return nil
}

func (f *inMemoryNDR) GetAsset(_ context.Context, _ ndrclient.RequestMeta, _ int64) (ndrclient.Asset, error) {
	return ndrclient.Asset{}, nil
}

func (f *inMemoryNDR) GetAssetDownloadURL(_ context.Context, _ ndrclient.RequestMeta, _ int64) (ndrclient.AssetDownloadURLResponse, error) {
	return ndrclient.AssetDownloadURLResponse{}, nil
}

func (f *inMemoryNDR) DeleteAsset(_ context.Context, _ ndrclient.RequestMeta, _ int64) error {
	return nil
}

func TestListDocumentsIDFilter(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	docA, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{Title: "Doc A"})
	if err != nil {
		t.Fatalf("create document A error: %v", err)
	}
	docB, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{Title: "Doc B"})
	if err != nil {
		t.Fatalf("create document B error: %v", err)
	}
	docC, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{Title: "Doc C"})
	if err != nil {
		t.Fatalf("create document C error: %v", err)
	}

	url := fmt.Sprintf("/api/v1/documents?id=%d&id=%d", docA.ID, docC.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var page ndrclient.DocumentsPage
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decode documents page error: %v", err)
	}

	if len(page.Items) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(page.Items))
	}
	if page.Total != 2 {
		t.Fatalf("expected total 2, got %d", page.Total)
	}

	ids := []int64{page.Items[0].ID, page.Items[1].ID}
	for _, unexpected := range []int64{docB.ID} {
		for _, id := range ids {
			if id == unexpected {
				t.Fatalf("unexpected document id %d in response", unexpected)
			}
		}
	}
	if ids[0] != docA.ID || ids[1] != docC.ID {
		t.Fatalf("expected documents [%d %d], got %v", docA.ID, docC.ID, ids)
	}
}

func TestListNodeDocumentsIDFilter(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	root := createCategory(t, router, `{"name":"Root"}`)

	docA, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{Title: "Doc A"})
	if err != nil {
		t.Fatalf("create document A error: %v", err)
	}
	docB, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{Title: "Doc B"})
	if err != nil {
		t.Fatalf("create document B error: %v", err)
	}
	docC, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{Title: "Doc C"})
	if err != nil {
		t.Fatalf("create document C error: %v", err)
	}

	if err := ndr.BindDocument(context.Background(), ndrclient.RequestMeta{}, root.ID, docA.ID); err != nil {
		t.Fatalf("bind document A error: %v", err)
	}
	if err := ndr.BindDocument(context.Background(), ndrclient.RequestMeta{}, root.ID, docB.ID); err != nil {
		t.Fatalf("bind document B error: %v", err)
	}
	if err := ndr.BindDocument(context.Background(), ndrclient.RequestMeta{}, root.ID, docC.ID); err != nil {
		t.Fatalf("bind document C error: %v", err)
	}

	url := fmt.Sprintf("/api/v1/nodes/%d/subtree-documents?id=%d&id=%d", root.ID, docA.ID, docC.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var page ndrclient.DocumentsPage
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decode documents page error: %v", err)
	}

	if len(page.Items) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(page.Items))
	}
	if page.Total != 2 {
		t.Fatalf("expected total 2, got %d", page.Total)
	}
	if page.Items[0].ID != docA.ID || page.Items[1].ID != docC.ID {
		t.Fatalf("expected document ids [%d %d], got [%d %d]", docA.ID, docC.ID, page.Items[0].ID, page.Items[1].ID)
	}
	for _, doc := range page.Items {
		if doc.ID == docB.ID {
			t.Fatalf("unexpected document id %d in response", docB.ID)
		}
	}
}

func TestDocumentReorderEndpoint(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	// Create some documents
	docType := "knowledge_overview_v1"
	doc1, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{
		Title:   "Document A",
		Type:    &docType,
		Content: map[string]any{"format": "html", "data": "<p>Content A</p>"},
	})
	if err != nil {
		t.Fatalf("create document 1 error: %v", err)
	}

	doc2, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{
		Title:   "Document B",
		Type:    &docType,
		Content: map[string]any{"format": "html", "data": "<p>Content B</p>"},
	})
	if err != nil {
		t.Fatalf("create document 2 error: %v", err)
	}

	// Test reordering documents
	payload := fmt.Sprintf(`{"ordered_ids":[%d,%d]}`, doc2.ID, doc1.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/reorder", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for document reorder, got %d body=%s", rec.Code, rec.Body.String())
	}

	var docs []ndrclient.Document
	if err := json.NewDecoder(rec.Body).Decode(&docs); err != nil {
		t.Fatalf("decode reorder response error: %v", err)
	}

	if len(docs) != 2 {
		t.Fatalf("expected 2 documents in response, got %d", len(docs))
	}

	// Check that positions were updated correctly (zero-based)
	for i, doc := range docs {
		if doc.Position != i {
			t.Fatalf("expected document %d to have position %d, got %d", doc.ID, i, doc.Position)
		}
	}

	// Doc2 should be first (position 0), Doc1 should be second (position 1)
	if docs[0].ID != doc2.ID || docs[0].Position != 0 {
		t.Fatalf("expected first document to be doc2 with position 0, got ID=%d position=%d", docs[0].ID, docs[0].Position)
	}
	if docs[1].ID != doc1.ID || docs[1].Position != 1 {
		t.Fatalf("expected second document to be doc1 with position 1, got ID=%d position=%d", docs[1].ID, docs[1].Position)
	}
}

func TestDocumentReorderEndpoint_EmptyOrderedIDs(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	payload := `{"ordered_ids":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/reorder", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for empty ordered_ids, got %d", rec.Code)
	}
}

func TestDocumentReorderEndpoint_InvalidJSON(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/reorder", strings.NewReader("{"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for invalid JSON, got %d", rec.Code)
	}
}

func TestDocumentReorderEndpoint_NotFound(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/reorder", strings.NewReader(`{"ordered_ids":[999]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 for missing document, got %d", rec.Code)
	}
}

func TestDocumentReorderEndpoint_BadRequest(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	// Create a document so that duplicate detection can trigger
	doc, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{Title: "Doc"})
	if err != nil {
		t.Fatalf("create document error: %v", err)
	}

	payload := fmt.Sprintf(`{"ordered_ids":[%d,%d]}`, doc.ID, doc.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/reorder", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for duplicate ids, got %d", rec.Code)
	}
}

func TestDeleteDocumentEndpoint(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	doc, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{Title: "Doc"})
	if err != nil {
		t.Fatalf("create document error: %v", err)
	}

	url := fmt.Sprintf("/api/v1/documents/%d", doc.ID)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req = withTestUser(req, nil) // 添加测试用户
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	stored, ok := ndr.documents[doc.ID]
	if !ok {
		t.Fatalf("expected document to remain after soft delete")
	}
	if stored.DeletedAt == nil {
		t.Fatalf("expected document to be marked deleted")
	}
}

func TestRestoreDocumentEndpoint(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	doc, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{Title: "Doc"})
	if err != nil {
		t.Fatalf("create document error: %v", err)
	}
	if err := ndr.DeleteDocument(context.Background(), ndrclient.RequestMeta{}, doc.ID); err != nil {
		t.Fatalf("delete document error: %v", err)
	}

	url := fmt.Sprintf("/api/v1/documents/%d/restore", doc.ID)
	req := httptest.NewRequest(http.MethodPost, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var restored ndrclient.Document
	if err := json.NewDecoder(rec.Body).Decode(&restored); err != nil {
		t.Fatalf("decode response error: %v", err)
	}
	if restored.ID != doc.ID {
		t.Fatalf("expected restored doc id %d, got %d", doc.ID, restored.ID)
	}
	stored, ok := ndr.documents[doc.ID]
	if !ok {
		t.Fatalf("document missing after restore")
	}
	if stored.DeletedAt != nil {
		t.Fatalf("expected deleted_at cleared after restore")
	}
}

func TestPurgeDocumentEndpoint(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	doc, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{Title: "Doc"})
	if err != nil {
		t.Fatalf("create document error: %v", err)
	}

	url := fmt.Sprintf("/api/v1/documents/%d/purge", doc.ID)
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if _, ok := ndr.documents[doc.ID]; ok {
		t.Fatalf("expected document to be purged")
	}
}

func TestListDeletedDocumentsEndpoint(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	doc, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{Title: "Doc"})
	if err != nil {
		t.Fatalf("create document error: %v", err)
	}
	if err := ndr.DeleteDocument(context.Background(), ndrclient.RequestMeta{}, doc.ID); err != nil {
		t.Fatalf("delete document error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/documents/trash", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var page ndrclient.DocumentsPage
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decode response error: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("expected one deleted document, got %+v", page)
	}
	if page.Items[0].ID != doc.ID {
		t.Fatalf("expected document id %d in trash, got %d", doc.ID, page.Items[0].ID)
	}
}

func TestGetDocumentEndpoint(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	doc, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{Title: "Doc"})
	if err != nil {
		t.Fatalf("create document error: %v", err)
	}

	url := fmt.Sprintf("/api/v1/documents/%d", doc.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var fetched ndrclient.Document
	if err := json.NewDecoder(rec.Body).Decode(&fetched); err != nil {
		t.Fatalf("decode response error: %v", err)
	}
	if fetched.ID != doc.ID {
		t.Fatalf("expected document id %d, got %d", doc.ID, fetched.ID)
	}
}

func TestDocumentCreationWithTypeAndPosition(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	payload := `{"title":"Test Document","type":"knowledge_overview_v1","position":5,"content":{"format":"html","data":"<p>Hello World</p>"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", strings.NewReader(payload))
	req = withTestUser(req, nil) // 添加测试用户
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 for document creation, got %d body=%s", rec.Code, rec.Body.String())
	}

	var doc ndrclient.Document
	if err := json.NewDecoder(rec.Body).Decode(&doc); err != nil {
		t.Fatalf("decode document response error: %v", err)
	}

	if doc.Title != "Test Document" {
		t.Fatalf("expected title 'Test Document', got '%s'", doc.Title)
	}
	if doc.Type == nil || *doc.Type != "knowledge_overview_v1" {
		t.Fatalf("expected type 'knowledge_overview_v1', got %v", doc.Type)
	}
	if doc.Position != 5 {
		t.Fatalf("expected position 5, got %d", doc.Position)
	}
}

func TestDocumentUpdateWithTypeAndPosition(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{})
	router := NewRouter(handler)

	// Create a document first
	docType := "knowledge_overview_v1"
	doc, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{
		Title:   "Original Title",
		Type:    &docType,
		Content: map[string]any{"format": "html", "data": "<p>Original content</p>"},
	})
	if err != nil {
		t.Fatalf("create document error: %v", err)
	}

	// Update the document
	payload := `{"title":"Updated Title","type":"dictation_v1","position":3,"content":{"format":"yaml","data":"word: 单词"}}`
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/documents/%d", doc.ID), strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for document update, got %d body=%s", rec.Code, rec.Body.String())
	}

	var updatedDoc ndrclient.Document
	if err := json.NewDecoder(rec.Body).Decode(&updatedDoc); err != nil {
		t.Fatalf("decode updated document response error: %v", err)
	}

	if updatedDoc.Title != "Updated Title" {
		t.Fatalf("expected updated title 'Updated Title', got '%s'", updatedDoc.Title)
	}
	if updatedDoc.Type == nil || *updatedDoc.Type != "dictation_v1" {
		t.Fatalf("expected updated type 'dictation_v1', got %v", updatedDoc.Type)
	}
	if updatedDoc.Position != 3 {
		t.Fatalf("expected updated position 3, got %d", updatedDoc.Position)
	}
}
