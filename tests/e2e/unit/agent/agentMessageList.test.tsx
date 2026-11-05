import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AgentMessageList } from "@/components/agent/agentMessageList";
import {
  resetAgentStore,
  useAgentStore,
} from "@/controllers/stores/agentStore";
import type { AgentMessagePart } from "@/models/agentContracts";

function failedAssistantWithError(retryable: boolean) {
  const errorPart = {
    id: "error-1",
    messageId: "assistant-1",
    runId: "run-1",
    index: 1,
    type: "error",
    status: "failed",
    content: {
      type: "error",
      error: {
        code: "AGENT_PROVIDER_FAILED",
        kind: "server",
        message: "模型服务请求失败",
        retryable,
      },
    },
    createdAt: "2026-11-05T00:00:01.000Z",
    updatedAt: "2026-11-05T00:00:01.000Z",
  } satisfies AgentMessagePart;
  return {
    messages: [
      {
        id: "assistant-1",
        sessionId: "session-1",
        seq: 2,
        role: "assistant" as const,
        status: "failed" as const,
        activeRunId: "run-1",
        createdAt: "2026-11-05T00:00:00.000Z",
        updatedAt: "2026-11-05T00:00:01.000Z",
      },
    ],
    parts: [errorPart],
  };
}

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
    fireEvent.click(screen.getByRole("button", { name: "重试" }));

    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("renders a retryable agent error inside the failed assistant message", () => {
    const onRetry = vi.fn();
    const failed = failedAssistantWithError(true);
    useAgentStore.getState().setSessionMessages(
      "session-1",
      failed.messages,
      failed.parts,
    );

    render(<AgentMessageList sessionId="session-1" onRetry={onRetry} />);

    expect(screen.getByText("模型服务请求失败")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("hides retry when the agent error is not retryable", () => {
    const failed = failedAssistantWithError(false);
    useAgentStore.getState().setSessionMessages(
      "session-1",
      failed.messages,
      failed.parts,
    );

    render(<AgentMessageList sessionId="session-1" onRetry={vi.fn()} />);

    expect(screen.getByText("模型服务请求失败")).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "重试" }),
    ).not.toBeInTheDocument();
  });
});
