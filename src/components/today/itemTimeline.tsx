import { CalendarDays, Check, Circle } from "lucide-react";
import type { Item } from "@/models/contracts";

type Props = {
  items: Item[];
  onToggle: (item: Item) => void;
};

const time = (value?: string) =>
  value
    ? new Intl.DateTimeFormat("zh-CN", {
        hour: "2-digit",
        minute: "2-digit",
      }).format(new Date(value))
    : "随时";

export function ItemTimeline({ items, onToggle }: Props) {
  if (items.length === 0) {
    return (
      <div className="mt-8 rounded-lg border border-dashed border-border bg-surface px-6 py-14 text-center">
        <p className="text-sm font-medium text-textPrimary">今天还没有安排</p>
        <p className="mt-1 text-xs text-textSecondary">
          从新建待办或 Quick Capture 开始。
        </p>
      </div>
    );
  }

  return (
    <ol className="relative mt-6 space-y-3 before:absolute before:bottom-3 before:left-[7px] before:top-3 before:w-px before:bg-border">
      {items.map((item) => (
        <li
          key={item.id}
          className="relative grid grid-cols-[16px_minmax(0,1fr)] gap-3"
        >
          <span className="mt-4 size-[15px] rounded-full border-2 border-surface bg-brand ring-1 ring-border" />
          <article className="min-w-0 rounded-lg border border-border bg-surface p-4">
            <div className="flex items-start gap-3">
              <button
                type="button"
                aria-label={
                  item.status === "done"
                    ? `取消完成 ${item.title}`
                    : `完成 ${item.title}`
                }
                aria-pressed={item.status === "done"}
                onClick={() => onToggle(item)}
                className="mt-0.5 text-textSecondary hover:text-brand"
              >
                {item.status === "done" ? (
                  <Check aria-hidden="true" size={17} />
                ) : (
                  <Circle aria-hidden="true" size={17} />
                )}
              </button>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2 text-xs text-textSecondary">
                  {item.kind === "event" && (
                    <CalendarDays aria-hidden="true" size={13} />
                  )}
                  <span>{time(item.startAt)}</span>
                </div>
                <p
                  className={`mt-1 break-words text-sm font-medium ${
                    item.status === "done"
                      ? "text-textSecondary line-through"
                      : "text-textPrimary"
                  }`}
                >
                  {item.title}
                </p>
              </div>
            </div>
          </article>
        </li>
      ))}
    </ol>
  );
}
