import type { Result } from "@/models/agentContracts";
import {
  invalidProtocolError,
  unexpectedEndError,
  type AgentStreamEvent,
  type AgentStreamPart,
} from "@/models/agentStreamSchema";

export type AgentStreamPhase = "awaiting-start" | "streaming" | "terminal";

export type AgentStreamState = {
  phase: AgentStreamPhase;
  lastCursor: number;
};

export type AgentStreamFrame = {
  cursor: number;
  part: AgentStreamPart;
};

export type AgentStreamTransition = {
  state: AgentStreamState;
  events: AgentStreamEvent[];
};

export function createAgentStreamState(
  lastCursor = 0,
): AgentStreamState {
  return { phase: "awaiting-start", lastCursor };
}

function invalid<T>(): Result<T> {
  return [null, invalidProtocolError];
}

export function reduceAgentStream(
  state: AgentStreamState,
  frame: AgentStreamFrame,
): Result<AgentStreamTransition> {
  if (
    !Number.isSafeInteger(frame.cursor) ||
    frame.cursor <= state.lastCursor
  ) {
    return invalid();
  }
  const part = frame.part;
  const nextState = (phase: AgentStreamPhase): AgentStreamState => ({
    phase,
    lastCursor: frame.cursor,
  });
  if (state.phase === "terminal") {
    return invalid();
  }

  if (state.phase === "awaiting-start") {
    if (part.type !== "start") {
      return invalid();
    }
    return [{ state: nextState("streaming"), events: [] }, null];
  }

  switch (part.type) {
    case "start":
      return invalid();
    case "start-step":
    case "text-start":
    case "text-end":
    case "finish-step":
      return [{ state: nextState(state.phase), events: [] }, null];
    case "text-delta":
      return [
        {
          state: nextState("streaming"),
          events: [
            {
              type: "text-delta",
              id: part.id,
              delta: part.delta,
              cursor: frame.cursor,
            },
          ],
        },
        null,
      ];
    case "finish":
      return [
        {
          state: nextState("terminal"),
          events: [
            {
              type: "finish",
              finishReason: part.finishReason,
              runId: part.messageMetadata.runId,
              status: part.messageMetadata.status,
              cursor: frame.cursor,
            },
          ],
        },
        null,
      ];
    case "error":
      return [
        {
          state: nextState("terminal"),
          events: [
            {
              type: "error",
              error: part.agentError ?? {
                code: "AGENT_PROVIDER_FAILED",
                kind: "server",
                message: part.errorText,
                retryable: true,
              },
              cursor: frame.cursor,
            },
          ],
        },
        null,
      ];
  }
}

export function advanceAgentStreamCursor(
  state: AgentStreamState,
  cursor: number,
): Result<AgentStreamState> {
  if (
    state.phase === "terminal" ||
    !Number.isSafeInteger(cursor) ||
    cursor <= state.lastCursor
  ) {
    return invalid();
  }
  return [{ ...state, lastCursor: cursor }, null];
}

export function completeAgentStream(
  state: AgentStreamState,
): Result<null> {
  return state.phase === "terminal"
    ? [null, null]
    : [null, unexpectedEndError];
}
