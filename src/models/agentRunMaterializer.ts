import type {
  AgentError,
  AgentMessage,
  AgentMessagePart,
  AgentMessageSnapshot,
  AgentMessageStatus,
  AgentReasoningPart,
  AgentRunHandle,
  AgentRunStatus,
} from "@/models/agentContracts";

export type RunMaterializer = {
  start: (handle: AgentRunHandle, timestamp: string) => AgentMessageSnapshot;
  reasoningStart: () => AgentMessageSnapshot;
  reasoningDelta: (text: string, cursor: number) => AgentMessageSnapshot;
  delta: (text: string, cursor: number) => AgentMessageSnapshot;
  finish: (status: AgentRunStatus, timestamp: string) => AgentMessageSnapshot;
  error: (error: AgentError, timestamp: string) => AgentMessageSnapshot;
};

type MaterializerInput = {
  sessionId: string;
  runId: string;
  userMessageId: string;
  assistantMessageId: string;
  assistantPartId: string;
  reasoningPartId?: string;
  initialSnapshot?: AgentMessageSnapshot;
};

function messageStatus(status: AgentRunStatus): AgentMessageStatus {
  if (status === "completed") {
    return "complete";
  }
  if (status === "running") {
    return "streaming";
  }
  return status;
}

function clonePart(part: AgentMessagePart): AgentMessagePart {
  if (part.type === "error") {
    return {
      ...part,
      content: {
        ...part.content,
        error: { ...part.content.error },
      },
    };
  }
  if (part.type === "reasoning") {
    return {
      ...part,
      content: { type: "reasoning", text: part.content.text },
    };
  }
  return {
    ...part,
    content: { type: "text", text: part.content.text },
  };
}

function cloneSnapshot(snapshot: AgentMessageSnapshot): AgentMessageSnapshot {
  return {
    message: { ...snapshot.message },
    parts: snapshot.parts.map(clonePart),
  };
}

export function createAgentRunMaterializer(
  input: MaterializerInput,
): RunMaterializer {
  const timestamp = new Date().toISOString();
  let message: AgentMessage = input.initialSnapshot
    ? { ...input.initialSnapshot.message }
    : {
        id: input.assistantMessageId,
        sessionId: input.sessionId,
        seq: 0,
        role: "assistant",
        status: "streaming",
        activeRunId: input.runId,
        userMessageId: input.userMessageId,
        createdAt: timestamp,
        updatedAt: timestamp,
      };
  const initialParts = input.initialSnapshot?.parts
    .filter((part) => !part.runId || part.runId === input.runId)
    .map(clonePart);
  let reasoningPart = initialParts?.find(
    (part): part is AgentReasoningPart => part.type === "reasoning",
  );
  let textPart = initialParts?.find(
    (part): part is Extract<AgentMessagePart, { type: "text" }> =>
      part.type === "text",
  );
  let errorPart = initialParts?.find(
    (part): part is Extract<AgentMessagePart, { type: "error" }> =>
      part.type === "error",
  );

  if (!textPart) {
    textPart = {
      id: input.assistantPartId,
      messageId: input.assistantMessageId,
      runId: input.runId,
      index: reasoningPart || input.reasoningPartId ? 1 : 0,
      type: "text",
      status: "streaming",
      content: { type: "text", text: "" },
      createdAt: timestamp,
      updatedAt: timestamp,
    };
  }

  const snapshot = (): AgentMessageSnapshot => ({
    message: { ...message },
    parts: [
      ...(reasoningPart ? [clonePart(reasoningPart)] : []),
      clonePart(textPart!),
      ...(errorPart ? [clonePart(errorPart)] : []),
    ],
  });

  const setTerminalStatus = (
    status: AgentRunStatus,
    updatedAt: string,
  ) => {
    const terminalStatus = messageStatus(status);
    message = {
      ...message,
      status: terminalStatus,
      updatedAt,
    };
    textPart = { ...textPart!, status: terminalStatus, updatedAt };
    if (reasoningPart) {
      reasoningPart = { ...reasoningPart, status: terminalStatus, updatedAt };
    }
    if (errorPart) {
      errorPart = {
        ...errorPart,
        status: terminalStatus,
        updatedAt,
      };
    }
  };

  return {
    start(handle, startedAt) {
      message = {
        id: handle.assistantMessageId,
        sessionId: handle.sessionId,
        seq: message.seq,
        role: "assistant",
        status: "streaming",
        activeRunId: handle.runId,
        userMessageId: handle.userMessageId,
        createdAt: message.createdAt,
        updatedAt: startedAt,
      };
      reasoningPart = undefined;
      textPart = {
        id: handle.assistantPartId,
        messageId: handle.assistantMessageId,
        runId: handle.runId,
        index: input.reasoningPartId ? 1 : 0,
        type: "text",
        status: "streaming",
        content: { type: "text", text: "" },
        createdAt: startedAt,
        updatedAt: startedAt,
      };
      errorPart = undefined;
      return snapshot();
    },

    reasoningStart() {
      if (reasoningPart || !input.reasoningPartId) {
        return snapshot();
      }
      const updatedAt = new Date().toISOString();
      reasoningPart = {
        id: input.reasoningPartId,
        messageId: input.assistantMessageId,
        runId: input.runId,
        index: 0,
        type: "reasoning",
        status: "streaming",
        content: { type: "reasoning", text: "" },
        createdAt: updatedAt,
        updatedAt,
      };
      textPart = { ...textPart!, index: 1, updatedAt };
      message = { ...message, status: "streaming", updatedAt };
      return snapshot();
    },

    reasoningDelta(text, _cursor) {
      if (!reasoningPart) {
        return snapshot();
      }
      const updatedAt = new Date().toISOString();
      reasoningPart = {
        ...reasoningPart,
        content: {
          type: "reasoning",
          text: reasoningPart.content.text + text,
        },
        updatedAt,
      };
      message = { ...message, status: "streaming", updatedAt };
      return snapshot();
    },

    delta(text, _cursor) {
      const updatedAt = new Date().toISOString();
      textPart = {
        ...textPart!,
        content: {
          type: "text",
          text: textPart!.content.text + text,
        },
        updatedAt,
      };
      message = {
        ...message,
        status: "streaming",
        updatedAt,
      };
      return snapshot();
    },

    finish(status, finishedAt) {
      setTerminalStatus(status, finishedAt);
      return snapshot();
    },

    error(agentError, failedAt) {
      setTerminalStatus("failed", failedAt);
      errorPart = {
        id: `${input.assistantMessageId}:error:${runIndex(errorPart)}`,
        messageId: input.assistantMessageId,
        runId: input.runId,
        index: (textPart?.index ?? 0) + 1,
        type: "error",
        status: "failed",
        content: {
          type: "error",
          error: { ...agentError },
        },
        createdAt: failedAt,
        updatedAt: failedAt,
      };
      return snapshot();
    },
  };
}

function runIndex(errorPart?: AgentMessagePart) {
  return errorPart ? errorPart.index + 1 : 0;
}
