import type { FastifyInstance, FastifyReply } from "fastify";
import { z } from "zod";
import { fail } from "../views/jsonView";

export type AgentConfig = {
  baseUrl: string;
  apiKey: string;
  model: string;
  fetchImpl?: typeof fetch;
};

const requestSchema = z.object({
  messages: z
    .array(
      z.object({
        role: z.enum(["user", "assistant"]),
        content: z.string().max(100_000),
      }),
    )
    .min(1)
    .max(100),
});

type UpstreamEvent = {
  choices?: Array<{
    delta?: {
      content?: string | null;
    };
    finish_reason?: string | null;
  }>;
};

function writeEvent(reply: FastifyReply, event: string, data: unknown) {
  reply.raw.write(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`);
}

function parseUpstreamEvents(buffer: string) {
  const events: string[] = [];
  const values: UpstreamEvent[] = [];
  for (const block of buffer.split(/\r?\n\r?\n/)) {
    for (const line of block.split(/\r?\n/)) {
      if (!line.startsWith("data:")) {
        continue;
      }
      const data = line.slice(5).trim();
      if (!data) {
        continue;
      }
      events.push(data);
      if (data !== "[DONE]") {
        try {
          values.push(JSON.parse(data) as UpstreamEvent);
        } catch {
          throw new Error("模型服务返回了无效事件");
        }
      }
    }
  }
  return { events, values };
}

export function registerAgentController(
  app: FastifyInstance,
  config?: AgentConfig,
) {
  app.post("/api/v1/agent/stream", async (request, reply) => {
    if (!config?.baseUrl || !config.apiKey || !config.model) {
      return reply.code(503).send(
        fail("AGENT_NOT_CONFIGURED", "Agent 模型服务尚未配置", {
          kind: "server",
          retryable: false,
        }),
      );
    }

    const parsed = requestSchema.safeParse(request.body);
    if (!parsed.success) {
      return reply
        .code(400)
        .send(
          fail("VALIDATION_ERROR", "消息参数无效", {
            kind: "validation",
            retryable: false,
          }),
        );
    }

    const abortController = new AbortController();
    const abortUpstream = () => abortController.abort();
    const abortOnResponseClose = () => {
      if (!reply.raw.writableFinished) {
        abortUpstream();
      }
    };
    request.raw.once("aborted", abortUpstream);
    reply.raw.once("close", abortOnResponseClose);
    reply.raw.socket?.once("close", abortUpstream);
    const upstreamFetch = config.fetchImpl ?? fetch;
    const upstreamUrl = `${config.baseUrl.replace(/\/$/, "")}/chat/completions`;

    let upstream: Response;
    try {
      upstream = await upstreamFetch(upstreamUrl, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${config.apiKey}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          model: config.model,
          messages: parsed.data.messages,
          stream: true,
        }),
        signal: abortController.signal,
      });
    } catch (error) {
      const message =
        error instanceof Error && error.name === "AbortError"
          ? "模型请求已取消"
          : "无法连接模型服务";
      return reply.code(502).send(
        fail("AGENT_PROVIDER_FAILED", message, {
          kind: "server",
          retryable: true,
        }),
      );
    }

    if (!upstream.ok || !upstream.body) {
      return reply
        .code(502)
        .send(
          fail("AGENT_PROVIDER_FAILED", "模型服务请求失败", {
            kind: "server",
            retryable: true,
          }),
        );
    }

    reply.hijack();
    reply.raw.writeHead(200, {
      "Content-Type": "text/event-stream; charset=utf-8",
      "Cache-Control": "no-cache, no-transform",
      Connection: "keep-alive",
      "X-Accel-Buffering": "no",
    });

    const reader = upstream.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    let finished = false;
    const processBlock = (block: string) => {
      const { events, values } = parseUpstreamEvents(block);
      for (let index = 0; index < events.length; index += 1) {
        if (events[index] === "[DONE]") {
          writeEvent(reply, "done", { status: "completed" });
          return true;
        }
        const text = values[index]?.choices?.[0]?.delta?.content;
        if (text) {
          writeEvent(reply, "delta", { text });
        }
        if (values[index]?.choices?.[0]?.finish_reason) {
          writeEvent(reply, "done", { status: "completed" });
          return true;
        }
      }
      return false;
    };

    try {
      while (true) {
        const { done, value } = await reader.read();
        buffer += decoder.decode(value, { stream: !done });
        const blocks = buffer.split(/\r?\n\r?\n/);
        buffer = blocks.pop() ?? "";

        for (const block of blocks) {
          if (processBlock(block)) {
            finished = true;
            break;
          }
        }
        if (done || finished) {
          break;
        }
      }

      if (!finished && buffer.trim()) {
        finished = processBlock(buffer);
      }

      if (!finished && !abortController.signal.aborted) {
        writeEvent(reply, "error", {
          code: "PROTOCOL_UNEXPECTED_EOF",
          kind: "protocol",
          message: "模型响应意外中断",
          retryable: true,
        });
      }
    } catch {
      if (!abortController.signal.aborted) {
        writeEvent(reply, "error", {
          code: "AGENT_PROVIDER_FAILED",
          kind: "server",
          message: "模型生成失败",
          retryable: true,
        });
      }
    } finally {
      request.raw.removeListener("aborted", abortUpstream);
      reply.raw.removeListener("close", abortOnResponseClose);
      reply.raw.socket?.removeListener("close", abortUpstream);
      try {
        await reader.cancel();
      } catch {
        // The upstream body may already be closed after cancellation.
      }
      reader.releaseLock();
      if (!reply.raw.writableEnded) {
        reply.raw.end();
      }
    }

    return reply;
  });
}
