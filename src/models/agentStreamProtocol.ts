import { EventSourceParserStream } from "eventsource-parser/stream";
import {
  advanceAgentStreamCursor,
  completeAgentStream,
  createAgentStreamState,
  reduceAgentStream,
} from "@/models/agentStreamReducer";
import {
  invalidProtocolError,
  parseAgentStreamPart,
  type AgentStreamEvent,
} from "@/models/agentStreamSchema";
import { agentRequestError } from "@/models/agentTransport";

export type { AgentStreamEvent } from "@/models/agentStreamSchema";

export async function* readAgentStream(
  body: ReadableStream<Uint8Array>,
): AsyncIterable<AgentStreamEvent> {
  let state = createAgentStreamState();
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
      if (done || value?.data === "[DONE]") {
        const [, completionError] = completeAgentStream(state);
        if (completionError) {
          throw agentRequestError(completionError);
        }
        return;
      }
      if (!value) {
        continue;
      }

      const cursor = Number.parseInt(value.id ?? "", 10);
      if (!Number.isSafeInteger(cursor) || cursor < 1) {
        throw agentRequestError(invalidProtocolError);
      }

      let data: unknown;
      try {
        data = JSON.parse(value.data);
      } catch {
        throw agentRequestError(invalidProtocolError);
      }

      const [parsed, parseError] = parseAgentStreamPart(data);
      if (parseError) {
        throw agentRequestError(parseError);
      }
      if (parsed.kind === "unknown") {
        const [nextState, cursorError] = advanceAgentStreamCursor(
          state,
          cursor,
        );
        if (cursorError) {
          throw agentRequestError(cursorError);
        }
        state = nextState;
        continue;
      }

      const [transition, reduceError] = reduceAgentStream(
        state,
        { cursor, part: parsed.part },
      );
      if (reduceError) {
        throw agentRequestError(reduceError);
      }

      state = transition.state;
      for (const event of transition.events) {
        yield event;
      }
    }
  } finally {
    eventStream.releaseLock();
  }
}
