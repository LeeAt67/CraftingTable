import type { ComponentProps } from "react";
import { Toaster as Sonner } from "sonner";
import { useThemeStore } from "@/controllers/stores/themeStore";

export function Toaster(props: ComponentProps<typeof Sonner>) {
  const theme = useThemeStore((state) => state.theme);
  return (
    <Sonner
      theme={theme}
      position="top-right"
      richColors
      closeButton
      {...props}
    />
  );
}
