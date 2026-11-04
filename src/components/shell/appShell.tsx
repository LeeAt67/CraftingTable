import type { ReactNode } from "react";
import { ContextSidebar } from "./contextSidebar";
import { NavRail } from "./navRail";

export function AppShell({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-screen min-w-0 overflow-hidden bg-background">
      <NavRail />
      <ContextSidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <main className="min-h-0 min-w-0 flex-1 overflow-y-auto">{children}</main>
      </div>
    </div>
  );
}
