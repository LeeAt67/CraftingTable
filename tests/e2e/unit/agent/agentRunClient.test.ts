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
    handlers: {
      onStarted: () => undefined,
      onSnapshot: () => undefined,
    },
    retryDelayMs: 0,
  };
}

describe("AgentRunClient", () => {
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
});
