import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AgentMessageList } from "@/components/agent/agentMessageList";
import {
  resetAgentStore,
  useAgentStore,
} from "@/controllers/stores/agentStore";

describe("AgentMessageList", () => {
  beforeEach(() => {
    resetAgentStore();
    Element.prototype.scrollIntoView = vi.fn();
  });

  it("announces coarse stream state instead of every message update", () => {
    useAgentStore.getState().setSessionMessages(
      "session-1",
      [
        {
          id: "assistant-1",
          sessionId: "session-1",
          seq: 1,
          role: "assistant",
          status: "streaming",
          createdAt: "2026-11-05T00:00:00.000Z",
          updatedAt: "2026-11-05T00:00:00.000Z",
        },
      ],
      [
        {
          id: "part-1",
          messageId: "assistant-1",
          runId: "run-1",
          index: 0,
          type: "text",
          status: "streaming",
          content: { type: "text", text: "正在生成" },
          createdAt: "2026-11-05T00:00:00.000Z",
          updatedAt: "2026-11-05T00:00:00.000Z",
        },
      ],
    );

    render(<AgentMessageList sessionId="session-1" />);

    expect(screen.getByRole("log", { name: "会话消息" })).not.toHaveAttribute(
      "aria-live",
    );
    expect(screen.getByRole("status")).toHaveTextContent("Agent 正在回复");
  });

  it("offers retry for the last stopped assistant message", () => {
    const onRetry = vi.fn();
    useAgentStore.getState().setSessionMessages(
      "session-1",
      [
        {
          id: "user-1",
          sessionId: "session-1",
          seq: 1,
          role: "user",
          status: "complete",
          createdAt: "2026-11-05T00:00:00.000Z",
          updatedAt: "2026-11-05T00:00:00.000Z",
        },
        {
          id: "assistant-1",
          sessionId: "session-1",
          seq: 2,
          role: "assistant",
          status: "cancelled",
          activeRunId: "run-1",
          userMessageId: "user-1",
          createdAt: "2026-11-05T00:00:00.000Z",
          updatedAt: "2026-11-05T00:00:01.000Z",
        },
      ],
      [
        {
          id: "part-1",
          messageId: "user-1",
          index: 0,
          type: "text",
          status: "complete",
          content: { type: "text", text: "测试停止" },
          createdAt: "2026-11-05T00:00:00.000Z",
          updatedAt: "2026-11-05T00:00:00.000Z",
        },
        {
          id: "part-2",
          messageId: "assistant-1",
          runId: "run-1",
          index: 0,
          type: "text",
          status: "cancelled",
          content: { type: "text", text: "部分回复" },
          createdAt: "2026-11-05T00:00:00.000Z",
          updatedAt: "2026-11-05T00:00:01.000Z",
        },
      ],
    );

    render(<AgentMessageList sessionId="session-1" onRetry={onRetry} />);
    fireEvent.click(screen.getByRole("button", { name: "重试回复" }));

    expect(onRetry).toHaveBeenCalledTimes(1);
  });
});
