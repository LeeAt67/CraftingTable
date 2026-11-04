import type { ReactNode } from "react";
import { NavRail } from "./navRail";

export function AppShell({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-screen min-w-0 overflow-hidden bg-background">
      <NavRail />
      <div className="min-h-0 min-w-0 flex-1 overflow-hidden">{children}</div>
    </div>
  );
}
