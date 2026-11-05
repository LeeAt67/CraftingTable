import { useEffect, useRef, useState } from "react";
import { cn } from "@/components/utils";
import type { SaveStatus } from "@/controllers/stores/docsStore";

type Props = {
  value: string;
  status: SaveStatus;
  onChange: (value: string) => void;
  onRetry?: () => void;
};

const MIN_SAVING_VISIBLE = 500;

const statusText: Record<SaveStatus, string> = {
  idle: "",
  saving: "保存中…",
  saved: "已保存",
  error: "保存失败，点击重试",
};

export function DocTitleInput({ value, status, onChange, onRetry }: Props) {
  const [visibleStatus, setVisibleStatus] = useState<SaveStatus>(status);
  const savingUntil = useRef(0);

  useEffect(() => {
    if (status === "saving") {
      savingUntil.current = Date.now() + MIN_SAVING_VISIBLE;
      setVisibleStatus("saving");
      return;
    }
    const remaining = savingUntil.current - Date.now();
    if (remaining <= 0) {
      setVisibleStatus(status);
      return;
    }
    const timer = setTimeout(() => setVisibleStatus(status), remaining);
    return () => clearTimeout(timer);
  }, [status]);

  return (
    <div className="flex flex-wrap items-center gap-3">
      <input
        aria-label="文档标题"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder="无标题文档"
        className="min-w-0 flex-1 border-none bg-transparent text-2xl font-semibold text-textPrimary outline-none placeholder:text-textSecondary"
      />
      <span
        role="status"
        className={cn(
          "text-xs",
          visibleStatus === "error" ? "text-destructive" : "text-textSecondary",
        )}
      >
        {visibleStatus === "error" ? (
          <button
            type="button"
            onClick={onRetry}
            className="rounded text-destructive underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
          >
            {statusText.error}
          </button>
        ) : (
          statusText[visibleStatus]
        )}
      </span>
    </div>
  );
}
