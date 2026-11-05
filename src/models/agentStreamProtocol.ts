import { EventSourceParserStream } from "eventsource-parser/stream";
import { z } from "zod";
import {
  agentErrorSchema,
  type AgentError,
} from "@/models/agentContracts";
import { agentRequestError } from "@/models/agentTransport";

const textDeltaPartSchema = z.object({
  type: z.literal("text-delta"),
  id: z.string(),
  delta: z.string(),
});

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

const startStepPartSchema = z.object({
  type: z.literal("start-step"),
});

const textStartPartSchema = z.object({
  type: z.literal("text-start"),
  id: z.string().min(1),
});

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

const textEndPartSchema = z.object({
  type: z.literal("text-end"),
  id: z.string().min(1),
});

const finishStepPartSchema = z.object({
  type: z.literal("finish-step"),
});

const streamPartSchema = z.discriminatedUnion("type", [
  startPartSchema,
  startStepPartSchema,
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
  "text-start",
  "text-delta",
  "text-end",
  "finish-step",
  "finish",
  "error",
]);

export type AgentStreamEvent =
  | {
      type: "text-delta";
      id: string;
      delta: string;
    }
  | {
      type: "finish";
      finishReason?: string;
      runId?: string;
      status?: string;
    }
  | {
      type: "error";
      error: AgentError;
    };

const invalidProtocolError: AgentError = {
  code: "PROTOCOL_INVALID_METADATA",
  kind: "protocol",
  message: "模型服务返回了无效事件",
  retryable: false,
};

const unexpectedEndError: AgentError = {
  code: "PROTOCOL_UNEXPECTED_EOF",
  kind: "protocol",
  message: "模型响应意外中断",
  retryable: true,
};

export function agentStreamUnexpectedEnd() {
  return agentRequestError(unexpectedEndError);
}

function hasPartType(value: unknown): value is { type: string } {
  return (
    !!value &&
    typeof value === "object" &&
    "type" in value &&
    typeof value.type === "string"
  );
}

export async function* readAgentStream(
  body: ReadableStream<Uint8Array>,
): AsyncIterable<AgentStreamEvent> {
  let started = false;
  let terminalSeen = false;
  const eventStream = body
    .pipeThrough(
      new TextDecoderStream() as unknown as TransformStream<
        Uint8Array,
        string
      >,
    )
    .pipeThrough(
      new EventSourceParserStream({
        maxBufferSize: 1_048_576,
        onError: "terminate",
      }),
    )
    .getReader();

  try {
    while (true) {
      const { value, done } = await eventStream.read();
      if (done) {
        return;
      }
      if (!value || value.data === "[DONE]") {
        if (value?.data === "[DONE]") {
          return;
        }
        continue;
      }

      let data: unknown;
      try {
        data = JSON.parse(value.data);
      } catch {
        throw agentRequestError(invalidProtocolError);
      }

      const parsed = streamPartSchema.safeParse(data);
      if (!parsed.success) {
        if (hasPartType(data) && !knownPartTypes.has(data.type)) {
          continue;
        }
        throw agentRequestError(invalidProtocolError);
      }

      if (parsed.data.type === "start") {
        if (started || terminalSeen) {
          throw agentRequestError(invalidProtocolError);
        }
        started = true;
      } else if (
        parsed.data.type === "start-step" ||
        parsed.data.type === "text-start" ||
        parsed.data.type === "text-end" ||
        parsed.data.type === "finish-step"
      ) {
        if (!started || terminalSeen) {
          throw agentRequestError(invalidProtocolError);
        }
      } else if (parsed.data.type === "text-delta") {
        if (!started) {
          throw agentRequestError(invalidProtocolError);
        }
        yield {
          type: "text-delta",
          id: parsed.data.id,
          delta: parsed.data.delta,
        };
      } else if (parsed.data.type === "finish") {
        if (!started || terminalSeen) {
          throw agentRequestError(invalidProtocolError);
        }
        terminalSeen = true;
        yield {
          type: "finish",
          finishReason: parsed.data.finishReason,
          runId: parsed.data.messageMetadata?.runId,
          status: parsed.data.messageMetadata?.status,
        };
      } else {
        if (!started || terminalSeen) {
          throw agentRequestError(invalidProtocolError);
        }
        terminalSeen = true;
        yield {
          type: "error",
          error: parsed.data.agentError ?? {
            code: "AGENT_PROVIDER_FAILED",
            kind: "server",
            message: parsed.data.errorText,
            retryable: true,
          },
        };
      }
    }
  } finally {
    eventStream.releaseLock();
  }
}
