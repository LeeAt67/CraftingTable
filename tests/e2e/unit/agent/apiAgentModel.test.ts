import { describe, expect, it } from "vitest";
import { createApiAgentModel } from "@/models/apiAgentModel";

describe("ApiAgentModel", () => {
  it("consumes normalized server SSE events without exposing provider keys", async () => {
    const encoder = new TextEncoder();
    let requestBody = "";
    const model = createApiAgentModel({
      baseUrl: "/api/v1",
      fetchImpl: async (_input, init) => {
        requestBody = String(init?.body);
        return new Response(
          new ReadableStream({
            start(controller) {
              controller.enqueue(
                encoder.encode('event: delta\ndata: {"text":"你好"}\n\n'),
              );
              controller.enqueue(
                encoder.encode('event: delta\ndata: {"text":"，Leeat"}\n\n'),
              );
              controller.enqueue(
                encoder.encode('event: done\ndata: {"status":"completed"}\n\n'),
              );
              controller.close();
            },
          }),
          {
            status: 200,
            headers: { "Content-Type": "text/event-stream" },
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
    expect(requestBody).not.toContain("AGENT_API_KEY");
    expect(requestBody).toContain('"content":"你好"');
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
