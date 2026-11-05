import { useEffect, useRef } from "react";
import { AlertCircle } from "lucide-react";
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
      <p className="animate-pulse text-sm leading-6 text-textSecondary motion-reduce:animate-none">
        正在思考…
      </p>
    );
  }

  return (
    <div className="space-y-2">
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
        ) : (
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
