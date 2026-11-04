import { MessageSquare, Plus } from "lucide-react";
import { NavLink, useNavigate } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { cn } from "@/components/utils";
import { useAgentEffect } from "@/controllers/effects/useAgentEffect";
import { useAgentStore } from "@/controllers/stores/agentStore";

function formatSessionTime(value: string) {
  const date = new Date(value);
  const today = new Date();
  if (date.toDateString() === today.toDateString()) {
    return new Intl.DateTimeFormat("zh-CN", {
      hour: "2-digit",
      minute: "2-digit",
    }).format(date);
  }
  return new Intl.DateTimeFormat("zh-CN", {
    month: "numeric",
    day: "numeric",
  }).format(date);
}

export function AgentSidebar() {
  const navigate = useNavigate();
  const sessions = useAgentStore((state) => state.sessions);
  const activeSessionId = useAgentStore((state) => state.activeSessionId);
  const effect = useAgentEffect();

  const createSession = async () => {
    const [created, error] = await effect.createSession();
    if (!error && created) {
      navigate(`/agent/${created.session.id}`);
    }
  };

  return (
    <aside
      aria-label="Agent 会话"
      className="flex h-full w-[92px] shrink-0 flex-col border-r border-border bg-surface px-2 py-4 min-[768px]:w-[220px] min-[768px]:px-3 min-[1280px]:w-[252px]"
    >
      <div className="mb-5 flex min-w-0 items-center gap-2 px-1">
        <div className="grid size-7 shrink-0 place-items-center rounded-md bg-brandSoft text-brand">
          <MessageSquare aria-hidden="true" size={15} />
        </div>
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold text-textPrimary">
            Agent
          </p>
          <p className="hidden text-xs text-textSecondary min-[768px]:block">
            个人工作助手
          </p>
        </div>
      </div>

      <Button
        type="button"
        size="sm"
        aria-label="新建会话"
        onClick={() => void createSession()}
        className="h-9 w-full shrink-0 px-0 min-[768px]:px-3"
      >
        <Plus aria-hidden="true" size={16} />
        <span className="hidden min-[768px]:inline">新建会话</span>
      </Button>

      <nav aria-label="会话列表" className="mt-5 min-h-0 flex-1 overflow-y-auto">
        <p className="mb-2 hidden px-2 text-[11px] font-medium text-textSecondary min-[768px]:block">
          最近会话
        </p>
        <div className="space-y-1">
          {sessions.map((session) => (
            <NavLink
              key={session.id}
              to={`/agent/${session.id}`}
              aria-label={session.title}
              className={cn(
                "block min-w-0 rounded-md px-2 py-2 text-textSecondary transition-colors hover:bg-surfaceSubtle hover:text-textPrimary",
                session.id === activeSessionId &&
                  "bg-brandSoft text-brand hover:bg-brandSoft hover:text-brand",
              )}
            >
              <span className="block truncate text-center text-xs font-medium min-[768px]:text-left min-[768px]:text-sm">
                {session.title}
              </span>
              <span className="mt-1 hidden text-[11px] text-textSecondary min-[768px]:block">
                {formatSessionTime(session.updatedAt)}
              </span>
            </NavLink>
          ))}
        </div>
      </nav>
    </aside>
  );
}
