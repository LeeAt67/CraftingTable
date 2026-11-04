import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import App from "@/app";

describe("App", () => {
  it("renders the Leeat brand", () => {
    render(<App />);
    expect(screen.getByText("LEEAT")).toBeInTheDocument();
  });
});
