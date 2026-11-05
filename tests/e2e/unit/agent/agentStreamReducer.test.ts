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

describe("agentStreamReducer", () => {
  it("moves from awaiting-start to streaming on start", () => {
    const [transition, error] = reduceAgentStream(
      createAgentStreamState(),
      startPart,
    );

    expect(error).toBeNull();
    expect(transition).toEqual({
      state: { phase: "streaming" },
      events: [],
    });
  });

  it("emits text deltas while remaining in streaming", () => {
    const [transition, error] = reduceAgentStream(
      { phase: "streaming" },
      { type: "text-delta", id: "part-1", delta: "你好" },
    );

    expect(error).toBeNull();
    expect(transition).toEqual({
      state: { phase: "streaming" },
      events: [{ type: "text-delta", id: "part-1", delta: "你好" }],
    });
  });

  it("moves to terminal and emits finish", () => {
    const [transition, error] = reduceAgentStream(
      { phase: "streaming" },
      {
        type: "finish",
        finishReason: "stop",
        messageMetadata: { runId: "run-1", status: "completed" },
      },
    );

    expect(error).toBeNull();
    expect(transition).toEqual({
      state: { phase: "terminal" },
      events: [
        {
          type: "finish",
          finishReason: "stop",
          runId: "run-1",
          status: "completed",
        },
      ],
    });
  });

  it("rejects a second terminal event", () => {
    const [, error] = reduceAgentStream(
      { phase: "terminal" },
      {
        type: "finish",
        finishReason: "stop",
        messageMetadata: { runId: "run-1", status: "completed" },
      },
    );

    expect(error).toMatchObject({
      code: "PROTOCOL_INVALID_METADATA",
      kind: "protocol",
      retryable: false,
    });
  });

  it("reports unexpected EOF unless terminal was reached", () => {
    expect(completeAgentStream({ phase: "streaming" })).toEqual([
      null,
      expect.objectContaining({ code: "PROTOCOL_UNEXPECTED_EOF" }),
    ]);
    expect(completeAgentStream({ phase: "terminal" })).toEqual([null, null]);
  });

  it("maps a structured error chunk to a terminal error event", () => {
    const [transition, error] = reduceAgentStream(
      { phase: "streaming" },
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
    );

    expect(error).toBeNull();
    expect(transition).toEqual({
      state: { phase: "terminal" },
      events: [
        {
          type: "error",
          error: {
            code: "AGENT_PROVIDER_FAILED",
            kind: "server",
            message: "模型生成失败",
            retryable: true,
          },
        },
      ],
    });
  });
});
