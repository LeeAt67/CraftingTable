import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { routes } from "@/router";

describe("AppRouter", () => {
  it("shows home navigation and the context sidebar", () => {
    const router = createMemoryRouter(routes, { initialEntries: ["/"] });
    render(<RouterProvider router={router} />);
    expect(screen.getByRole("heading", { name: "今日" })).toBeInTheDocument();
    expect(screen.getByText("全部")).toBeInTheDocument();
    expect(screen.queryByText("LEEAT")).not.toBeInTheDocument();
  });

  it("opens the settings placeholder", () => {
    const router = createMemoryRouter(routes, { initialEntries: ["/settings"] });
    render(<RouterProvider router={router} />);
    expect(screen.getByRole("heading", { name: "设置" })).toBeInTheDocument();
  });

  it("opens the Agent workspace with its persistent session sidebar", () => {
    const router = createMemoryRouter(routes, { initialEntries: ["/agent"] });
    render(<RouterProvider router={router} />);

    expect(screen.getByRole("heading", { name: "Agent" })).toBeInTheDocument();
    expect(
      screen.getByRole("complementary", { name: "Agent 会话" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Agent" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("opens the documents workspace with its tree sidebar", () => {
    const router = createMemoryRouter(routes, { initialEntries: ["/docs"] });
    render(<RouterProvider router={router} />);

    expect(screen.getByRole("complementary", { name: "文档目录" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "文档" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByText("选择或新建一篇文档")).toBeInTheDocument();
  });
});
