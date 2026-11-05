import { describe, expect, it } from "vitest";
import { buildAgentContext } from "@/models/agentContext";
import type {
  AgentMessage,
  AgentMessagePart,
} from "@/models/agentContracts";

function completeTurn(turn: number, text: string) {
  const createdAt = `2026-11-05T00:00:${String(turn).padStart(2, "0")}.000Z`;
  const userMessage: AgentMessage = {
    id: `user-${turn}`,
    sessionId: "session-1",
    seq: turn * 2 - 1,
    role: "user",
    status: "complete",
    createdAt,
    updatedAt: createdAt,
  };
  const assistantMessage: AgentMessage = {
    id: `assistant-${turn}`,
    sessionId: "session-1",
    seq: turn * 2,
    role: "assistant",
    status: "complete",
    createdAt,
    updatedAt: createdAt,
  };
  const part = (message: AgentMessage): AgentMessagePart => ({
    id: `part-${message.id}`,
    messageId: message.id,
    index: 0,
    type: "text",
    status: "complete",
    content: { type: "text", text: `${text}-${message.role}` },
    createdAt,
    updatedAt: createdAt,
  });
  return {
    messages: [userMessage, assistantMessage],
    parts: [part(userMessage), part(assistantMessage)],
  };
}

describe("buildAgentContext", () => {
  it("keeps only the latest 20 complete turns", () => {
    const turns = Array.from({ length: 22 }, (_, index) =>
      completeTurn(index + 1, `内容-${index + 1}`),
    );
    const messages = turns.flatMap((turn) => turn.messages);
    const parts = turns.flatMap((turn) => turn.parts);

    const context = buildAgentContext(
      messages,
      Object.fromEntries(
        parts.map((part) => [part.messageId, [part]]),
      ),
    );

    expect(context).toHaveLength(40);
    expect(context[0]?.content).toBe("内容-3-user");
  });

  it("drops the oldest complete turns to stay within 20,000 characters", () => {
    const longText = "x".repeat(8000);
    const turns = Array.from({ length: 3 }, (_, index) =>
      completeTurn(index + 1, longText),
    );
    const messages = turns.flatMap((turn) => turn.messages);
    const parts = turns.flatMap((turn) => turn.parts);

    const context = buildAgentContext(
      messages,
      Object.fromEntries(
        parts.map((part) => [part.messageId, [part]]),
      ),
    );

    expect(context).toHaveLength(2);
    expect(context[0]?.content).toBe(`${longText}-user`);
  });

  it("keeps a failed error part out of the model context", () => {
    const createdAt = "2026-11-05T00:00:00.000Z";
    const messages: AgentMessage[] = [
      {
        id: "user-1",
        sessionId: "session-1",
        seq: 1,
        role: "user",
        status: "complete",
        createdAt,
        updatedAt: createdAt,
      },
      {
        id: "assistant-1",
        sessionId: "session-1",
        seq: 2,
        role: "assistant",
        status: "failed",
        activeRunId: "run-1",
        userMessageId: "user-1",
        createdAt,
        updatedAt: createdAt,
      },
    ];
    const parts: AgentMessagePart[] = [
      {
        id: "user-part-1",
        messageId: "user-1",
        index: 0,
        type: "text",
        status: "complete",
        content: { type: "text", text: "触发失败" },
        createdAt,
        updatedAt: createdAt,
      },
      {
        id: "error-part-1",
        messageId: "assistant-1",
        runId: "run-1",
        index: 0,
        type: "error",
        status: "failed",
        content: {
          type: "error",
          error: {
            code: "AGENT_PROVIDER_FAILED",
            kind: "server",
            message: "模型服务请求失败",
            retryable: true,
          },
        },
        createdAt,
        updatedAt: createdAt,
      },
    ];

    const context = buildAgentContext(
      messages,
      Object.fromEntries(
        parts.map((part) => [part.messageId, [part]]),
      ),
    );

    expect(context).toEqual([]);
  });

  it("uses only the active run parts after a retry succeeds", () => {
    const createdAt = "2026-11-05T00:00:00.000Z";
    const messages: AgentMessage[] = [
      {
        id: "user-1",
        sessionId: "session-1",
        seq: 1,
        role: "user",
        status: "complete",
        createdAt,
        updatedAt: createdAt,
      },
      {
        id: "assistant-1",
        sessionId: "session-1",
        seq: 2,
        role: "assistant",
        status: "complete",
        activeRunId: "run-2",
        userMessageId: "user-1",
        createdAt,
        updatedAt: createdAt,
      },
    ];
    const parts: AgentMessagePart[] = [
      {
        id: "user-part-1",
        messageId: "user-1",
        index: 0,
        type: "text",
        status: "complete",
        content: { type: "text", text: "重试这个问题" },
        createdAt,
        updatedAt: createdAt,
      },
      {
        id: "old-text-1",
        messageId: "assistant-1",
        runId: "run-1",
        index: 0,
        type: "text",
        status: "failed",
        content: { type: "text", text: "失败前的部分" },
        createdAt,
        updatedAt: createdAt,
      },
      {
        id: "current-text-1",
        messageId: "assistant-1",
        runId: "run-2",
        index: 1,
        type: "text",
        status: "complete",
        content: { type: "text", text: "重试成功" },
        createdAt,
        updatedAt: createdAt,
      },
    ];

    const context = buildAgentContext(
      messages,
      {
        "user-1": [parts[0]],
        "assistant-1": [parts[1], parts[2]],
      },
    );

    expect(context).toEqual([
      { role: "user", content: "重试这个问题" },
      { role: "assistant", content: "重试成功" },
    ]);
  });
});
