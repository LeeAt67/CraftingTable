package integration

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/leeat/craftingtable/server/testkit"
)

func TestAgentToolRuntimeIsWiredThroughHttpServer(t *testing.T) {
	provider := &toolHTTPProvider{}
	provider.handler = func(
		index int,
		_ testkit.ModelRequest,
		w http.ResponseWriter,
	) {
		if index == 0 {
			_, _ = io.WriteString(w, toolCallScript(struct {
				ID        string
				Name      string
				Arguments string
			}{
				ID:        "runtime-summary",
				Name:      "today_summary",
				Arguments: `{"date":"2026-11-06","timezone":"Asia/Shanghai"}`,
			}))
			return
		}
		_, _ = io.WriteString(w, textScript("工具运行时已接通。"))
	}
	providerServer := httptest.NewServer(provider)
	defer providerServer.Close()
	server := startServer(t, providerServer.URL+"/v1")
	_, runID := createSessionAndRun(t, server, "总结今天")
	waitForRunTerminal(t, server, runID)

	if provider.requestCount() != 2 {
		t.Fatalf("provider request count = %d, want 2", provider.requestCount())
	}
	first := provider.request(0)
	if len(first.Tools) != 3 || first.ToolChoice != "auto" {
		t.Fatalf("first provider request = %#v", first)
	}
	second := provider.request(1)
	if !requestContainsToolResult(second, "runtime-summary", "completionRate") {
		t.Fatalf("second provider request missing tool result: %#v", second)
	}
}

func TestAgentToolsCanBeDisabledAtStartup(t *testing.T) {
	provider := &toolHTTPProvider{}
	provider.handler = func(
		_ int,
		_ testkit.ModelRequest,
		w http.ResponseWriter,
	) {
		_, _ = io.WriteString(w, textScript("未启用工具。"))
	}
	providerServer := httptest.NewServer(provider)
	defer providerServer.Close()
	server := startServerWithEnvironment(
		t,
		providerServer.URL+"/v1",
		filepath.Join(t.TempDir(), "disabled-tools.sqlite"),
		map[string]string{
			"CRAFTINGTABLE_AGENT_TOOLS_ENABLED": "false",
		},
	)
	_, runID := createSessionAndRun(t, server, "普通回答")
	waitForRunTerminal(t, server, runID)

	request := provider.request(0)
	if len(request.Tools) != 0 || request.ToolChoice != "" {
		t.Fatalf("provider request = %#v", request)
	}
}
