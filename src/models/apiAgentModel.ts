import { z } from "zod";
import {
  agentErrorSchema,
  type AgentError,
  type AgentSendInput,
} from "@/models/agentContracts";
import type { AgentModel } from "@/models/agentModel";
import { createFakeAgentModel } from "@/models/fakeAgentModel";

type FetchLike = (
  input: RequestInfo | URL,
  init?: RequestInit,
) => Promise<Response>;

type ApiAgentModelOptions = {
  baseUrl?: string;
  fetchImpl?: FetchLike;
};

type SseEvent = {
  event: string;
  data: string;
};

const errorResponseSchema = z.object({
  error: agentErrorSchema,
});

function requestError(agentError: AgentError) {
  const error = new Error(agentError.message) as Error & {
    agentError: AgentError;
  };
  error.agentError = agentError;
  return error;
}

function parseEvent(block: string): SseEvent | null {
  let event = "message";
  const data: string[] = [];
  for (const line of block.split(/\r?\n/)) {
    if (line.startsWith("event:")) {
      event = line.slice(6).trim();
    } else if (line.startsWith("data:")) {
      data.push(line.slice(5).trimStart());
    }
  }
  return data.length > 0 ? { event, data: data.join("\n") } : null;
}

async function* readServerEvents(
  response: Response,
): AsyncIterable<SseEvent> {
  if (!response.body) {
    throw new Error("模型服务未返回流");
  }
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    buffer += decoder.decode(value, { stream: !done });
    const blocks = buffer.split(/\r?\n\r?\n/);
    buffer = blocks.pop() ?? "";
    for (const block of blocks) {
      const event = parseEvent(block);
      if (event) {
        yield event;
      }
    }
    if (done) {
      break;
    }
  }

  const finalEvent = parseEvent(buffer);
  if (finalEvent) {
    yield finalEvent;
  }
}

async function* streamAssistantText(
  history: Array<{ role: "user" | "assistant"; content: string }>,
  signal: AbortSignal,
  baseUrl: string,
  fetchImpl: FetchLike,
) {
  const response = await fetchImpl(`${baseUrl}/agent/stream`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ messages: history }),
    signal,
  });

  if (!response.ok) {
    let agentError: AgentError = {
      code: "AGENT_PROVIDER_FAILED",
      kind: "server",
      message: "模型服务请求失败",
      retryable: true,
    };
    try {
      const parsed = errorResponseSchema.safeParse(await response.json());
      if (parsed.success) {
        agentError = parsed.data.error;
      }
    } catch {
      // Keep the stable fallback when the upstream response is not JSON.
    }
    throw requestError(agentError);
  }

  let finished = false;
  for await (const event of readServerEvents(response)) {
    if (event.event === "delta") {
      const payload = JSON.parse(event.data) as { text?: string };
      if (payload.text) {
        yield payload.text;
      }
    } else if (event.event === "error") {
      const parsed = agentErrorSchema.safeParse(JSON.parse(event.data));
      if (parsed.success) {
        throw requestError(parsed.data);
      }
      throw new Error("模型生成失败");
    } else if (event.event === "done") {
      finished = true;
      return;
    }
  }

  if (!finished) {
    throw new Error("模型响应意外中断");
  }
}

export function createApiAgentModel(
  options: ApiAgentModelOptions = {},
): AgentModel {
  const baseUrl = options.baseUrl ?? "/api/v1";
  const fetchImpl = options.fetchImpl ?? fetch;

  return createFakeAgentModel({
    chunkDelayMs: 0,
    responseChunks: (input: AgentSendInput, history, signal) =>
      streamAssistantText(history, signal, baseUrl, fetchImpl),
  });
}
