import { describe, expect, it, vi } from "vitest";
import { createApiAgentModel } from "@/models/apiAgentModel";

const encoder = new TextEncoder();

function jsonResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify({ data }), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function errorResponse(error: unknown, status: number) {
  return new Response(JSON.stringify({ error }), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

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
      headers: {
        "Content-Type": "text/event-stream",
        "x-vercel-ai-ui-message-stream": "v1",
      },
    },
  );
}

function startFrame({
  cursor = 1,
  messageId = "assistant-1",
  runId = "run-1",
  sessionId = "session-1",
  userMessageId = "user-1",
}: {
  cursor?: number;
  messageId?: string;
  runId?: string;
  sessionId?: string;
  userMessageId?: string;
} = {}) {
  return `id: ${cursor}\ndata: ${JSON.stringify({
    type: "start",
    messageId,
    messageMetadata: {
      sessionId,
      assistantMessageId: messageId,
      runId,
      userMessageId,
    },
  })}\n\n`;
}

function textFrame(cursor: number, text: string, partId = "part-1") {
  return `id: ${cursor}\ndata: ${JSON.stringify({
    type: "text-delta",
    id: partId,
    delta: text,
  })}\n\n`;
}

function finishFrame(cursor: number, runId = "run-1", status = "completed") {
  return `id: ${cursor}\ndata: ${JSON.stringify({
    type: "finish",
    finishReason: "stop",
    messageMetadata: { runId, status },
  })}\n\n`;
}

const doneFrame = "data: [DONE]\n\n";

const handle = {
  runId: "run-1",
  sessionId: "session-1",
  userMessageId: "user-1",
  assistantMessageId: "assistant-1",
  assistantPartId: "part-1",
  status: "running" as const,
};

describe("ApiAgentModel", () => {
  it("creates sessions through the persistent session API", async () => {
    const calls: Array<{ url: string; init?: RequestInit }> = [];
    const model = createApiAgentModel({
      baseUrl: "/api/v1",
      fetchImpl: async (input, init) => {
        calls.push({ url: String(input), init });
        return jsonResponse({
          session: {
            id: "session-1",
            title: "新会话",
            createdAt: "2026-11-05T00:00:00Z",
            updatedAt: "2026-11-05T00:00:00Z",
          },
        });
      },
    });

    const [created, error] = await model.createSession({
      clientRequestId: "request-1",
    });

    expect(error).toBeNull();
    expect(created?.session.id).toBe("session-1");
    expect(calls).toHaveLength(1);
    expect(calls[0]?.url).toBe("/api/v1/agent/session/create");
    expect(calls[0]?.init).toMatchObject({
      method: "POST",
      body: JSON.stringify({ clientRequestId: "request-1" }),
    });
  });

  it("loads session lists and details from HTTP", async () => {
    const requestedUrls: string[] = [];
    const session = {
      id: "session-1",
      title: "持久化会话",
      createdAt: "2026-11-05T00:00:00Z",
      updatedAt: "2026-11-05T00:00:01Z",
    };
    const model = createApiAgentModel({
      baseUrl: "/api/v1",
      fetchImpl: async (input) => {
        const url = String(input);
        requestedUrls.push(url);
        if (url.endsWith("/agent/session/list")) {
          return jsonResponse([session]);
        }
        return jsonResponse({
          session,
          messages: [],
          messageParts: [],
          activeRun: null,
          nextMessageCursor: null,
        });
      },
    });

    const [sessions, listError] = await model.listSessions();
    const [detail, detailError] = await model.getSession("session-1");

    expect(listError).toBeNull();
    expect(sessions).toEqual([session]);
    expect(detailError).toBeNull();
    expect(detail?.session.id).toBe("session-1");
    expect(requestedUrls).toEqual([
      "/api/v1/agent/session/list",
      "/api/v1/agent/session/get?sessionId=session-1",
    ]);
  });

  it("creates a run before subscribing to its cursor stream", async () => {
    const calls: Array<{ url: string; init?: RequestInit }> = [];
    const model = createApiAgentModel({
      baseUrl: "/api/v1",
      fetchImpl: async (input, init) => {
        const url = String(input);
        calls.push({ url, init });
        if (url.endsWith("/agent/message/send")) {
          return jsonResponse(handle);
        }
        return streamResponse([
          startFrame(),
          textFrame(2, "你好"),
          textFrame(3, "，Leeat"),
          finishFrame(4),
          doneFrame,
        ]);
      },
    });
    const started = vi.fn();
    const snapshots: string[] = [];

    const [result, error] = await model.sendMessage(
      {
        sessionId: "session-1",
        clientRequestId: "send-1",
        message: {
          id: "user-1",
          role: "user",
          parts: [{ type: "text", text: "你好" }],
        },
        signal: new AbortController().signal,
      },
      {
        onStarted: started,
        onSnapshot: (snapshot) => {
          const textPart = snapshot.parts.find((part) => part.type === "text");
          if (textPart?.type === "text") {
            snapshots.push(textPart.content.text);
          }
        },
      },
    );

    expect(error).toBeNull();
    expect(result).toEqual({
      assistantMessageId: "assistant-1",
      runId: "run-1",
      status: "completed",
    });
    expect(started).toHaveBeenCalledWith({
      assistantMessageId: "assistant-1",
      runId: "run-1",
      status: "running",
    });
    expect(snapshots.at(-1)).toBe("你好，Leeat");
    expect(calls.map((call) => call.url)).toEqual([
      "/api/v1/agent/message/send",
      "/api/v1/agent/run/stream?runId=run-1&after=0",
    ]);
    expect(JSON.parse(String(calls[0]?.init?.body))).toEqual({
      sessionId: "session-1",
      clientRequestId: "send-1",
      message: {
        id: "user-1",
        role: "user",
        parts: [{ type: "text", text: "你好" }],
      },
    });
    expect(String(calls[0]?.init?.body)).not.toContain("AGENT_API_KEY");
    expect(String(calls[0]?.init?.body)).not.toContain("messages");
  });

  it("retries through the run API and keeps the existing assistant identity", async () => {
    const calls: string[] = [];
    const model = createApiAgentModel({
      baseUrl: "/api/v1",
      fetchImpl: async (input) => {
        const url = String(input);
        calls.push(url);
        if (url.endsWith("/agent/message/retry")) {
          return jsonResponse(handle);
        }
        return streamResponse([
          startFrame(),
          textFrame(2, "重试成功"),
          finishFrame(3),
          doneFrame,
        ]);
      },
    });
    const snapshots: string[] = [];

    const [result, error] = await model.retryMessage(
      {
        sessionId: "session-1",
        assistantMessageId: "assistant-1",
        clientRequestId: "retry-1",
        signal: new AbortController().signal,
      },
      {
        onStarted: () => undefined,
        onSnapshot: (snapshot) => {
          const textPart = snapshot.parts.find((part) => part.type === "text");
          if (textPart?.type === "text") {
            snapshots.push(textPart.content.text);
          }
        },
      },
    );

    expect(error).toBeNull();
    expect(result?.assistantMessageId).toBe("assistant-1");
    expect(snapshots.at(-1)).toBe("重试成功");
    expect(calls).toEqual([
      "/api/v1/agent/message/retry",
      "/api/v1/agent/run/stream?runId=run-1&after=0",
    ]);
  });

  it("cancels the server run through the cancel endpoint", async () => {
    let requestBody: unknown;
    const model = createApiAgentModel({
      baseUrl: "/api/v1",
      fetchImpl: async (input, init) => {
        expect(String(input)).toBe("/api/v1/agent/run/cancel");
        requestBody = JSON.parse(String(init?.body));
        return jsonResponse({ runId: "run-1", status: "cancelled" });
      },
    });

    const [cancelled, error] = await model.cancelRun({
      sessionId: "session-1",
      runId: "run-1",
    });

    expect(error).toBeNull();
    expect(cancelled).toEqual({ runId: "run-1", status: "cancelled" });
    expect(requestBody).toEqual({
      sessionId: "session-1",
      runId: "run-1",
    });
  });

  it("resumes a persisted active run after the supplied cursor", async () => {
    const requestedUrls: string[] = [];
    const model = createApiAgentModel({
      baseUrl: "/api/v1",
      fetchImpl: async (input) => {
        const url = String(input);
        requestedUrls.push(url);
        if (url.includes("/agent/session/get")) {
          return jsonResponse({
            session: {
              id: "session-1",
              title: "恢复会话",
              createdAt: "2026-11-05T00:00:00Z",
              updatedAt: "2026-11-05T00:00:02Z",
            },
            messages: [
              {
                id: "user-1",
                sessionId: "session-1",
                seq: 1,
                role: "user",
                status: "complete",
                createdAt: "2026-11-05T00:00:00Z",
                updatedAt: "2026-11-05T00:00:00Z",
              },
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
              lastSequence: 4,
            },
            nextMessageCursor: null,
          });
        }
        return streamResponse([
          startFrame({ cursor: 4 }),
          textFrame(5, "后续"),
          finishFrame(6),
          doneFrame,
        ]);
      },
    });
    const snapshots: string[] = [];

    const [result, error] = await model.resumeRun(
      {
        sessionId: "session-1",
        runId: "run-1",
        afterSequence: 4,
        signal: new AbortController().signal,
      },
      {
        onStarted: () => undefined,
        onSnapshot: (snapshot) => {
          const textPart = snapshot.parts.find((part) => part.type === "text");
          if (textPart?.type === "text") {
            snapshots.push(textPart.content.text);
          }
        },
      },
    );

    expect(error).toBeNull();
    expect(result?.status).toBe("completed");
    expect(snapshots.at(0)).toBe("已有");
    expect(snapshots.at(-1)).toBe("已有后续");
    expect(requestedUrls).toEqual([
      "/api/v1/agent/session/get?sessionId=session-1",
      "/api/v1/agent/run/stream?runId=run-1&after=4",
    ]);
  });

  it("preserves a stable server error when the run cannot start", async () => {
    const model = createApiAgentModel({
      baseUrl: "/api/v1",
      fetchImpl: async () =>
        errorResponse(
          {
            code: "AGENT_NOT_CONFIGURED",
            kind: "server",
            message: "Agent 模型服务尚未配置",
            retryable: false,
            requestId: "request-1",
          },
          503,
        ),
    });
    const started = vi.fn();

    const [result, error] = await model.sendMessage(
      {
        sessionId: "session-1",
        clientRequestId: "send-1",
        message: {
          id: "user-1",
          role: "user",
          parts: [{ type: "text", text: "你好" }],
        },
        signal: new AbortController().signal,
      },
      { onStarted: started, onSnapshot: () => undefined },
    );

    expect(result).toBeNull();
    expect(error).toMatchObject({
      code: "AGENT_NOT_CONFIGURED",
      kind: "server",
      message: "Agent 模型服务尚未配置",
      retryable: false,
      requestId: "request-1",
    });
    expect(started).not.toHaveBeenCalled();
  });
});
