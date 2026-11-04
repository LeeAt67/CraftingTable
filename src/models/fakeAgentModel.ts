import { embeddedAgentError } from "@/models/agentContracts";
import type {
  AgentError,
  AgentMessage,
  AgentMessagePart,
  AgentSendHandlers,
  AgentSendResult,
  AgentRunStatus,
  AgentSendInput,
  AgentSession,
  AgentSessionDetail,
  Result,
} from "@/models/agentContracts";
import type { AgentModel } from "@/models/agentModel";
import { buildAgentContext } from "@/models/agentContext";

type FakeAgentModelOptions = {
  chunkDelayMs?: number;
  responseChunks?: (
    input: AgentSendInput,
    history: Array<{ role: "user" | "assistant"; content: string }>,
    signal: AbortSignal,
  ) => AsyncIterable<string>;
};

type FakeRun = {
  runId: string;
  status: AgentRunStatus;
  sessionId: string;
  abortController: AbortController;
};

type FakeStore = {
  sessions: Map<string, AgentSession>;
  messagesBySessionId: Map<string, AgentMessage[]>;
  partsByMessageId: Map<string, AgentMessagePart[]>;
  sessionsByRequestId: Map<string, string>;
  activeRunsBySessionId: Map<string, FakeRun>;
};

const DEFAULT_CHUNK_DELAY_MS = 12;
const TITLE_LENGTH = 32;

function createId() {
  return globalThis.crypto.randomUUID();
}

function now() {
  return new Date().toISOString();
}

function failure(
  code: AgentError["code"],
  kind: AgentError["kind"],
  message: string,
  retryable = false,
): Result<never> {
  return [
    null,
    {
      code,
      kind,
      message,
      retryable,
    },
  ];
}

function sessionNotFound(): Result<never> {
  return failure(
    "AGENT_SESSION_NOT_FOUND",
    "not_found",
    "会话不存在或已被删除",
  );
}

function textOf(input: AgentSendInput) {
  return input.message.parts
    .filter((part) => part.type === "text")
    .map((part) => part.text)
    .join("");
}

function titleFrom(text: string) {
  const normalized = text.trim();
  return Array.from(normalized).slice(0, TITLE_LENGTH).join("") || "新会话";
}

function cloneMessage(message: AgentMessage): AgentMessage {
  return { ...message };
}

function clonePart(part: AgentMessagePart): AgentMessagePart {
  return {
    ...part,
    content: { ...part.content },
  };
}

function isRetryableMessage(message: AgentMessage) {
  return (
    message.role === "assistant" &&
    (message.status === "failed" ||
      message.status === "cancelled" ||
      message.status === "interrupted")
  );
}

function wait(delayMs: number, signal: AbortSignal) {
  if (delayMs === 0) {
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

async function* defaultResponseChunks(
  userText: string,
  signal: AbortSignal,
  delayMs: number,
) {
  const chunks = [
    "收到你的请求。",
    `\n\n我会围绕「${userText.trim()}」`,
    "整理重点，并给出下一步行动。",
  ];
  for (const chunk of chunks) {
    await wait(delayMs, signal);
    if (signal.aborted) {
      return;
    }
    yield chunk;
  }
}

export function createFakeAgentModel(
  options: FakeAgentModelOptions = {},
): AgentModel {
  const chunkDelayMs = options.chunkDelayMs ?? DEFAULT_CHUNK_DELAY_MS;
  const store: FakeStore = {
    sessions: new Map(),
    messagesBySessionId: new Map(),
    partsByMessageId: new Map(),
    sessionsByRequestId: new Map(),
    activeRunsBySessionId: new Map(),
  };

  const runAssistant = async ({
    session,
    assistantMessage,
    requestInput,
    history,
    userText,
    signal,
    handlers,
  }: {
    session: AgentSession;
    assistantMessage: AgentMessage;
    requestInput: AgentSendInput;
    history: Array<{ role: "user" | "assistant"; content: string }>;
    userText: string;
    signal: AbortSignal;
    handlers: AgentSendHandlers;
  }): Promise<Result<AgentSendResult>> => {
    const timestamp = now();
    const runId = createId();
    const assistantPart: AgentMessagePart = {
      id: createId(),
      messageId: assistantMessage.id,
      runId,
      index: 0,
      type: "text",
      status: "streaming",
      content: { type: "text", text: "" },
      createdAt: timestamp,
      updatedAt: timestamp,
    };
    assistantMessage.status = "streaming";
    assistantMessage.activeRunId = runId;
    assistantMessage.updatedAt = timestamp;
    store.partsByMessageId.set(assistantMessage.id, [
      ...(store.partsByMessageId.get(assistantMessage.id) ?? []),
      assistantPart,
    ]);
    session.updatedAt = timestamp;

    const run: FakeRun = {
      runId,
      status: "running",
      sessionId: session.id,
      abortController: new AbortController(),
    };
    store.activeRunsBySessionId.set(session.id, run);

    const abort = () => run.abortController.abort();
    signal.addEventListener("abort", abort, { once: true });
    if (signal.aborted) {
      run.abortController.abort();
    }

    handlers.onStarted({
      assistantMessageId: assistantMessage.id,
      runId,
      status: "running",
    });

    let accumulated = "";
    let streamFailure: unknown = null;

    try {
      const chunks = options.responseChunks
        ? options.responseChunks(
            requestInput,
            history,
            run.abortController.signal,
          )
        : defaultResponseChunks(
            userText,
            run.abortController.signal,
            chunkDelayMs,
          );
      for await (const chunk of chunks) {
        if (run.abortController.signal.aborted) {
          break;
        }
        accumulated += chunk;
        assistantPart.content.text = accumulated;
        assistantPart.updatedAt = now();
        assistantMessage.updatedAt = assistantPart.updatedAt;
        handlers.onSnapshot({
          message: cloneMessage(assistantMessage),
          parts: [clonePart(assistantPart)],
        });
      }
    } catch (cause) {
      streamFailure = cause;
    }

    const terminalStatus: AgentRunStatus = run.abortController.signal.aborted
      ? "cancelled"
      : streamFailure
        ? "failed"
        : "completed";
    const terminalMessageStatus =
      terminalStatus === "cancelled"
        ? "cancelled"
        : terminalStatus === "failed"
          ? "failed"
          : "complete";
    const terminalPartStatus =
      terminalStatus === "cancelled"
        ? "cancelled"
        : terminalStatus === "failed"
          ? "failed"
          : "complete";
    const finishedAt = now();
    assistantMessage.status = terminalMessageStatus;
    assistantMessage.updatedAt = finishedAt;
    assistantPart.status = terminalPartStatus;
    assistantPart.updatedAt = finishedAt;
    run.status = terminalStatus;
    session.updatedAt = finishedAt;
    store.activeRunsBySessionId.delete(session.id);
    signal.removeEventListener("abort", abort);
    handlers.onSnapshot({
      message: cloneMessage(assistantMessage),
      parts: [clonePart(assistantPart)],
    });

    if (streamFailure && terminalStatus !== "cancelled") {
      const agentError = embeddedAgentError(streamFailure);
      if (agentError) {
        return [null, agentError];
      }
      return [
        null,
        {
          code: "AGENT_PROVIDER_FAILED",
          kind: "server",
          message:
            streamFailure instanceof Error
              ? streamFailure.message
              : "模型响应失败",
          retryable: true,
        },
      ];
    }
    return [
      {
        assistantMessageId: assistantMessage.id,
        runId,
        status: terminalStatus,
      },
      null,
    ];
  };

  const model: AgentModel = {
    async listSessions() {
      const sessions = [...store.sessions.values()].sort((left, right) =>
        right.updatedAt.localeCompare(left.updatedAt),
      );
      return [sessions.map((session) => ({ ...session })), null];
    },

    async getSession(sessionId) {
      const session = store.sessions.get(sessionId);
      if (!session) {
        return sessionNotFound();
      }

      const messages = (store.messagesBySessionId.get(sessionId) ?? []).map(
        cloneMessage,
      );
      const messageParts = messages.flatMap((message) =>
        (store.partsByMessageId.get(message.id) ?? [])
          .filter(
            (part) =>
              !message.activeRunId ||
              !part.runId ||
              part.runId === message.activeRunId,
          )
          .map(clonePart),
      );
      const run = store.activeRunsBySessionId.get(sessionId);
      const detail: AgentSessionDetail = {
        session: { ...session },
        messages,
        messageParts,
        activeRun: run ? { runId: run.runId, status: run.status } : null,
        nextMessageCursor: null,
      };
      return [detail, null];
    },

    async createSession(input) {
      const existingId = store.sessionsByRequestId.get(input.clientRequestId);
      if (existingId) {
        return [{ session: { ...store.sessions.get(existingId)! } }, null];
      }

      const timestamp = now();
      const session: AgentSession = {
        id: createId(),
        title: "新会话",
        createdAt: timestamp,
        updatedAt: timestamp,
      };
      store.sessions.set(session.id, session);
      store.messagesBySessionId.set(session.id, []);
      store.sessionsByRequestId.set(input.clientRequestId, session.id);
      return [{ session: { ...session } }, null];
    },

    async sendMessage(input, handlers) {
      const session = store.sessions.get(input.sessionId);
      if (!session) {
        return sessionNotFound();
      }
      if (store.activeRunsBySessionId.has(input.sessionId)) {
        return failure(
          "AGENT_SESSION_BUSY",
          "conflict",
          "当前会话仍在生成回复",
        );
      }

      const timestamp = now();
      const messages = store.messagesBySessionId.get(input.sessionId) ?? [];
      const userText = textOf(input);
      const userMessage: AgentMessage = {
        id: input.message.id,
        sessionId: input.sessionId,
        seq: messages.length + 1,
        role: "user",
        status: "complete",
        createdAt: timestamp,
        updatedAt: timestamp,
      };
      const userParts = input.message.parts.map(
        (part, index): AgentMessagePart => ({
          id: createId(),
          messageId: userMessage.id,
          index,
          type: "text",
          status: "complete",
          content: { type: "text", text: part.text },
          createdAt: timestamp,
          updatedAt: timestamp,
        }),
      );
      messages.push(userMessage);
      store.messagesBySessionId.set(input.sessionId, messages);
      store.partsByMessageId.set(userMessage.id, userParts);

      session.title = titleFrom(userText);
      session.updatedAt = timestamp;

      const history = buildAgentContext(
        messages,
        Object.fromEntries(store.partsByMessageId),
      );
      const assistantMessage: AgentMessage = {
        id: createId(),
        sessionId: input.sessionId,
        seq: messages.length + 1,
        role: "assistant",
        status: "streaming",
        userMessageId: userMessage.id,
        createdAt: timestamp,
        updatedAt: timestamp,
      };
      messages.push(assistantMessage);
      return runAssistant({
        session,
        assistantMessage,
        requestInput: input,
        history,
        userText,
        signal: input.signal,
        handlers,
      });
    },

    async retryMessage(input, handlers) {
      const session = store.sessions.get(input.sessionId);
      if (!session) {
        return sessionNotFound();
      }
      if (store.activeRunsBySessionId.has(input.sessionId)) {
        return failure(
          "AGENT_SESSION_BUSY",
          "conflict",
          "当前会话仍在生成回复",
        );
      }

      const messages = store.messagesBySessionId.get(input.sessionId) ?? [];
      const assistantMessage = messages.find(
        (message) => message.id === input.assistantMessageId,
      );
      if (!assistantMessage || !isRetryableMessage(assistantMessage)) {
        return failure(
          "AGENT_MESSAGE_NOT_RETRYABLE",
          "conflict",
          "该回复当前不能重试",
        );
      }

      const userMessage = messages.find(
        (message) =>
          message.id === assistantMessage.userMessageId &&
          message.role === "user",
      );
      if (!userMessage) {
        return failure(
          "AGENT_MESSAGE_NOT_RETRYABLE",
          "conflict",
          "找不到这条回复对应的用户消息",
        );
      }

      const userText = (
        store.partsByMessageId.get(userMessage.id) ?? []
      )
        .map((part) => part.content.text)
        .join("");
      const retryHistory = buildAgentContext(
        messages.filter((message) => message.id !== assistantMessage.id),
        Object.fromEntries(store.partsByMessageId),
      );

      return runAssistant({
        session,
        assistantMessage,
        requestInput: {
          sessionId: input.sessionId,
          clientRequestId: input.clientRequestId,
          message: {
            id: userMessage.id,
            role: "user",
            parts: [{ type: "text", text: userText }],
          },
          signal: input.signal,
        },
        history: retryHistory,
        userText,
        signal: input.signal,
        handlers,
      });
    },

    async cancelRun(input) {
      const run = store.activeRunsBySessionId.get(input.sessionId);
      if (!run || run.runId !== input.runId) {
        return failure("AGENT_RUN_NOT_FOUND", "not_found", "运行不存在或已结束");
      }
      run.status = "cancelled";
      run.abortController.abort();
      return [{ runId: run.runId, status: run.status }, null];
    },
  };

  return model;
}
