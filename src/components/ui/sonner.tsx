import type { ComponentProps } from "react";
import { Toaster as Sonner } from "sonner";

export function Toaster(props: ComponentProps<typeof Sonner>) {
  return <Sonner theme="system" richColors {...props} />;
}
