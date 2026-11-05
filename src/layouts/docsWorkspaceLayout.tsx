import { Outlet } from "react-router-dom";
import { DocSidebar } from "@/components/docs/docSidebar";

export function DocsWorkspaceLayout() {
  return (
    <div className="flex h-full min-w-0">
      <DocSidebar />
      <main className="min-h-0 min-w-0 flex-1 overflow-hidden">
        <Outlet />
      </main>
    </div>
  );
}
