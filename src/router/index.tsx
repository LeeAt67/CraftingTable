import { useMemo } from "react";
import { createHashRouter, RouterProvider, type RouteObject } from "react-router-dom";
import { AppLayout } from "@/layouts/appLayout";
import SettingsPage from "@/pages/settings";
import TodayPage from "@/pages/today";

export const routes: RouteObject[] = [
  {
    element: <AppLayout />,
    children: [
      { path: "/", element: <TodayPage /> },
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
