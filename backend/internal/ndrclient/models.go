package ndrclient

import (
	"encoding/json"
	"time"
)

// Node represents the NDR node resource.
type Node struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	Slug            string     `json:"slug"`
	Path            string     `json:"path"`
	ParentID        *int64     `json:"parent_id"`
	Position        int        `json:"position"`
	SubtreeDocCount int        `json:"subtree_doc_count"`
	CreatedBy       string     `json:"created_by"`
	UpdatedBy       string     `json:"updated_by"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       *time.Time `json:"deleted_at"`
}

// NodeCreate mirrors NDR create payload.
type NodeCreate struct {
	Name       string  `json:"name"`
	Slug       *string `json:"slug,omitempty"`
	ParentPath *string `json:"parent_path,omitempty"`
}

// NodeUpdate mirrors NDR update payload.
type NodeUpdate struct {
	Name       *string         `json:"name,omitempty"`
	Slug       *string         `json:"slug,omitempty"`
	ParentPath *OptionalString `json:"parent_path,omitempty"`
}

// NodeReorderPayload represents the body for batch reordering nodes.
type NodeReorderPayload struct {
	ParentID   *int64  `json:"parent_id"`
	OrderedIDs []int64 `json:"ordered_ids"`
}

// NodesPage wraps paginated node results.
type NodesPage struct {
	Page  int    `json:"page"`
	Size  int    `json:"size"`
	Total int    `json:"total"`
	Items []Node `json:"items"`
}

// ListNodesParams describes optional query params.
type ListNodesParams struct {
	Page           int
	Size           int
	IncludeDeleted *bool
}

// ListChildrenParams describes optional query params for children listing.
type ListChildrenParams struct {
	Depth int
}

// GetNodeOptions describes optional query params for fetching node detail.
type GetNodeOptions struct {
	IncludeDeleted *bool
}

// Document represents the NDR document resource.
type Document struct {
	ID        int64          `json:"id"`
	Title     string         `json:"title"`
	Version   *int           `json:"version_number,omitempty"`
	Content   map[string]any `json:"content"`
	Type      *string        `json:"type"`
	Position  int            `json:"position"`
	CreatedBy string         `json:"created_by"`
	UpdatedBy string         `json:"updated_by"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt *time.Time     `json:"deleted_at"`
	Metadata  map[string]any `json:"metadata"`
}

// DocumentCreate mirrors the upstream create payload.
type DocumentCreate struct {
	Title    string         `json:"title"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Content  map[string]any `json:"content,omitempty"`
	Type     *string        `json:"type,omitempty"`
	Position *int           `json:"position,omitempty"`
}

// DocumentUpdate mirrors the upstream update payload.
type DocumentUpdate struct {
	Title    *string        `json:"title,omitempty"`
	Content  map[string]any `json:"content,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Type     *string        `json:"type,omitempty"`
	Position *int           `json:"position,omitempty"`
}

// DocumentReorderPayload represents a request to reorder documents.
type DocumentReorderPayload struct {
	OrderedIDs      []int64 `json:"ordered_ids"`
	Type            *string `json:"type,omitempty"`
	ApplyTypeFilter bool    `json:"-"`
}

// MarshalJSON ensures the type field is only emitted when explicitly requested.
func (p DocumentReorderPayload) MarshalJSON() ([]byte, error) {
	if !p.ApplyTypeFilter {
		type base struct {
			OrderedIDs []int64 `json:"ordered_ids"`
		}
		return json.Marshal(base{OrderedIDs: p.OrderedIDs})
	}
	type withType struct {
		OrderedIDs []int64 `json:"ordered_ids"`
		Type       *string `json:"type"`
	}
	return json.Marshal(withType{OrderedIDs: p.OrderedIDs, Type: p.Type})
}

// DocumentsPage wraps paginated document results.
type DocumentsPage struct {
	Page  int        `json:"page"`
	Size  int        `json:"size"`
	Total int        `json:"total"`
	Items []Document `json:"items"`
}

// OptionalString allows distinguishing between "not set" and "explicit null".
type OptionalString struct {
	value *string
}

// NewOptionalString returns an OptionalString that will marshal to the provided value.
// Passing nil produces an explicit `null`.
func NewOptionalString(value *string) *OptionalString {
	return &OptionalString{value: value}
}

// Value reports the underlying pointer and whether the field was set.
func (o *OptionalString) Value() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return o.value, true
}

// MarshalJSON ensures the field is present even when value is nil.
func (o *OptionalString) MarshalJSON() ([]byte, error) {
	if o == nil {
		return []byte("null"), nil
	}
	if o.value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*o.value)
}

// Relationship represents the NDR relationship resource.
type Relationship struct {
	NodeID     int64  `json:"node_id"`
	DocumentID int64  `json:"document_id"`
	CreatedBy  string `json:"created_by"`
}

// DocumentBindingStatus represents the binding status of a document.
type DocumentBindingStatus struct {
	TotalBindings int     `json:"total_bindings"`
	NodeIDs       []int64 `json:"node_ids"`
}

// DocumentVersion represents a version of a document.
type DocumentVersion struct {
	DocumentID    int64          `json:"document_id"`
	VersionNumber int            `json:"version_number"`
	Title         string         `json:"title"`
	Content       map[string]any `json:"content"`
	Metadata      map[string]any `json:"metadata"`
	Type          *string        `json:"type"`
	CreatedBy     string         `json:"created_by"`
	CreatedAt     time.Time      `json:"created_at"`
	ChangeMessage *string        `json:"change_message"`
}

// DocumentVersionsPage wraps paginated version results.
type DocumentVersionsPage struct {
	Page     int               `json:"page"`
	Size     int               `json:"size"`
	Total    int               `json:"total"`
	Versions []DocumentVersion `json:"versions,omitempty"`
	Items    []DocumentVersion `json:"items,omitempty"`
}

// DocumentVersionDiff represents differences between two versions.
type DocumentVersionDiff struct {
	FromVersion int            `json:"from_version"`
	ToVersion   int            `json:"to_version"`
	TitleDiff   *DiffDetail    `json:"title_diff,omitempty"`
	ContentDiff map[string]any `json:"content_diff,omitempty"`
	MetaDiff    map[string]any `json:"metadata_diff,omitempty"`
}

// DiffDetail represents the difference in a specific field.
type DiffDetail struct {
	Old any `json:"old"`
	New any `json:"new"`
}
