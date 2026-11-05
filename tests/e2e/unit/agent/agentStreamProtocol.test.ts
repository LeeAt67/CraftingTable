import { describe, expect, it } from "vitest";
import {
  readAgentStream,
  type AgentStreamEvent,
} from "@/models/agentStreamProtocol";

function streamOf(text: string) {
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
});
