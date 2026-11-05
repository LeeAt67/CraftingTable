import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AgentReasoningPanel } from "@/components/agent/agentReasoningPanel";
import type { AgentReasoningPart } from "@/models/agentContracts";

function reasoningPart(
  text: string,
  status: AgentReasoningPart["status"] = "streaming",
): AgentReasoningPart {
  return {
    id: "reasoning-1",
    messageId: "assistant-1",
    runId: "run-1",
    index: 0,
    type: "reasoning",
    status,
    content: { type: "reasoning", text },
    createdAt: "2026-11-05T00:00:00.000Z",
    updatedAt: "2026-11-05T00:00:00.000Z",
  };
}

describe("AgentReasoningPanel", () => {
  it("renders nothing before reasoning content arrives", () => {
    render(<AgentReasoningPanel part={reasoningPart("")} textStarted={false} />);

    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("expands while reasoning streams", () => {
    const { rerender } = render(
      <AgentReasoningPanel part={reasoningPart("")} textStarted={false} />,
    );
    rerender(
      <AgentReasoningPanel part={reasoningPart("先分析")} textStarted={false} />,
    );

    expect(screen.getByRole("button", { name: "Reasoning" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
    expect(screen.getByText("先分析")).toBeVisible();
  });

  it("collapses automatically when the answer starts", () => {
    const { rerender } = render(
      <AgentReasoningPanel part={reasoningPart("")} textStarted={false} />,
    );
    rerender(
      <AgentReasoningPanel part={reasoningPart("先分析")} textStarted={false} />,
    );
    rerender(
      <AgentReasoningPanel part={reasoningPart("先分析")} textStarted />,
    );

    expect(screen.getByRole("button", { name: "Reasoning" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    expect(screen.queryByText("先分析")).not.toBeInTheDocument();
  });

  it("keeps a manual expansion after the answer starts", () => {
    const { rerender } = render(
      <AgentReasoningPanel part={reasoningPart("")} textStarted={false} />,
    );
    rerender(
      <AgentReasoningPanel part={reasoningPart("先分析")} textStarted={false} />,
    );
    const toggle = screen.getByRole("button", { name: "Reasoning" });
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");

    rerender(
      <AgentReasoningPanel part={reasoningPart("先分析")} textStarted />,
    );

    expect(screen.getByRole("button", { name: "Reasoning" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
  });

  it("starts collapsed when restored content is already present", () => {
    render(
      <AgentReasoningPanel
        part={reasoningPart("旧推理", "complete")}
        textStarted
      />,
    );

    expect(screen.getByRole("button", { name: "Reasoning" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  });

  it("only follows reasoning updates while the content is near the bottom", () => {
    const { container, rerender } = render(
      <AgentReasoningPanel
        part={reasoningPart("")}
        textStarted={false}
      />,
    );
    rerender(
      <AgentReasoningPanel
        part={reasoningPart("第一段")}
        textStarted={false}
      />,
    );
    const content = container.querySelector(
      "#reasoning-panel-reasoning-1",
    ) as HTMLDivElement;
    Object.defineProperties(content, {
      scrollHeight: { configurable: true, value: 1_000 },
      clientHeight: { configurable: true, value: 500 },
      scrollTop: { configurable: true, writable: true, value: 0 },
    });

    fireEvent.scroll(content);
    rerender(
      <AgentReasoningPanel
        part={reasoningPart("第一段更新")}
        textStarted={false}
      />,
    );
    expect(content.scrollTop).toBe(0);

    content.scrollTop = 500;
    fireEvent.scroll(content);
    rerender(
      <AgentReasoningPanel
        part={reasoningPart("第一段再次更新")}
        textStarted={false}
      />,
    );
    expect(content.scrollTop).toBe(1_000);
  });
});
