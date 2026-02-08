package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type config struct {
	Types []typeSpec `yaml:"types"`
}

type typeSpec struct {
	ID            string       `yaml:"id"`
	Label         string       `yaml:"label"`
	ContentFormat string       `yaml:"content_format"`
	Template      string       `yaml:"template"`
	Backend       hookSpec     `yaml:"backend,omitempty"`
	Frontend      frontendSpec `yaml:"frontend,omitempty"`
}

type frontendSpec struct {
	HookImport string          `yaml:"hook_import"`
	Themes     []themeFileSpec `yaml:"themes,omitempty"`
}

type themeFileSpec struct {
	ID          string `yaml:"id"`
	Label       string `yaml:"label"`
	Path        string `yaml:"path"`
	Description string `yaml:"description,omitempty"`
}

type documentTypeDefinition struct {
	typeSpec
	TemplatePath    string
	TemplateContent string
	Themes          []themeDefinition
}

type themeDefinition struct {
	ID          string
	Label       string
	Description string
	CSSPath     string
}

type hookSpec struct {
	HookImport string `yaml:"hook_import"`
}

func main() {
	var (
		configPath  string
		repoRoot    string
		docTypesDir string
		backendDir  string
		frontendDir string
	)

	flag.StringVar(&configPath, "config", "../doc-types/config.yaml", "Path to document type configuration file")
	flag.StringVar(&repoRoot, "repo-root", "..", "Path to repository root")
	flag.StringVar(&docTypesDir, "doc-types", "", "Path to doc-types directory (defaults to <repo-root>/doc-types)")
	flag.StringVar(&backendDir, "backend-dir", ".", "Path to backend module root")
	flag.StringVar(&frontendDir, "frontend-dir", "../frontend", "Path to frontend application root")
	flag.Parse()

	if docTypesDir == "" {
		docTypesDir = filepath.Join(repoRoot, "doc-types")
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		fail(err)
	}

	definitions, err := loadDefinitions(cfg, docTypesDir)
	if err != nil {
		fail(err)
	}
	if len(definitions) == 0 {
		fail(fmt.Errorf("no document types found in %s", configPath))
	}

	if err := generateBackend(definitions, filepath.Join(backendDir, "internal", "service", "document_types_gen.go")); err != nil {
		fail(err)
	}

	generatedDir := filepath.Join(frontendDir, "src", "generated")
	if err := os.MkdirAll(generatedDir, 0o755); err != nil {
		fail(fmt.Errorf("create frontend generated dir: %w", err))
	}
	if err := generateFrontendDefinitions(definitions, filepath.Join(generatedDir, "documentTypes.ts")); err != nil {
		fail(err)
	}
	if err := generateFrontendTemplates(definitions, filepath.Join(generatedDir, "documentTemplates.ts")); err != nil {
		fail(err)
	}
	if err := generateFrontendHookImports(definitions, filepath.Join(generatedDir, "documentTypeImports.ts")); err != nil {
		fail(err)
	}
	if err := generateFrontendThemes(definitions, filepath.Join(generatedDir, "documentTypeThemes.ts")); err != nil {
		fail(err)
	}
}

func loadConfig(path string) (config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return config{}, fmt.Errorf("unmarshal config: %w", err)
	}
	return cfg, nil
}

func loadDefinitions(cfg config, docTypesDir string) ([]documentTypeDefinition, error) {
	if len(cfg.Types) == 0 {
		return nil, nil
	}
	defs := make([]documentTypeDefinition, 0, len(cfg.Types))
	seen := make(map[string]struct{}, len(cfg.Types))
	for _, spec := range cfg.Types {
		if spec.ID == "" {
			return nil, fmt.Errorf("document type entry missing id")
		}
		if _, exists := seen[spec.ID]; exists {
			return nil, fmt.Errorf("duplicate document type id %q", spec.ID)
		}
		seen[spec.ID] = struct{}{}

		if spec.ContentFormat == "" {
			return nil, fmt.Errorf("document type %q missing content_format", spec.ID)
		}
		if spec.Template == "" {
			return nil, fmt.Errorf("document type %q missing template", spec.ID)
		}

		templatePath := filepath.Join(docTypesDir, spec.ID, spec.Template)
		templateBytes, err := os.ReadFile(templatePath)
		if err != nil {
			return nil, fmt.Errorf("read template for %q: %w", spec.ID, err)
		}

		themeDefs, err := resolveThemeDefinitions(spec, docTypesDir)
		if err != nil {
			return nil, err
		}

		defs = append(defs, documentTypeDefinition{
			typeSpec:        spec,
			TemplatePath:    templatePath,
			TemplateContent: string(templateBytes),
			Themes:          themeDefs,
		})
	}

	return defs, nil
}

func resolveThemeDefinitions(spec typeSpec, docTypesDir string) ([]themeDefinition, error) {
	if len(spec.Frontend.Themes) == 0 {
		return nil, nil
	}
	out := make([]themeDefinition, 0, len(spec.Frontend.Themes))
	for _, theme := range spec.Frontend.Themes {
		if theme.ID == "" {
			return nil, fmt.Errorf("frontend theme entry for %s missing id", spec.ID)
		}
		if theme.Path == "" {
			return nil, fmt.Errorf("frontend theme %s for %s missing path", theme.ID, spec.ID)
		}

		resolvedPath := theme.Path
		if !filepath.IsAbs(resolvedPath) {
			baseDir := filepath.Join(docTypesDir, spec.ID)
			resolvedPath = filepath.Join(baseDir, theme.Path)
		}
		absPath, err := filepath.Abs(resolvedPath)
		if err != nil {
			return nil, fmt.Errorf("resolve theme path for %s: %w", spec.ID, err)
		}
		if _, err := os.Stat(absPath); err != nil {
			return nil, fmt.Errorf("theme file for %s (theme %s) not found: %w", spec.ID, theme.ID, err)
		}

		out = append(out, themeDefinition{
			ID:          theme.ID,
			Label:       theme.Label,
			Description: theme.Description,
			CSSPath:     absPath,
		})
	}
	return out, nil
}

func generateBackend(defs []documentTypeDefinition, dest string) error {
	var buf bytes.Buffer
	buf.WriteString("// Code generated by docgen; DO NOT EDIT.\n")
	buf.WriteString("package service\n\n")

	imports := collectBackendHookImports(defs)
	if len(imports) > 0 {
		buf.WriteString("import (\n")
		for _, imp := range imports {
			buf.WriteString(fmt.Sprintf("\t_ %q\n", imp))
		}
		buf.WriteString(")\n\n")
	}

	buf.WriteString("func init() {\n")
	buf.WriteString("\tdocumentTypeDefinitions = map[DocumentType]DocumentTypeDefinition{\n")
	for _, def := range defs {
		formatConst, err := contentFormatConst(def.ContentFormat)
		if err != nil {
			return fmt.Errorf("document type %q: %w", def.ID, err)
		}
		path := relativePath(dest, def.TemplatePath)
		buf.WriteString(fmt.Sprintf("\t\tDocumentType(%q): {\n", def.ID))
		buf.WriteString(fmt.Sprintf("\t\t\tID: DocumentType(%q),\n", def.ID))
		buf.WriteString(fmt.Sprintf("\t\t\tLabel: %s,\n", quoteGoString(def.Label)))
		buf.WriteString(fmt.Sprintf("\t\t\tContentFormat: %s,\n", formatConst))
		buf.WriteString(fmt.Sprintf("\t\t\tTemplatePath: %s,\n", quoteGoString(path)))
		buf.WriteString("\t\t},\n")
	}
	buf.WriteString("\t}\n")

	buf.WriteString("\tdocumentTypeOrder = []DocumentType{\n")
	for _, def := range defs {
		buf.WriteString(fmt.Sprintf("\t\tDocumentType(%q),\n", def.ID))
	}
	buf.WriteString("\t}\n")
	buf.WriteString("}\n")

	return writeFileIfChanged(dest, buf.Bytes(), 0o644)
}

func generateFrontendDefinitions(defs []documentTypeDefinition, dest string) error {
	var buf bytes.Buffer
	buf.WriteString("/* eslint-disable */\n")
	buf.WriteString("// Code generated by docgen; DO NOT EDIT.\n\n")

	buf.WriteString("export interface DocumentTypeDefinition {\n")
	buf.WriteString("  id: string;\n")
	buf.WriteString("  label: string;\n")
	buf.WriteString("  contentFormat: \"html\" | \"yaml\" | \"markdown\";\n")
	buf.WriteString("  templatePath: string;\n")
	buf.WriteString("}\n\n")

	buf.WriteString("export const DOCUMENT_TYPE_DEFINITIONS = [\n")
	for _, def := range defs {
		path := relativePath(dest, def.TemplatePath)
		buf.WriteString("  {\n")
		buf.WriteString(fmt.Sprintf("    id: %s,\n", quoteTSString(def.ID)))
		buf.WriteString(fmt.Sprintf("    label: %s,\n", quoteTSString(def.Label)))
		buf.WriteString(fmt.Sprintf("    contentFormat: %s,\n", quoteTSString(def.ContentFormat)))
		buf.WriteString(fmt.Sprintf("    templatePath: %s,\n", quoteTSString(path)))
		buf.WriteString("  },\n")
	}
	buf.WriteString("] as const satisfies readonly DocumentTypeDefinition[];\n\n")

	buf.WriteString("export const DOCUMENT_TYPES = {\n")
	for _, def := range defs {
		buf.WriteString(fmt.Sprintf("  %s: %s,\n", toExportKey(def.ID), quoteTSString(def.ID)))
	}
	buf.WriteString("} as const;\n\n")

	buf.WriteString("export type DocumentType = typeof DOCUMENT_TYPES[keyof typeof DOCUMENT_TYPES];\n\n")

	buf.WriteString("export const DOCUMENT_TYPE_OPTIONS = [\n")
	for _, def := range defs {
		buf.WriteString("  {\n")
		buf.WriteString(fmt.Sprintf("    value: %s,\n", quoteTSString(def.ID)))
		buf.WriteString(fmt.Sprintf("    label: %s,\n", quoteTSString(def.Label)))
		buf.WriteString("  },\n")
	}
	buf.WriteString("] as const;\n\n")

	buf.WriteString("export const DOCUMENT_TYPE_MAP = {\n")
	for i, def := range defs {
		buf.WriteString(fmt.Sprintf("  %s: DOCUMENT_TYPE_DEFINITIONS[%d],\n", quoteTSKey(def.ID), i))
	}
	buf.WriteString("} as const;\n")

	return writeFileIfChanged(dest, buf.Bytes(), 0o644)
}

func generateFrontendTemplates(defs []documentTypeDefinition, dest string) error {
	var buf bytes.Buffer
	buf.WriteString("/* eslint-disable */\n")
	buf.WriteString("// Code generated by docgen; DO NOT EDIT.\n\n")
	buf.WriteString("export const DOCUMENT_TEMPLATES = {\n")
	for _, def := range defs {
		buf.WriteString(fmt.Sprintf("  %s: { format: %s, data: %s },\n",
			quoteTSKey(def.ID),
			quoteTSString(def.ContentFormat),
			quoteTSString(def.TemplateContent),
		))
	}
	buf.WriteString("} as const;\n\n")
	buf.WriteString("export type DocumentTemplateType = keyof typeof DOCUMENT_TEMPLATES;\n\n")
	buf.WriteString("export function getDocumentTemplate(type: string) {\n")
	buf.WriteString("  const template = (DOCUMENT_TEMPLATES as Record<string, { format: string; data: string }>)[type];\n")
	buf.WriteString("  return template ?? null;\n")
	buf.WriteString("}\n")

	return writeFileIfChanged(dest, buf.Bytes(), 0o644)
}

func generateFrontendHookImports(defs []documentTypeDefinition, dest string) error {
	imports := collectFrontendHookImports(defs)
	if len(imports) == 0 {
		var buf bytes.Buffer
		buf.WriteString("/* eslint-disable */\n")
		buf.WriteString("// Code generated by docgen; DO NOT EDIT.\n\n")
		buf.WriteString("// No document type frontend hooks registered.\n")
		buf.WriteString("export const DOCUMENT_TYPE_HOOKS_IMPORTED = false;\n")
		return writeFileIfChanged(dest, buf.Bytes(), 0o644)
	}

	var buf bytes.Buffer
	buf.WriteString("/* eslint-disable */\n")
	buf.WriteString("// Code generated by docgen; DO NOT EDIT.\n\n")
	for _, imp := range imports {
		buf.WriteString(fmt.Sprintf("import %q;\n", imp))
	}
	buf.WriteString("\nexport const DOCUMENT_TYPE_HOOKS_IMPORTED = true;\n")
	return writeFileIfChanged(dest, buf.Bytes(), 0o644)
}

func contentFormatConst(format string) (string, error) {
	switch strings.ToLower(format) {
	case "html":
		return "ContentFormatHTML", nil
	case "yaml":
		return "ContentFormatYAML", nil
	case "markdown":
		return "ContentFormatMarkdown", nil
	case "json":
		return "ContentFormatJSON", nil
	default:
		return "", fmt.Errorf("unsupported content format %q", format)
	}
}

func relativePath(from, target string) string {
	fromDir := filepath.Dir(from)
	rel, err := filepath.Rel(fromDir, target)
	if err != nil {
		return target
	}
	return filepath.ToSlash(rel)
}

func quoteGoString(value string) string {
	return strconv.Quote(value)
}

func quoteTSString(value string) string {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		// fallback
		return strconv.Quote(value)
	}
	out := strings.TrimSpace(buf.String())
	return out
}

func quoteTSKey(value string) string {
	if isValidTSIdentifier(value) {
		return value
	}
	return quoteTSString(value)
}

var tsIdentifierPattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

func isValidTSIdentifier(value string) bool {
	return tsIdentifierPattern.MatchString(value)
}

func toExportKey(id string) string {
	replacer := strings.NewReplacer("-", "_", ".", "_")
	return strings.ToUpper(replacer.Replace(id))
}

func generateFrontendThemes(defs []documentTypeDefinition, dest string) error {
	type themeEntry struct {
		def   documentTypeDefinition
		theme themeDefinition
		varID string
	}

	entries := make([]themeEntry, 0)
	for _, def := range defs {
		for _, theme := range def.Themes {
			entries = append(entries, themeEntry{def: def, theme: theme})
		}
	}

	var buf bytes.Buffer
	buf.WriteString("/* eslint-disable */\n")
	buf.WriteString("// Code generated by docgen; DO NOT EDIT.\n\n")

	if len(entries) == 0 {
		buf.WriteString("export const DOCUMENT_TYPE_THEMES = {} as const;\n")
		return writeFileIfChanged(dest, buf.Bytes(), 0o644)
	}

	destDirAbs, err := filepath.Abs(filepath.Dir(dest))
	if err != nil {
		return fmt.Errorf("determine absolute dir for %s: %w", dest, err)
	}

	for i := range entries {
		rel, err := filepath.Rel(destDirAbs, entries[i].theme.CSSPath)
		if err != nil {
			return fmt.Errorf("compute relative path for theme %s/%s: %w", entries[i].def.ID, entries[i].theme.ID, err)
		}
		rel = filepath.ToSlash(rel)
		entries[i].varID = fmt.Sprintf("themeCss_%d", i)
		buf.WriteString(fmt.Sprintf("import %s from %q;\n", entries[i].varID, rel+"?inline"))
	}

	buf.WriteString("\nexport const DOCUMENT_TYPE_THEMES = {\n")

	type themesByType struct {
		typeID string
		items  []themeEntry
	}

	groups := make([]themesByType, 0)
	byType := make(map[string][]themeEntry)
	for _, entry := range entries {
		byType[entry.def.ID] = append(byType[entry.def.ID], entry)
	}
	keys := make([]string, 0, len(byType))
	for typeID := range byType {
		keys = append(keys, typeID)
	}
	sort.Strings(keys)
	for _, typeID := range keys {
		items := byType[typeID]
		groups = append(groups, themesByType{typeID: typeID, items: items})
	}

	for _, group := range groups {
		buf.WriteString(fmt.Sprintf("  %q: [\n", group.typeID))
		for _, entry := range group.items {
			label := entry.theme.Label
			if label == "" {
				label = entry.theme.ID
			}
			buf.WriteString("    {")
			buf.WriteString(fmt.Sprintf(" id: %q,", entry.theme.ID))
			buf.WriteString(fmt.Sprintf(" label: %q,", label))
			if entry.theme.Description != "" {
				buf.WriteString(fmt.Sprintf(" description: %q,", entry.theme.Description))
			}
			buf.WriteString(fmt.Sprintf(" css: %s", entry.varID))
			buf.WriteString(" },\n")
		}
		buf.WriteString("  ],\n")
	}

	buf.WriteString("} as const;\n")

	return writeFileIfChanged(dest, buf.Bytes(), 0o644)
}

func collectBackendHookImports(defs []documentTypeDefinition) []string {
	return collectHookImports(defs, func(def documentTypeDefinition) string {
		return def.Backend.HookImport
	})
}

func collectFrontendHookImports(defs []documentTypeDefinition) []string {
	return collectHookImports(defs, func(def documentTypeDefinition) string {
		return def.Frontend.HookImport
	})
}

func collectHookImports(defs []documentTypeDefinition, extract func(documentTypeDefinition) string) []string {
	if len(defs) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	for _, def := range defs {
		importPath := strings.TrimSpace(extract(def))
		if importPath == "" {
			continue
		}
		seen[importPath] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for importPath := range seen {
		out = append(out, importPath)
	}
	sort.Strings(out)
	return out
}

func writeFileIfChanged(path string, data []byte, perm os.FileMode) error {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "docgen: %v\n", err)
	os.Exit(1)
}
