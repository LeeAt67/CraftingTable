import { Outlet } from "react-router-dom";
import { AgentSidebar } from "@/components/agent/agentSidebar";

export function AgentWorkspaceLayout() {
  return (
    <div className="flex h-full min-w-0">
      <AgentSidebar />
      <main className="min-h-0 min-w-0 flex-1 overflow-hidden">
        <Outlet />
      </main>
    </div>
  );
}
