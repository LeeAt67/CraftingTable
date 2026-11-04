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
});
