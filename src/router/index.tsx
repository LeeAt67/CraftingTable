import { useMemo } from "react";
import { createHashRouter, RouterProvider, type RouteObject } from "react-router-dom";
import { AppLayout } from "@/layouts/appLayout";
import { AgentWorkspaceLayout } from "@/layouts/agentWorkspaceLayout";
import { DocsWorkspaceLayout } from "@/layouts/docsWorkspaceLayout";
import { TodayWorkspaceLayout } from "@/layouts/todayWorkspaceLayout";
import AgentPage from "@/pages/agent";
import DocsEmptyPage from "@/pages/docs";
import DocPage from "@/pages/docs/docPage";
import SettingsPage from "@/pages/settings";
import TodayPage from "@/pages/today";

export const routes: RouteObject[] = [
  {
    element: <AppLayout />,
    children: [
      {
        element: <TodayWorkspaceLayout />,
        children: [{ path: "/", element: <TodayPage /> }],
      },
      {
        path: "/agent",
        element: <AgentWorkspaceLayout />,
        children: [
          { index: true, element: <AgentPage /> },
          { path: ":sessionId", element: <AgentPage /> },
        ],
      },
      {
        path: "/docs",
        element: <DocsWorkspaceLayout />,
        children: [
          { index: true, element: <DocsEmptyPage /> },
          { path: ":nodeId", element: <DocPage /> },
        ],
      },
      { path: "/settings", element: <SettingsPage /> },
    ],
  },
];

export function createAppRouter() {
  return createHashRouter(routes);
}

export function AppRouter() {
  const router = useMemo(() => createAppRouter(), []);
  return <RouterProvider router={router} />;
}
