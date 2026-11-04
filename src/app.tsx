import { Toaster } from "@/components/ui/sonner";
import { AppRouter } from "@/router";

export default function App() {
  return (
    <>
      <AppRouter />
      <Toaster />
    </>
  );
}
