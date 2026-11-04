import { useEffect, useRef } from "react";
import { cn } from "@/components/utils";
import { Button } from "@/components/ui/button";
import { useAgentStore } from "@/controllers/stores/agentStore";
import type {
  AgentMessage,
  AgentMessagePart,
} from "@/models/agentContracts";

function MessageBody({
  message,
  parts,
}: {
  message: AgentMessage;
  parts: AgentMessagePart[];
}) {
  const visibleParts = parts.filter(
    (part) =>
      !message.activeRunId ||
      !part.runId ||
      part.runId === message.activeRunId,
  );

  if (visibleParts.length === 0) {
    return (
      <p className="animate-pulse text-sm text-textSecondary">正在思考…</p>
    );
  }

  return (
    <div className="space-y-2">
      {visibleParts.map((part) => (
        <p
          key={part.id}
          className="whitespace-pre-wrap break-words text-sm leading-6"
        >
          {part.content.text}
          {part.status === "streaming" && (
            <span
              aria-hidden="true"
              className="ml-0.5 inline-block h-4 w-0.5 animate-pulse bg-brand align-middle"
            />
          )}
        </p>
      ))}
    </div>
  );
}

function runAnnouncement(messages: AgentMessage[]) {
  if (messages.some((message) => message.status === "streaming")) {
    return "Agent 正在回复";
  }

  let lastAssistant: AgentMessage | undefined;
  for (const message of messages) {
    if (message.role === "assistant") {
      lastAssistant = message;
    }
  }
  if (lastAssistant?.status === "failed") {
    return "Agent 回复失败";
  }
  if (
    lastAssistant?.status === "cancelled" ||
    lastAssistant?.status === "interrupted"
  ) {
    return "Agent 回复已停止";
  }
  return "Agent 回复完成";
}

export function AgentMessageList({
  sessionId,
  onRetry,
}: {
  sessionId: string;
  onRetry?: () => void;
}) {
  const messages = useAgentStore(
    (state) => state.messagesBySessionId[sessionId] ?? [],
  );
  const partsByMessageId = useAgentStore((state) => state.partsByMessageId);
  const endRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    endRef.current?.scrollIntoView({ block: "end" });
  }, [messages, partsByMessageId]);

  return (
    <div
      role="log"
      aria-label="会话消息"
      className="h-full overflow-y-auto"
    >
      <p role="status" aria-atomic="true" className="sr-only">
        {runAnnouncement(messages)}
      </p>
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 py-6 min-[768px]:px-8 min-[768px]:py-8">
        {messages.map((message) => {
          const isUser = message.role === "user";
          const canRetry =
            onRetry &&
            message.id === messages.at(-1)?.id &&
            message.role === "assistant" &&
            (message.status === "failed" ||
              message.status === "cancelled" ||
              message.status === "interrupted");
          return (
            <article
              key={message.id}
              aria-label={isUser ? "你的消息" : "Agent 消息"}
              className={cn(
                "flex min-w-0",
                isUser ? "justify-end" : "justify-start",
              )}
            >
              <div
                className={cn(
                  "min-w-0 max-w-[90%] min-[768px]:max-w-[78%]",
                  isUser
                    ? "rounded-lg bg-brand px-4 py-3 text-white"
                    : "border-l-2 border-brand bg-surface px-4 py-3 text-textPrimary",
                  message.status === "failed" && "border-destructive",
                )}
              >
                <p
                  className={cn(
                    "mb-1 text-[11px] font-medium",
                    isUser ? "text-white/75" : "text-textSecondary",
                  )}
                >
                  {isUser ? "你" : "Agent"}
                </p>
                <MessageBody
                  message={message}
                  parts={partsByMessageId[message.id] ?? []}
                />
                {canRetry && (
                  <Button
                    type="button"
                    variant="link"
                    size="sm"
                    onClick={onRetry}
                    className="mt-2 h-auto p-0"
                  >
                    重试回复
                  </Button>
                )}
              </div>
            </article>
          );
        })}
        <div ref={endRef} />
      </div>
    </div>
  );
}
