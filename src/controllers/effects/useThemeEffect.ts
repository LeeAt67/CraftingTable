import { useEffect } from "react";
import { useThemeStore } from "@/controllers/stores/themeStore";

export function useThemeEffect() {
  const theme = useThemeStore((state) => state.theme);
  const setTheme = useThemeStore((state) => state.setTheme);
  const toggleTheme = useThemeStore((state) => state.toggleTheme);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    document.documentElement.classList.toggle("dark", theme === "dark");
  }, [theme]);

  return { theme, setTheme, toggleTheme };
}
