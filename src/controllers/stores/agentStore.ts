import { create } from "zustand";
import type {
  AgentError,
  AgentMessage,
  AgentMessagePart,
  AgentSession,
} from "@/models/agentContracts";

export type AgentSendPhase = "idle" | "submitting" | "streaming" | "error";

type AgentState = {
  sessions: AgentSession[];
  activeSessionId: string | null;
  messagesBySessionId: Record<string, AgentMessage[]>;
  partsByMessageId: Record<string, AgentMessagePart[]>;
  sendPhase: AgentSendPhase;
  liveRun: { sessionId: string; runId: string } | null;
  error: AgentError | null;
};

type AgentActions = {
  setSessions: (sessions: AgentSession[]) => void;
  upsertSession: (session: AgentSession) => void;
  setActiveSession: (sessionId: string | null) => void;
  setSessionMessages: (
    sessionId: string,
    messages: AgentMessage[],
    parts?: AgentMessagePart[],
  ) => void;
  appendMessage: (sessionId: string, message: AgentMessage) => void;
  upsertMessage: (sessionId: string, message: AgentMessage) => void;
  upsertMessagePart: (messageId: string, part: AgentMessagePart) => void;
  setSendPhase: (phase: AgentSendPhase) => void;
  setLiveRun: (run: AgentState["liveRun"]) => void;
  setError: (error: AgentError | null) => void;
};

export const initialAgentState: AgentState = {
  sessions: [],
  activeSessionId: null,
  messagesBySessionId: {},
  partsByMessageId: {},
  sendPhase: "idle",
  liveRun: null,
  error: null,
};

export const useAgentStore = create<AgentState & AgentActions>()((set) => ({
  ...initialAgentState,
  setSessions: (sessions) => set({ sessions }),
  upsertSession: (session) =>
    set((state) => {
      const exists = state.sessions.some((current) => current.id === session.id);
      const sessions = exists
        ? state.sessions.map((current) =>
            current.id === session.id ? session : current,
          )
        : [session, ...state.sessions];
      return {
        sessions: sessions.sort((left, right) =>
          right.updatedAt.localeCompare(left.updatedAt),
        ),
      };
    }),
  setActiveSession: (activeSessionId) => set({ activeSessionId }),
  setSessionMessages: (sessionId, messages, parts = []) =>
    set((state) => {
      const partsByMessageId = { ...state.partsByMessageId };
      for (const message of messages) {
        partsByMessageId[message.id] = parts
          .filter((part) => part.messageId === message.id)
          .sort((left, right) => left.index - right.index);
      }
      return {
        messagesBySessionId: {
          ...state.messagesBySessionId,
          [sessionId]: messages,
        },
        partsByMessageId,
      };
    }),
  appendMessage: (sessionId, message) =>
    set((state) => ({
      messagesBySessionId: {
        ...state.messagesBySessionId,
        [sessionId]: [...(state.messagesBySessionId[sessionId] ?? []), message],
      },
    })),
  upsertMessage: (sessionId, message) =>
    set((state) => {
      const messages = state.messagesBySessionId[sessionId] ?? [];
      const exists = messages.some((current) => current.id === message.id);
      return {
        messagesBySessionId: {
          ...state.messagesBySessionId,
          [sessionId]: exists
            ? messages.map((current) =>
                current.id === message.id ? message : current,
              )
            : [...messages, message],
        },
      };
    }),
  upsertMessagePart: (messageId, part) =>
    set((state) => {
      const parts = state.partsByMessageId[messageId] ?? [];
      const exists = parts.some((current) => current.id === part.id);
      return {
        partsByMessageId: {
          ...state.partsByMessageId,
          [messageId]: (
            exists
              ? parts.map((current) => (current.id === part.id ? part : current))
              : [...parts, part]
          ).sort((left, right) => left.index - right.index),
        },
      };
    }),
  setSendPhase: (sendPhase) => set({ sendPhase }),
  setLiveRun: (liveRun) => set({ liveRun }),
  setError: (error) => set({ error }),
}));

export function resetAgentStore() {
  useAgentStore.setState(initialAgentState);
}
