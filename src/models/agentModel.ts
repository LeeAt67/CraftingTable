import type {
  AgentRunStatus,
  AgentRetryInput,
  AgentSendHandlers,
  AgentSendInput,
  AgentSendResult,
  AgentSession,
  AgentSessionDetail,
  Result,
} from "@/models/agentContracts";
import { createFakeAgentModel } from "@/models/fakeAgentModel";
import { createApiAgentModel } from "@/models/apiAgentModel";

export type AgentModel = {
  listSessions: () => Promise<Result<AgentSession[]>>;
  getSession: (sessionId: string) => Promise<Result<AgentSessionDetail>>;
  createSession: (input: {
    clientRequestId: string;
  }) => Promise<Result<{ session: AgentSession }>>;
  sendMessage: (
    input: AgentSendInput,
    handlers: AgentSendHandlers,
  ) => Promise<Result<AgentSendResult>>;
  retryMessage: (
    input: AgentRetryInput,
    handlers: AgentSendHandlers,
  ) => Promise<Result<AgentSendResult>>;
  cancelRun: (input: {
    sessionId: string;
    runId: string;
  }) => Promise<Result<{ runId: string; status: AgentRunStatus }>>;
};

const notConfiguredError = {
  code: "AGENT_NOT_CONFIGURED",
  kind: "server",
  message: "Agent 服务尚未配置",
  retryable: false,
} as const;

const notConfiguredAgentModel: AgentModel = {
  listSessions: async () => [[], null],
  getSession: async () => [null, notConfiguredError],
  createSession: async () => [null, notConfiguredError],
  sendMessage: async () => [null, notConfiguredError],
  retryMessage: async () => [null, notConfiguredError],
  cancelRun: async () => [null, notConfiguredError],
};

function fakeModelOptions() {
  const value = import.meta.env.VITE_AGENT_CHUNK_DELAY_MS;
  if (!value) {
    return undefined;
  }
  const chunkDelayMs = Number(value);
  return Number.isFinite(chunkDelayMs) && chunkDelayMs >= 0
    ? { chunkDelayMs }
    : undefined;
}

export const agentModel: AgentModel =
  import.meta.env.MODE === "test" ||
  import.meta.env.MODE === "e2e" ||
  import.meta.env.VITE_AGENT_SOURCE === "fake"
    ? createFakeAgentModel(fakeModelOptions())
    : import.meta.env.VITE_AGENT_SOURCE === "api" ||
        import.meta.env.MODE === "development"
      ? createApiAgentModel({
          baseUrl: import.meta.env.VITE_API_BASE ?? "/api/v1",
        })
      : notConfiguredAgentModel;
