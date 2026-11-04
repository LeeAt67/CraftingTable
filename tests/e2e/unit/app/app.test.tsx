import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import App from "@/app";

describe("App", () => {
  it("renders the workbench without a top application bar", () => {
    render(<App />);
    expect(screen.getByRole("complementary", { name: "主导航" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "今日" })).toBeInTheDocument();
    expect(screen.queryByText("LEEAT")).not.toBeInTheDocument();
  });
});
