import { z } from "zod";
import {
  agentErrorSchema,
  type AgentError,
  type AgentRunHandle,
  type AgentSession,
  type AgentSessionDetail,
  type CraftingTableSendMessage,
} from "@/models/agentContracts";

export type AgentFetch = (
  input: RequestInfo | URL,
  init?: RequestInit,
) => Promise<Response>;

type Envelope<T> = { data: T } | { error: AgentError };

const errorResponseSchema = z.object({
  error: agentErrorSchema,
});

const runHandleSchema = z.object({
  runId: z.string().min(1),
  sessionId: z.string().min(1),
  userMessageId: z.string().min(1),
  assistantMessageId: z.string().min(1),
  assistantPartId: z.string().min(1),
  status: z.enum(["running", "completed", "failed", "cancelled", "interrupted"]),
});

const fallbackRequestError: AgentError = {
  code: "AGENT_INTERNAL_FAILED",
  kind: "server",
  message: "Agent 服务请求失败",
  retryable: true,
};

const invalidRunHandleError: AgentError = {
  code: "AGENT_RUN_START_FAILED",
  kind: "protocol",
  message: "Agent 运行标识无效",
  retryable: false,
};

export function agentRequestError(agentError: AgentError) {
  const error = new Error(agentError.message) as Error & {
    agentError: AgentError;
  };
  error.agentError = agentError;
  return error;
}

async function responseError(
  response: Response,
  fallback: AgentError = fallbackRequestError,
) {
  try {
    const parsed = errorResponseSchema.safeParse(await response.json());
    if (parsed.success) {
      return parsed.data.error;
    }
  } catch {
    // Keep the stable fallback for non-JSON error responses.
  }
  return fallback;
}

async function readAgentEnvelope<T>(response: Response): Promise<T> {
  if (!response.ok) {
    throw agentRequestError(await responseError(response));
  }
  const body = (await response.json()) as Envelope<T>;
  if ("error" in body) {
    throw agentRequestError(body.error);
  }
  return body.data;
}

async function postJSON<T>(
  url: string,
  body: unknown,
  fetchImpl: AgentFetch,
  signal?: AbortSignal,
) {
  const response = await fetchImpl(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    signal,
  });
  return readAgentEnvelope<T>(response);
}

export async function createAgentSession(
  baseUrl: string,
  clientRequestId: string,
  fetchImpl: AgentFetch,
) {
  return postJSON<{ session: AgentSession }>(
    `${baseUrl}/agent/session/create`,
    { clientRequestId },
    fetchImpl,
  );
}

export async function listAgentSessions(
  baseUrl: string,
  fetchImpl: AgentFetch,
) {
  const response = await fetchImpl(`${baseUrl}/agent/session/list`);
  return readAgentEnvelope<AgentSession[]>(response);
}

export async function getAgentSession(
  baseUrl: string,
  sessionId: string,
  fetchImpl: AgentFetch,
) {
  const response = await fetchImpl(
    `${baseUrl}/agent/session/get?sessionId=${encodeURIComponent(sessionId)}`,
  );
  return readAgentEnvelope<AgentSessionDetail>(response);
}

export async function startAgentRun(
  input: {
    baseUrl: string;
    sessionId: string;
    clientRequestId: string;
    message: CraftingTableSendMessage;
    fetchImpl: AgentFetch;
  },
) {
  const handle = await postJSON<AgentRunHandle>(
    `${input.baseUrl}/agent/message/send`,
    {
      sessionId: input.sessionId,
      clientRequestId: input.clientRequestId,
      message: input.message,
    },
    input.fetchImpl,
  );
  return readRunHandle(handle);
}

export async function retryAgentRun(input: {
  baseUrl: string;
  sessionId: string;
  assistantMessageId: string;
  clientRequestId: string;
  fetchImpl: AgentFetch;
}) {
  const handle = await postJSON<AgentRunHandle>(
    `${input.baseUrl}/agent/message/retry`,
    {
      sessionId: input.sessionId,
      assistantMessageId: input.assistantMessageId,
      clientRequestId: input.clientRequestId,
    },
    input.fetchImpl,
  );
  return readRunHandle(handle);
}

export async function cancelAgentRun(input: {
  baseUrl: string;
  sessionId: string;
  runId: string;
  fetchImpl: AgentFetch;
}) {
  return postJSON<{ runId: string; status: AgentRunHandle["status"] }>(
    `${input.baseUrl}/agent/run/cancel`,
    { sessionId: input.sessionId, runId: input.runId },
    input.fetchImpl,
  );
}

export async function openAgentRunStream(input: {
  baseUrl: string;
  runId: string;
  afterSequence: number;
  signal: AbortSignal;
  fetchImpl: AgentFetch;
}): Promise<ReadableStream<Uint8Array>> {
  const response = await input.fetchImpl(
    `${input.baseUrl}/agent/run/stream?runId=${encodeURIComponent(input.runId)}&after=${input.afterSequence}`,
    { signal: input.signal },
  );
  if (!response.ok) {
    throw agentRequestError(await responseError(response));
  }
  if (!response.body) {
    throw agentRequestError({
      code: "AGENT_RUN_INTERRUPTED",
      kind: "server",
      message: "Agent 运行未返回响应流",
      retryable: true,
    });
  }
  return response.body;
}

function readRunHandle(value: AgentRunHandle) {
  const parsed = runHandleSchema.safeParse(value);
  if (!parsed.success) {
    throw agentRequestError(invalidRunHandleError);
  }
  return parsed.data;
}
