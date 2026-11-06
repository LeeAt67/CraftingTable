package main

import "testing"

func TestAgentToolConfigurationDefaultsAndOverrides(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		t.Setenv("CRAFTINGTABLE_AGENT_TOOLS_ENABLED", "")
		t.Setenv("CRAFTINGTABLE_AGENT_MAX_MODEL_STEPS", "")
		t.Setenv("CRAFTINGTABLE_AGENT_MAX_TOOL_CALLS", "")

		config, err := resolveAgentRuntimeConfig()
		if err != nil {
			t.Fatalf("resolveAgentRuntimeConfig() error = %v", err)
		}
		if !config.ToolsEnabled ||
			config.MaxModelSteps != 4 ||
			config.MaxToolCalls != 8 {
			t.Fatalf("config = %#v", config)
		}
	})

	t.Run("overrides", func(t *testing.T) {
		t.Setenv("CRAFTINGTABLE_AGENT_TOOLS_ENABLED", "false")
		t.Setenv("CRAFTINGTABLE_AGENT_MAX_MODEL_STEPS", "6")
		t.Setenv("CRAFTINGTABLE_AGENT_MAX_TOOL_CALLS", "12")

		config, err := resolveAgentRuntimeConfig()
		if err != nil {
			t.Fatalf("resolveAgentRuntimeConfig() error = %v", err)
		}
		if config.ToolsEnabled ||
			config.MaxModelSteps != 6 ||
			config.MaxToolCalls != 12 {
			t.Fatalf("config = %#v", config)
		}
	})
}

func TestAgentToolConfigurationRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name  string
		key   string
		value string
	}{
		{name: "enabled not boolean", key: "CRAFTINGTABLE_AGENT_TOOLS_ENABLED", value: "yes"},
		{name: "model steps zero", key: "CRAFTINGTABLE_AGENT_MAX_MODEL_STEPS", value: "0"},
		{name: "model steps negative", key: "CRAFTINGTABLE_AGENT_MAX_MODEL_STEPS", value: "-1"},
		{name: "model steps not integer", key: "CRAFTINGTABLE_AGENT_MAX_MODEL_STEPS", value: "many"},
		{name: "model steps overflow", key: "CRAFTINGTABLE_AGENT_MAX_MODEL_STEPS", value: "999999999999999999999"},
		{name: "tool calls zero", key: "CRAFTINGTABLE_AGENT_MAX_TOOL_CALLS", value: "0"},
		{name: "tool calls negative", key: "CRAFTINGTABLE_AGENT_MAX_TOOL_CALLS", value: "-1"},
		{name: "tool calls not integer", key: "CRAFTINGTABLE_AGENT_MAX_TOOL_CALLS", value: "many"},
		{name: "tool calls overflow", key: "CRAFTINGTABLE_AGENT_MAX_TOOL_CALLS", value: "999999999999999999999"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv(testCase.key, testCase.value)
			if _, err := resolveAgentRuntimeConfig(); err == nil {
				t.Fatal("resolveAgentRuntimeConfig() error = nil")
			}
		})
	}
}
