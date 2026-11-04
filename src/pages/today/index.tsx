import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { ItemTimeline } from "@/components/today/itemTimeline";
import { QuickCapture } from "@/components/today/quickCapture";
import { SummaryStrip } from "@/components/today/summaryStrip";
import { TodayHeader } from "@/components/today/todayHeader";
import { useTodayEffect } from "@/controllers/effects/useTodayEffect";
import { useTodayStore } from "@/controllers/stores/todayStore";
import type { Item } from "@/models/contracts";

export default function TodayPage() {
  const [open, setOpen] = useState(false);
  const { items, categories, phase, errorMessage } = useTodayStore();
  const { createItem, completeItem, uncompleteItem, retryToday } =
    useTodayEffect();

  const toggleItem = (item: Item) => {
    void (item.status === "done"
      ? uncompleteItem(item.id)
      : completeItem(item.id));
  };

  return (
    <section className="mx-auto w-full max-w-5xl px-4 py-8 min-[768px]:px-8">
      <TodayHeader onCreate={() => setOpen(true)} />

      {phase === "loading" && (
        <div className="mt-8 space-y-4">
          <Skeleton className="h-20 w-full" />
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-24 w-full" />
        </div>
      )}

      {phase === "error" && (
        <div className="mt-8 rounded-lg border border-border bg-surface p-6">
          <p className="text-sm font-medium text-textPrimary">{errorMessage}</p>
          <Button
            className="mt-4"
            variant="outline"
            onClick={() => void retryToday()}
          >
            重试
          </Button>
        </div>
      )}

      {(phase === "ready" || phase === "idle") && (
        <>
          <SummaryStrip items={items} />
          <div className="mt-4 flex flex-wrap gap-2">
            {categories.map((category) => (
              <span
                key={category.id}
                className="rounded-full border border-border bg-surface px-3 py-1 text-xs text-textBody"
              >
                {category.name}
              </span>
            ))}
          </div>
          <ItemTimeline items={items} onToggle={toggleItem} />
        </>
      )}

      <QuickCapture
        open={open}
        onOpenChange={setOpen}
        onCreate={(title) => createItem({ title })}
      />
    </section>
  );
}
