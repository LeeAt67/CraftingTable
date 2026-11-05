import type { AgentSendInput } from "@/models/agentContracts";
import type { AgentModel } from "@/models/agentModel";
import { readAgentStream } from "@/models/agentStreamProtocol";
import {
  agentRequestError,
  sendAgentStream,
  type AgentFetch,
} from "@/models/agentTransport";
import { createFakeAgentModel } from "@/models/fakeAgentModel";

type ApiAgentModelOptions = {
  baseUrl?: string;
  fetchImpl?: AgentFetch;
};

type AgentRuntime = {
  assistantMessageId: string;
  assistantPartId: string;
  runId: string;
};

async function* streamAssistantText({
  input,
  history,
  signal,
  runtime,
  baseUrl,
  fetchImpl,
}: {
  input: AgentSendInput;
  history: Array<{ role: "user" | "assistant"; content: string }>;
  signal: AbortSignal;
  runtime: AgentRuntime;
  baseUrl: string;
  fetchImpl: AgentFetch;
}) {
  const body = await sendAgentStream({
    baseUrl,
    sessionId: input.sessionId,
    clientRequestId: input.clientRequestId,
    userMessageId: input.message.id,
    assistantMessageId: runtime.assistantMessageId,
    assistantPartId: runtime.assistantPartId,
    runId: runtime.runId,
    messages: history,
    signal,
    fetchImpl,
  });

  for await (const event of readAgentStream(body)) {
    if (event.type === "text-delta") {
      yield event.delta;
    } else if (event.type === "error") {
      throw agentRequestError(event.error);
    }
  }
}

export function createApiAgentModel(
  options: ApiAgentModelOptions = {},
): AgentModel {
  const baseUrl = options.baseUrl ?? "/api/v1";
  const fetchImpl = options.fetchImpl ?? fetch;

  return createFakeAgentModel({
    chunkDelayMs: 0,
    responseChunks: (input, history, signal, runtime) =>
      streamAssistantText({
        input,
        history,
        signal,
        runtime,
        baseUrl,
        fetchImpl,
      }),
  });
}
