import { useState, type FormEvent, type KeyboardEvent } from "react";
import { Send, Square } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";

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
      className="mx-auto w-full max-w-3xl px-3 pb-3 pt-2 min-[768px]:px-8 min-[768px]:pb-6"
    >
      <div className="rounded-lg border border-border bg-surface p-2 shadow-sm">
        <Textarea
          value={text}
          aria-label="发送给 Agent"
          placeholder="写下你想整理的事情…"
          disabled={disabled}
          onChange={(event) => setText(event.target.value)}
          onKeyDown={handleKeyDown}
          onCompositionStart={() => setIsComposing(true)}
          onCompositionEnd={() => setIsComposing(false)}
          className="min-h-[72px] border-0 bg-transparent px-2 py-1 focus-visible:ring-0"
        />
        <div className="flex items-center justify-between gap-3 px-1 pb-1">
          <span className="hidden text-[11px] text-textSecondary min-[768px]:inline">
            Enter 发送 · Shift + Enter 换行
          </span>
          <Button
            type={isStreaming ? "button" : "submit"}
            size="sm"
            aria-label={isStreaming ? "停止" : "发送"}
            disabled={disabled}
            onClick={isStreaming ? onStop : undefined}
            className="ml-auto min-w-16"
          >
            {isStreaming ? (
              <>
                <Square aria-hidden="true" size={14} fill="currentColor" />
                停止
              </>
            ) : (
              <>
                <Send aria-hidden="true" size={14} />
                发送
              </>
            )}
          </Button>
        </div>
      </div>
    </form>
  );
}
