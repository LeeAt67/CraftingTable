import { useState, type FormEvent, type KeyboardEvent } from "react";
import { ArrowUp, Bot, Square } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/components/utils";

type AgentComposerProps = {
  disabled?: boolean;
  isStreaming: boolean;
  onSend: (text: string) => void;
  onStop: () => void;
};

export function AgentComposer({
  disabled = false,
  isStreaming,
  onSend,
  onStop,
}: AgentComposerProps) {
  const [text, setText] = useState("");
  const [isComposing, setIsComposing] = useState(false);

  const submit = () => {
    const normalized = text.trim();
    if (!normalized || disabled || isStreaming) {
      return;
    }
    setText("");
    onSend(normalized);
  };

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    submit();
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (
      event.key !== "Enter" ||
      event.shiftKey ||
      isComposing ||
      event.nativeEvent.isComposing
    ) {
      return;
    }
    event.preventDefault();
    submit();
  };

  return (
    <form
      onSubmit={handleSubmit}
      className="mx-auto w-full max-w-[760px] px-3 pb-3 pt-2 min-[768px]:px-5 min-[768px]:pb-5"
    >
      <div className="overflow-hidden rounded-2xl border border-border bg-surface shadow-[0_8px_28px_rgba(15,23,42,0.07)] transition-[border-color,box-shadow] focus-within:border-brand/60 focus-within:shadow-[0_8px_28px_rgba(15,23,42,0.08),0_0_0_1px_var(--brand)] dark:shadow-[0_10px_32px_rgba(0,0,0,0.22)]">
        <Textarea
          value={text}
          aria-label="发送给 Agent"
          placeholder="给 Agent 发消息…"
          disabled={disabled}
          onChange={(event) => setText(event.target.value)}
          onKeyDown={handleKeyDown}
          onCompositionStart={() => setIsComposing(true)}
          onCompositionEnd={() => setIsComposing(false)}
          className="min-h-[86px] resize-none border-0 bg-transparent px-3.5 py-3 text-sm leading-6 shadow-none focus-visible:ring-0"
        />
        <div className="flex items-center justify-between gap-3 px-2.5 pb-2.5">
          <span className="inline-flex min-w-0 items-center gap-1.5 rounded-md px-1.5 py-1 text-[11px] text-textSecondary">
            <Bot aria-hidden="true" size={13} />
            <span className="truncate">单 Agent</span>
            <span
              aria-hidden="true"
              className={cn(
                "size-1.5 shrink-0 rounded-full",
                isStreaming ? "animate-pulse bg-brand" : "bg-success",
              )}
            />
          </span>
          <Button
            type={isStreaming ? "button" : "submit"}
            variant={isStreaming ? "outline" : "default"}
            size="icon"
            aria-label={isStreaming ? "停止" : "发送"}
            title={isStreaming ? "停止" : "发送"}
            disabled={disabled}
            onClick={isStreaming ? onStop : undefined}
            className="size-9 shrink-0 rounded-full p-0"
          >
            {isStreaming ? (
              <Square aria-hidden="true" size={13} fill="currentColor" />
            ) : (
              <ArrowUp aria-hidden="true" size={18} strokeWidth={2} />
            )}
          </Button>
        </div>
      </div>
      <p className="mt-2 px-2 text-[10.5px] text-textSecondary">
        Enter 发送 · Shift + Enter 换行
      </p>
    </form>
  );
}
