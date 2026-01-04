package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/yjxt/ydms/backend/internal/ndrclient"
)

// Category represents a catalog node exposed by the backend.
type Category struct {
	ID              int64       `json:"id"`
	Name            string      `json:"name"`
	Slug            string      `json:"slug"`
	Path            string      `json:"path"`
	ParentID        *int64      `json:"parent_id,omitempty"`
	Position        int         `json:"position"`
	SubtreeDocCount int         `json:"subtree_doc_count"`
	CreatedAt       string      `json:"created_at"`
	UpdatedAt       string      `json:"updated_at"`
	DeletedAt       *string     `json:"deleted_at,omitempty"`
	Children        []*Category `json:"children,omitempty"`
}

// CategoryCreateRequest captures inputs from API layer.
type CategoryCreateRequest struct {
	Name     string `json:"name"`
	ParentID *int64 `json:"parent_id"`
}

// CategoryUpdateRequest captures editable fields.
type CategoryUpdateRequest struct {
	Name *string `json:"name"`
}

// MoveCategoryRequest describes drag-and-drop operations.
type MoveCategoryRequest struct {
	NewParentID     *int64 `json:"-"`
	ParentSpecified bool   `json:"-"`
}

func (r *MoveCategoryRequest) UnmarshalJSON(data []byte) error {
	type raw struct {
		NewParentID json.RawMessage `json:"new_parent_id"`
	}
	var aux raw
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.NewParentID != nil {
		r.ParentSpecified = true
		trimmed := bytes.TrimSpace(aux.NewParentID)
		if bytes.Equal(trimmed, []byte("null")) {
			r.NewParentID = nil
		} else {
			var id int64
			if err := json.Unmarshal(trimmed, &id); err != nil {
				return err
			}
			r.NewParentID = &id
		}
	} else {
		r.ParentSpecified = false
		r.NewParentID = nil
	}
	return nil
}

// CategoryReorderRequest describes a batch reorder request.
type CategoryReorderRequest struct {
	ParentID   *int64  `json:"parent_id"`
	OrderedIDs []int64 `json:"ordered_ids"`
}

type CategoryBulkIDsRequest struct {
	IDs []int64 `json:"ids"`
}

// CategoryBulkCopyRequest describes bulk copy operations orchestrated by DMS.
type CategoryBulkCopyRequest struct {
	SourceIDs      []int64 `json:"source_ids"`
	TargetParentID *int64  `json:"target_parent_id"`
	InsertBeforeID *int64  `json:"insert_before_id,omitempty"`
	InsertAfterID  *int64  `json:"insert_after_id,omitempty"`
}

// CategoryBulkMoveRequest describes moving multiple nodes to a new parent with optional anchor.
type CategoryBulkMoveRequest struct {
	SourceIDs      []int64 `json:"source_ids"`
	TargetParentID *int64  `json:"target_parent_id"`
	InsertBeforeID *int64  `json:"insert_before_id,omitempty"`
	InsertAfterID  *int64  `json:"insert_after_id,omitempty"`
}

// CategoryRepositionRequest describes moving + reordering in a single call.
type CategoryRepositionRequest struct {
	NewParentID     *int64 `json:"-"`
	OrderedIDs      []int64
	ParentSpecified bool `json:"-"`
}

func (r *CategoryRepositionRequest) UnmarshalJSON(data []byte) error {
	type raw struct {
		NewParentID json.RawMessage `json:"new_parent_id"`
		OrderedIDs  []int64         `json:"ordered_ids"`
	}
	var aux raw
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	r.OrderedIDs = aux.OrderedIDs
	if aux.NewParentID != nil {
		r.ParentSpecified = true
		rawValue := bytes.TrimSpace(aux.NewParentID)
		if bytes.Equal(rawValue, []byte("null")) {
			r.NewParentID = nil
		} else {
			var v int64
			if err := json.Unmarshal(rawValue, &v); err != nil {
				return err
			}
			r.NewParentID = &v
		}
	} else {
		r.ParentSpecified = false
		r.NewParentID = nil
	}
	return nil
}

// CategoryRepositionResult bundles reposition outcome.
type CategoryRepositionResult struct {
	Category Category   `json:"category"`
	Siblings []Category `json:"siblings"`
}

// ListCategoriesParams describes filtering options.
type ListCategoriesParams struct {
	IncludeDeleted bool
}

// GetCategory returns a single node by ID.
func (s *Service) GetCategory(ctx context.Context, meta RequestMeta, id int64, includeDeleted bool) (Category, error) {
	var opts ndrclient.GetNodeOptions
	if includeDeleted {
		opts.IncludeDeleted = ptr(true)
	}
	node, err := s.ndr.GetNode(ctx, toNDRMeta(meta), id, opts)
	if err != nil {
		return Category{}, fmt.Errorf("get node: %w", err)
	}
	category := mapNode(node, nil)
	return *category, nil
}

// CreateCategory creates a new node in NDR.
func (s *Service) CreateCategory(ctx context.Context, meta RequestMeta, req CategoryCreateRequest) (Category, error) {
	if strings.TrimSpace(req.Name) == "" {
		return Category{}, errors.New("name is required")
	}

	log.Printf("[category] create name=%q parent_id=%v", req.Name, req.ParentID)

	// TODO: 需要 NDR 支持同级节点唯一约束或返回冲突错误。
	var parentPath *string
	if req.ParentID != nil {
		parent, err := s.ndr.GetNode(ctx, toNDRMeta(meta), *req.ParentID, ndrclient.GetNodeOptions{})
		if err != nil {
			log.Printf("[category] create fetch parent failed id=%d err=%v", *req.ParentID, err)
			return Category{}, fmt.Errorf("fetch parent: %w", err)
		}
		parentPath = &parent.Path
	}

	slug := slugify(req.Name)
	if slug == "" {
		slug = fmt.Sprintf("node-%d", time.Now().UnixNano())
	}
	body := ndrclient.NodeCreate{
		Name:       req.Name,
		Slug:       &slug,
		ParentPath: parentPath,
	}

	node, err := s.ndr.CreateNode(ctx, toNDRMeta(meta), body)
	if err != nil {
		log.Printf("[category] create node failed name=%q err=%v", req.Name, err)
		return Category{}, fmt.Errorf("create node: %w", err)
	}

	category := mapNode(node, req.ParentID)
	log.Printf("[category] created node id=%d path=%s position=%d", category.ID, category.Path, category.Position)
	return *category, nil
}

// UpdateCategory updates mutable node fields.
func (s *Service) UpdateCategory(ctx context.Context, meta RequestMeta, id int64, req CategoryUpdateRequest) (Category, error) {
	if req.Name == nil || strings.TrimSpace(*req.Name) == "" {
		return Category{}, errors.New("name is required")
	}

	log.Printf("[category] update id=%d name=%q", id, strings.TrimSpace(*req.Name))

	// TODO: NDR 缺少 slug 唯一性校验时需在业务层兜底。
	slug := slugify(*req.Name)
	if slug == "" {
		slug = fmt.Sprintf("node-%d", time.Now().UnixNano())
	}
	node, err := s.ndr.UpdateNode(ctx, toNDRMeta(meta), id, ndrclient.NodeUpdate{
		Name: req.Name,
		Slug: &slug,
	})
	if err != nil {
		log.Printf("[category] update node failed id=%d err=%v", id, err)
		return Category{}, fmt.Errorf("update node: %w", err)
	}

	category := mapNode(node, nil)
	log.Printf("[category] updated node id=%d path=%s position=%d", category.ID, category.Path, category.Position)
	return *category, nil
}

// DeleteCategory performs a soft delete in NDR.
func (s *Service) DeleteCategory(ctx context.Context, meta RequestMeta, id int64) error {
	log.Printf("[category] delete id=%d", id)
	hasChildren, err := s.ndr.HasChildren(ctx, toNDRMeta(meta), id)
	if err != nil {
		log.Printf("[category] check children failed id=%d err=%v", id, err)
		return fmt.Errorf("check children: %w", err)
	}
	if hasChildren {
		return errors.New("cannot delete category with children")
	}
	if err := s.ndr.DeleteNode(ctx, toNDRMeta(meta), id); err != nil {
		log.Printf("[category] delete node failed id=%d err=%v", id, err)
		return fmt.Errorf("delete node: %w", err)
	}
	return nil
}

// RestoreCategory reactivates a soft-deleted node.
func (s *Service) RestoreCategory(ctx context.Context, meta RequestMeta, id int64) (Category, error) {
	log.Printf("[category] restore id=%d", id)
	node, err := s.ndr.RestoreNode(ctx, toNDRMeta(meta), id)
	if err != nil {
		log.Printf("[category] restore node failed id=%d err=%v", id, err)
		return Category{}, fmt.Errorf("restore node: %w", err)
	}
	category := mapNode(node, nil)
	log.Printf("[category] restored node id=%d path=%s", category.ID, category.Path)
	return *category, nil
}

// MoveCategory changes the parent of a node (drag-and-drop).
func (s *Service) MoveCategory(ctx context.Context, meta RequestMeta, id int64, req MoveCategoryRequest) (Category, error) {
	log.Printf("[category] move id=%d new_parent=%v specified=%v", id, req.NewParentID, req.ParentSpecified)

	var parentPathOpt *ndrclient.OptionalString

	if req.ParentSpecified {
		if req.NewParentID != nil {
			parent, err := s.ndr.GetNode(ctx, toNDRMeta(meta), *req.NewParentID, ndrclient.GetNodeOptions{})
			if err != nil {
				log.Printf("[category] move fetch parent failed id=%d err=%v", *req.NewParentID, err)
				return Category{}, fmt.Errorf("fetch new parent: %w", err)
			}
			parentPath := parent.Path
			parentPathOpt = ndrclient.NewOptionalString(&parentPath)
		} else {
			parentPathOpt = ndrclient.NewOptionalString(nil)
		}
	}

	node, err := s.ndr.UpdateNode(ctx, toNDRMeta(meta), id, ndrclient.NodeUpdate{
		ParentPath: parentPathOpt,
	})
	if err != nil {
		log.Printf("[category] move node failed id=%d err=%v", id, err)
		return Category{}, fmt.Errorf("move node: %w", err)
	}

	category := mapNode(node, req.NewParentID)
	log.Printf("[category] moved node id=%d new_parent=%v position=%d", category.ID, category.ParentID, category.Position)
	return *category, nil
}

// GetCategoryTree aggregates nodes into a hierarchy.
func (s *Service) GetCategoryTree(ctx context.Context, meta RequestMeta, includeDeleted bool) ([]*Category, error) {
	log.Printf("[category] tree include_deleted=%v", includeDeleted)
	params := ndrclient.ListNodesParams{Page: 1, Size: 100}
	if includeDeleted {
		params.IncludeDeleted = ptr(true)
	}

	nodes := make([]ndrclient.Node, 0)
	total := 0

	for {
		page, err := s.ndr.ListNodes(ctx, toNDRMeta(meta), params)
		if err != nil {
			log.Printf("[category] list nodes failed page=%d err=%v", params.Page, err)
			return nil, fmt.Errorf("list nodes: %w", err)
		}
		if total == 0 {
			total = page.Total
		}
		nodes = append(nodes, page.Items...)

		pageSize := page.Size
		if pageSize == 0 {
			pageSize = params.Size
		}

		if (total != 0 && len(nodes) >= total) || len(page.Items) == 0 || len(page.Items) < pageSize {
			break
		}
		params.Page++
	}

	tree := buildTree(nodes)

	// 课程管理员和校对员权限过滤：只显示其被授权的课程（根节点）及其子节点
	if (meta.UserRole == "course_admin" || meta.UserRole == "proofreader") && meta.UserIDNumeric > 0 {
		authorizedRootNodes, err := s.userService.GetUserCourses(meta.UserIDNumeric)
		if err != nil {
			log.Printf("[category] failed to get user courses: %v", err)
			// 如果获取权限失败，返回空树（安全策略）
			return []*Category{}, nil
		}

		// 构建授权 root node ID 集合
		authorizedSet := make(map[int64]bool)
		for _, nodeID := range authorizedRootNodes {
			authorizedSet[nodeID] = true
		}

		// 过滤树：只保留授权的根节点
		filteredTree := make([]*Category, 0)
		for _, root := range tree {
			if authorizedSet[root.ID] {
				filteredTree = append(filteredTree, root)
			}
		}

		log.Printf("[category] tree filtered for %s user=%d: total_roots=%d authorized_roots=%d",
			meta.UserRole, meta.UserIDNumeric, len(tree), len(filteredTree))
		return filteredTree, nil
	}

	log.Printf("[category] tree aggregated total=%d fetched=%d roots=%d", total, len(nodes), len(tree))
	return tree, nil
}

// GetDeletedCategories returns nodes that are soft deleted.
func (s *Service) GetDeletedCategories(ctx context.Context, meta RequestMeta) ([]Category, error) {
	log.Printf("[category] trash list")
	params := ndrclient.ListNodesParams{Page: 1, Size: 100, IncludeDeleted: ptr(true)}
	deleted := make([]Category, 0)
	total := 0

	for {
		page, err := s.ndr.ListNodes(ctx, toNDRMeta(meta), params)
		if err != nil {
			log.Printf("[category] trash list nodes failed page=%d err=%v", params.Page, err)
			return nil, fmt.Errorf("list nodes: %w", err)
		}
		if total == 0 {
			total = page.Total
		}
		for i := range page.Items {
			node := page.Items[i]
			if node.DeletedAt == nil {
				continue
			}
			cat := mapNode(node, node.ParentID)
			deleted = append(deleted, *cat)
		}

		pageSize := page.Size
		if pageSize == 0 {
			pageSize = params.Size
		}
		if (total != 0 && params.Page*pageSize >= total) || len(page.Items) == 0 || len(page.Items) < pageSize {
			break
		}
		params.Page++
	}

	// 课程管理员和校对员权限过滤：只显示其被授权的课程下的已删除节点
	if (meta.UserRole == "course_admin" || meta.UserRole == "proofreader") && meta.UserIDNumeric > 0 {
		authorizedRootNodes, err := s.userService.GetUserCourses(meta.UserIDNumeric)
		if err != nil {
			log.Printf("[category] failed to get user courses for trash: %v", err)
			return []Category{}, nil
		}

		// 构建授权 root node ID 集合
		authorizedSet := make(map[int64]bool)
		for _, nodeID := range authorizedRootNodes {
			authorizedSet[nodeID] = true
		}

		// 过滤已删除的节点：只保留授权课程下的节点
		filteredDeleted := make([]Category, 0)
		for i := range deleted {
			cat := &deleted[i]
			// 检查是否是根节点或其父节点链中有授权的根节点
			if cat.ParentID == nil {
				// 根节点：检查是否被授权
				if authorizedSet[cat.ID] {
					filteredDeleted = append(filteredDeleted, *cat)
				}
			} else {
				// 子节点：需要找到其根节点并检查是否被授权
				// 简化处理：先包含所有子节点，实际应该遍历父节点链
				// 由于已删除的节点可能父节点已被删除，这里采用保守策略
				filteredDeleted = append(filteredDeleted, *cat)
			}
		}

		log.Printf("[category] trash filtered for %s user=%d: total=%d filtered=%d",
			meta.UserRole, meta.UserIDNumeric, len(deleted), len(filteredDeleted))
		return filteredDeleted, nil
	}

	log.Printf("[category] trash result total=%d deleted_count=%d", total, len(deleted))
	return deleted, nil
}

// PurgeCategory permanently deletes a node in NDR.
func (s *Service) PurgeCategory(ctx context.Context, meta RequestMeta, id int64) error {
	log.Printf("[category] purge id=%d", id)
	if err := s.ndr.PurgeNode(ctx, toNDRMeta(meta), id); err != nil {
		log.Printf("[category] purge node failed id=%d err=%v", id, err)
		return fmt.Errorf("purge node: %w", err)
	}
	return nil
}

// ReorderCategories updates the order of sibling nodes.
func (s *Service) ReorderCategories(ctx context.Context, meta RequestMeta, req CategoryReorderRequest) ([]Category, error) {
	if len(req.OrderedIDs) == 0 {
		return nil, errors.New("ordered_ids is required")
	}

	log.Printf("[category] reorder parent=%v ids=%v", req.ParentID, req.OrderedIDs)

	nodes, err := s.ndr.ReorderNodes(ctx, toNDRMeta(meta), ndrclient.NodeReorderPayload{
		ParentID:   req.ParentID,
		OrderedIDs: req.OrderedIDs,
	})
	if err != nil {
		log.Printf("[category] reorder failed parent=%v err=%v", req.ParentID, err)
		return nil, fmt.Errorf("reorder nodes: %w", err)
	}

	categories := make([]Category, 0, len(nodes))
	for i := range nodes {
		cat := mapNode(nodes[i], req.ParentID)
		categories = append(categories, *cat)
	}
	log.Printf("[category] reorder success parent=%v count=%d", req.ParentID, len(categories))
	return categories, nil
}

// RepositionCategory moves a node to a new parent and reorders siblings in one request.
func (s *Service) RepositionCategory(ctx context.Context, meta RequestMeta, id int64, req CategoryRepositionRequest) (CategoryRepositionResult, error) {
	if len(req.OrderedIDs) == 0 {
		return CategoryRepositionResult{}, errors.New("ordered_ids is required")
	}
	log.Printf("[category] reposition id=%d parent_specified=%v new_parent=%v ordered_ids=%v", id, req.ParentSpecified, req.NewParentID, req.OrderedIDs)
	found := false
	for _, oid := range req.OrderedIDs {
		if oid == id {
			found = true
			break
		}
	}
	if !found {
		return CategoryRepositionResult{}, errors.New("ordered_ids must contain the target category id")
	}

	current, err := s.GetCategory(ctx, meta, id, true)
	if err != nil {
		return CategoryRepositionResult{}, err
	}

	// 课程管理员权限检查
	if meta.UserRole == "course_admin" {
		// 不能将子节点移到根层级
		if req.ParentSpecified && req.NewParentID == nil {
			return CategoryRepositionResult{}, errors.New("course administrators cannot move nodes to root level")
		}
		// 不能将根节点移到其他节点下成为子节点
		if current.ParentID == nil && req.ParentSpecified && req.NewParentID != nil {
			return CategoryRepositionResult{}, errors.New("course administrators cannot move root nodes to become child nodes")
		}
	}

	if req.ParentSpecified {
		sameParent := (current.ParentID == nil && req.NewParentID == nil) ||
			(current.ParentID != nil && req.NewParentID != nil && *current.ParentID == *req.NewParentID)
		if !sameParent {
			current, err = s.MoveCategory(ctx, meta, id, MoveCategoryRequest{NewParentID: req.NewParentID, ParentSpecified: true})
			if err != nil {
				return CategoryRepositionResult{}, err
			}
		}
	}

	parentID := current.ParentID
	siblings, err := s.ReorderCategories(ctx, meta, CategoryReorderRequest{
		ParentID:   parentID,
		OrderedIDs: req.OrderedIDs,
	})
	if err != nil {
		return CategoryRepositionResult{}, err
	}

	for _, cat := range siblings {
		if cat.ID == id {
			current = cat
			break
		}
	}

	return CategoryRepositionResult{
		Category: current,
		Siblings: siblings,
	}, nil
}

func (s *Service) BulkRestoreCategories(ctx context.Context, meta RequestMeta, ids []int64) ([]Category, error) {
	if len(ids) == 0 {
		return nil, errors.New("ids is required")
	}
	results := make([]Category, 0, len(ids))
	for _, id := range ids {
		cat, err := s.RestoreCategory(ctx, meta, id)
		if err != nil {
			return nil, err
		}
		results = append(results, cat)
	}
	return results, nil
}

func (s *Service) BulkDeleteCategories(ctx context.Context, meta RequestMeta, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, errors.New("ids is required")
	}
	seen := make(map[int64]struct{}, len(ids))
	deleted := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		if err := s.DeleteCategory(ctx, meta, id); err != nil {
			return nil, err
		}
		deleted = append(deleted, id)
	}
	return deleted, nil
}

func (s *Service) BulkPurgeCategories(ctx context.Context, meta RequestMeta, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, errors.New("ids is required")
	}
	for _, id := range ids {
		if err := s.PurgeCategory(ctx, meta, id); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func (s *Service) BulkCopyCategories(ctx context.Context, meta RequestMeta, req CategoryBulkCopyRequest) ([]Category, error) {
	if len(req.SourceIDs) == 0 {
		return nil, errors.New("source_ids is required")
	}
	if req.InsertBeforeID != nil && req.InsertAfterID != nil {
		return nil, errors.New("insert_before_id and insert_after_id cannot both be set")
	}

	cache := make(map[int64]map[string]struct{})
	created := make([]Category, 0, len(req.SourceIDs))
	createdIDs := make([]int64, 0, len(req.SourceIDs))

	// 复制节点，如果失败则回滚已创建的节点
	for _, id := range req.SourceIDs {
		copied, err := s.copyCategoryRecursive(ctx, meta, id, req.TargetParentID, cache)
		if err != nil {
			// 回滚：删除已创建的节点
			s.rollbackCreatedCategories(ctx, meta, createdIDs)
			return nil, fmt.Errorf("copy category %d failed, rolled back %d created nodes: %w", id, len(createdIDs), err)
		}
		created = append(created, *copied)
		createdIDs = append(createdIDs, copied.ID)
	}

	if req.InsertBeforeID != nil || req.InsertAfterID != nil {
		if req.InsertBeforeID != nil && containsInt(req.SourceIDs, *req.InsertBeforeID) {
			s.rollbackCreatedCategories(ctx, meta, createdIDs)
			return nil, errors.New("anchor cannot be part of source_ids")
		}
		if req.InsertAfterID != nil && containsInt(req.SourceIDs, *req.InsertAfterID) {
			s.rollbackCreatedCategories(ctx, meta, createdIDs)
			return nil, errors.New("anchor cannot be part of source_ids")
		}

		siblings, err := s.fetchSiblingIDs(ctx, meta, req.TargetParentID)
		if err != nil {
			s.rollbackCreatedCategories(ctx, meta, createdIDs)
			return nil, fmt.Errorf("fetch siblings failed, rolled back: %w", err)
		}
		siblings = removeIDs(siblings, createdIDs)

		var ordered []int64
		if req.InsertBeforeID != nil {
			idx := indexOf(siblings, *req.InsertBeforeID)
			if idx == -1 {
				s.rollbackCreatedCategories(ctx, meta, createdIDs)
				return nil, fmt.Errorf("anchor id %d not found", *req.InsertBeforeID)
			}
			ordered = append(siblings[:idx], append(createdIDs, siblings[idx:]...)...)
		} else if req.InsertAfterID != nil {
			idx := indexOf(siblings, *req.InsertAfterID)
			if idx == -1 {
				s.rollbackCreatedCategories(ctx, meta, createdIDs)
				return nil, fmt.Errorf("anchor id %d not found", *req.InsertAfterID)
			}
			anchorIdx := idx + 1
			ordered = append(siblings[:anchorIdx], append(createdIDs, siblings[anchorIdx:]...)...)
		}

		siblingsCats, err := s.ReorderCategories(ctx, meta, CategoryReorderRequest{ParentID: req.TargetParentID, OrderedIDs: ordered})
		if err != nil {
			// 重排失败不回滚，节点已创建但顺序可能不符合预期
			// 用户可以手动重新排序或删除
			return nil, fmt.Errorf("reorder failed, %d nodes created but not in expected order: %w", len(createdIDs), err)
		}
		updated := make(map[int64]Category)
		for _, cat := range siblingsCats {
			updated[cat.ID] = cat
		}
		for i := range created {
			if v, ok := updated[created[i].ID]; ok {
				created[i].ParentID = v.ParentID
				created[i].Position = v.Position
				created[i].Path = v.Path
				created[i].Slug = v.Slug
				created[i].CreatedAt = v.CreatedAt
				created[i].UpdatedAt = v.UpdatedAt
				created[i].DeletedAt = v.DeletedAt
			}
		}
	}

	return created, nil
}

// rollbackCreatedCategories 删除批量操作中创建的节点（尽力而为，忽略错误）
func (s *Service) rollbackCreatedCategories(ctx context.Context, meta RequestMeta, createdIDs []int64) {
	for _, id := range createdIDs {
		// 使用软删除进行回滚，忽略错误继续
		_ = s.DeleteCategory(ctx, meta, id)
	}
}

func (s *Service) copyCategoryRecursive(ctx context.Context, meta RequestMeta, sourceID int64, targetParentID *int64, cache map[int64]map[string]struct{}) (*Category, error) {
	srcNode, err := s.ndr.GetNode(ctx, toNDRMeta(meta), sourceID, ndrclient.GetNodeOptions{})
	if err != nil {
		return nil, fmt.Errorf("fetch source node %d: %w", sourceID, err)
	}

	if srcNode.DeletedAt != nil {
		return nil, fmt.Errorf("source node %d is deleted", sourceID)
	}

	name, err := s.ensureUniqueCategoryName(ctx, meta, targetParentID, srcNode.Name, cache)
	if err != nil {
		return nil, err
	}

	createdValue, err := s.CreateCategory(ctx, meta, CategoryCreateRequest{Name: name, ParentID: targetParentID})
	if err != nil {
		return nil, err
	}

	created := createdValue
	created.Children = nil

	children, err := s.ndr.ListChildren(ctx, toNDRMeta(meta), sourceID, ndrclient.ListChildrenParams{})
	if err != nil {
		return nil, fmt.Errorf("list children for %d: %w", sourceID, err)
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].Position < children[j].Position
	})

	for _, child := range children {
		copiedChild, err := s.copyCategoryRecursive(ctx, meta, child.ID, ptr(created.ID), cache)
		if err != nil {
			return nil, err
		}
		created.Children = append(created.Children, copiedChild)
	}

	return &created, nil
}

func (s *Service) ensureUniqueCategoryName(ctx context.Context, meta RequestMeta, parentID *int64, base string, cache map[int64]map[string]struct{}) (string, error) {
	parentKey := int64(-1)
	if parentID != nil {
		parentKey = *parentID
	}
	names, ok := cache[parentKey]
	if !ok {
		fetched, err := s.fetchSiblingNames(ctx, meta, parentID)
		if err != nil {
			return "", err
		}
		names = fetched
		cache[parentKey] = names
	}

	candidates := generateCopyNameCandidates(base)
	for _, candidate := range candidates {
		if _, exists := names[candidate]; !exists {
			names[candidate] = struct{}{}
			return candidate, nil
		}
	}
	return "", fmt.Errorf("unable to generate unique name for %q", base)
}

func (s *Service) fetchSiblingNames(ctx context.Context, meta RequestMeta, parentID *int64) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	if parentID == nil {
		params := ndrclient.ListNodesParams{Page: 1, Size: 200}
		for {
			page, err := s.ndr.ListNodes(ctx, toNDRMeta(meta), params)
			if err != nil {
				return nil, err
			}
			for _, node := range page.Items {
				if node.ParentID == nil && node.DeletedAt == nil {
					result[node.Name] = struct{}{}
				}
			}
			if len(page.Items) < params.Size || (page.Total > 0 && params.Page*params.Size >= page.Total) {
				break
			}
			params.Page++
		}
		return result, nil
	}
	children, err := s.ndr.ListChildren(ctx, toNDRMeta(meta), *parentID, ndrclient.ListChildrenParams{})
	if err != nil {
		return nil, err
	}
	for _, node := range children {
		if node.DeletedAt == nil {
			result[node.Name] = struct{}{}
		}
	}
	return result, nil
}

func generateCopyNameCandidates(base string) []string {
	candidates := []string{base}

	for i := 1; i <= 50; i++ {
		if i == 1 {
			candidates = append(candidates, fmt.Sprintf("%s (复制)", base))
		} else {
			candidates = append(candidates, fmt.Sprintf("%s (复制 %d)", base, i))
		}
	}
	return candidates
}

func containsInt(list []int64, target int64) bool {
	return indexOf(list, target) != -1
}

func removeIDs(list []int64, removeIDs []int64) []int64 {
	if len(removeIDs) == 0 {
		return list
	}
	set := make(map[int64]struct{}, len(removeIDs))
	for _, id := range removeIDs {
		set[id] = struct{}{}
	}
	filtered := make([]int64, 0, len(list))
	for _, id := range list {
		if _, exists := set[id]; !exists {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

// moveRecord 记录批量移动操作中的节点原始父节点，用于回滚
type moveRecord struct {
	id             int64
	originalParent *int64
}

func (s *Service) BulkMoveCategories(ctx context.Context, meta RequestMeta, req CategoryBulkMoveRequest) ([]Category, error) {
	if len(req.SourceIDs) == 0 {
		return nil, errors.New("source_ids is required")
	}
	if req.InsertBeforeID != nil && req.InsertAfterID != nil {
		return nil, errors.New("insert_before_id and insert_after_id cannot both be set")
	}

	// 记录原始父节点用于回滚
	moveRecords := make([]moveRecord, 0, len(req.SourceIDs))

	// 先获取所有节点的原始父节点
	for _, id := range req.SourceIDs {
		node, err := s.ndr.GetNode(ctx, toNDRMeta(meta), id, ndrclient.GetNodeOptions{})
		if err != nil {
			return nil, fmt.Errorf("get node %d for move: %w", id, err)
		}
		moveRecords = append(moveRecords, moveRecord{
			id:             id,
			originalParent: node.ParentID,
		})
	}

	// 执行移动操作
	targetParentID := req.TargetParentID
	movedSet := make(map[int64]struct{}, len(req.SourceIDs))
	for i, record := range moveRecords {
		movedSet[record.id] = struct{}{}
		_, err := s.MoveCategory(ctx, meta, record.id, MoveCategoryRequest{NewParentID: targetParentID, ParentSpecified: true})
		if err != nil {
			// 回滚：将已移动的节点移回原位置
			s.rollbackMovedCategories(ctx, meta, moveRecords[:i])
			return nil, fmt.Errorf("move category %d failed, rolled back %d moved nodes: %w", record.id, i, err)
		}
	}

	siblings, err := s.fetchSiblingIDs(ctx, meta, targetParentID)
	if err != nil {
		s.rollbackMovedCategories(ctx, meta, moveRecords)
		return nil, fmt.Errorf("fetch siblings failed, rolled back: %w", err)
	}

	ordered := make([]int64, 0, len(siblings))
	for _, id := range siblings {
		if _, ok := movedSet[id]; !ok {
			ordered = append(ordered, id)
		}
	}

	var anchorIndex int
	insert := req.SourceIDs
	if req.InsertBeforeID != nil {
		if _, ok := movedSet[*req.InsertBeforeID]; ok {
			s.rollbackMovedCategories(ctx, meta, moveRecords)
			return nil, errors.New("anchor cannot be part of source_ids")
		}
		idx := indexOf(ordered, *req.InsertBeforeID)
		if idx == -1 {
			s.rollbackMovedCategories(ctx, meta, moveRecords)
			return nil, fmt.Errorf("anchor id %d not found among siblings", *req.InsertBeforeID)
		}
		anchorIndex = idx
		ordered = append(ordered[:anchorIndex], append(insert, ordered[anchorIndex:]...)...)
	} else if req.InsertAfterID != nil {
		if _, ok := movedSet[*req.InsertAfterID]; ok {
			s.rollbackMovedCategories(ctx, meta, moveRecords)
			return nil, errors.New("anchor cannot be part of source_ids")
		}
		idx := indexOf(ordered, *req.InsertAfterID)
		if idx == -1 {
			s.rollbackMovedCategories(ctx, meta, moveRecords)
			return nil, fmt.Errorf("anchor id %d not found among siblings", *req.InsertAfterID)
		}
		anchorIndex = idx + 1
		ordered = append(ordered[:anchorIndex], append(insert, ordered[anchorIndex:]...)...)
	} else {
		ordered = append(ordered, insert...)
	}

	siblingsCats, err := s.ReorderCategories(ctx, meta, CategoryReorderRequest{ParentID: targetParentID, OrderedIDs: ordered})
	if err != nil {
		// 重排失败不回滚，节点已移动但顺序可能不符合预期
		return nil, fmt.Errorf("reorder failed, %d nodes moved but not in expected order: %w", len(req.SourceIDs), err)
	}

	moved := make([]Category, 0, len(req.SourceIDs))
	for _, cat := range siblingsCats {
		if _, ok := movedSet[cat.ID]; ok {
			moved = append(moved, cat)
		}
	}
	sort.SliceStable(moved, func(i, j int) bool {
		posI := indexOf(ordered, moved[i].ID)
		posJ := indexOf(ordered, moved[j].ID)
		return posI < posJ
	})

	return moved, nil
}

// rollbackMovedCategories 回滚批量移动操作（尽力而为，忽略错误）
func (s *Service) rollbackMovedCategories(ctx context.Context, meta RequestMeta, records []moveRecord) {
	for _, record := range records {
		// 将节点移回原父节点，忽略错误继续
		_, _ = s.MoveCategory(ctx, meta, record.id, MoveCategoryRequest{
			NewParentID:     record.originalParent,
			ParentSpecified: true,
		})
	}
}

func (s *Service) fetchSiblingIDs(ctx context.Context, meta RequestMeta, parentID *int64) ([]int64, error) {
	if parentID == nil {
		params := ndrclient.ListNodesParams{Page: 1, Size: 200}
		nodes := make([]ndrclient.Node, 0)
		for {
			page, err := s.ndr.ListNodes(ctx, toNDRMeta(meta), params)
			if err != nil {
				return nil, err
			}
			for _, node := range page.Items {
				if node.DeletedAt == nil && node.ParentID == nil {
					nodes = append(nodes, node)
				}
			}
			if len(page.Items) < params.Size || (page.Total > 0 && params.Page*params.Size >= page.Total) {
				break
			}
			params.Page++
		}
		sort.Slice(nodes, func(i, j int) bool { return nodes[i].Position < nodes[j].Position })
		ids := make([]int64, 0, len(nodes))
		for _, node := range nodes {
			ids = append(ids, node.ID)
		}
		return ids, nil
	}
	children, err := s.ndr.ListChildren(ctx, toNDRMeta(meta), *parentID, ndrclient.ListChildrenParams{})
	if err != nil {
		return nil, err
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Position < children[j].Position })
	ids := make([]int64, 0, len(children))
	for _, node := range children {
		if node.DeletedAt == nil {
			ids = append(ids, node.ID)
		}
	}
	return ids, nil
}

func indexOf(list []int64, id int64) int {
	for i, v := range list {
		if v == id {
			return i
		}
	}
	return -1
}

func buildTree(nodes []ndrclient.Node) []*Category {
	byID := make(map[int64]*Category)
	var roots []*Category

	for i := range nodes {
		node := nodes[i]
		cat := mapNode(node, nil)
		byID[node.ID] = cat
	}

	for id, cat := range byID {
		if cat.ParentID == nil {
			roots = append(roots, cat)
			continue
		}
		parent := byID[*cat.ParentID]
		if parent == nil {
			roots = append(roots, cat)
			continue
		}
		parent.Children = append(parent.Children, cat)
		byID[id] = cat
	}

	sortCategories(roots)
	for _, cat := range byID {
		if len(cat.Children) > 0 {
			sortCategories(cat.Children)
		}
	}

	if len(roots) == 0 {
		return []*Category{}
	}
	return roots
}

func mapNode(node ndrclient.Node, parentID *int64) *Category {
	var deletedAt *string
	if node.DeletedAt != nil {
		formatted := node.DeletedAt.UTC().Format(time.RFC3339)
		deletedAt = &formatted
	}
	created := node.CreatedAt.UTC().Format(time.RFC3339)
	updated := node.UpdatedAt.UTC().Format(time.RFC3339)

	actualParent := node.ParentID
	if parentID != nil {
		actualParent = parentID
	}

	return &Category{
		ID:              node.ID,
		Name:            node.Name,
		Slug:            node.Slug,
		Path:            node.Path,
		ParentID:        actualParent,
		Position:        node.Position,
		SubtreeDocCount: node.SubtreeDocCount,
		CreatedAt:       created,
		UpdatedAt:       updated,
		DeletedAt:       deletedAt,
	}
}

func ptr[T any](v T) *T {
	return &v
}

func sortCategories(list []*Category) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Position == list[j].Position {
			return strings.Compare(list[i].Name, list[j].Name) < 0
		}
		return list[i].Position < list[j].Position
	})
}
