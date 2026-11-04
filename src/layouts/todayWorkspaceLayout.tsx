import { Outlet } from "react-router-dom";
import { ContextSidebar } from "@/components/shell/contextSidebar";

export function TodayWorkspaceLayout() {
  return (
    <div className="flex h-full min-w-0">
      <ContextSidebar />
      <main className="min-h-0 min-w-0 flex-1 overflow-y-auto">
        <Outlet />
      </main>
    </div>
  );
}
