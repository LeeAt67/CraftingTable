package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type compiledTool struct {
	tool   Tool
	schema *jsonschema.Schema
}

// ToolRegistry is an immutable, startup-validated set of tools.
type ToolRegistry struct {
	tools        map[string]compiledTool
	catalog      []ToolDefinition
	providerTool []ProviderToolDefinition
	catalogHash  string
}

type rejectingSchemaLoader struct{}

func (rejectingSchemaLoader) Load(string) (any, error) {
	return nil, errors.New("external schema references are not allowed")
}

// NewToolRegistry validates and compiles every tool.
func NewToolRegistry(tools []Tool) (*ToolRegistry, error) {
	registry := &ToolRegistry{
		tools: make(map[string]compiledTool, len(tools)),
	}
	for index, candidate := range tools {
		normalized, compiled, err := compileTool(candidate, index)
		if err != nil {
			return nil, err
		}
		if _, exists := registry.tools[normalized.Definition.Name]; exists {
			return nil, fmt.Errorf("duplicate tool name %q", normalized.Definition.Name)
		}
		registry.tools[normalized.Definition.Name] = compiledTool{
			tool:   normalized,
			schema: compiled,
		}
		registry.catalog = append(
			registry.catalog,
			cloneToolDefinition(normalized.Definition),
		)
	}
	sort.Slice(registry.catalog, func(i, j int) bool {
		return registry.catalog[i].Name < registry.catalog[j].Name
	})
	registry.providerTool = make(
		[]ProviderToolDefinition,
		0,
		len(registry.catalog),
	)
	for _, definition := range registry.catalog {
		registry.providerTool = append(registry.providerTool, ProviderToolDefinition{
			Type: "function",
			Function: ProviderFunctionTool{
				Name:        definition.Name,
				Description: definition.Description,
				Parameters:  append(json.RawMessage(nil), definition.InputSchema...),
			},
		})
	}
	hashBody, err := json.Marshal(registry.catalog)
	if err != nil {
		return nil, fmt.Errorf("encode tool catalog hash: %w", err)
	}
	hash := sha256.Sum256(hashBody)
	registry.catalogHash = hex.EncodeToString(hash[:])
	return registry, nil
}

func compileTool(
	candidate Tool,
	index int,
) (Tool, *jsonschema.Schema, error) {
	definition := cloneToolDefinition(candidate.Definition)
	definition.Name = strings.TrimSpace(definition.Name)
	definition.Namespace = strings.TrimSpace(definition.Namespace)
	definition.Version = strings.TrimSpace(definition.Version)
	definition.Title = strings.TrimSpace(definition.Title)
	definition.Description = strings.TrimSpace(definition.Description)
	switch {
	case !toolNamePattern.MatchString(definition.Name):
		return Tool{}, nil, fmt.Errorf(
			"tool %d name must match %s",
			index,
			toolNamePattern.String(),
		)
	case definition.Version == "":
		return Tool{}, nil, fmt.Errorf("tool %q version is required", definition.Name)
	case candidate.Handler == nil:
		return Tool{}, nil, fmt.Errorf("tool %q handler is required", definition.Name)
	case definition.Timeout <= 0:
		return Tool{}, nil, fmt.Errorf("tool %q timeout must be positive", definition.Name)
	case definition.MaxInputBytes <= 0:
		return Tool{}, nil, fmt.Errorf("tool %q max input bytes must be positive", definition.Name)
	case definition.MaxResultBytes <= 0:
		return Tool{}, nil, fmt.Errorf("tool %q max result bytes must be positive", definition.Name)
	case len(definition.InputSchema) == 0:
		return Tool{}, nil, fmt.Errorf("tool %q input schema is required", definition.Name)
	}

	document, err := jsonschema.UnmarshalJSON(
		bytes.NewReader(definition.InputSchema),
	)
	if err != nil {
		return Tool{}, nil, fmt.Errorf("tool %q schema is invalid JSON: %w", definition.Name, err)
	}
	schemaObject, ok := document.(map[string]any)
	if !ok || schemaObject["type"] != "object" {
		return Tool{}, nil, fmt.Errorf("tool %q input schema must be an object", definition.Name)
	}
	if schemaObject["additionalProperties"] != false {
		return Tool{}, nil, fmt.Errorf(
			"tool %q input schema must set additionalProperties=false",
			definition.Name,
		)
	}

	location := toolSchemaLocation(definition)
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.UseLoader(rejectingSchemaLoader{})
	if err := compiler.AddResource(location, document); err != nil {
		return Tool{}, nil, fmt.Errorf("add tool %q schema: %w", definition.Name, err)
	}
	compiled, err := compiler.Compile(location)
	if err != nil {
		return Tool{}, nil, fmt.Errorf("compile tool %q schema: %w", definition.Name, err)
	}
	return Tool{
		Definition: definition,
		Handler:    candidate.Handler,
	}, compiled, nil
}

func toolSchemaLocation(definition ToolDefinition) string {
	return fmt.Sprintf(
		"urn:craftingtable:tool:%s:%s",
		definition.Name,
		definition.Version,
	)
}

func cloneToolDefinition(definition ToolDefinition) ToolDefinition {
	definition.InputSchema = append(json.RawMessage(nil), definition.InputSchema...)
	return definition
}

// Catalog returns a defensive, name-sorted copy of all definitions.
func (r *ToolRegistry) Catalog() []ToolDefinition {
	if r == nil {
		return nil
	}
	result := make([]ToolDefinition, len(r.catalog))
	for index, definition := range r.catalog {
		result[index] = cloneToolDefinition(definition)
	}
	return result
}

// CatalogHash returns the stable SHA-256 hash of the sorted catalog.
func (r *ToolRegistry) CatalogHash() string {
	if r == nil {
		return ""
	}
	return r.catalogHash
}

// ProviderTools returns a defensive copy of the provider catalog.
func (r *ToolRegistry) ProviderTools() []ProviderToolDefinition {
	if r == nil {
		return nil
	}
	result := make([]ProviderToolDefinition, len(r.providerTool))
	copy(result, r.providerTool)
	for index := range result {
		result[index].Function.Parameters = append(
			json.RawMessage(nil),
			result[index].Function.Parameters...,
		)
	}
	return result
}

// Resolve returns one exact name/version pair.
func (r *ToolRegistry) Resolve(name, version string) (Tool, bool) {
	if r == nil {
		return Tool{}, false
	}
	registered, exists := r.tools[name]
	if !exists || registered.tool.Definition.Version != version {
		return Tool{}, false
	}
	return registered.tool, true
}

// Validate parses, validates, and canonicalizes one tool argument object.
func (r *ToolRegistry) Validate(
	name string,
	version string,
	arguments json.RawMessage,
) (json.RawMessage, *ToolError) {
	registered, exists := r.tools[name]
	if !exists || registered.tool.Definition.Version != version {
		return nil, notFoundToolError(name)
	}
	if len(bytes.TrimSpace(arguments)) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(arguments))
	if err != nil {
		return nil, validationToolError("工具参数不是合法 JSON")
	}
	if err := registered.schema.Validate(document); err != nil {
		return nil, validationToolError("工具参数不符合 Schema")
	}
	canonical, err := json.Marshal(document)
	if err != nil {
		return nil, internalToolError()
	}
	return canonical, nil
}

func notFoundToolError(name string) *ToolError {
	return &ToolError{
		Code:      "TOOL_NOT_FOUND",
		Kind:      "not_found",
		Message:   "工具不存在",
		Retryable: false,
		Hint:      name,
	}
}

func validationToolError(message string) *ToolError {
	return &ToolError{
		Code:      "TOOL_ARGUMENTS_INVALID",
		Kind:      "validation",
		Message:   message,
		Retryable: false,
	}
}

func internalToolError() *ToolError {
	return &ToolError{
		Code:      "TOOL_INTERNAL_ERROR",
		Kind:      "internal",
		Message:   "工具执行失败",
		Retryable: false,
	}
}

func timeoutToolError() *ToolError {
	return &ToolError{
		Code:      "TOOL_TIMEOUT",
		Kind:      "timeout",
		Message:   "工具执行超时",
		Retryable: true,
	}
}

func defaultToolTimeout() time.Duration {
	return 10 * time.Second
}
