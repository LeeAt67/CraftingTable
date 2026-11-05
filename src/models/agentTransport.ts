import { z } from "zod";
import {
  agentErrorSchema,
  type AgentError,
} from "@/models/agentContracts";

export type AgentFetch = (
  input: RequestInfo | URL,
  init?: RequestInit,
) => Promise<Response>;

type AgentStreamRequest = {
  baseUrl: string;
  sessionId: string;
  clientRequestId: string;
  userMessageId: string;
  assistantMessageId: string;
  assistantPartId: string;
  runId: string;
  messages: Array<{ role: "user" | "assistant"; content: string }>;
  signal: AbortSignal;
  fetchImpl: AgentFetch;
};

const errorResponseSchema = z.object({
  error: agentErrorSchema,
});

const fallbackRequestError: AgentError = {
  code: "AGENT_PROVIDER_FAILED",
  kind: "server",
  message: "模型服务请求失败",
  retryable: true,
};

export function agentRequestError(agentError: AgentError) {
  const error = new Error(agentError.message) as Error & {
    agentError: AgentError;
  };
  error.agentError = agentError;
  return error;
}

export async function sendAgentStream({
  baseUrl,
  sessionId,
  clientRequestId,
  userMessageId,
  assistantMessageId,
  assistantPartId,
  runId,
  messages,
  signal,
  fetchImpl,
}: AgentStreamRequest): Promise<ReadableStream<Uint8Array>> {
  const response = await fetchImpl(`${baseUrl}/agent/stream`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      sessionId,
      clientRequestId,
      userMessageId,
      assistantMessageId,
      assistantPartId,
      runId,
      messages,
    }),
    signal,
  });

  if (!response.ok) {
    let agentError = fallbackRequestError;
    try {
      const parsed = errorResponseSchema.safeParse(await response.json());
      if (parsed.success) {
        agentError = parsed.data.error;
      }
    } catch {
      // Keep the stable fallback for non-JSON error responses.
    }
    throw agentRequestError(agentError);
  }

  if (!response.body) {
    throw agentRequestError({
      code: "AGENT_PROVIDER_FAILED",
      kind: "server",
      message: "模型服务未返回响应流",
      retryable: true,
    });
  }

  return response.body;
}
