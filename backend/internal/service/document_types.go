//go:generate go run ../../cmd/docgen --config ../../../doc-types/config.yaml --repo-root ../../.. --frontend-dir ../../../frontend --backend-dir ../..

package service

import (
	"fmt"
	"sort"
)

// DocumentType defines the type of document content.
type DocumentType string

// ContentFormat defines the storage format of document content.
type ContentFormat string

const (
	ContentFormatHTML     ContentFormat = "html"
	ContentFormatYAML     ContentFormat = "yaml"
	ContentFormatMarkdown ContentFormat = "markdown"
	ContentFormatJSON     ContentFormat = "json"
)

// DocumentContent represents the structured content of a document.
type DocumentContent struct {
	Format ContentFormat `json:"format"`
	Data   string        `json:"data"`
}

// DocumentTypeDefinition captures configuration information for a document type.
type DocumentTypeDefinition struct {
	ID            DocumentType
	Label         string
	ContentFormat ContentFormat
	TemplatePath  string
}

var (
	documentTypeDefinitions map[DocumentType]DocumentTypeDefinition
	documentTypeOrder       []DocumentType
)

// ValidDocumentTypes returns all valid document types in configuration order.
func ValidDocumentTypes() []DocumentType {
	if len(documentTypeOrder) > 0 {
		out := make([]DocumentType, len(documentTypeOrder))
		copy(out, documentTypeOrder)
		return out
	}
	if len(documentTypeDefinitions) == 0 {
		return nil
	}
	derived := make([]DocumentType, 0, len(documentTypeDefinitions))
	for id := range documentTypeDefinitions {
		derived = append(derived, id)
	}
	sort.Slice(derived, func(i, j int) bool {
		return string(derived[i]) < string(derived[j])
	})
	return derived
}

// DocumentTypeDefinitions exposes a copy of configured document types keyed by ID.
func DocumentTypeDefinitions() map[DocumentType]DocumentTypeDefinition {
	if len(documentTypeDefinitions) == 0 {
		return nil
	}
	out := make(map[DocumentType]DocumentTypeDefinition, len(documentTypeDefinitions))
	for id, def := range documentTypeDefinitions {
		out[id] = def
	}
	return out
}

// IsValidDocumentType checks if a document type is valid.
func IsValidDocumentType(t string) bool {
	if len(documentTypeDefinitions) == 0 {
		return false
	}
	_, ok := documentTypeDefinitions[DocumentType(t)]
	return ok
}

// GetContentFormat returns the expected content format for a document type.
func GetContentFormat(docType DocumentType) ContentFormat {
	if len(documentTypeDefinitions) == 0 {
		return ContentFormatYAML
	}
	if def, ok := documentTypeDefinitions[docType]; ok {
		return def.ContentFormat
	}
	return ContentFormatYAML
}

// ValidateDocumentContent validates the document content structure.
func ValidateDocumentContent(content map[string]any, docType string) error {
	if !IsValidDocumentType(docType) {
		return fmt.Errorf("invalid document type: %s. Valid types: %v", docType, ValidDocumentTypes())
	}

	if content == nil {
		return nil // Empty content is allowed
	}

	// Check format field
	formatVal, hasFormat := content["format"]
	if !hasFormat {
		return fmt.Errorf("content must have 'format' field")
	}

	format, ok := formatVal.(string)
	if !ok {
		return fmt.Errorf("content.format must be a string")
	}

	expectedFormat := GetContentFormat(DocumentType(docType))
	if format != string(expectedFormat) {
		return fmt.Errorf("content format '%s' does not match expected format '%s' for type '%s'",
			format, expectedFormat, docType)
	}

	// Check data field
	dataVal, hasData := content["data"]
	if !hasData {
		return fmt.Errorf("content must have 'data' field")
	}

	if _, ok := dataVal.(string); !ok {
		return fmt.Errorf("content.data must be a string")
	}

	return nil
}

// ValidateDocumentMetadata validates common metadata fields.
func ValidateDocumentMetadata(metadata map[string]any) error {
	if metadata == nil {
		return nil
	}

	// Validate difficulty if present
	if diffVal, hasDiff := metadata["difficulty"]; hasDiff {
		switch v := diffVal.(type) {
		case float64:
			if v < 1 || v > 5 {
				return fmt.Errorf("difficulty must be between 1 and 5")
			}
		case int:
			if v < 1 || v > 5 {
				return fmt.Errorf("difficulty must be between 1 and 5")
			}
		default:
			return fmt.Errorf("difficulty must be a number")
		}
	}

	// Validate tags if present
	if tagsVal, hasTags := metadata["tags"]; hasTags {
		if _, ok := tagsVal.([]interface{}); !ok {
			return fmt.Errorf("tags must be an array")
		}
	}

	// Validate references if present
	// Note: nil value is allowed (RFC 7396) to delete the field
	if refsVal, hasRefs := metadata["references"]; hasRefs {
		// Allow nil to delete the field
		if refsVal == nil {
			return nil
		}

		refsArray, ok := refsVal.([]interface{})
		if !ok {
			return fmt.Errorf("references must be an array")
		}

		for i, refVal := range refsArray {
			refMap, ok := refVal.(map[string]interface{})
			if !ok {
				return fmt.Errorf("references[%d] must be an object", i)
			}

			// Validate document_id
			docIDVal, hasDocID := refMap["document_id"]
			if !hasDocID {
				return fmt.Errorf("references[%d] must have document_id field", i)
			}
			switch v := docIDVal.(type) {
			case float64:
				if v <= 0 {
					return fmt.Errorf("references[%d].document_id must be positive", i)
				}
			case int:
				if v <= 0 {
					return fmt.Errorf("references[%d].document_id must be positive", i)
				}
			case int64:
				if v <= 0 {
					return fmt.Errorf("references[%d].document_id must be positive", i)
				}
			default:
				return fmt.Errorf("references[%d].document_id must be a number", i)
			}

			// Validate title
			titleVal, hasTitle := refMap["title"]
			if !hasTitle {
				return fmt.Errorf("references[%d] must have title field", i)
			}
			if _, ok := titleVal.(string); !ok {
				return fmt.Errorf("references[%d].title must be a string", i)
			}

			// Validate added_at
			addedAtVal, hasAddedAt := refMap["added_at"]
			if !hasAddedAt {
				return fmt.Errorf("references[%d] must have added_at field", i)
			}
			if _, ok := addedAtVal.(string); !ok {
				return fmt.Errorf("references[%d].added_at must be a string", i)
			}
		}
	}

	return nil
}
