import {
  embeddedAgentError,
  type AgentError,
  type AgentMessageStatus,
  type AgentMessageSnapshot,
  type AgentRunStatus,
  type AgentSendHandlers,
  type AgentSendResult,
  type Result,
} from "@/models/agentContracts";
import type { RunMaterializer } from "@/models/agentRunMaterializer";
import { readAgentStream } from "@/models/agentStreamProtocol";
import {
  getAgentSession,
  openAgentRunStream,
  type AgentFetch,
} from "@/models/agentTransport";

export type ConsumeAgentRunInput = {
  baseUrl: string;
  fetchImpl: AgentFetch;
  sessionId: string;
  runId: string;
  assistantMessageId: string;
  afterSequence: number;
  signal: AbortSignal;
  materializer: RunMaterializer;
  recreateMaterializer?: (snapshot: AgentMessageSnapshot) => RunMaterializer;
  handlers: AgentSendHandlers;
  maxRetries?: number;
  retryDelayMs?: number;
};

const DEFAULT_MAX_RETRIES = 5;
const DEFAULT_RETRY_DELAY_MS = 250;

function isAbortError(cause: unknown) {
  return (
    cause instanceof DOMException
      ? cause.name === "AbortError"
      : typeof cause === "object" &&
        cause !== null &&
        "name" in cause &&
        cause.name === "AbortError"
  );
}

function networkError(cause: unknown): AgentError {
  return (
    embeddedAgentError(cause) ?? {
      code: isAbortError(cause) ? "REQUEST_CANCELLED" : "NETWORK_ERROR",
      kind: isAbortError(cause) ? "cancelled" : "network",
      message: isAbortError(cause) ? "Agent 订阅已取消" : "Agent 连接中断",
      retryable: !isAbortError(cause),
    }
  );
}

function wait(delayMs: number, signal: AbortSignal) {
  if (delayMs <= 0 || signal.aborted) {
    return Promise.resolve();
  }
  return new Promise<void>((resolve) => {
    const timer = window.setTimeout(resolve, delayMs);
    signal.addEventListener(
      "abort",
      () => {
        window.clearTimeout(timer);
        resolve();
      },
      { once: true },
    );
  });
}

function requestCancelledResult(): Result<AgentSendResult> {
  return [
    null,
    {
      code: "REQUEST_CANCELLED",
      kind: "cancelled",
      message: "Agent 订阅已取消",
      retryable: false,
    },
  ];
}

function isRunStatus(value: string): value is AgentRunStatus {
  return (
    value === "running" ||
    value === "completed" ||
    value === "failed" ||
    value === "cancelled" ||
    value === "interrupted"
  );
}

function statusFromPersistedMessage(status: AgentMessageStatus): AgentRunStatus {
  switch (status) {
    case "complete":
      return "completed";
    case "streaming":
      return "running";
    default:
      return status;
  }
}

type PersistedRunRecovery = {
  result: Result<AgentSendResult>;
  continueAfter: number | null;
};

async function recoverPersistedRun(
  input: ConsumeAgentRunInput,
): Promise<PersistedRunRecovery> {
  let detail;
  try {
    detail = await getAgentSession(
      input.baseUrl,
      input.sessionId,
      input.fetchImpl,
    );
  } catch (cause) {
    return {
      result: [null, networkError(cause)],
      continueAfter: null,
    };
  }
  const message = detail.messages.find(
    (candidate) =>
      candidate.id === input.assistantMessageId &&
      candidate.activeRunId === input.runId,
  );
  if (!message) {
    return {
      result: [
        null,
        {
          code: "AGENT_RUN_INTERRUPTED",
          kind: "not_found",
          message: "恢复目标已被新的 Run 取代",
          retryable: false,
        },
      ],
      continueAfter: null,
    };
  }
  input.handlers.onSnapshot({
    message,
    parts: detail.messageParts.filter(
      (part) =>
        part.messageId === message.id && part.runId === input.runId,
    ),
  });
  if (input.recreateMaterializer) {
    input.materializer = input.recreateMaterializer({
      message,
      parts: detail.messageParts.filter(
        (part) =>
          part.messageId === message.id && part.runId === input.runId,
      ),
    });
  }
  const status = statusFromPersistedMessage(message.status);
  const activeRun =
    detail.activeRun?.runId === input.runId ? detail.activeRun : null;
  return {
    result: [
      {
        assistantMessageId: message.id,
        runId: input.runId,
        status,
      },
      null,
    ],
    continueAfter:
      status === "running"
        ? Math.max(input.afterSequence, activeRun?.lastSequence ?? 0)
        : null,
  };
}

export async function consumeAgentRun(
  input: ConsumeAgentRunInput,
): Promise<Result<AgentSendResult>> {
  const maxRetries = input.maxRetries ?? DEFAULT_MAX_RETRIES;
  const retryDelayMs = input.retryDelayMs ?? DEFAULT_RETRY_DELAY_MS;
  let cursor = input.afterSequence;
  let attempt = 0;
  let recoveryCycles = 0;

  while (!input.signal.aborted) {
    try {
      const body = await openAgentRunStream({
        baseUrl: input.baseUrl,
        runId: input.runId,
        afterSequence: cursor,
        signal: input.signal,
        fetchImpl: input.fetchImpl,
      });
      for await (const event of readAgentStream(body)) {
        cursor = Math.max(cursor, event.cursor);
        if (event.type === "text-delta") {
          input.handlers.onSnapshot(
            input.materializer.delta(event.delta, event.cursor),
          );
          continue;
        }
        if (event.type === "error") {
          input.handlers.onSnapshot(
            input.materializer.error(
              event.error,
              new Date().toISOString(),
            ),
          );
          return [null, event.error];
        }
        if (!event.status || !isRunStatus(event.status)) {
          const error: AgentError = {
            code: "PROTOCOL_INVALID_METADATA",
            kind: "protocol",
            message: "Agent 运行终态无效",
            retryable: false,
          };
          input.handlers.onSnapshot(
            input.materializer.error(error, new Date().toISOString()),
          );
          return [null, error];
        }
        input.handlers.onSnapshot(
          input.materializer.finish(event.status, new Date().toISOString()),
        );
        return [
          {
            assistantMessageId: input.assistantMessageId,
            runId: input.runId,
            status: event.status,
          },
          null,
        ];
      }
    } catch (cause) {
      if (input.signal.aborted || isAbortError(cause)) {
        return requestCancelledResult();
      }
      const error = networkError(cause);
      if (
        error.code === "AGENT_STREAM_CURSOR_EXPIRED" ||
        (error.retryable && attempt >= maxRetries)
      ) {
        const recovery = await recoverPersistedRun(input);
        if (recovery.continueAfter !== null) {
          recoveryCycles += 1;
          if (recoveryCycles > Math.max(1, maxRetries + 1)) {
            return [
              null,
              {
                code: "AGENT_RUN_INTERRUPTED",
                kind: "server",
                message: "Agent 恢复连接中断",
                retryable: true,
              },
            ];
          }
          cursor = recovery.continueAfter;
          attempt = 0;
          await wait(retryDelayMs, input.signal);
          continue;
        }
        return recovery.result;
      }
      if (!error.retryable || attempt >= maxRetries) {
        return [null, error];
      }
      attempt += 1;
      await wait(
        Math.min(retryDelayMs * 2 ** (attempt - 1), 4_000),
        input.signal,
      );
    }
  }

  return requestCancelledResult();
}
