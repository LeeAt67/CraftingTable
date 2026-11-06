package core

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func testToolDefinition(name string) ToolDefinition {
	return ToolDefinition{
		Name:        name,
		Namespace:   "test",
		Version:     "1.0.0",
		Title:       "Test Tool",
		Description: "A test tool.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"name":{"type":"string"}},
			"required":["name"],
			"additionalProperties":false
		}`),
		ReadOnly:       true,
		Idempotent:     true,
		Timeout:        time.Second,
		MaxInputBytes:  4096,
		MaxResultBytes: 4096,
	}
}

func testTool(name string) Tool {
	return Tool{
		Definition: testToolDefinition(name),
		Handler: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{
				Content: []ToolResultContent{{Type: "text", Text: "ok"}},
			}, nil
		},
	}
}

func TestNewToolRegistryRejectsDuplicateToolNames(t *testing.T) {
	t.Parallel()

	_, err := NewToolRegistry([]Tool{
		testTool("duplicate"),
		testTool("duplicate"),
	})
	if err == nil {
		t.Fatal("NewToolRegistry() accepted duplicate tool names")
	}
}

func TestNewToolRegistryRejectsExternalSchemaReference(t *testing.T) {
	t.Parallel()

	tool := testTool("external-ref")
	tool.Definition.InputSchema = json.RawMessage(`{
		"type":"object",
		"properties":{"secret":{"$ref":"https://example.com/schema.json"}},
		"additionalProperties":false
	}`)
	_, err := NewToolRegistry([]Tool{tool})
	if err == nil {
		t.Fatal("NewToolRegistry() accepted an external schema reference")
	}
}

func TestNewToolRegistryRejectsNonObjectInputSchema(t *testing.T) {
	t.Parallel()

	tool := testTool("array-input")
	tool.Definition.InputSchema = json.RawMessage(`{
		"type":"array",
		"items":{"type":"string"}
	}`)
	_, err := NewToolRegistry([]Tool{tool})
	if err == nil {
		t.Fatal("NewToolRegistry() accepted a non-object input schema")
	}
}

func TestToolRegistryCatalogIsSortedAndCannotBeMutatedByCaller(t *testing.T) {
	t.Parallel()

	registry, err := NewToolRegistry([]Tool{
		testTool("zeta"),
		testTool("alpha"),
	})
	if err != nil {
		t.Fatalf("NewToolRegistry() error = %v", err)
	}

	first := registry.Catalog()
	if len(first) != 2 || first[0].Name != "alpha" || first[1].Name != "zeta" {
		t.Fatalf("Catalog() order = %#v, want alpha then zeta", first)
	}
	first[0].Name = "mutated"
	first[0].InputSchema[0] = '['

	second := registry.Catalog()
	if second[0].Name != "alpha" {
		t.Fatalf("Catalog() leaked a caller mutation: %#v", second[0])
	}
	var schema map[string]any
	if err := json.Unmarshal(second[0].InputSchema, &schema); err != nil {
		t.Fatalf("Catalog() leaked a malformed schema: %v", err)
	}
}

func TestToolRegistryProviderToolsUseFunctionWireShape(t *testing.T) {
	t.Parallel()

	registry, err := NewToolRegistry([]Tool{testTool("today_search")})
	if err != nil {
		t.Fatalf("NewToolRegistry() error = %v", err)
	}
	providerTools := registry.ProviderTools()
	if len(providerTools) != 1 {
		t.Fatalf("ProviderTools() length = %d, want 1", len(providerTools))
	}
	tool := providerTools[0]
	if tool.Type != "function" ||
		tool.Function.Name != "today_search" ||
		tool.Function.Description != "A test tool." {
		t.Fatalf("ProviderTools()[0] = %#v", tool)
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Function.Parameters, &schema); err != nil {
		t.Fatalf("provider schema is not JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Fatalf("provider schema = %#v", schema)
	}
}

func TestToolRegistryResolveRequiresExactVersion(t *testing.T) {
	t.Parallel()

	registry, err := NewToolRegistry([]Tool{testTool("versioned")})
	if err != nil {
		t.Fatalf("NewToolRegistry() error = %v", err)
	}
	if _, found := registry.Resolve("versioned", "1.0.0"); !found {
		t.Fatal("Resolve() did not find the exact version")
	}
	if _, found := registry.Resolve("versioned", "2.0.0"); found {
		t.Fatal("Resolve() matched a different version")
	}
}
