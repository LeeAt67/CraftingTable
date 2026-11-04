import { z } from "zod";

export const agentErrorCodes = [
  "VALIDATION_ERROR",
  "NETWORK_ERROR",
  "REQUEST_CANCELLED",
  "AGENT_SESSION_NOT_FOUND",
  "AGENT_SESSION_BUSY",
  "AGENT_REQUEST_ALREADY_PROCESSED",
  "AGENT_REQUEST_ID_CONFLICT",
  "AGENT_RUN_NOT_FOUND",
  "AGENT_MESSAGE_NOT_RETRYABLE",
  "AGENT_NOT_CONFIGURED",
  "AGENT_PROVIDER_FAILED",
  "AGENT_PROVIDER_TIMEOUT",
  "AGENT_STEP_LIMIT_EXCEEDED",
  "AGENT_INTERNAL_FAILED",
  "PROTOCOL_INVALID_METADATA",
  "PROTOCOL_UNEXPECTED_EOF",
  "INTERNAL_ERROR",
] as const;

export const agentErrorKinds = [
  "network",
  "http",
  "validation",
  "not_found",
  "protocol",
  "auth",
  "conflict",
  "cancelled",
  "server",
  "unknown",
] as const;

export type AgentErrorCode = (typeof agentErrorCodes)[number];
export type AgentErrorKind = (typeof agentErrorKinds)[number];

export type AgentError = {
  code: AgentErrorCode;
  kind: AgentErrorKind;
  message: string;
  retryable: boolean;
  requestId?: string;
  details?: Record<string, unknown>;
  cause?: unknown;
};

export const agentErrorSchema = z.object({
  code: z.enum(agentErrorCodes),
  kind: z.enum(agentErrorKinds),
  message: z.string(),
  retryable: z.boolean(),
  requestId: z.string().optional(),
  details: z.record(z.unknown()).optional(),
  cause: z.unknown().optional(),
});

export function embeddedAgentError(cause: unknown): AgentError | null {
  if (!cause || typeof cause !== "object" || !("agentError" in cause)) {
    return null;
  }
  const parsed = agentErrorSchema.safeParse(
    (cause as { agentError?: unknown }).agentError,
  );
  return parsed.success ? parsed.data : null;
}

export type Result<T, E extends AgentError = AgentError> =
  | readonly [data: T, error: null]
  | readonly [data: null, error: E];

export type AgentSession = {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
};

export type AgentMessageRole = "user" | "assistant";

export type AgentMessageStatus =
  | "streaming"
  | "complete"
  | "cancelled"
  | "failed"
  | "interrupted";

export type AgentMessage = {
  id: string;
  sessionId: string;
  seq: number;
  role: AgentMessageRole;
  status: AgentMessageStatus;
  activeRunId?: string;
  userMessageId?: string;
  createdAt: string;
  updatedAt: string;
};

export type AgentMessagePartStatus =
  | "streaming"
  | "complete"
  | "cancelled"
  | "failed"
  | "interrupted";

export type AgentTextPartContent = {
  type: "text";
  text: string;
};

export type AgentMessagePart = {
  id: string;
  messageId: string;
  runId?: string;
  index: number;
  type: "text";
  status: AgentMessagePartStatus;
  content: AgentTextPartContent;
  createdAt: string;
  updatedAt: string;
};

export type AgentRunStatus =
  | "running"
  | "completed"
  | "failed"
  | "cancelled"
  | "interrupted";

export type CraftingTableSendMessage = {
  id: string;
  role: "user";
  parts: Array<{
    type: "text";
    text: string;
  }>;
};

export type AgentSendInput = {
  sessionId: string;
  clientRequestId: string;
  message: CraftingTableSendMessage;
  signal: AbortSignal;
};

export type AgentRetryInput = {
  sessionId: string;
  assistantMessageId: string;
  clientRequestId: string;
  signal: AbortSignal;
};

export type AgentSendResult = {
  assistantMessageId: string;
  runId: string;
  status: AgentRunStatus;
};

export type AgentMessageSnapshot = {
  message: AgentMessage;
  parts: AgentMessagePart[];
};

export type AgentSendHandlers = {
  onStarted: (started: AgentSendResult) => void;
  onSnapshot: (snapshot: AgentMessageSnapshot) => void;
};

export type AgentSessionDetail = {
  session: AgentSession;
  messages: AgentMessage[];
  messageParts: AgentMessagePart[];
  activeRun: null | {
    runId: string;
    status: AgentRunStatus;
  };
  nextMessageCursor: string | null;
};
