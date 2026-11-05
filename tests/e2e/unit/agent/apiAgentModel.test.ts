import { describe, expect, it } from "vitest";
import { createApiAgentModel } from "@/models/apiAgentModel";

describe("ApiAgentModel", () => {
  it("consumes the UI Message Stream through the protocol adapter", async () => {
    const encoder = new TextEncoder();
    let requestBody: Record<string, unknown> = {};
    let requestBodyText = "";
    const model = createApiAgentModel({
      baseUrl: "/api/v1",
      fetchImpl: async (_input, init) => {
        requestBodyText = String(init?.body);
        requestBody = JSON.parse(requestBodyText) as Record<string, unknown>;
        const start = JSON.stringify({
          type: "start",
          messageId: requestBody.assistantMessageId,
          messageMetadata: {
            sessionId: requestBody.sessionId,
            assistantMessageId: requestBody.assistantMessageId,
            runId: requestBody.runId,
            userMessageId: requestBody.userMessageId,
          },
        });
        const finish = JSON.stringify({
          type: "finish",
          finishReason: "stop",
          messageMetadata: {
            runId: requestBody.runId,
            status: "completed",
          },
        });
        return new Response(
          new ReadableStream({
            start(controller) {
              controller.enqueue(
                encoder.encode(`data: ${start}\n\n`),
              );
              controller.enqueue(
                encoder.encode('data: {"type":"start-step"}\n\n'),
              );
              controller.enqueue(
                encoder.encode(
                  'data: {"type":"text-start","id":"assistant-part"}\n\n',
                ),
              );
              controller.enqueue(
                encoder.encode(
                  'data: {"type":"text-delta","id":"assistant-part","delta":"你好"}\n\n',
                ),
              );
              controller.enqueue(
                encoder.encode(
                  'data: {"type":"text-delta","id":"assistant-part","delta":"，Leeat"}\n\n',
                ),
              );
              controller.enqueue(
                encoder.encode(
                  'data: {"type":"text-end","id":"assistant-part"}\n\n',
                ),
              );
              controller.enqueue(
                encoder.encode('data: {"type":"finish-step"}\n\n'),
              );
              controller.enqueue(
                encoder.encode(`data: ${finish}\n\n`),
              );
              controller.enqueue(encoder.encode("data: [DONE]\n\n"));
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
      },
    });
    const [created] = await model.createSession({ clientRequestId: "request-1" });
    const snapshots: string[] = [];

    const [result, error] = await model.sendMessage(
      {
        sessionId: created!.session.id,
        clientRequestId: "send-1",
        message: {
          id: "message-1",
          role: "user",
          parts: [{ type: "text", text: "你好" }],
        },
        signal: new AbortController().signal,
      },
      {
        onStarted: () => undefined,
        onSnapshot: (snapshot) => {
          const part = snapshot.parts[0];
          if (part.type === "text") {
            snapshots.push(part.content.text);
          }
        },
      },
    );

    expect(error).toBeNull();
    expect(result?.status).toBe("completed");
    expect(snapshots.at(-1)).toBe("你好，Leeat");
    expect(requestBodyText).not.toContain("AGENT_API_KEY");
    expect(requestBody).toMatchObject({
      sessionId: created!.session.id,
      clientRequestId: "send-1",
      userMessageId: "message-1",
      messages: [{ role: "user", content: "你好" }],
    });
    expect(requestBody).toEqual(
      expect.objectContaining({
        assistantMessageId: expect.any(String),
        assistantPartId: expect.any(String),
        runId: expect.any(String),
      }),
    );
  });

  it("preserves stable server error details at the model boundary", async () => {
    const model = createApiAgentModel({
      baseUrl: "/api/v1",
      fetchImpl: async () =>
        new Response(
          JSON.stringify({
            error: {
              code: "AGENT_NOT_CONFIGURED",
              kind: "server",
              message: "Agent 模型服务尚未配置",
              retryable: false,
              requestId: "request-1",
            },
          }),
          {
            status: 503,
            headers: { "Content-Type": "application/json" },
          },
        ),
    });
    const [created] = await model.createSession({ clientRequestId: "request-1" });

    const [result, error] = await model.sendMessage(
      {
        sessionId: created!.session.id,
        clientRequestId: "send-1",
        message: {
          id: "message-1",
          role: "user",
          parts: [{ type: "text", text: "你好" }],
        },
        signal: new AbortController().signal,
      },
      {
        onStarted: () => undefined,
        onSnapshot: () => undefined,
      },
    );

    expect(result).toBeNull();
    expect(error).toMatchObject({
      code: "AGENT_NOT_CONFIGURED",
      kind: "server",
      message: "Agent 模型服务尚未配置",
      retryable: false,
      requestId: "request-1",
    });
  });
});
