import { describe, expect, it } from "vitest";
import {
  readAgentStream,
  type AgentStreamEvent,
} from "@/models/agentStreamProtocol";

function streamOf(text: string) {
  let sequence = 0;
  const events = text
    .split(/\n\n+/)
    .filter((event) => event.trim() !== "")
    .map((event) => {
      if (event.includes("data: [DONE]")) {
        return event;
      }
      const explicitCursor = event.match(/^id:\s*(\d+)$/m);
      if (explicitCursor) {
        sequence = Math.max(sequence, Number(explicitCursor[1]));
        return event;
      }
      sequence += 1;
      return `id: ${sequence}\n${event}`;
    })
    .join("\n\n");
  return rawStreamOf(`${events}\n\n`);
}

function rawStreamOf(text: string) {
  const encoder = new TextEncoder();
  return new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(encoder.encode(text));
      controller.close();
    },
  });
}

async function collect(
  stream: AsyncIterable<AgentStreamEvent>,
): Promise<AgentStreamEvent[]> {
  const events: AgentStreamEvent[] = [];
  for await (const event of stream) {
    events.push(event);
  }
  return events;
}

describe("agentStreamProtocol", () => {
  it("attaches and validates the SSE cursor", async () => {
    const stream = streamOf(
      [
        'id: 1\ndata: {"type":"start","messageId":"assistant-1","messageMetadata":{"sessionId":"session-1","runId":"run-1","userMessageId":"user-1"}}',
        'id: 2\ndata: {"type":"text-delta","id":"part-1","delta":"你好"}',
        'id: 3\ndata: {"type":"finish","messageMetadata":{"runId":"run-1","status":"completed"}}',
        "data: [DONE]",
        "",
        "",
      ].join("\n\n"),
    );

    await expect(collect(readAgentStream(stream))).resolves.toEqual([
      expect.objectContaining({ type: "text-delta", cursor: 2 }),
      expect.objectContaining({ type: "finish", cursor: 3 }),
    ]);
  });

  it("rejects missing, non-numeric, or regressing cursors", async () => {
    const missing = rawStreamOf(
      'data: {"type":"start","messageId":"message-1","messageMetadata":{"sessionId":"session-1","runId":"run-1","userMessageId":"user-1"}}\n\n',
    );
    await expect(collect(readAgentStream(missing))).rejects.toMatchObject({
      agentError: { code: "PROTOCOL_INVALID_METADATA" },
    });

    const nonNumeric = rawStreamOf(
      'id: nope\ndata: {"type":"start","messageId":"message-1","messageMetadata":{"sessionId":"session-1","runId":"run-1","userMessageId":"user-1"}}\n\n',
    );
    await expect(collect(readAgentStream(nonNumeric))).rejects.toMatchObject({
      agentError: { code: "PROTOCOL_INVALID_METADATA" },
    });

    const regressing = rawStreamOf(
      [
        'id: 1\ndata: {"type":"start","messageId":"message-1","messageMetadata":{"sessionId":"session-1","runId":"run-1","userMessageId":"user-1"}}',
        'id: 1\ndata: {"type":"text-delta","id":"part-1","delta":"重复"}',
        "",
        "",
      ].join("\n\n"),
    );
    await expect(collect(readAgentStream(regressing))).rejects.toMatchObject({
      agentError: { code: "PROTOCOL_INVALID_METADATA" },
    });
  });

  it("rejects a stream that starts with text before the start event", async () => {
    const stream = streamOf(
      'data: {"type":"text-delta","id":"part-1","delta":"你好"}\n\n',
    );

    await expect(collect(readAgentStream(stream))).rejects.toMatchObject({
      agentError: {
        code: "PROTOCOL_INVALID_METADATA",
        kind: "protocol",
        retryable: false,
      },
    });
  });

  it("rejects duplicate terminal events", async () => {
    const stream = streamOf(
      [
        'data: {"type":"start","messageId":"message-1","messageMetadata":{"sessionId":"session-1","runId":"run-1","userMessageId":"user-1"}}',
        'data: {"type":"finish","finishReason":"stop","messageMetadata":{"runId":"run-1","status":"completed"}}',
        'data: {"type":"finish","finishReason":"stop","messageMetadata":{"runId":"run-1","status":"completed"}}',
        "data: [DONE]",
        "",
        "",
      ].join("\n\n"),
    );

    await expect(collect(readAgentStream(stream))).rejects.toMatchObject({
      agentError: {
        code: "PROTOCOL_INVALID_METADATA",
        kind: "protocol",
        retryable: false,
      },
    });
  });

  it("rejects start events without required run metadata", async () => {
    const stream = streamOf(
      [
        'data: {"type":"start","messageId":"message-1","messageMetadata":{"sessionId":"session-1","userMessageId":"user-1"}}',
        'data: {"type":"text-delta","id":"part-1","delta":"你好"}',
        "data: [DONE]",
        "",
        "",
      ].join("\n\n"),
    );

    await expect(collect(readAgentStream(stream))).rejects.toMatchObject({
      agentError: {
        code: "PROTOCOL_INVALID_METADATA",
        kind: "protocol",
        retryable: false,
      },
    });
  });

  it("rejects finish events without terminal run metadata", async () => {
    const stream = streamOf(
      [
        'data: {"type":"start","messageId":"message-1","messageMetadata":{"sessionId":"session-1","runId":"run-1","userMessageId":"user-1"}}',
        'data: {"type":"finish","finishReason":"stop","messageMetadata":{"status":"completed"}}',
        "data: [DONE]",
        "",
        "",
      ].join("\n\n"),
    );

    await expect(collect(readAgentStream(stream))).rejects.toMatchObject({
      agentError: {
        code: "PROTOCOL_INVALID_METADATA",
        kind: "protocol",
        retryable: false,
      },
    });
  });

  it("rejects finish events that omit message metadata", async () => {
    const stream = streamOf(
      [
        'data: {"type":"start","messageId":"message-1","messageMetadata":{"sessionId":"session-1","runId":"run-1","userMessageId":"user-1"}}',
        'data: {"type":"finish","finishReason":"stop"}',
        "data: [DONE]",
        "",
        "",
      ].join("\n\n"),
    );

    await expect(collect(readAgentStream(stream))).rejects.toMatchObject({
      agentError: {
        code: "PROTOCOL_INVALID_METADATA",
        kind: "protocol",
        retryable: false,
      },
    });
  });

  it("rejects [DONE] before a terminal event", async () => {
    const stream = streamOf(
      [
        'data: {"type":"start","messageId":"message-1","messageMetadata":{"sessionId":"session-1","runId":"run-1","userMessageId":"user-1"}}',
        "data: [DONE]",
        "",
        "",
      ].join("\n\n"),
    );

    await expect(collect(readAgentStream(stream))).rejects.toMatchObject({
      agentError: {
        code: "PROTOCOL_UNEXPECTED_EOF",
        kind: "protocol",
        retryable: true,
      },
    });
  });

  it("rejects EOF before a terminal event", async () => {
    const stream = streamOf(
      'data: {"type":"start","messageId":"message-1","messageMetadata":{"sessionId":"session-1","runId":"run-1","userMessageId":"user-1"}}\n\n',
    );

    await expect(collect(readAgentStream(stream))).rejects.toMatchObject({
      agentError: {
        code: "PROTOCOL_UNEXPECTED_EOF",
        kind: "protocol",
        retryable: true,
      },
    });
  });

  it("rejects malformed JSON", async () => {
    const stream = streamOf("data: not-json\n\n");

    await expect(collect(readAgentStream(stream))).rejects.toMatchObject({
      agentError: {
        code: "PROTOCOL_INVALID_METADATA",
        kind: "protocol",
        retryable: false,
      },
    });
  });

  it("rejects duplicate start events", async () => {
    const start =
      'data: {"type":"start","messageId":"message-1","messageMetadata":{"sessionId":"session-1","runId":"run-1","userMessageId":"user-1"}}';
    const stream = streamOf([start, start, "data: [DONE]", "", ""].join("\n\n"));

    await expect(collect(readAgentStream(stream))).rejects.toMatchObject({
      agentError: {
        code: "PROTOCOL_INVALID_METADATA",
        kind: "protocol",
        retryable: false,
      },
    });
  });

  it("ignores unknown chunks while streaming", async () => {
    const stream = streamOf(
      [
        'data: {"type":"start","messageId":"message-1","messageMetadata":{"sessionId":"session-1","runId":"run-1","userMessageId":"user-1"}}',
        'data: {"type":"future-warning","message":"ignored"}',
        'data: {"type":"text-delta","id":"part-1","delta":"你好"}',
        'data: {"type":"finish","finishReason":"stop","messageMetadata":{"runId":"run-1","status":"completed"}}',
        "data: [DONE]",
        "",
        "",
      ].join("\n\n"),
    );

    await expect(collect(readAgentStream(stream))).resolves.toEqual([
      { type: "text-delta", id: "part-1", delta: "你好", cursor: 3 },
      {
        type: "finish",
        finishReason: "stop",
        runId: "run-1",
        status: "completed",
        cursor: 4,
      },
    ]);
  });

  it("rejects a known chunk after the terminal event", async () => {
    const stream = streamOf(
      [
        'data: {"type":"start","messageId":"message-1","messageMetadata":{"sessionId":"session-1","runId":"run-1","userMessageId":"user-1"}}',
        'data: {"type":"finish","finishReason":"stop","messageMetadata":{"runId":"run-1","status":"completed"}}',
        'data: {"type":"text-delta","id":"part-1","delta":"too late"}',
        "data: [DONE]",
        "",
        "",
      ].join("\n\n"),
    );

    await expect(collect(readAgentStream(stream))).rejects.toMatchObject({
      agentError: {
        code: "PROTOCOL_INVALID_METADATA",
        kind: "protocol",
        retryable: false,
      },
    });
  });

  it("maps an SSE error chunk to a terminal error event", async () => {
    const stream = streamOf(
      [
        'data: {"type":"start","messageId":"message-1","messageMetadata":{"sessionId":"session-1","runId":"run-1","userMessageId":"user-1"}}',
        'data: {"type":"error","errorText":"模型生成失败"}',
        "data: [DONE]",
        "",
        "",
      ].join("\n\n"),
    );

    await expect(collect(readAgentStream(stream))).resolves.toEqual([
      {
        type: "error",
        error: {
          code: "AGENT_PROVIDER_FAILED",
          kind: "server",
          message: "模型生成失败",
          retryable: true,
        },
        cursor: 2,
      },
    ]);
  });
});
