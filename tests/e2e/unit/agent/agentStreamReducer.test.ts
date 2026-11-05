import { describe, expect, it } from "vitest";
import type { AgentStreamPart } from "@/models/agentStreamSchema";
import {
  completeAgentStream,
  createAgentStreamState,
  reduceAgentStream,
} from "@/models/agentStreamReducer";

const startPart = {
  type: "start",
  messageId: "message-1",
  messageMetadata: {
    sessionId: "session-1",
    runId: "run-1",
    userMessageId: "user-1",
  },
} satisfies AgentStreamPart;

function frame(part: AgentStreamPart, cursor: number) {
  return { cursor, part };
}

describe("agentStreamReducer", () => {
  it("moves from awaiting-start to streaming on start", () => {
    const [transition, error] = reduceAgentStream(
      createAgentStreamState(),
      frame(startPart, 1),
    );

    expect(error).toBeNull();
    expect(transition).toEqual({
      state: { phase: "streaming", lastCursor: 1 },
      events: [],
    });
  });

  it("emits text deltas with their cursor while remaining in streaming", () => {
    const [transition, error] = reduceAgentStream(
      { phase: "streaming", lastCursor: 1 },
      frame({ type: "text-delta", id: "part-1", delta: "你好" }, 2),
    );

    expect(error).toBeNull();
    expect(transition).toEqual({
      state: { phase: "streaming", lastCursor: 2 },
      events: [
        {
          type: "text-delta",
          id: "part-1",
          delta: "你好",
          cursor: 2,
        },
      ],
    });
  });

  it("moves to terminal and emits finish", () => {
    const [transition, error] = reduceAgentStream(
      { phase: "streaming", lastCursor: 1 },
      frame(
        {
          type: "finish",
          finishReason: "stop",
          messageMetadata: { runId: "run-1", status: "completed" },
        },
        2,
      ),
    );

    expect(error).toBeNull();
    expect(transition).toEqual({
      state: { phase: "terminal", lastCursor: 2 },
      events: [
        {
          type: "finish",
          finishReason: "stop",
          runId: "run-1",
          status: "completed",
          cursor: 2,
        },
      ],
    });
  });

  it("rejects a second terminal event", () => {
    const [, error] = reduceAgentStream(
      { phase: "terminal", lastCursor: 2 },
      frame(
        {
          type: "finish",
          finishReason: "stop",
          messageMetadata: { runId: "run-1", status: "completed" },
        },
        3,
      ),
    );

    expect(error).toMatchObject({
      code: "PROTOCOL_INVALID_METADATA",
      kind: "protocol",
      retryable: false,
    });
  });

  it("rejects a cursor that does not advance", () => {
    const [, error] = reduceAgentStream(
      { phase: "streaming", lastCursor: 2 },
      frame({ type: "text-delta", id: "part-1", delta: "重复" }, 2),
    );

    expect(error).toMatchObject({
      code: "PROTOCOL_INVALID_METADATA",
      kind: "protocol",
      retryable: false,
    });
  });

  it("reports unexpected EOF unless terminal was reached", () => {
    expect(
      completeAgentStream({ phase: "streaming", lastCursor: 2 }),
    ).toEqual([
      null,
      expect.objectContaining({ code: "PROTOCOL_UNEXPECTED_EOF" }),
    ]);
    expect(
      completeAgentStream({ phase: "terminal", lastCursor: 3 }),
    ).toEqual([null, null]);
  });

  it("maps a structured error chunk to a terminal error event", () => {
    const [transition, error] = reduceAgentStream(
      { phase: "streaming", lastCursor: 1 },
      frame(
        {
          type: "error",
          errorText: "模型生成失败",
          agentError: {
            code: "AGENT_PROVIDER_FAILED",
            kind: "server",
            message: "模型生成失败",
            retryable: true,
          },
        },
        2,
      ),
    );

    expect(error).toBeNull();
    expect(transition).toEqual({
      state: { phase: "terminal", lastCursor: 2 },
      events: [
        {
          type: "error",
          error: {
            code: "AGENT_PROVIDER_FAILED",
            kind: "server",
            message: "模型生成失败",
            retryable: true,
          },
          cursor: 2,
        },
      ],
    });
  });
});
