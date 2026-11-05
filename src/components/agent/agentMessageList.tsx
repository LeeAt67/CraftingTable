import { useEffect, useRef } from "react";
import { AlertCircle } from "lucide-react";
import { AgentReasoningPanel } from "@/components/agent/agentReasoningPanel";
import { cn } from "@/components/utils";
import { Button } from "@/components/ui/button";
import { useAgentStore } from "@/controllers/stores/agentStore";
import type {
  AgentMessage,
  AgentMessagePart,
  AgentReasoningPart,
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
  const reasoningParts = visibleParts.filter(
    (part): part is AgentReasoningPart => part.type === "reasoning",
  );
  const textStarted = visibleParts.some(
    (part) => part.type === "text" && part.content.text.length > 0,
  );

  if (visibleParts.length === 0) {
    return (
      <p className="animate-pulse text-sm leading-6 text-textSecondary motion-reduce:animate-none">
        正在思考…
      </p>
    );
  }

  return (
    <div className="space-y-2">
      {reasoningParts.map((part) => (
        <AgentReasoningPanel
          key={part.id}
          part={part}
          textStarted={textStarted}
        />
      ))}
      {visibleParts.map((part) => (
        part.type === "error" ? (
          <div
            key={part.id}
            role="alert"
            className="flex items-start gap-2 text-sm leading-6 text-destructive"
          >
            <AlertCircle
              aria-hidden="true"
              size={15}
              className="mt-1 shrink-0"
            />
            <span className="min-w-0 break-words">
              {part.content.error.message}
            </span>
          </div>
        ) : part.type === "reasoning" ? null : (
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
        )
      ))}
    </div>
  );
}

function runAnnouncement(
  messages: AgentMessage[],
  partsByMessageId: Record<string, AgentMessagePart[]>,
) {
  const streaming = messages.find(
    (message) =>
      message.role === "assistant" && message.status === "streaming",
  );
  if (streaming) {
    const hasText = (partsByMessageId[streaming.id] ?? []).some(
      (part) => part.type === "text" && part.content.text.length > 0,
    );
    return hasText ? "Agent 正在回复" : "Agent 正在思考";
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
  const listRef = useRef<HTMLDivElement>(null);
  const endRef = useRef<HTMLDivElement>(null);
  const followTail = useRef(true);

  useEffect(() => {
    if (!followTail.current) {
      return;
    }
    endRef.current?.scrollIntoView({ block: "end" });
  }, [messages, partsByMessageId]);

  return (
    <div
      ref={listRef}
      role="log"
      aria-label="会话消息"
      className="h-full overflow-y-auto"
      onScroll={(event) => {
        const node = event.currentTarget;
        followTail.current =
          node.scrollHeight - node.scrollTop - node.clientHeight < 8;
      }}
    >
      <p role="status" aria-atomic="true" className="sr-only">
        {runAnnouncement(messages, partsByMessageId)}
      </p>
      <div className="mx-auto flex w-full max-w-[760px] flex-col gap-6 px-4 py-5 min-[768px]:px-5 min-[768px]:py-6">
        {messages.map((message) => {
          const isUser = message.role === "user";
          const messageParts = partsByMessageId[message.id] ?? [];
          const hasNonRetryableError = messageParts.some(
            (part) =>
              part.type === "error" && !part.content.error.retryable,
          );
          const canRetry =
            onRetry &&
            message.id === messages.at(-1)?.id &&
            message.role === "assistant" &&
            (message.status === "failed" ||
              message.status === "cancelled" ||
              message.status === "interrupted") &&
            !hasNonRetryableError;
          return (
            <article
              key={message.id}
              aria-label={isUser ? "你的消息" : "Agent 消息"}
              className="flex min-w-0 flex-col gap-1.5"
            >
              <p
                className={cn(
                  "flex min-h-5 items-center gap-1.5 px-1 text-[11px] font-medium",
                  isUser
                    ? "justify-end text-brand"
                    : "justify-start text-textSecondary",
                )}
              >
                {!isUser && (
                  <span
                    aria-hidden="true"
                    className={cn(
                      "size-1.5 rounded-full bg-textSecondary",
                      message.status === "streaming" &&
                        "animate-pulse bg-brand motion-reduce:animate-none",
                    )}
                  />
                )}
                <span>{isUser ? "leeat" : "Agent"}</span>
                {message.status === "streaming" && (
                  <span className="font-normal text-textSecondary">
                    · 生成中
                  </span>
                )}
              </p>
              <div
                className={cn(
                  "min-w-0",
                  isUser
                    ? "ml-auto max-w-[92%] rounded-2xl rounded-br-md bg-brandSoft px-3.5 py-2.5 text-textPrimary min-[768px]:max-w-[600px]"
                    : "max-w-full px-3.5 py-1 text-textPrimary min-[768px]:pr-10",
                  message.status === "failed" && "text-destructive",
                )}
              >
                <MessageBody
                  message={message}
                  parts={messageParts}
                />
                {canRetry && (
                  <Button
                    type="button"
                    variant="link"
                    size="sm"
                    onClick={onRetry}
                    className="mt-2 h-auto p-0 text-xs"
                  >
                    重试
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
