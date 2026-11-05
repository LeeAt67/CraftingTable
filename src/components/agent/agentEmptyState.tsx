import { MessageSquare } from "lucide-react";

export function AgentEmptyState({ hasSession }: { hasSession: boolean }) {
  return (
    <div className="grid min-h-full place-items-center px-5 py-10">
      <div className="max-w-sm -translate-y-6 text-center">
        <div className="mx-auto grid size-10 place-items-center rounded-xl bg-surfaceSubtle text-textSecondary">
          <MessageSquare aria-hidden="true" size={19} strokeWidth={1.7} />
        </div>
        <h2 className="mt-3 text-sm font-semibold text-textPrimary">
          {hasSession ? "开始一段对话" : "暂无会话"}
        </h2>
        <p className="mt-1.5 text-xs leading-5 text-textSecondary">
          {hasSession
            ? "在下方的输入框中写下要整理的事情。"
            : "从左侧新建会话，开始整理今天的重点。会话会按最近更新时间排列，方便继续之前的工作。"}
        </p>
      </div>
    </div>
  );
}
