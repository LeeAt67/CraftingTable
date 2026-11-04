import type { Item } from "@/models/contracts";

export function SummaryStrip({ items }: { items: Item[] }) {
  const tasks = items.filter((item) => item.kind === "task").length;
  const events = items.filter((item) => item.kind === "event").length;
  const focusMinutes = Math.round(
    items
      .filter((item) => item.kind === "event" && item.startAt && item.endAt)
      .reduce(
        (total, item) =>
          total +
          (new Date(item.endAt!).getTime() - new Date(item.startAt!).getTime()) /
            60000,
        0,
      ),
  );

  return (
    <dl className="mt-8 grid grid-cols-3 divide-x divide-border rounded-lg border border-border bg-surface">
      <div className="p-3 min-[768px]:p-4">
        <dt className="text-xs text-textSecondary">待办</dt>
        <dd className="mt-1 text-lg font-semibold text-textPrimary">{tasks}</dd>
      </div>
      <div className="p-3 min-[768px]:p-4">
        <dt className="text-xs text-textSecondary">日程</dt>
        <dd className="mt-1 text-lg font-semibold text-textPrimary">{events}</dd>
      </div>
      <div className="p-3 min-[768px]:p-4">
        <dt className="text-xs text-textSecondary">专注</dt>
        <dd className="mt-1 text-lg font-semibold text-textPrimary">
          {focusMinutes} 分钟
        </dd>
      </div>
    </dl>
  );
}
