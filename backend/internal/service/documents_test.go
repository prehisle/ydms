package service

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/yjxt/ydms/backend/internal/cache"
	"github.com/yjxt/ydms/backend/internal/ndrclient"
)

func TestCreateDocument(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	fake.createDocResp = sampleDocument(1, "Test Doc", "knowledge_overview_v1", 1, now, now)
	svc := NewService(cache.NewNoop(), fake, nil)

	docType := "knowledge_overview_v1"
	position := 1
	doc, err := svc.CreateDocument(context.Background(), RequestMeta{}, DocumentCreateRequest{
		Title:    "Test Doc",
		Type:     &docType,
		Position: &position,
		Content:  map[string]any{"format": "html", "data": "<p>Hello World</p>"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if doc.Title != "Test Doc" || doc.Type == nil || *doc.Type != "knowledge_overview_v1" {
		t.Fatalf("unexpected document %+v", doc)
	}
	if len(fake.createdDocs) != 1 {
		t.Fatalf("expected one create call, got %d", len(fake.createdDocs))
	}
	created := fake.createdDocs[0]
	if created.Title != "Test Doc" || created.Type == nil || *created.Type != "knowledge_overview_v1" {
		t.Fatalf("unexpected created document payload %+v", created)
	}
}

func TestCreateDocumentWithoutType(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	fake.createDocResp = sampleDocument(2, "Plain Doc", "", 1, now, now)
	svc := NewService(cache.NewNoop(), fake, nil)

	doc, err := svc.CreateDocument(context.Background(), RequestMeta{}, DocumentCreateRequest{
		Title:   "Plain Doc",
		Content: map[string]any{"text": "Content"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if doc.Title != "Plain Doc" {
		t.Fatalf("unexpected document title %s", doc.Title)
	}
	if len(fake.createdDocs) != 1 {
		t.Fatalf("expected one create call, got %d", len(fake.createdDocs))
	}
}

func TestUpdateDocument(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	fake.updateDocResp = sampleDocument(3, "Updated Doc", "dictation_v1", 2, now, now)
	svc := NewService(cache.NewNoop(), fake, nil)

	newTitle := "Updated Doc"
	newType := "dictation_v1"
	newPosition := 2
	doc, err := svc.UpdateDocument(context.Background(), RequestMeta{}, 3, DocumentUpdateRequest{
		Title:    &newTitle,
		Type:     &newType,
		Position: &newPosition,
		Content:  map[string]any{"format": "yaml", "data": "word: 单词"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.Title != "Updated Doc" || doc.Type == nil || *doc.Type != "dictation_v1" {
		t.Fatalf("unexpected updated document %+v", doc)
	}
	if len(fake.updatedDocs) != 1 {
		t.Fatalf("expected update call")
	}
	updated := fake.updatedDocs[0]
	if updated.ID != 3 || updated.Body.Title == nil || *updated.Body.Title != "Updated Doc" {
		t.Fatalf("unexpected update payload %+v", updated)
	}
}

func TestReorderDocuments(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()

	// Setup fake responses for reorder call
	docA := sampleDocument(10, "Doc A", "knowledge_overview_v1", 0, now, now)
	docB := sampleDocument(11, "Doc B", "knowledge_overview_v1", 1, now, now)
	fake.reorderDocResp = []ndrclient.Document{docB, docA}

	svc := NewService(cache.NewNoop(), fake, nil)

	docs, err := svc.ReorderDocuments(context.Background(), RequestMeta{}, DocumentReorderRequest{
		OrderedIDs: []int64{10, 11},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(docs))
	}
	if len(fake.reorderDocPayloads) != 1 {
		t.Fatalf("expected 1 reorder call, got %d", len(fake.reorderDocPayloads))
	}
	payload := fake.reorderDocPayloads[0]
	if len(payload.OrderedIDs) != 2 || payload.OrderedIDs[0] != 10 || payload.OrderedIDs[1] != 11 {
		t.Fatalf("unexpected payload %+v", payload)
	}
	if payload.ApplyTypeFilter {
		t.Fatalf("expected ApplyTypeFilter to be false")
	}
	if docs[0].ID != docB.ID || docs[1].ID != docA.ID {
		t.Fatalf("unexpected reorder response %+v", docs)
	}
}

func TestReorderDocumentsWithTypeFilter(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	fake.reorderDocResp = []ndrclient.Document{
		sampleDocument(20, "Doc", "dictation_v1", 0, now, now),
	}

	typeValue := " dictation_v1 "
	svc := NewService(cache.NewNoop(), fake, nil)
	_, err := svc.ReorderDocuments(context.Background(), RequestMeta{}, DocumentReorderRequest{
		OrderedIDs: []int64{20},
		Type:       &typeValue,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.reorderDocPayloads) != 1 {
		t.Fatalf("expected 1 reorder call, got %d", len(fake.reorderDocPayloads))
	}
	payload := fake.reorderDocPayloads[0]
	if !payload.ApplyTypeFilter {
		t.Fatalf("expected ApplyTypeFilter to be true")
	}
	if payload.Type == nil || *payload.Type != "dictation_v1" {
		t.Fatalf("unexpected type payload %+v", payload.Type)
	}
}

func TestReorderDocumentsWithWhitespaceType(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()
	fake.reorderDocResp = []ndrclient.Document{
		sampleDocument(30, "Doc", "knowledge_overview_v1", 0, now, now),
	}

	blank := "  \t"
	svc := NewService(cache.NewNoop(), fake, nil)
	_, err := svc.ReorderDocuments(context.Background(), RequestMeta{}, DocumentReorderRequest{
		OrderedIDs: []int64{30},
		Type:       &blank,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.reorderDocPayloads) != 1 {
		t.Fatalf("expected 1 reorder call, got %d", len(fake.reorderDocPayloads))
	}
	payload := fake.reorderDocPayloads[0]
	if !payload.ApplyTypeFilter {
		t.Fatalf("expected ApplyTypeFilter to be true")
	}
	if payload.Type != nil {
		t.Fatalf("expected type to be nil for blank input, got %v", payload.Type)
	}
}

func TestReorderDocumentsEmptyOrderedIDs(t *testing.T) {
	fake := newFakeNDR()
	svc := NewService(cache.NewNoop(), fake, nil)

	_, err := svc.ReorderDocuments(context.Background(), RequestMeta{}, DocumentReorderRequest{
		OrderedIDs: []int64{},
	})
	if err == nil {
		t.Fatalf("expected error for empty ordered_ids")
	}
	if !errors.Is(err, ErrInvalidDocumentReorder) {
		t.Fatalf("expected ErrInvalidDocumentReorder, got %v", err)
	}
	if len(fake.reorderDocPayloads) != 0 {
		t.Fatalf("expected no reorder calls for invalid request")
	}
}

func TestDeleteDocument(t *testing.T) {
	fake := newFakeNDR()
	svc := NewService(cache.NewNoop(), fake, nil)

	if err := svc.DeleteDocument(context.Background(), RequestMeta{}, 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.deletedDocIDs) != 1 || fake.deletedDocIDs[0] != 42 {
		t.Fatalf("expected document 42 to be deleted, got %+v", fake.deletedDocIDs)
	}
}

func TestRestoreDocument(t *testing.T) {
	fake := newFakeNDR()
	fake.restoreDocResp = ndrclient.Document{ID: 7, Title: "Restored"}
	svc := NewService(cache.NewNoop(), fake, nil)

	doc, err := svc.RestoreDocument(context.Background(), RequestMeta{}, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.ID != 7 || doc.Title != "Restored" {
		t.Fatalf("unexpected restore response %+v", doc)
	}
	if len(fake.restoredDocIDs) != 1 || fake.restoredDocIDs[0] != 7 {
		t.Fatalf("expected restore call for document 7")
	}
}

func TestPurgeDocument(t *testing.T) {
	fake := newFakeNDR()
	svc := NewService(cache.NewNoop(), fake, nil)

	if err := svc.PurgeDocument(context.Background(), RequestMeta{}, 55); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.purgedDocIDs) != 1 || fake.purgedDocIDs[0] != 55 {
		t.Fatalf("expected purge call for document 55")
	}
}

func TestListDeletedDocuments(t *testing.T) {
	now := time.Now().UTC()
	fake := newFakeNDR()
	fake.docsListResp = ndrclient.DocumentsPage{
		Page:  1,
		Size:  2,
		Total: 2,
		Items: []ndrclient.Document{
			{
				ID:        1,
				Title:     "Active",
				Position:  1,
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				ID:        2,
				Title:     "Deleted",
				Position:  2,
				CreatedAt: now,
				UpdatedAt: now,
				DeletedAt: &now,
			},
		},
	}

	svc := NewService(cache.NewNoop(), fake, nil)

	page, err := svc.ListDeletedDocuments(context.Background(), RequestMeta{}, url.Values{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("expected one deleted document, got %+v", page)
	}
	if page.Items[0].ID != 2 {
		t.Fatalf("expected deleted document ID 2, got %d", page.Items[0].ID)
	}
}

func TestListDocumentVersionsUsesItemsFallback(t *testing.T) {
	fake := newFakeNDR()
	fake.docVersionsResp = ndrclient.DocumentVersionsPage{
		Page:  1,
		Size:  2,
		Total: 2,
		Items: []ndrclient.DocumentVersion{
			{DocumentID: 10, VersionNumber: 1, Title: "v1"},
			{DocumentID: 10, VersionNumber: 2, Title: "v2"},
		},
	}

	svc := NewService(cache.NewNoop(), fake, nil)

	page, err := svc.ListDocumentVersions(context.Background(), RequestMeta{}, 10, 1, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Total != 2 {
		t.Fatalf("expected total 2, got %d", page.Total)
	}
	if len(page.Versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(page.Versions))
	}
	if page.Versions[1].VersionNumber != 2 {
		t.Fatalf("expected version number 2, got %d", page.Versions[1].VersionNumber)
	}
}

func sampleDocument(id int64, title string, docType string, position int, created, updated time.Time) ndrclient.Document {
	var typePtr *string
	if docType != "" {
		typePtr = &docType
	}
	return ndrclient.Document{
		ID:        id,
		Title:     title,
		Type:      typePtr,
		Position:  position,
		CreatedAt: created,
		UpdatedAt: updated,
		Content:   map[string]any{},
		Metadata:  map[string]any{},
	}
}

func TestAddDocumentReference(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()

	// 源文档（doc 1）
	fake.getDocResp = sampleDocument(1, "Source Doc", "knowledge_overview_v1", 1, now, now)

	// 第二次调用 GetDocument 返回被引用的文档（doc 2）
	refDoc := sampleDocument(2, "Referenced Doc", "knowledge_overview_v1", 1, now, now)

	// 设置更新响应，包含新的引用
	updatedDoc := sampleDocument(1, "Source Doc", "knowledge_overview_v1", 1, now, now)
	updatedDoc.Metadata = map[string]any{
		"references": []any{
			map[string]any{
				"document_id": float64(2),
				"title":       "Referenced Doc",
				"added_at":    now.Format(time.RFC3339),
			},
		},
	}
	fake.updateDocResp = updatedDoc

	svc := NewService(cache.NewNoop(), fake, nil)

	// 使用自定义的 fake 实现来处理多个 GetDocument 调用
	callCount := 0
	oldGetDoc := fake.getDocResp
	fakeWithCounter := &fakeNDRWithGetDocCounter{
		fakeNDR: fake,
		counter: &callCount,
		docs:    []ndrclient.Document{oldGetDoc, refDoc},
	}
	svc.ndr = fakeWithCounter

	doc, err := svc.AddDocumentReference(context.Background(), RequestMeta{}, 1, 2)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// 验证返回的文档包含引用
	refs, ok := doc.Metadata["references"]
	if !ok {
		t.Fatal("expected metadata to contain references")
	}
	refsArray, ok := refs.([]any)
	if !ok {
		t.Fatal("expected references to be an array")
	}
	if len(refsArray) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(refsArray))
	}

	ref := refsArray[0].(map[string]any)
	if ref["document_id"] != float64(2) {
		t.Fatalf("expected document_id 2, got %v", ref["document_id"])
	}
	if ref["title"] != "Referenced Doc" {
		t.Fatalf("expected title 'Referenced Doc', got %v", ref["title"])
	}
}

func TestAddDocumentReference_SelfReference(t *testing.T) {
	fake := newFakeNDR()
	svc := NewService(cache.NewNoop(), fake, nil)

	_, err := svc.AddDocumentReference(context.Background(), RequestMeta{}, 1, 1)
	if err == nil {
		t.Fatal("expected error for self-reference")
	}
	if err.Error() != "cannot add self-reference" {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestRemoveDocumentReference(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()

	// 创建一个包含引用的文档
	docWithRef := sampleDocument(1, "Source Doc", "knowledge_overview_v1", 1, now, now)
	docWithRef.Metadata = map[string]any{
		"references": []any{
			map[string]any{
				"document_id": float64(2),
				"title":       "Referenced Doc",
				"added_at":    now.Format(time.RFC3339),
			},
			map[string]any{
				"document_id": float64(3),
				"title":       "Another Doc",
				"added_at":    now.Format(time.RFC3339),
			},
		},
	}
	fake.getDocResp = docWithRef

	// 设置更新响应，移除了一个引用
	updatedDoc := sampleDocument(1, "Source Doc", "knowledge_overview_v1", 1, now, now)
	updatedDoc.Metadata = map[string]any{
		"references": []any{
			map[string]any{
				"document_id": float64(3),
				"title":       "Another Doc",
				"added_at":    now.Format(time.RFC3339),
			},
		},
	}
	fake.updateDocResp = updatedDoc

	svc := NewService(cache.NewNoop(), fake, nil)

	doc, err := svc.RemoveDocumentReference(context.Background(), RequestMeta{}, 1, 2)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// 验证引用已被删除
	refs, ok := doc.Metadata["references"]
	if !ok {
		t.Fatal("expected metadata to contain references")
	}
	refsArray, ok := refs.([]any)
	if !ok {
		t.Fatal("expected references to be an array")
	}
	if len(refsArray) != 1 {
		t.Fatalf("expected 1 reference remaining, got %d", len(refsArray))
	}

	ref := refsArray[0].(map[string]any)
	if ref["document_id"] != float64(3) {
		t.Fatalf("expected remaining reference to be doc 3, got %v", ref["document_id"])
	}
}

func TestRemoveDocumentReference_LastReference(t *testing.T) {
	fake := newFakeNDR()
	now := time.Now().UTC()

	// 创建一个只有一个引用的文档
	docWithOneRef := sampleDocument(1, "Source Doc", "knowledge_overview_v1", 1, now, now)
	docWithOneRef.Metadata = map[string]any{
		"references": []any{
			map[string]any{
				"document_id": float64(2),
				"title":       "Only Reference",
				"added_at":    now.Format(time.RFC3339),
			},
		},
		"other_field": "should remain",
	}
	fake.getDocResp = docWithOneRef

	// 设置更新响应：删除最后一个引用后，references 字段应该被删除
	updatedDoc := sampleDocument(1, "Source Doc", "knowledge_overview_v1", 1, now, now)
	updatedDoc.Metadata = map[string]any{
		"other_field": "should remain",
		// references 字段已被删除
	}
	fake.updateDocResp = updatedDoc

	svc := NewService(cache.NewNoop(), fake, nil)

	doc, err := svc.RemoveDocumentReference(context.Background(), RequestMeta{}, 1, 2)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// 验证 references 字段已被删除
	_, hasReferences := doc.Metadata["references"]
	if hasReferences {
		t.Fatal("expected references field to be deleted when last reference is removed")
	}

	// 验证其他字段仍然存在
	otherField, ok := doc.Metadata["other_field"]
	if !ok {
		t.Fatal("expected other_field to remain")
	}
	if otherField != "should remain" {
		t.Fatalf("expected other_field to be 'should remain', got %v", otherField)
	}
}

func TestGetReferencingDocuments(t *testing.T) {
	now := time.Now().UTC()
	fake := newFakeNDR()

	// 设置文档列表响应：doc 1 和 doc 3 引用了 doc 2
	doc1 := sampleDocument(1, "Doc 1", "knowledge_overview_v1", 1, now, now)
	doc1.Metadata = map[string]any{
		"references": []any{
			map[string]any{"document_id": float64(2), "title": "Doc 2", "added_at": now.Format(time.RFC3339)},
		},
	}

	doc2 := sampleDocument(2, "Doc 2", "knowledge_overview_v1", 1, now, now)
	doc2.Metadata = map[string]any{}

	doc3 := sampleDocument(3, "Doc 3", "knowledge_overview_v1", 1, now, now)
	doc3.Metadata = map[string]any{
		"references": []any{
			map[string]any{"document_id": float64(2), "title": "Doc 2", "added_at": now.Format(time.RFC3339)},
		},
	}

	fake.docsListResp = ndrclient.DocumentsPage{
		Page:  1,
		Size:  3,
		Total: 3,
		Items: []ndrclient.Document{doc1, doc2, doc3},
	}

	svc := NewService(cache.NewNoop(), fake, nil)

	docs, err := svc.GetReferencingDocuments(context.Background(), RequestMeta{}, 2, url.Values{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(docs) != 2 {
		t.Fatalf("expected 2 referencing documents, got %d", len(docs))
	}

	// 验证返回的文档是 doc 1 和 doc 3
	foundDoc1, foundDoc3 := false, false
	for _, doc := range docs {
		if doc.ID == 1 {
			foundDoc1 = true
		}
		if doc.ID == 3 {
			foundDoc3 = true
		}
	}

	if !foundDoc1 || !foundDoc3 {
		t.Fatal("expected to find doc 1 and doc 3 as referencing documents")
	}
}

// fakeNDRWithGetDocCounter 用于测试多次调用 GetDocument 的场景
type fakeNDRWithGetDocCounter struct {
	*fakeNDR
	counter *int
	docs    []ndrclient.Document
}

func (f *fakeNDRWithGetDocCounter) GetDocument(_ context.Context, _ ndrclient.RequestMeta, _ int64) (ndrclient.Document, error) {
	if *f.counter >= len(f.docs) {
		return ndrclient.Document{}, errors.New("no more documents")
	}
	doc := f.docs[*f.counter]
	*f.counter++
	return doc, nil
}
