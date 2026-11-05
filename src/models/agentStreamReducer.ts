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
};

export type AgentStreamTransition = {
  state: AgentStreamState;
  events: AgentStreamEvent[];
};

export function createAgentStreamState(): AgentStreamState {
  return { phase: "awaiting-start" };
}

function invalid<T>(): Result<T> {
  return [null, invalidProtocolError];
}

export function reduceAgentStream(
  state: AgentStreamState,
  part: AgentStreamPart,
): Result<AgentStreamTransition> {
  if (state.phase === "terminal") {
    return invalid();
  }

  if (state.phase === "awaiting-start") {
    if (part.type !== "start") {
      return invalid();
    }
    return [{ state: { phase: "streaming" }, events: [] }, null];
  }

  switch (part.type) {
    case "start":
      return invalid();
    case "start-step":
    case "text-start":
    case "text-end":
    case "finish-step":
      return [{ state, events: [] }, null];
    case "text-delta":
      return [
        {
          state,
          events: [{ type: "text-delta", id: part.id, delta: part.delta }],
        },
        null,
      ];
    case "finish":
      return [
        {
          state: { phase: "terminal" },
          events: [
            {
              type: "finish",
              finishReason: part.finishReason,
              runId: part.messageMetadata.runId,
              status: part.messageMetadata.status,
            },
          ],
        },
        null,
      ];
    case "error":
      return [
        {
          state: { phase: "terminal" },
          events: [
            {
              type: "error",
              error: part.agentError ?? {
                code: "AGENT_PROVIDER_FAILED",
                kind: "server",
                message: part.errorText,
                retryable: true,
              },
            },
          ],
        },
        null,
      ];
  }
}

export function completeAgentStream(
  state: AgentStreamState,
): Result<null> {
  return state.phase === "terminal"
    ? [null, null]
    : [null, unexpectedEndError];
}
