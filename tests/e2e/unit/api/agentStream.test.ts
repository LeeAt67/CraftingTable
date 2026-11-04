import { request as httpRequest } from "node:http";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createApp } from "../../../../api/src/app";

describe("Agent stream API", () => {
  const apps: Array<ReturnType<typeof createApp>> = [];

  afterEach(async () => {
    await Promise.all(apps.splice(0).map((app) => app.close()));
  });

  it("relays normalized deltas without exposing the provider key", async () => {
    const encoder = new TextEncoder();
    const upstreamFetch = vi.fn(async () => {
      return new Response(
        new ReadableStream({
          start(controller) {
            controller.enqueue(
              encoder.encode(
                'data: {"choices":[{"delta":{"content":"你好"}}]}\n\n',
              ),
            );
            controller.enqueue(
              encoder.encode(
                'data: {"choices":[{"delta":{"content":"，Leeat"}}]}\n\n',
              ),
            );
            controller.enqueue(encoder.encode("data: [DONE]\n\n"));
            controller.close();
          },
        }),
        { status: 200, headers: { "Content-Type": "text/event-stream" } },
      );
    });
    const app = createApp({
      databasePath: ":memory:",
      agent: {
        baseUrl: "https://llm.example/v1",
        apiKey: "secret-key",
        model: "test-model",
        fetchImpl: upstreamFetch,
      },
    });
    apps.push(app);

    const response = await app.inject({
      method: "POST",
      url: "/api/v1/agent/stream",
      payload: {
        messages: [{ role: "user", content: "你好" }],
      },
    });

    expect(response.statusCode).toBe(200);
    expect(response.body).toContain('event: delta\ndata: {"text":"你好"}');
    expect(response.body).toContain(
      'event: delta\ndata: {"text":"，Leeat"}',
    );
    expect(response.body).toContain("event: done");
    expect(response.body).not.toContain("secret-key");
    const [, init] = upstreamFetch.mock.calls[0] as unknown as [
      RequestInfo | URL,
      RequestInit,
    ];
    expect(init?.headers).toMatchObject({
      Authorization: "Bearer secret-key",
    });
  });

  it("returns an actionable error when the model is not configured", async () => {
    const app = createApp({ databasePath: ":memory:" });
    apps.push(app);

    const response = await app.inject({
      method: "POST",
      url: "/api/v1/agent/stream",
      payload: {
        messages: [{ role: "user", content: "你好" }],
      },
    });

    expect(response.statusCode).toBe(503);
    expect(response.json()).toMatchObject({
      error: { code: "AGENT_NOT_CONFIGURED" },
    });
  });

  it("processes a final upstream event without a trailing blank line", async () => {
    const encoder = new TextEncoder();
    const upstreamFetch = vi.fn(async () => {
      return new Response(
        new ReadableStream({
          start(controller) {
            controller.enqueue(
              encoder.encode(
                'data: {"choices":[{"delta":{"content":"你好"}}]}\n\n',
              ),
            );
            controller.enqueue(encoder.encode("data: [DONE]\n"));
            controller.close();
          },
        }),
        { status: 200, headers: { "Content-Type": "text/event-stream" } },
      );
    });
    const app = createApp({
      databasePath: ":memory:",
      agent: {
        baseUrl: "https://llm.example/v1",
        apiKey: "secret-key",
        model: "test-model",
        fetchImpl: upstreamFetch,
      },
    });
    apps.push(app);

    const response = await app.inject({
      method: "POST",
      url: "/api/v1/agent/stream",
      payload: {
        messages: [{ role: "user", content: "你好" }],
      },
    });

    expect(response.statusCode).toBe(200);
    expect(response.body).toContain('event: delta\ndata: {"text":"你好"}');
    expect(response.body).toContain("event: done");
    expect(response.body).not.toContain("event: error");
  });

  it("finishes when the provider sends finish_reason without DONE", async () => {
    const encoder = new TextEncoder();
    const upstreamFetch = vi.fn(async () => {
      return new Response(
        new ReadableStream({
          start(controller) {
            controller.enqueue(
              encoder.encode(
                'data: {"choices":[{"delta":{"content":"完成"},"finish_reason":"stop"}]}\n\n',
              ),
            );
            controller.close();
          },
        }),
        { status: 200, headers: { "Content-Type": "text/event-stream" } },
      );
    });
    const app = createApp({
      databasePath: ":memory:",
      agent: {
        baseUrl: "https://llm.example/v1",
        apiKey: "secret-key",
        model: "test-model",
        fetchImpl: upstreamFetch,
      },
    });
    apps.push(app);

    const response = await app.inject({
      method: "POST",
      url: "/api/v1/agent/stream",
      payload: {
        messages: [{ role: "user", content: "你好" }],
      },
    });

    expect(response.statusCode).toBe(200);
    expect(response.body).toContain('event: delta\ndata: {"text":"完成"}');
    expect(response.body).toContain("event: done");
    expect(response.body).not.toContain("event: error");
  });

  it("does not treat a completed request body as a client disconnect", async () => {
    const encoder = new TextEncoder();
    const upstreamFetch = vi.fn(async () => {
      await new Promise<void>((resolve) => setTimeout(resolve, 500));
      return new Response(
        new ReadableStream({
          start(controller) {
            controller.enqueue(
              encoder.encode(
                'data: {"choices":[{"delta":{"content":"正常完成"}}]}\n\n',
              ),
            );
            controller.enqueue(encoder.encode("data: [DONE]\n\n"));
            controller.close();
          },
        }),
        { status: 200, headers: { "Content-Type": "text/event-stream" } },
      );
    });
    const app = createApp({
      databasePath: ":memory:",
      agent: {
        baseUrl: "https://llm.example/v1",
        apiKey: "secret-key",
        model: "test-model",
        fetchImpl: upstreamFetch,
      },
    });
    apps.push(app);
    await app.listen({ host: "127.0.0.1", port: 0 });
    const address = app.server.address();
    if (!address || typeof address === "string") {
      throw new Error("测试服务未取得端口");
    }

    const { status, body } = await new Promise<{
      status: number;
      body: string;
    }>((resolve, reject) => {
      const clientRequest = httpRequest(
        {
          host: "127.0.0.1",
          port: address.port,
          path: "/api/v1/agent/stream",
          method: "POST",
          headers: { "Content-Type": "application/json" },
        },
        (response) => {
          let responseBody = "";
          response.setEncoding("utf8");
          response.on("data", (chunk: string) => {
            responseBody += chunk;
          });
          response.on("end", () => {
            resolve({ status: response.statusCode ?? 0, body: responseBody });
          });
        },
      );
      clientRequest.once("error", reject);
      clientRequest.end(
        JSON.stringify({
          messages: [{ role: "user", content: "正常请求" }],
        }),
      );
    });

    expect(status).toBe(200);
    expect(body).toContain('event: delta\ndata: {"text":"正常完成"}');
    expect(body).toContain("event: done");
    expect(body).not.toContain('"code":"AGENT_PROVIDER_FAILED"');
  });

  it("aborts the upstream model request when the client closes the stream", async () => {
    const encoder = new TextEncoder();
    let upstreamSignal: AbortSignal | undefined;
    let notifyUpstreamAbort: () => void = () => undefined;
    const upstreamAborted = new Promise<void>((resolve) => {
      notifyUpstreamAbort = resolve;
    });
    const upstreamFetch = vi.fn(async (_input, init) => {
      upstreamSignal = init?.signal ?? undefined;
      return new Response(
        new ReadableStream({
          start(controller) {
            controller.enqueue(
              encoder.encode(
                'data: {"choices":[{"delta":{"content":"开始"}}]}\n\n',
              ),
            );
            upstreamSignal?.addEventListener(
              "abort",
              () => {
                notifyUpstreamAbort();
                controller.close();
              },
              { once: true },
            );
          },
        }),
        { status: 200, headers: { "Content-Type": "text/event-stream" } },
      );
    });
    const app = createApp({
      databasePath: ":memory:",
      agent: {
        baseUrl: "https://llm.example/v1",
        apiKey: "secret-key",
        model: "test-model",
        fetchImpl: upstreamFetch,
      },
    });
    apps.push(app);
    await app.listen({ host: "127.0.0.1", port: 0 });
    const address = app.server.address();
    if (!address || typeof address === "string") {
      throw new Error("测试服务未取得端口");
    }

    await new Promise<void>((resolve, reject) => {
      const clientRequest = httpRequest(
        {
          host: "127.0.0.1",
          port: address.port,
          path: "/api/v1/agent/stream",
          method: "POST",
          headers: { "Content-Type": "application/json" },
        },
        (response) => {
          response.once("data", () => {
            clientRequest.destroy();
            resolve();
          });
          response.resume();
        },
      );
      clientRequest.once("error", (error) => {
        if ((error as NodeJS.ErrnoException).code === "ECONNRESET") {
          resolve();
          return;
        }
        reject(error);
      });
      clientRequest.end(
        JSON.stringify({
          messages: [{ role: "user", content: "测试断开" }],
        }),
      );
    });

    await Promise.race([
      upstreamAborted,
      new Promise<never>((_resolve, reject) => {
        setTimeout(() => reject(new Error("上游请求没有在断开后取消")), 1000);
      }),
    ]);

    expect(upstreamSignal?.aborted).toBe(true);
  });
});
