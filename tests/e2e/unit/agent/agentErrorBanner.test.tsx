import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AgentErrorBanner } from "@/components/agent/agentErrorBanner";

describe("AgentErrorBanner", () => {
  it("only offers retry for an error marked retryable", () => {
    const { rerender } = render(
      <AgentErrorBanner
        error={{
          code: "AGENT_PROVIDER_FAILED",
          kind: "server",
          message: "模型暂时不可用",
          retryable: true,
        }}
        onRetry={() => undefined}
      />,
    );
    expect(screen.getByRole("button", { name: "重试" })).toBeInTheDocument();

    rerender(
      <AgentErrorBanner
        error={{
          code: "AGENT_NOT_CONFIGURED",
          kind: "server",
          message: "Agent 模型服务尚未配置",
          retryable: false,
        }}
        onRetry={vi.fn()}
      />,
    );
    expect(
      screen.queryByRole("button", { name: "重试" }),
    ).not.toBeInTheDocument();
  });
});
