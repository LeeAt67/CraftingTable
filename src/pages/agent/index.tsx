import { useEffect, useMemo } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { AgentComposer } from "@/components/agent/agentComposer";
import { AgentEmptyState } from "@/components/agent/agentEmptyState";
import { AgentErrorBanner } from "@/components/agent/agentErrorBanner";
import { AgentMessageList } from "@/components/agent/agentMessageList";
import { useAgentEffect } from "@/controllers/effects/useAgentEffect";
import { useAgentStore } from "@/controllers/stores/agentStore";

function formatUpdatedAt(value?: string) {
  if (!value) {
    return "刚刚";
  }
  return new Intl.DateTimeFormat("zh-CN", {
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

export default function AgentPage() {
  const { sessionId } = useParams();
  const navigate = useNavigate();
  const effect = useAgentEffect();
  const sessions = useAgentStore((state) => state.sessions);
  const activeSessionId = useAgentStore((state) => state.activeSessionId);
  const liveRun = useAgentStore((state) => state.liveRun);
  const sendPhase = useAgentStore((state) => state.sendPhase);
  const error = useAgentStore((state) => state.error);

  const activeSession = useMemo(
    () => sessions.find((session) => session.id === activeSessionId),
    [activeSessionId, sessions],
  );
  const messages = useAgentStore(
    (state) =>
      (activeSessionId &&
        state.messagesBySessionId[activeSessionId]?.length) ||
      0,
  );
  const isCurrentSessionStreaming =
    liveRun?.sessionId === activeSessionId &&
    (sendPhase === "submitting" || sendPhase === "streaming");
  const hasBlockingRun = Boolean(liveRun);

  useEffect(() => {
    if (!sessionId) {
      useAgentStore.getState().setActiveSession(null);
      return;
    }
    void effect.loadSession(sessionId);
  }, [effect, sessionId]);

  const createSession = async () => {
    const [created, createError] = await effect.createSession();
    if (!createError && created) {
      navigate(`/agent/${created.session.id}`);
    }
  };

  const sendMessage = (text: string) => {
    void effect.sendMessage(text);
  };

  const retry = () => {
    if (!activeSessionId) {
      void createSession();
      return;
    }
    void effect.retryLastMessage();
  };

  return (
    <section className="flex h-full min-w-0 flex-col bg-background">
      <header className="flex min-h-16 shrink-0 items-center justify-between gap-4 border-b border-border bg-surface px-4 py-3 min-[768px]:px-8">
        <div className="min-w-0">
          <h1 className="text-lg font-semibold text-textPrimary">Agent</h1>
          <p className="truncate text-xs text-textSecondary">
            {activeSession
              ? `${activeSession.title} · ${formatUpdatedAt(activeSession.updatedAt)}`
              : "个人工作助手"}
          </p>
        </div>
        {activeSession && (
          <span className="shrink-0 rounded-full border border-border bg-surfaceSubtle px-2.5 py-1 text-[11px] text-textSecondary">
            单 Agent
          </span>
        )}
      </header>

      {error && <AgentErrorBanner error={error} onRetry={retry} />}

      <div className="min-h-0 flex-1">
        {!activeSessionId ? (
          <AgentEmptyState onCreate={() => void createSession()} />
        ) : messages === 0 ? (
          <AgentEmptyState onCreate={() => void createSession()} />
        ) : (
          <AgentMessageList
            sessionId={activeSessionId}
            onRetry={error ? undefined : retry}
          />
        )}
      </div>

      {activeSessionId && (
        <AgentComposer
          disabled={hasBlockingRun && !isCurrentSessionStreaming}
          isStreaming={isCurrentSessionStreaming}
          onSend={sendMessage}
          onStop={() => void effect.cancelRun()}
        />
      )}
    </section>
  );
}
