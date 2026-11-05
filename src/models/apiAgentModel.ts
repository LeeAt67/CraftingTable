import {
  embeddedAgentError,
  type AgentError,
  type AgentMessageSnapshot,
  type AgentRunHandle,
  type AgentSendHandlers,
  type AgentSendResult,
  type AgentSessionDetail,
  type Result,
} from "@/models/agentContracts";
import type { AgentModel } from "@/models/agentModel";
import {
  consumeAgentRun,
  type ConsumeAgentRunInput,
} from "@/models/agentRunClient";
import {
  createAgentRunMaterializer,
  type RunMaterializer,
} from "@/models/agentRunMaterializer";
import {
  cancelAgentRun,
  createAgentSession,
  getAgentSession,
  listAgentSessions,
  retryAgentRun,
  startAgentRun,
  type AgentFetch,
} from "@/models/agentTransport";

type ApiAgentModelOptions = {
  baseUrl?: string;
  fetchImpl?: AgentFetch;
};

function modelError(
  cause: unknown,
  code: AgentError["code"],
  message: string,
): AgentError {
  return (
    embeddedAgentError(cause) ?? {
      code,
      kind: "network",
      message,
      retryable: true,
      cause,
    }
  );
}

function materializerFromHandle(handle: AgentRunHandle): RunMaterializer {
  return createAgentRunMaterializer({
    sessionId: handle.sessionId,
    runId: handle.runId,
    userMessageId: handle.userMessageId,
    assistantMessageId: handle.assistantMessageId,
    assistantPartId: handle.assistantPartId,
  });
}

function materializerFromSnapshot(
  runId: string,
  snapshot: AgentMessageSnapshot,
): RunMaterializer {
  const textPart = snapshot.parts.find((part) => part.type === "text");
  return createAgentRunMaterializer({
    sessionId: snapshot.message.sessionId,
    runId,
    userMessageId: snapshot.message.userMessageId ?? "",
    assistantMessageId: snapshot.message.id,
    assistantPartId: textPart?.id ?? "",
    initialSnapshot: snapshot,
  });
}

async function consumeStartedRun(
  input: Omit<ConsumeAgentRunInput, "assistantMessageId" | "materializer">,
  handle: AgentRunHandle,
): Promise<Result<AgentSendResult>> {
  return consumeAgentRun({
    ...input,
    assistantMessageId: handle.assistantMessageId,
    materializer: materializerFromHandle(handle),
  });
}

function activeAssistantSnapshot(
  detail: AgentSessionDetail,
  runId: string,
): AgentMessageSnapshot | null {
  const message =
    (detail.activeRun?.assistantMessageId
      ? detail.messages.find(
          (candidate) => candidate.id === detail.activeRun?.assistantMessageId,
        )
      : undefined) ??
    detail.messages.find(
      (candidate) =>
        candidate.role === "assistant" &&
        candidate.activeRunId === runId,
    );
  if (!message) {
    return null;
  }
  return {
    message,
    parts: detail.messageParts.filter((part) => part.messageId === message.id),
  };
}

export function createApiAgentModel(
  options: ApiAgentModelOptions = {},
): AgentModel {
  const baseUrl = options.baseUrl ?? "/api/v1";
  const fetchImpl = options.fetchImpl ?? fetch;

  return {
    async listSessions() {
      try {
        return [await listAgentSessions(baseUrl, fetchImpl), null];
      } catch (cause) {
        return [
          null,
          modelError(cause, "NETWORK_ERROR", "加载会话失败"),
        ];
      }
    },

    async getSession(sessionId) {
      try {
        return [
          await getAgentSession(baseUrl, sessionId, fetchImpl),
          null,
        ];
      } catch (cause) {
        return [
          null,
          modelError(cause, "NETWORK_ERROR", "加载会话详情失败"),
        ];
      }
    },

    async createSession(input) {
      try {
        return [
          await createAgentSession(
            baseUrl,
            input.clientRequestId,
            fetchImpl,
          ),
          null,
        ];
      } catch (cause) {
        return [
          null,
          modelError(cause, "NETWORK_ERROR", "创建会话失败"),
        ];
      }
    },

    async sendMessage(input, handlers: AgentSendHandlers) {
      try {
        const handle = await startAgentRun({
          baseUrl,
          sessionId: input.sessionId,
          clientRequestId: input.clientRequestId,
          message: input.message,
          fetchImpl,
        });
        handlers.onStarted({
          assistantMessageId: handle.assistantMessageId,
          runId: handle.runId,
          status: handle.status,
        });
        const materializer = materializerFromHandle(handle);
        handlers.onSnapshot(
          materializer.start(handle, new Date().toISOString()),
        );
        return consumeStartedRun(
          {
            baseUrl,
            fetchImpl,
            sessionId: input.sessionId,
            runId: handle.runId,
            afterSequence: 0,
            signal: input.signal,
            handlers,
          },
          handle,
        );
      } catch (cause) {
        return [
          null,
          modelError(cause, "AGENT_RUN_START_FAILED", "启动 Agent 运行失败"),
        ];
      }
    },

    async retryMessage(input, handlers: AgentSendHandlers) {
      try {
        const handle = await retryAgentRun({
          baseUrl,
          sessionId: input.sessionId,
          assistantMessageId: input.assistantMessageId,
          clientRequestId: input.clientRequestId,
          fetchImpl,
        });
        handlers.onStarted({
          assistantMessageId: handle.assistantMessageId,
          runId: handle.runId,
          status: handle.status,
        });
        const materializer = materializerFromHandle(handle);
        handlers.onSnapshot(
          materializer.start(handle, new Date().toISOString()),
        );
        return consumeStartedRun(
          {
            baseUrl,
            fetchImpl,
            sessionId: input.sessionId,
            runId: handle.runId,
            afterSequence: 0,
            signal: input.signal,
            handlers,
          },
          handle,
        );
      } catch (cause) {
        return [
          null,
          modelError(cause, "AGENT_RUN_START_FAILED", "重试 Agent 运行失败"),
        ];
      }
    },

    async resumeRun(input, handlers: AgentSendHandlers) {
      try {
        const detail = await getAgentSession(
          baseUrl,
          input.sessionId,
          fetchImpl,
        );
        const snapshot = activeAssistantSnapshot(detail, input.runId);
        if (!snapshot) {
          return [
            null,
            {
              code: "AGENT_RUN_NOT_FOUND",
              kind: "not_found",
              message: "找不到正在恢复的 Agent 回复",
              retryable: false,
            },
          ];
        }
        handlers.onSnapshot(snapshot);
        handlers.onStarted({
          assistantMessageId: snapshot.message.id,
          runId: input.runId,
          status: "running",
        });
        return consumeAgentRun({
          baseUrl,
          fetchImpl,
          sessionId: input.sessionId,
          runId: input.runId,
          assistantMessageId: snapshot.message.id,
          afterSequence: input.afterSequence,
          signal: input.signal,
          materializer: materializerFromSnapshot(input.runId, snapshot),
          handlers,
        });
      } catch (cause) {
        return [
          null,
          modelError(cause, "AGENT_RUN_INTERRUPTED", "恢复 Agent 运行失败"),
        ];
      }
    },

    async cancelRun(input) {
      try {
        return [
          await cancelAgentRun({
            baseUrl,
            sessionId: input.sessionId,
            runId: input.runId,
            fetchImpl,
          }),
          null,
        ];
      } catch (cause) {
        return [
          null,
          modelError(cause, "NETWORK_ERROR", "停止 Agent 运行失败"),
        ];
      }
    },
  };
}
