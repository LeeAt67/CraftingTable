import { z } from "zod";
import {
  agentErrorSchema,
  type AgentError,
  type Result,
} from "@/models/agentContracts";

const startPartSchema = z.object({
  type: z.literal("start"),
  messageId: z.string().min(1),
  messageMetadata: z
    .object({
      sessionId: z.string().min(1),
      runId: z.string().min(1),
      userMessageId: z.string().min(1),
      assistantMessageId: z.string().min(1).optional(),
    })
    .passthrough(),
});

const startStepPartSchema = z.object({ type: z.literal("start-step") });
const reasoningStartPartSchema = z.object({
  type: z.literal("reasoning-start"),
  id: z.string().min(1),
});
const reasoningDeltaPartSchema = z.object({
  type: z.literal("reasoning-delta"),
  id: z.string().min(1),
  delta: z.string(),
});
const reasoningEndPartSchema = z.object({
  type: z.literal("reasoning-end"),
  id: z.string().min(1),
});
const textStartPartSchema = z.object({
  type: z.literal("text-start"),
  id: z.string().min(1),
});
const textDeltaPartSchema = z.object({
  type: z.literal("text-delta"),
  id: z.string().min(1),
  delta: z.string(),
});
const textEndPartSchema = z.object({
  type: z.literal("text-end"),
  id: z.string().min(1),
});
const finishStepPartSchema = z.object({ type: z.literal("finish-step") });
const finishPartSchema = z.object({
  type: z.literal("finish"),
  finishReason: z.string().optional(),
  messageMetadata: z
    .object({
      runId: z.string().min(1),
      status: z.string().min(1),
    })
    .passthrough(),
});
const errorPartSchema = z.object({
  type: z.literal("error"),
  errorText: z.string(),
  agentError: agentErrorSchema.optional(),
});

const streamPartSchema = z.discriminatedUnion("type", [
  startPartSchema,
  startStepPartSchema,
  reasoningStartPartSchema,
  reasoningDeltaPartSchema,
  reasoningEndPartSchema,
  textStartPartSchema,
  textDeltaPartSchema,
  textEndPartSchema,
  finishStepPartSchema,
  finishPartSchema,
  errorPartSchema,
]);

const knownPartTypes = new Set([
  "start",
  "start-step",
  "reasoning-start",
  "reasoning-delta",
  "reasoning-end",
  "text-start",
  "text-delta",
  "text-end",
  "finish-step",
  "finish",
  "error",
]);

export type AgentStreamPart = z.infer<typeof streamPartSchema>;

export type AgentStreamEvent =
  | { type: "reasoning-start"; id: string; cursor: number }
  | { type: "reasoning-delta"; id: string; delta: string; cursor: number }
  | { type: "text-delta"; id: string; delta: string; cursor: number }
  | {
      type: "finish";
      finishReason?: string;
      runId?: string;
      status?: string;
      cursor: number;
    }
  | { type: "error"; error: AgentError; cursor: number };

export type ParsedAgentStreamPart =
  | { kind: "known"; part: AgentStreamPart }
  | { kind: "unknown" };

export const invalidProtocolError: AgentError = {
  code: "PROTOCOL_INVALID_METADATA",
  kind: "protocol",
  message: "模型服务返回了无效事件",
  retryable: false,
};

export const unexpectedEndError: AgentError = {
  code: "PROTOCOL_UNEXPECTED_EOF",
  kind: "protocol",
  message: "模型响应意外中断",
  retryable: true,
};

function hasPartType(value: unknown): value is { type: string } {
  return (
    !!value &&
    typeof value === "object" &&
    "type" in value &&
    typeof value.type === "string"
  );
}

export function parseAgentStreamPart(
  value: unknown,
): Result<ParsedAgentStreamPart> {
  const parsed = streamPartSchema.safeParse(value);
  if (!parsed.success) {
    if (hasPartType(value) && !knownPartTypes.has(value.type)) {
      return [{ kind: "unknown" }, null];
    }
    return [null, invalidProtocolError];
  }
  return [{ kind: "known", part: parsed.data }, null];
}
