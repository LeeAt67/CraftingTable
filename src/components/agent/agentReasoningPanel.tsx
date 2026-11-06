import { useEffect, useRef, useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import type { AgentReasoningPart } from "@/models/agentContracts";

type ExpansionState = "auto" | "open" | "closed";

export function AgentReasoningPanel({
  part,
  textStarted,
}: {
  part: AgentReasoningPart;
  textStarted: boolean;
}) {
  const [expansion, setExpansion] = useState<ExpansionState>(() =>
    part.content.text.length > 0 ? "closed" : "auto",
  );
  const contentRef = useRef<HTMLDivElement>(null);
  const followTail = useRef(true);
  const expanded =
    expansion === "open" || (expansion === "auto" && !textStarted);

  useEffect(() => {
    if (!textStarted) {
      return;
    }
    setExpansion((current) => (current === "auto" ? "closed" : current));
  }, [textStarted]);

  useEffect(() => {
    const node = contentRef.current;
    if (!node || !expanded || !followTail.current) {
      return;
    }
    node.scrollTop = node.scrollHeight;
  }, [expanded, part.content.text]);

  if (part.content.text.length === 0) {
    return null;
  }

  const panelId = `reasoning-panel-${part.id}`;
  return (
    <div className="space-y-1">
      <button
        type="button"
        aria-expanded={expanded}
        aria-controls={panelId}
        onClick={() => {
          setExpansion(expanded ? "closed" : "open");
        }}
        className="flex items-center gap-1 rounded text-xs font-medium text-textSecondary transition-colors hover:text-textPrimary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/40"
      >
        {expanded ? (
          <ChevronDown aria-hidden="true" size={13} />
        ) : (
          <ChevronRight aria-hidden="true" size={13} />
        )}
        <span>Reasoning</span>
      </button>
      {expanded && (
        <div
          id={panelId}
          ref={contentRef}
          tabIndex={0}
          aria-label="Reasoning 内容"
          onScroll={(event) => {
            const node = event.currentTarget;
            followTail.current =
              node.scrollHeight - node.scrollTop - node.clientHeight < 8;
          }}
          className="max-h-[40vh] overflow-y-auto overscroll-contain whitespace-pre-wrap break-words border-l border-border pl-2.5 text-xs leading-5 text-textSecondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/40"
        >
          {part.content.text}
        </div>
      )}
    </div>
  );
}
