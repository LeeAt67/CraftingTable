import { Bot, MessageSquare, SquarePen } from "lucide-react";
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
      className="flex h-full w-[64px] shrink-0 flex-col border-r border-border bg-surfaceSubtle px-1.5 py-3 min-[768px]:w-[220px] min-[768px]:px-3 min-[768px]:py-4 min-[1280px]:w-[260px]"
    >
      <div className="mb-4 flex min-w-0 items-center justify-center gap-2 px-0.5 min-[768px]:justify-start min-[768px]:px-1">
        <div className="grid size-8 shrink-0 place-items-center rounded-lg bg-brandSoft text-brand">
          <Bot aria-hidden="true" size={16} />
        </div>
        <div className="min-w-0">
          <h1 className="sr-only text-sm font-semibold text-textPrimary min-[768px]:not-sr-only min-[768px]:truncate">
            Agent
          </h1>
          <p className="hidden text-xs text-textSecondary min-[768px]:block">
            个人工作台
          </p>
        </div>
      </div>

      <Button
        type="button"
        variant="ghost"
        aria-label="新建会话"
        title="新建会话"
        onClick={() => void createSession()}
        className="h-9 w-full shrink-0 justify-center rounded-[10px] px-0 text-textPrimary hover:bg-surface min-[768px]:justify-start min-[768px]:px-2"
      >
        <SquarePen aria-hidden="true" size={16} />
        <span className="hidden min-[768px]:inline">新建会话</span>
      </Button>

      <nav
        aria-label="会话列表"
        className="mt-4 min-h-0 flex-1 overflow-y-auto overflow-x-hidden"
      >
        <p className="mb-2 hidden px-2 text-[11px] font-medium text-textSecondary min-[768px]:block">
          会话
        </p>
        <div className="space-y-1">
          {sessions.map((session) => (
            <NavLink
              key={session.id}
              to={`/agent/${session.id}`}
              aria-label={session.title}
              className={cn(
                "group flex h-10 min-w-0 items-center justify-center gap-2 rounded-[10px] border border-transparent px-1.5 text-textSecondary transition-colors hover:bg-surface hover:text-textPrimary min-[768px]:h-9 min-[768px]:justify-start",
                session.id === activeSessionId &&
                  "border-border/70 bg-surface text-textPrimary shadow-sm hover:bg-surface",
              )}
            >
              <MessageSquare
                aria-hidden="true"
                size={15}
                className="shrink-0 opacity-70"
              />
              <span className="hidden min-w-0 flex-1 truncate text-left text-sm min-[768px]:block">
                {session.title}
              </span>
              <span className="hidden shrink-0 text-[10px] tabular-nums text-textSecondary min-[768px]:block">
                {formatSessionTime(session.updatedAt)}
              </span>
            </NavLink>
          ))}
        </div>
      </nav>
    </aside>
  );
}
