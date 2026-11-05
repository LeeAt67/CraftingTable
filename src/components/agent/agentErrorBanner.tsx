import { AlertCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { AgentError } from "@/models/agentContracts";

export function AgentErrorBanner({
  error,
  onRetry,
}: {
  error: AgentError;
  onRetry: () => void;
}) {
  return (
    <div
      role="alert"
      className="mx-auto mt-3 flex w-[calc(100%-1.5rem)] max-w-[760px] items-start gap-2.5 rounded-xl border border-destructive/30 bg-surface px-3.5 py-3 min-[768px]:w-[calc(100%-2.5rem)]"
    >
      <AlertCircle
        aria-hidden="true"
        size={16}
        className="mt-0.5 shrink-0 text-destructive"
      />
      <div className="min-w-0 flex-1">
        <p className="text-sm text-textPrimary">{error.message}</p>
        {error.retryable && (
          <Button
            type="button"
            variant="link"
            size="sm"
            onClick={onRetry}
            className="mt-1 h-auto p-0"
          >
            重试
          </Button>
        )}
      </div>
    </div>
  );
}
