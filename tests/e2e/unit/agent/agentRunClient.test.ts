import { describe, expect, it } from "vitest";
import {
  consumeAgentRun,
  type ConsumeAgentRunInput,
} from "@/models/agentRunClient";
import { createAgentRunMaterializer } from "@/models/agentRunMaterializer";

const encoder = new TextEncoder();

function streamResponse(frames: string[]) {
  return new Response(
    new ReadableStream({
      start(controller) {
        for (const frame of frames) {
          controller.enqueue(encoder.encode(frame));
        }
        controller.close();
      },
    }),
    {
      status: 200,
      headers: { "Content-Type": "text/event-stream" },
    },
  );
}

function jsonResponse(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function startFrame(cursor: number) {
  return `id: ${cursor}\ndata: ${JSON.stringify({
    type: "start",
    messageId: "assistant-1",
    messageMetadata: {
      sessionId: "session-1",
      assistantMessageId: "assistant-1",
      runId: "run-1",
      userMessageId: "user-1",
    },
  })}\n\n`;
}

function textFrame(cursor: number, delta: string) {
  return `id: ${cursor}\ndata: ${JSON.stringify({
    type: "text-delta",
    id: "part-1",
    delta,
  })}\n\n`;
}

function finishFrame(cursor: number) {
  return `id: ${cursor}\ndata: ${JSON.stringify({
    type: "finish",
    messageMetadata: { runId: "run-1", status: "completed" },
  })}\n\n`;
}

function materializer() {
  return createAgentRunMaterializer({
    sessionId: "session-1",
    runId: "run-1",
    userMessageId: "user-1",
    assistantMessageId: "assistant-1",
    assistantPartId: "part-1",
  });
}

function baseInput(
  fetchImpl: ConsumeAgentRunInput["fetchImpl"],
): ConsumeAgentRunInput {
  return {
    baseUrl: "/api/v1",
    fetchImpl,
    sessionId: "session-1",
    runId: "run-1",
    assistantMessageId: "assistant-1",
    afterSequence: 0,
    signal: new AbortController().signal,
    materializer: materializer(),
    recreateMaterializer: (snapshot) =>
      createAgentRunMaterializer({
        sessionId: "session-1",
        runId: "run-1",
        userMessageId: "user-1",
        assistantMessageId: "assistant-1",
        assistantPartId: "part-1",
        initialSnapshot: snapshot,
      }),
    handlers: {
      onStarted: () => undefined,
      onSnapshot: () => undefined,
    },
    retryDelayMs: 0,
  };
}

describe("AgentRunClient", () => {
  it("calls the injected fetch without an object receiver", async () => {
    const receivers: unknown[] = [];
    const input = baseInput(function (this: unknown) {
      receivers.push(this);
      return Promise.resolve(
        streamResponse([
          startFrame(1),
          textFrame(2, "好"),
          finishFrame(3),
          "data: [DONE]\n\n",
        ]),
      );
    });

    const [, error] = await consumeAgentRun(input);

    expect(error).toBeNull();
    expect(receivers).toEqual([undefined]);
  });

  it("reconnects after an interrupted stream using the last cursor", async () => {
    const requestedUrls: string[] = [];
    const snapshots: string[] = [];
    const input = baseInput(async (request) => {
      const url = String(request);
      requestedUrls.push(url);
      if (requestedUrls.length === 1) {
        return streamResponse([startFrame(1), textFrame(2, "你")]);
      }
      return streamResponse([
        startFrame(2),
        textFrame(3, "好"),
        finishFrame(4),
        "data: [DONE]\n\n",
      ]);
    });
    input.handlers.onSnapshot = (snapshot) => {
      const part = snapshot.parts.find((candidate) => candidate.type === "text");
      if (part?.type === "text") {
        snapshots.push(part.content.text);
      }
    };

    const [result, error] = await consumeAgentRun(input);

    expect(error).toBeNull();
    expect(result?.status).toBe("completed");
    expect(snapshots.at(-1)).toBe("你好");
    expect(requestedUrls).toEqual([
      "/api/v1/agent/run/stream?runId=run-1&after=0",
      "/api/v1/agent/run/stream?runId=run-1&after=2",
    ]);
  });

  it("falls back to persisted session detail when the cursor expired", async () => {
    const requestedUrls: string[] = [];
    const snapshots: string[] = [];
    const input = baseInput(async (request) => {
      const url = String(request);
      requestedUrls.push(url);
      if (url.includes("/agent/run/stream")) {
        return jsonResponse(
          {
            error: {
              code: "AGENT_STREAM_CURSOR_EXPIRED",
              kind: "conflict",
              message: "实时流游标已过期",
              retryable: true,
            },
          },
          409,
        );
      }
      return jsonResponse({
        data: {
          session: {
            id: "session-1",
            title: "恢复会话",
            createdAt: "2026-11-05T00:00:00Z",
            updatedAt: "2026-11-05T00:00:02Z",
          },
          messages: [
            {
              id: "assistant-1",
              sessionId: "session-1",
              seq: 2,
              role: "assistant",
              status: "complete",
              activeRunId: "run-1",
              userMessageId: "user-1",
              createdAt: "2026-11-05T00:00:01Z",
              updatedAt: "2026-11-05T00:00:02Z",
            },
          ],
          messageParts: [
            {
              id: "part-1",
              messageId: "assistant-1",
              runId: "run-1",
              index: 0,
              type: "text",
              status: "complete",
              content: { type: "text", text: "持久化结果" },
              createdAt: "2026-11-05T00:00:01Z",
              updatedAt: "2026-11-05T00:00:02Z",
            },
          ],
          activeRun: null,
          nextMessageCursor: null,
        },
      });
    });
    input.handlers.onSnapshot = (snapshot) => {
      const part = snapshot.parts.find((candidate) => candidate.type === "text");
      if (part?.type === "text") {
        snapshots.push(part.content.text);
      }
    };

    const [result, error] = await consumeAgentRun(input);

    expect(error).toBeNull();
    expect(result?.status).toBe("completed");
    expect(snapshots).toEqual(["持久化结果"]);
    expect(requestedUrls).toEqual([
      "/api/v1/agent/run/stream?runId=run-1&after=0",
      "/api/v1/agent/session/get?sessionId=session-1",
    ]);
  });

  it("does not reconnect after a non-retryable protocol failure", async () => {
    let attempts = 0;
    const input = baseInput(async () => {
      attempts += 1;
      return streamResponse([
        'data: {"type":"start","messageId":"assistant-1","messageMetadata":{"sessionId":"session-1","runId":"run-1","userMessageId":"user-1"}}\n\n',
      ]);
    });

    const [result, error] = await consumeAgentRun(input);

    expect(result).toBeNull();
    expect(error?.code).toBe("PROTOCOL_INVALID_METADATA");
    expect(attempts).toBe(1);
  });

  it("emits a failed snapshot for a terminal provider error", async () => {
    let attempts = 0;
    const input = baseInput(async () => {
      attempts += 1;
      return streamResponse([
        startFrame(1),
        `id: 2\ndata: ${JSON.stringify({
          type: "error",
          errorText: "模型生成失败",
          agentError: {
            code: "AGENT_PROVIDER_FAILED",
            kind: "server",
            message: "模型生成失败",
            retryable: true,
          },
        })}\n\n`,
        "data: [DONE]\n\n",
      ]);
    });
    let finalSnapshot:
      | {
          messageStatus: string;
          partStatuses: string[];
          errorCode?: string;
        }
      | undefined;
    input.handlers.onSnapshot = (snapshot) => {
      const errorPart = snapshot.parts.find((part) => part.type === "error");
      finalSnapshot = {
        messageStatus: snapshot.message.status,
        partStatuses: snapshot.parts.map((part) => part.status),
        errorCode: errorPart?.content.error.code,
      };
    };

    const [result, error] = await consumeAgentRun(input);

    expect(result).toBeNull();
    expect(error?.code).toBe("AGENT_PROVIDER_FAILED");
    expect(attempts).toBe(1);
    expect(finalSnapshot).toEqual({
      messageStatus: "failed",
      partStatuses: ["failed", "failed"],
      errorCode: "AGENT_PROVIDER_FAILED",
    });
  });

  it("reports a local abort without claiming the server run was cancelled", async () => {
    const controller = new AbortController();
    const input = baseInput(
      (_request, init) =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener(
            "abort",
            () => {
              reject(new DOMException("aborted", "AbortError"));
            },
            { once: true },
          );
        }),
    );
    input.signal = controller.signal;

    const pending = consumeAgentRun(input);
    controller.abort();
    const [result, error] = await pending;

    expect(result).toBeNull();
    expect(error?.code).toBe("REQUEST_CANCELLED");
  });

  it("reconciles persisted terminal state after the retry budget is exhausted", async () => {
    const requestedUrls: string[] = [];
    const input = baseInput(async (request) => {
      const url = String(request);
      requestedUrls.push(url);
      if (url.includes("/agent/run/stream")) {
        throw new TypeError("network unavailable");
      }
      return jsonResponse({
        data: {
          session: {
            id: "session-1",
            title: "恢复会话",
            createdAt: "2026-11-05T00:00:00Z",
            updatedAt: "2026-11-05T00:00:02Z",
          },
          messages: [
            {
              id: "assistant-1",
              sessionId: "session-1",
              seq: 2,
              role: "assistant",
              status: "complete",
              activeRunId: "run-1",
              userMessageId: "user-1",
              createdAt: "2026-11-05T00:00:01Z",
              updatedAt: "2026-11-05T00:00:02Z",
            },
          ],
          messageParts: [
            {
              id: "part-1",
              messageId: "assistant-1",
              runId: "run-1",
              index: 0,
              type: "text",
              status: "complete",
              content: { type: "text", text: "持久化完成" },
              createdAt: "2026-11-05T00:00:01Z",
              updatedAt: "2026-11-05T00:00:02Z",
            },
          ],
          activeRun: null,
          nextMessageCursor: null,
        },
      });
    });
    input.maxRetries = 0;
    const snapshots: string[] = [];
    input.handlers.onSnapshot = (snapshot) => {
      const part = snapshot.parts.find((candidate) => candidate.type === "text");
      if (part?.type === "text") {
        snapshots.push(part.content.text);
      }
    };

    const [result, error] = await consumeAgentRun(input);

    expect(error).toBeNull();
    expect(result?.status).toBe("completed");
    expect(snapshots).toEqual(["持久化完成"]);
    expect(requestedUrls).toEqual([
      "/api/v1/agent/run/stream?runId=run-1&after=0",
      "/api/v1/agent/session/get?sessionId=session-1",
    ]);
  });

  it("continues from persisted cursor when recovery still finds a running run", async () => {
    const requestedUrls: string[] = [];
    let streamAttempts = 0;
    const input = baseInput(async (request) => {
      const url = String(request);
      requestedUrls.push(url);
      if (url.includes("/agent/run/stream")) {
        streamAttempts += 1;
        if (streamAttempts === 1) {
          throw new TypeError("network unavailable");
        }
        return streamResponse([
          startFrame(2),
          textFrame(3, "后续"),
          finishFrame(4),
          "data: [DONE]\n\n",
        ]);
      }
      return jsonResponse({
        data: {
          session: {
            id: "session-1",
            title: "恢复会话",
            createdAt: "2026-11-05T00:00:00Z",
            updatedAt: "2026-11-05T00:00:02Z",
          },
          messages: [
            {
              id: "assistant-1",
              sessionId: "session-1",
              seq: 2,
              role: "assistant",
              status: "streaming",
              activeRunId: "run-1",
              userMessageId: "user-1",
              createdAt: "2026-11-05T00:00:01Z",
              updatedAt: "2026-11-05T00:00:02Z",
            },
          ],
          messageParts: [
            {
              id: "part-1",
              messageId: "assistant-1",
              runId: "run-1",
              index: 0,
              type: "text",
              status: "streaming",
              content: { type: "text", text: "已有" },
              createdAt: "2026-11-05T00:00:01Z",
              updatedAt: "2026-11-05T00:00:02Z",
            },
          ],
          activeRun: {
            runId: "run-1",
            status: "running",
            lastSequence: 2,
          },
          nextMessageCursor: null,
        },
      });
    });
    input.maxRetries = 0;
    const snapshots: string[] = [];
    input.handlers.onSnapshot = (snapshot) => {
      const part = snapshot.parts.find((candidate) => candidate.type === "text");
      if (part?.type === "text") {
        snapshots.push(part.content.text);
      }
    };

    const [result, error] = await consumeAgentRun(input);

    expect(error).toBeNull();
    expect(result?.status).toBe("completed");
    expect(snapshots.at(0)).toBe("已有");
    expect(snapshots.at(-1)).toBe("已有后续");
    expect(requestedUrls).toEqual([
      "/api/v1/agent/run/stream?runId=run-1&after=0",
      "/api/v1/agent/session/get?sessionId=session-1",
      "/api/v1/agent/run/stream?runId=run-1&after=2",
    ]);
  });

  it("rejects persisted detail when the assistant now belongs to another run", async () => {
    const snapshots: string[] = [];
    const input = baseInput(async (request) => {
      const url = String(request);
      if (url.includes("/agent/run/stream")) {
        throw new TypeError("network unavailable");
      }
      return jsonResponse({
        data: {
          session: {
            id: "session-1",
            title: "重试后的会话",
            createdAt: "2026-11-05T00:00:00Z",
            updatedAt: "2026-11-05T00:00:03Z",
          },
          messages: [
            {
              id: "assistant-1",
              sessionId: "session-1",
              seq: 2,
              role: "assistant",
              status: "complete",
              activeRunId: "run-2",
              userMessageId: "user-1",
              createdAt: "2026-11-05T00:00:01Z",
              updatedAt: "2026-11-05T00:00:03Z",
            },
          ],
          messageParts: [
            {
              id: "part-2",
              messageId: "assistant-1",
              runId: "run-2",
              index: 0,
              type: "text",
              status: "complete",
              content: { type: "text", text: "新 Run 的内容" },
              createdAt: "2026-11-05T00:00:02Z",
              updatedAt: "2026-11-05T00:00:03Z",
            },
          ],
          activeRun: null,
          nextMessageCursor: null,
        },
      });
    });
    input.maxRetries = 0;
    input.handlers.onSnapshot = (snapshot) => {
      const part = snapshot.parts.find((candidate) => candidate.type === "text");
      if (part?.type === "text") {
        snapshots.push(part.content.text);
      }
    };

    const [result, error] = await consumeAgentRun(input);

    expect(result).toBeNull();
    expect(error?.code).toBe("AGENT_RUN_INTERRUPTED");
    expect(snapshots).toEqual([]);
  });

  it("forwards reasoning deltas to the reasoning part only", async () => {
    const snapshots: Array<{ reasoning: string; text: string }> = [];
    const input = baseInput(() =>
      Promise.resolve(
        streamResponse([
          startFrame(1),
          `id: 2\ndata: ${JSON.stringify({
            type: "reasoning-start",
            id: "reasoning-1",
          })}\n\n`,
          `id: 3\ndata: ${JSON.stringify({
            type: "reasoning-delta",
            id: "reasoning-1",
            delta: "先分析",
          })}\n\n`,
          `id: 4\ndata: ${JSON.stringify({
            type: "reasoning-end",
            id: "reasoning-1",
          })}\n\n`,
          `id: 5\ndata: ${JSON.stringify({
            type: "text-start",
            id: "part-1",
          })}\n\n`,
          textFrame(6, "最终回答"),
          finishFrame(7),
          "data: [DONE]\n\n",
        ]),
      ),
    );
    input.materializer = createAgentRunMaterializer({
      sessionId: "session-1",
      runId: "run-1",
      userMessageId: "user-1",
      assistantMessageId: "assistant-1",
      assistantPartId: "part-1",
      reasoningPartId: "reasoning-1",
    });
    input.handlers.onSnapshot = (snapshot) => {
      snapshots.push({
        reasoning: snapshot.parts
          .filter((part) => part.type === "reasoning")
          .map((part) => part.content.text)
          .join(""),
        text: snapshot.parts
          .filter((part) => part.type === "text")
          .map((part) => part.content.text)
          .join(""),
      });
    };

    const [, error] = await consumeAgentRun(input);

    expect(error).toBeNull();
    expect(snapshots.at(-1)).toEqual({
      reasoning: "先分析",
      text: "最终回答",
    });
  });
});
