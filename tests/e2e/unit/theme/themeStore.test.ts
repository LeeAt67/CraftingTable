import { beforeEach, describe, expect, it } from "vitest";
import { useThemeStore } from "@/controllers/stores/themeStore";

describe("themeStore", () => {
  beforeEach(() => {
    window.localStorage.clear();
    useThemeStore.setState({ theme: "light" });
  });

  it("switches from light to dark", () => {
    useThemeStore.getState().toggleTheme();
    expect(useThemeStore.getState().theme).toBe("dark");
  });

  it("sets an explicit theme", () => {
    useThemeStore.getState().setTheme("dark");
    expect(useThemeStore.getState().theme).toBe("dark");
  });
});
