import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { routes } from "@/router";

describe("AppRouter", () => {
  it("shows home navigation and the context sidebar", () => {
    const router = createMemoryRouter(routes, { initialEntries: ["/"] });
    render(<RouterProvider router={router} />);
    expect(screen.getByText("LEEAT")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "今日" })).toBeInTheDocument();
    expect(screen.getByText("全部")).toBeInTheDocument();
  });

  it("opens the settings placeholder", () => {
    const router = createMemoryRouter(routes, { initialEntries: ["/settings"] });
    render(<RouterProvider router={router} />);
    expect(screen.getByRole("heading", { name: "设置" })).toBeInTheDocument();
  });
});
