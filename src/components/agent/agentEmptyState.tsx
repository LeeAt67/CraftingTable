import { MessageSquarePlus } from "lucide-react";
import { Button } from "@/components/ui/button";

export function AgentEmptyState({ onCreate }: { onCreate: () => void }) {
  return (
    <div className="grid min-h-full place-items-center px-5 py-10">
      <div className="max-w-sm text-center">
        <div className="mx-auto grid size-11 place-items-center rounded-lg border border-border bg-surface text-brand">
          <MessageSquarePlus aria-hidden="true" size={20} />
        </div>
        <h2 className="mt-4 text-base font-semibold text-textPrimary">
          开始一段工作对话
        </h2>
        <p className="mt-2 text-sm leading-6 text-textSecondary">
          把要梳理的事情交给 Agent，从今天的重点开始。
        </p>
        <Button
          type="button"
          aria-label="从头创建一段对话"
          onClick={onCreate}
          className="mt-5"
        >
          <MessageSquarePlus aria-hidden="true" size={16} />
          新建会话
        </Button>
      </div>
    </div>
  );
}
