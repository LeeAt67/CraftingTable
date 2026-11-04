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
      className="mx-3 mt-3 flex items-start gap-3 rounded-lg border border-destructive/40 bg-surface px-3 py-2.5 min-[768px]:mx-8"
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
