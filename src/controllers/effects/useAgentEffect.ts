import { useEffect, useMemo, useRef } from "react";
import { toast } from "sonner";
import { useAgentStore } from "@/controllers/stores/agentStore";
import type {
  AgentError,
  AgentMessage,
  AgentMessageStatus,
  AgentMessagePart,
  AgentSendResult,
  AgentSession,
  Result,
} from "@/models/agentContracts";
import { agentModel as defaultAgentModel } from "@/models/agentModel";
import type { AgentModel } from "@/models/agentModel";

type Notifier = {
  success: (message: string) => void;
};

const defaultNotifier: Notifier = {
  success: (message) => toast.success(message),
};

function createId() {
  return globalThis.crypto.randomUUID();
}

function now() {
  return new Date().toISOString();
}

function internalError(message: string, cause?: unknown): AgentError {
  return {
    code: "INTERNAL_ERROR",
    kind: "unknown",
    message,
    retryable: true,
    cause,
  };
}

function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === "AbortError";
}

function messageStatusFromRun(
  status: AgentSendResult["status"],
): AgentMessageStatus {
  switch (status) {
    case "running":
      return "streaming";
    case "completed":
      return "complete";
    default:
      return status;
  }
}

export function createAgentEffect(
  model: AgentModel,
  notifier: Notifier = defaultNotifier,
) {
  const abortControllers = new Map<string, AbortController>();
  let sessionsLoaded = false;
  let sessionsPromise: Promise<Result<AgentSession[]>> | null = null;
  let loadGeneration = 0;

  const reportFailure = (error: AgentError) => {
    useAgentStore.getState().setError(error);
    useAgentStore.getState().setSendPhase("error");
  };

  const loadSessions = async () => {
    if (sessionsLoaded) {
      return [useAgentStore.getState().sessions, null] as const;
    }
    if (sessionsPromise) {
      return sessionsPromise;
    }

    sessionsPromise = model.listSessions();
    try {
      const [sessions, error] = await sessionsPromise;
      if (error) {
        reportFailure(error);
        return [null, error] as const;
      }
      useAgentStore.getState().setSessions(sessions);
      sessionsLoaded = true;
      return [sessions, null] as const;
    } catch (cause) {
      const error = internalError("加载会话失败", cause);
      reportFailure(error);
      return [null, error] as const;
    } finally {
      sessionsPromise = null;
    }
  };

  const createSession = async () => {
    try {
      const result = await model.createSession({
        clientRequestId: createId(),
      });
      const [created, error] = result;
      if (error) {
        reportFailure(error);
        return result;
      }
      useAgentStore.getState().upsertSession(created.session);
      useAgentStore.getState().setActiveSession(created.session.id);
      useAgentStore.getState().setSendPhase("idle");
      useAgentStore.getState().setError(null);
      notifier.success("已创建会话");
      return result;
    } catch (cause) {
      const error = internalError("创建会话失败", cause);
      reportFailure(error);
      return [null, error] as const;
    }
  };

  const loadSession = async (sessionId: string) => {
    const generation = ++loadGeneration;
    try {
      const result = await model.getSession(sessionId);
      if (generation !== loadGeneration) {
        return result;
      }
      const [detail, error] = result;
      if (error) {
        reportFailure(error);
        return result;
      }
      const state = useAgentStore.getState();
      state.upsertSession(detail.session);
      state.setSessionMessages(
        detail.session.id,
        detail.messages,
        detail.messageParts,
      );
      state.setActiveSession(detail.session.id);
      state.setError(null);
      if (detail.activeRun?.status === "running") {
        state.setSendPhase("streaming");
        state.setLiveRun({
          sessionId: detail.session.id,
          runId: detail.activeRun.runId,
        });
      } else if (state.liveRun?.sessionId === detail.session.id) {
        state.setSendPhase("idle");
        state.setLiveRun(null);
      }
      return result;
    } catch (cause) {
      if (generation !== loadGeneration) {
        return [null, internalError("加载会话详情已过期", cause)] as const;
      }
      const error = internalError("加载会话详情失败", cause);
      reportFailure(error);
      return [null, error] as const;
    }
  };

  const executeRun = async (
    sessionId: string,
    request: (
      signal: AbortSignal,
      handlers: {
        onStarted: (started: AgentSendResult) => void;
        onSnapshot: Parameters<AgentModel["sendMessage"]>[1]["onSnapshot"];
      },
    ) => Promise<Result<AgentSendResult>>,
    onStarted: (started: AgentSendResult) => void,
  ) => {
    const abortController = new AbortController();
    let assistantMessageId: string | null = null;

    try {
      const result = await request(abortController.signal, {
        onStarted: (started) => {
          assistantMessageId = started.assistantMessageId;
          abortControllers.set(started.runId, abortController);
          onStarted(started);
          useAgentStore
            .getState()
            .setLiveRun({ sessionId, runId: started.runId });
          useAgentStore.getState().setSendPhase("streaming");
        },
        onSnapshot: (snapshot) => {
          const current = useAgentStore.getState();
          current.upsertMessage(snapshot.message.sessionId, snapshot.message);
          for (const part of snapshot.parts) {
            current.upsertMessagePart(snapshot.message.id, part);
          }
        },
      });
      const [sent, error] = result;
      if (error) {
        if (isAbortError(error.cause)) {
          useAgentStore.getState().setSendPhase("idle");
          return result;
        }
        reportFailure(error);
        return result;
      }

      if (assistantMessageId) {
        const assistant = useAgentStore
          .getState()
          .messagesBySessionId[sessionId]?.find(
            (message) => message.id === assistantMessageId,
          );
        if (assistant) {
          useAgentStore.getState().upsertMessage(sessionId, {
            ...assistant,
            status: messageStatusFromRun(sent.status),
            updatedAt: now(),
          });
        }
      }
      const [detail] = await model.getSession(sessionId);
      if (detail) {
        useAgentStore.getState().upsertSession(detail.session);
      }
      useAgentStore.getState().setSendPhase("idle");
      if (sent.status === "cancelled") {
        notifier.success("已停止生成");
      }
      return result;
    } catch (cause) {
      const error = internalError(
        abortController.signal.aborted ? "已停止生成" : "发送消息失败",
        cause,
      );
      reportFailure(error);
      return [null, error] as const;
    } finally {
      if (assistantMessageId) {
        const messages = useAgentStore.getState().messagesBySessionId[sessionId];
        const assistant = messages?.find(
          (message) => message.id === assistantMessageId,
        );
        if (assistant?.activeRunId) {
          abortControllers.delete(assistant.activeRunId);
        }
      }
      const liveRun = useAgentStore.getState().liveRun;
      if (liveRun?.sessionId === sessionId) {
        useAgentStore.getState().setLiveRun(null);
      }
    }
  };

  const sendMessage = async (rawText: string) => {
    const text = rawText.trim();
    if (!text) {
      return [
        null,
        {
          code: "VALIDATION_ERROR",
          kind: "validation",
          message: "请输入消息内容",
          retryable: false,
        },
      ] as const;
    }

    const sessionId = useAgentStore.getState().activeSessionId;
    if (!sessionId) {
      const error = internalError("请先创建或选择一个会话");
      reportFailure(error);
      return [null, error] as const;
    }
    if (useAgentStore.getState().liveRun) {
      const error: AgentError = {
        code: "AGENT_SESSION_BUSY",
        kind: "conflict",
        message: "当前仍有回复正在生成",
        retryable: false,
      };
      reportFailure(error);
      return [null, error] as const;
    }

    const timestamp = now();
    const userMessage: AgentMessage = {
      id: createId(),
      sessionId,
      seq: (useAgentStore.getState().messagesBySessionId[sessionId]?.length ??
        0) + 1,
      role: "user",
      status: "complete",
      createdAt: timestamp,
      updatedAt: timestamp,
    };
    const userPart: AgentMessagePart = {
      id: createId(),
      messageId: userMessage.id,
      index: 0,
      type: "text",
      status: "complete",
      content: { type: "text", text },
      createdAt: timestamp,
      updatedAt: timestamp,
    };
    const state = useAgentStore.getState();
    state.appendMessage(sessionId, userMessage);
    state.upsertMessagePart(userMessage.id, userPart);
    state.setSendPhase("submitting");
    state.setError(null);

    return executeRun(
      sessionId,
      (signal, handlers) =>
        model.sendMessage(
          {
            sessionId,
            clientRequestId: createId(),
            message: {
              id: userMessage.id,
              role: "user",
              parts: [{ type: "text", text }],
            },
            signal,
          },
          handlers,
        ),
      (started) => {
        const startedAt = now();
        useAgentStore.getState().appendMessage(sessionId, {
          id: started.assistantMessageId,
          sessionId,
          seq: userMessage.seq + 1,
          role: "assistant",
          status: messageStatusFromRun(started.status),
          activeRunId: started.runId,
          userMessageId: userMessage.id,
          createdAt: startedAt,
          updatedAt: startedAt,
        });
      },
    );
  };

  const retryLastMessage = async () => {
    const state = useAgentStore.getState();
    const sessionId = state.activeSessionId;
    if (!sessionId) {
      const error: AgentError = {
        code: "AGENT_MESSAGE_NOT_RETRYABLE",
        kind: "conflict",
        message: "请先选择一个失败或停止的回复",
        retryable: false,
      };
      reportFailure(error);
      return [null, error] as const;
    }
    if (state.liveRun) {
      const error: AgentError = {
        code: "AGENT_SESSION_BUSY",
        kind: "conflict",
        message: "当前仍有回复正在生成",
        retryable: false,
      };
      reportFailure(error);
      return [null, error] as const;
    }

    const target = state.messagesBySessionId[sessionId]?.at(-1);
    if (
      !target ||
      target.role !== "assistant" ||
      (target.status !== "failed" &&
        target.status !== "cancelled" &&
        target.status !== "interrupted")
    ) {
      const error: AgentError = {
        code: "AGENT_MESSAGE_NOT_RETRYABLE",
        kind: "conflict",
        message: "当前没有可以重试的回复",
        retryable: false,
      };
      reportFailure(error);
      return [null, error] as const;
    }

    useAgentStore.getState().setSendPhase("submitting");
    useAgentStore.getState().setError(null);

    return executeRun(
      sessionId,
      (signal, handlers) =>
        model.retryMessage(
          {
            sessionId,
            assistantMessageId: target.id,
            clientRequestId: createId(),
            signal,
          },
          handlers,
        ),
      (started) => {
        const current = useAgentStore
          .getState()
          .messagesBySessionId[sessionId]?.find(
            (message) => message.id === target.id,
          );
        if (current) {
          useAgentStore.getState().upsertMessage(sessionId, {
            ...current,
            status: "streaming",
            activeRunId: started.runId,
            updatedAt: now(),
          });
        }
      },
    );
  };

  const cancelRun = async () => {
    const liveRun = useAgentStore.getState().liveRun;
    if (!liveRun) {
      return [{ runId: "", status: "cancelled" as const }, null] as const;
    }
    try {
      const result = await model.cancelRun(liveRun);
      const [, error] = result;
      abortControllers.get(liveRun.runId)?.abort();
      if (error) {
        reportFailure(error);
        return result;
      }
      return result;
    } catch (cause) {
      abortControllers.get(liveRun.runId)?.abort();
      const error = internalError("停止生成失败", cause);
      reportFailure(error);
      return [null, error] as const;
    }
  };

  return {
    loadSessions,
    createSession,
    loadSession,
    sendMessage,
    retryLastMessage,
    cancelRun,
  };
}

const effectsByModel = new WeakMap<AgentModel, ReturnType<typeof createAgentEffect>>();

function getAgentEffect(model: AgentModel) {
  const existing = effectsByModel.get(model);
  if (existing) {
    return existing;
  }
  const effect = createAgentEffect(model);
  effectsByModel.set(model, effect);
  return effect;
}

export function useAgentEffect(model: AgentModel = defaultAgentModel) {
  const effect = useMemo(() => getAgentEffect(model), [model]);
  const loadedRef = useRef(false);

  useEffect(() => {
    if (loadedRef.current) {
      return;
    }
    loadedRef.current = true;
    void effect.loadSessions();
  }, [effect]);

  return effect;
}
