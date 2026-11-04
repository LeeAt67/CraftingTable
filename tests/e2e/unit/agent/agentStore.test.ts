import { beforeEach, describe, expect, it } from "vitest";
import {
  resetAgentStore,
  useAgentStore,
} from "@/controllers/stores/agentStore";

describe("AgentStore", () => {
  beforeEach(resetAgentStore);

  it("stores messages per session and merges parts by message and part id", () => {
    useAgentStore.getState().appendMessage("session-1", {
      id: "message-1",
      sessionId: "session-1",
      seq: 1,
      role: "assistant",
      status: "streaming",
      createdAt: "2026-11-05T00:00:00.000Z",
      updatedAt: "2026-11-05T00:00:00.000Z",
    });
    useAgentStore.getState().upsertMessagePart("message-1", {
      id: "part-1",
      messageId: "message-1",
      runId: "run-1",
      index: 0,
      type: "text",
      status: "streaming",
      content: { type: "text", text: "你" },
      createdAt: "2026-11-05T00:00:00.000Z",
      updatedAt: "2026-11-05T00:00:00.000Z",
    });
    useAgentStore.getState().upsertMessagePart("message-1", {
      id: "part-1",
      messageId: "message-1",
      runId: "run-1",
      index: 0,
      type: "text",
      status: "complete",
      content: { type: "text", text: "你好" },
      createdAt: "2026-11-05T00:00:00.000Z",
      updatedAt: "2026-11-05T00:00:01.000Z",
    });

    expect(
      useAgentStore.getState().partsByMessageId["message-1"],
    ).toHaveLength(1);
    expect(
      useAgentStore.getState().partsByMessageId["message-1"][0].content.text,
    ).toBe("你好");
  });
});
