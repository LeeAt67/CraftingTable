import { describe, expect, it } from "vitest";
import { createAgentRunMaterializer } from "@/models/agentRunMaterializer";

const handle = {
  runId: "run-1",
  sessionId: "session-1",
  userMessageId: "user-1",
  assistantMessageId: "assistant-1",
  assistantPartId: "text-1",
  reasoningPartId: "reasoning-1",
  status: "running" as const,
};

function materializer() {
  return createAgentRunMaterializer({
    sessionId: "session-1",
    runId: "run-1",
    userMessageId: "user-1",
    assistantMessageId: "assistant-1",
    assistantPartId: "text-1",
    reasoningPartId: "reasoning-1",
  });
}

describe("AgentRunMaterializer reasoning", () => {
  it("keeps reasoning and text in separate ordered parts", () => {
    const run = materializer();
    const started = run.start(handle, "2026-11-05T00:00:00.000Z");
    expect(started.parts.map((part) => part.type)).toEqual(["text"]);
    expect(started.parts[0]).toMatchObject({
      id: "text-1",
      index: 1,
    });

    const reasoningStarted = run.reasoningStart();
    expect(reasoningStarted.parts.map((part) => part.type)).toEqual([
      "reasoning",
      "text",
    ]);
    run.reasoningDelta("先分析", 3);
    const snapshot = run.delta("最终回答", 4);

    expect(snapshot.parts).toHaveLength(2);
    expect(snapshot.parts[0]).toMatchObject({
      id: "reasoning-1",
      index: 0,
      type: "reasoning",
      content: { type: "reasoning", text: "先分析" },
    });
    expect(snapshot.parts[1]).toMatchObject({
      id: "text-1",
      index: 1,
      type: "text",
      content: { type: "text", text: "最终回答" },
    });
  });

  it("marks both parts terminal when the run finishes", () => {
    const run = materializer();
    run.start(handle, "2026-11-05T00:00:00.000Z");
    run.reasoningStart();
    run.reasoningDelta("推理", 3);
    const snapshot = run.finish("completed", "2026-11-05T00:00:02.000Z");

    expect(snapshot.message.status).toBe("complete");
    for (const part of snapshot.parts) {
      expect(part.status).toBe("complete");
    }
  });

  it("restores both parts from an existing snapshot", () => {
    const run = createAgentRunMaterializer({
      sessionId: "session-1",
      runId: "run-1",
      userMessageId: "user-1",
      assistantMessageId: "assistant-1",
      assistantPartId: "text-1",
      reasoningPartId: "reasoning-1",
      initialSnapshot: {
        message: {
          id: "assistant-1",
          sessionId: "session-1",
          seq: 2,
          role: "assistant",
          status: "streaming",
          activeRunId: "run-1",
          userMessageId: "user-1",
          createdAt: "2026-11-05T00:00:00.000Z",
          updatedAt: "2026-11-05T00:00:01.000Z",
        },
        parts: [
          {
            id: "reasoning-1",
            messageId: "assistant-1",
            runId: "run-1",
            index: 0,
            type: "reasoning",
            status: "streaming",
            content: { type: "reasoning", text: "已恢复推理" },
            createdAt: "2026-11-05T00:00:00.000Z",
            updatedAt: "2026-11-05T00:00:01.000Z",
          },
          {
            id: "text-1",
            messageId: "assistant-1",
            runId: "run-1",
            index: 1,
            type: "text",
            status: "streaming",
            content: { type: "text", text: "" },
            createdAt: "2026-11-05T00:00:00.000Z",
            updatedAt: "2026-11-05T00:00:01.000Z",
          },
        ],
      },
    });

    const snapshot = run.reasoningDelta("，继续", 5);

    expect(snapshot.parts[0]).toMatchObject({
      id: "reasoning-1",
      content: { type: "reasoning", text: "已恢复推理，继续" },
    });
  });
});
