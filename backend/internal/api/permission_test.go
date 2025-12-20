package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yjxt/ydms/backend/internal/cache"
	"github.com/yjxt/ydms/backend/internal/database"
	"github.com/yjxt/ydms/backend/internal/ndrclient"
	"github.com/yjxt/ydms/backend/internal/service"
)

// 测试用户定义
var (
	testSuperAdmin = &database.User{
		ID:       1,
		Username: "super_admin",
		Role:     "super_admin",
	}

	testCourseAdmin = &database.User{
		ID:       2,
		Username: "course_admin",
		Role:     "course_admin",
	}

	testProofreader = &database.User{
		ID:       3,
		Username: "proofreader",
		Role:     "proofreader",
	}
)

func TestProofreaderPermissions_DocumentOperations(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{
		APIKey: "test-key",
		UserID: "tester",
	})
	router := NewRouter(handler)

	t.Run("校对员不能创建文档", func(t *testing.T) {
		payload := `{"title":"New Document","type":"knowledge_overview_v1","content":{"format":"html","data":"<p>Test</p>"}}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", strings.NewReader(payload))
		req = withTestUser(req, testProofreader)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected status 403 Forbidden for proofreader creating document, got %d", rec.Code)
		}
	})

	t.Run("校对员不能删除文档", func(t *testing.T) {
		// 先以管理员身份创建文档
		doc, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{
			Title: "Test Document",
		})
		if err != nil {
			t.Fatalf("create document error: %v", err)
		}

		// 校对员尝试删除
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/documents/%d", doc.ID), nil)
		req = withTestUser(req, testProofreader)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected status 403 Forbidden for proofreader deleting document, got %d", rec.Code)
		}
	})

	t.Run("校对员可以查看文档", func(t *testing.T) {
		// 创建文档
		doc, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{
			Title: "Viewable Document",
		})
		if err != nil {
			t.Fatalf("create document error: %v", err)
		}

		// 校对员查看文档
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/documents/%d", doc.ID), nil)
		req = withTestUser(req, testProofreader)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200 OK for proofreader viewing document, got %d", rec.Code)
		}
	})

	t.Run("校对员可以编辑文档", func(t *testing.T) {
		// 创建文档
		doc, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{
			Title: "Editable Document",
		})
		if err != nil {
			t.Fatalf("create document error: %v", err)
		}

		// 校对员编辑文档
		payload := `{"title":"Updated by Proofreader"}`
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/documents/%d", doc.ID), strings.NewReader(payload))
		req = withTestUser(req, testProofreader)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200 OK for proofreader editing document, got %d", rec.Code)
		}
	})
}

func TestProofreaderPermissions_CategoryOperations(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{
		APIKey: "test-key",
		UserID: "tester",
	})
	router := NewRouter(handler)

	t.Run("校对员不能创建分类", func(t *testing.T) {
		payload := `{"name":"New Category"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/categories", strings.NewReader(payload))
		req = withTestUser(req, testProofreader)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected status 403 Forbidden for proofreader creating category, got %d", rec.Code)
		}
	})

	t.Run("校对员不能删除分类", func(t *testing.T) {
		// 先以管理员身份创建分类
		cat := createCategory(t, router, `{"name":"Test Category"}`)

		// 校对员尝试删除
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/categories/%d", cat.ID), nil)
		req = withTestUser(req, testProofreader)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected status 403 Forbidden for proofreader deleting category, got %d", rec.Code)
		}
	})

	t.Run("校对员不能更新分类", func(t *testing.T) {
		// 先以管理员身份创建分类
		cat := createCategory(t, router, `{"name":"Original Name"}`)

		// 校对员尝试重命名
		payload := `{"name":"New Name"}`
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/categories/%d", cat.ID), strings.NewReader(payload))
		req = withTestUser(req, testProofreader)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected status 403 Forbidden for proofreader updating category, got %d", rec.Code)
		}
	})

}

func TestCourseAdminPermissions_UserManagement(t *testing.T) {
	t.Run("课程管理员可以创建校对员", func(t *testing.T) {
		// 这个测试需要 UserService，暂时跳过
		// 在实际实现时需要 mock UserService
		t.Skip("需要 UserService 实现")
	})

	t.Run("课程管理员不能创建超级管理员", func(t *testing.T) {
		// 这个测试需要 UserService，暂时跳过
		t.Skip("需要 UserService 实现")
	})

	t.Run("课程管理员不能创建其他课程管理员", func(t *testing.T) {
		// 这个测试需要 UserService，暂时跳过
		t.Skip("需要 UserService 实现")
	})
}

func TestCourseAdminPermissions_DocumentAndCategoryOperations(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{
		APIKey: "test-key",
		UserID: "tester",
	})
	router := NewRouter(handler)

	t.Run("课程管理员可以创建文档", func(t *testing.T) {
		payload := `{"title":"Course Admin Document","type":"knowledge_overview_v1","content":{"format":"html","data":"<p>Test</p>"}}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", strings.NewReader(payload))
		req = withTestUser(req, testCourseAdmin)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Errorf("expected status 201 Created for course admin creating document, got %d", rec.Code)
		}
	})

	t.Run("课程管理员可以删除文档", func(t *testing.T) {
		doc, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{
			Title: "To Delete",
		})
		if err != nil {
			t.Fatalf("create document error: %v", err)
		}

		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/documents/%d", doc.ID), nil)
		req = withTestUser(req, testCourseAdmin)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("expected status 204 No Content for course admin deleting document, got %d", rec.Code)
		}
	})

	t.Run("课程管理员可以创建分类", func(t *testing.T) {
		payload := `{"name":"Course Admin Category"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/categories", strings.NewReader(payload))
		req = withTestUser(req, testCourseAdmin)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Errorf("expected status 201 Created for course admin creating category, got %d", rec.Code)
		}
	})

	t.Run("课程管理员可以删除分类", func(t *testing.T) {
		cat := createCategory(t, router, `{"name":"To Delete"}`)

		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/categories/%d", cat.ID), nil)
		req = withTestUser(req, testCourseAdmin)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("expected status 204 No Content for course admin deleting category, got %d", rec.Code)
		}
	})
}

func TestSuperAdminPermissions(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{
		APIKey: "test-key",
		UserID: "tester",
	})
	router := NewRouter(handler)

	t.Run("超级管理员可以创建文档", func(t *testing.T) {
		payload := `{"title":"Super Admin Document","type":"knowledge_overview_v1","content":{"format":"html","data":"<p>Test</p>"}}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", strings.NewReader(payload))
		req = withTestUser(req, testSuperAdmin)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Errorf("expected status 201 Created for super admin creating document, got %d", rec.Code)
		}
	})

	t.Run("超级管理员可以创建分类", func(t *testing.T) {
		payload := `{"name":"Super Admin Category"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/categories", strings.NewReader(payload))
		req = withTestUser(req, testSuperAdmin)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Errorf("expected status 201 Created for super admin creating category, got %d", rec.Code)
		}
	})

	t.Run("超级管理员可以执行所有操作", func(t *testing.T) {
		// 创建、查看、编辑、删除文档
		doc, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{
			Title: "Super Admin Test",
		})
		if err != nil {
			t.Fatalf("create document error: %v", err)
		}

		// 查看
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/documents/%d", doc.ID), nil)
		req = withTestUser(req, testSuperAdmin)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("super admin should be able to view document, got %d", rec.Code)
		}

		// 编辑
		payload := `{"title":"Updated by Super Admin"}`
		req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/documents/%d", doc.ID), strings.NewReader(payload))
		req = withTestUser(req, testSuperAdmin)
		req.Header.Set("Content-Type", "application/json")
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("super admin should be able to edit document, got %d", rec.Code)
		}

		// 删除
		req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/documents/%d", doc.ID), nil)
		req = withTestUser(req, testSuperAdmin)
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Errorf("super admin should be able to delete document, got %d", rec.Code)
		}
	})
}

func TestUnauthenticatedRequests(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{
		APIKey: "test-key",
		UserID: "tester",
	})
	router := NewRouter(handler)

	t.Run("未认证用户不能创建文档", func(t *testing.T) {
		payload := `{"title":"Unauthorized Document","type":"knowledge_overview_v1","content":{"format":"html","data":"<p>Test</p>"}}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/documents", strings.NewReader(payload))
		// 不添加用户上下文
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401 Unauthorized for unauthenticated user creating document, got %d", rec.Code)
		}
	})

	t.Run("未认证用户不能创建分类", func(t *testing.T) {
		payload := `{"name":"Unauthorized Category"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/categories", strings.NewReader(payload))
		// 不添加用户上下文
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401 Unauthorized for unauthenticated user creating category, got %d", rec.Code)
		}
	})

	t.Run("未认证用户不能删除文档", func(t *testing.T) {
		doc, err := ndr.CreateDocument(context.Background(), ndrclient.RequestMeta{}, ndrclient.DocumentCreate{
			Title: "Test",
		})
		if err != nil {
			t.Fatalf("create document error: %v", err)
		}

		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/documents/%d", doc.ID), nil)
		// 不添加用户上下文
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401 Unauthorized for unauthenticated user deleting document, got %d", rec.Code)
		}
	})

	t.Run("未认证用户不能删除分类", func(t *testing.T) {
		cat := createCategory(t, router, `{"name":"Test"}`)

		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/categories/%d", cat.ID), nil)
		// 不添加用户上下文
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401 Unauthorized for unauthenticated user deleting category, got %d", rec.Code)
		}
	})
}

func TestPermissionHelpers(t *testing.T) {
	ndr := newInMemoryNDR()
	svc := service.NewService(cache.NewNoop(), ndr, nil)
	handler := NewHandler(svc, nil, HeaderDefaults{
		APIKey: "test-key",
		UserID: "tester",
	})

	t.Run("requireNotProofreader 正确拒绝校对员", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = withTestUser(req, testProofreader)

		user, httpErr := handler.requireNotProofreader(req, "test action")

		if httpErr == nil {
			t.Error("expected error for proofreader, got nil")
		}
		if user != nil {
			t.Error("expected nil user for proofreader")
		}
		if httpErr != nil && httpErr.code != http.StatusForbidden {
			t.Errorf("expected status 403, got %d", httpErr.code)
		}
	})

	t.Run("requireNotProofreader 允许课程管理员", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = withTestUser(req, testCourseAdmin)

		user, httpErr := handler.requireNotProofreader(req, "test action")

		if httpErr != nil {
			t.Errorf("expected no error for course admin, got %v", httpErr.message)
		}
		if user == nil {
			t.Error("expected user for course admin, got nil")
		}
		if user != nil && user.Role != "course_admin" {
			t.Errorf("expected role course_admin, got %s", user.Role)
		}
	})

	t.Run("requireNotProofreader 允许超级管理员", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = withTestUser(req, testSuperAdmin)

		user, httpErr := handler.requireNotProofreader(req, "test action")

		if httpErr != nil {
			t.Errorf("expected no error for super admin, got %v", httpErr.message)
		}
		if user == nil {
			t.Error("expected user for super admin, got nil")
		}
		if user != nil && user.Role != "super_admin" {
			t.Errorf("expected role super_admin, got %s", user.Role)
		}
	})

	t.Run("requireRole 正确验证角色", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = withTestUser(req, testCourseAdmin)

		user, httpErr := handler.requireRole(req, "super_admin", "course_admin")

		if httpErr != nil {
			t.Errorf("expected no error for allowed role, got %v", httpErr.message)
		}
		if user == nil {
			t.Error("expected user for allowed role, got nil")
		}
	})

	t.Run("requireRole 拒绝未授权角色", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = withTestUser(req, testProofreader)

		user, httpErr := handler.requireRole(req, "super_admin", "course_admin")

		if httpErr == nil {
			t.Error("expected error for disallowed role, got nil")
		}
		if user != nil {
			t.Error("expected nil user for disallowed role")
		}
		if httpErr != nil && httpErr.code != http.StatusForbidden {
			t.Errorf("expected status 403, got %d", httpErr.code)
		}
	})
}
