import { useEffect } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { AgentComposer } from "@/components/agent/agentComposer";
import { AgentEmptyState } from "@/components/agent/agentEmptyState";
import { AgentErrorBanner } from "@/components/agent/agentErrorBanner";
import { AgentMessageList } from "@/components/agent/agentMessageList";
import { useAgentEffect } from "@/controllers/effects/useAgentEffect";
import { useAgentStore } from "@/controllers/stores/agentStore";

export default function AgentPage() {
  const { sessionId } = useParams();
  const navigate = useNavigate();
  const effect = useAgentEffect();
  const activeSessionId = useAgentStore((state) => state.activeSessionId);
  const liveRun = useAgentStore((state) => state.liveRun);
  const sendPhase = useAgentStore((state) => state.sendPhase);
  const error = useAgentStore((state) => state.error);

  const messages = useAgentStore(
    (state) =>
      (activeSessionId &&
        state.messagesBySessionId[activeSessionId]?.length) ||
      0,
  );
  const hasInlineFailure = useAgentStore((state) => {
    if (!state.activeSessionId) {
      return false;
    }
    const lastMessage = state.messagesBySessionId[
      state.activeSessionId
    ]?.at(-1);
    return Boolean(
      lastMessage?.role === "assistant" &&
        lastMessage.status === "failed" &&
        state.partsByMessageId[lastMessage.id]?.some(
          (part) =>
            part.type === "error" &&
            (!lastMessage.activeRunId ||
              !part.runId ||
              part.runId === lastMessage.activeRunId),
        ),
    );
  });
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
    <section className="flex h-full min-w-0 flex-col bg-surface">
      {error && !hasInlineFailure && (
        <AgentErrorBanner error={error} onRetry={retry} />
      )}

      <div className="min-h-0 flex-1">
        {!activeSessionId ? (
          <AgentEmptyState hasSession={false} />
        ) : messages === 0 ? (
          <AgentEmptyState hasSession />
        ) : (
          <AgentMessageList
            sessionId={activeSessionId}
            onRetry={retry}
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
