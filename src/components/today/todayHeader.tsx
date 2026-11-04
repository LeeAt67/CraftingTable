import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";

export function TodayHeader({ onCreate }: { onCreate: () => void }) {
  const date = new Intl.DateTimeFormat("zh-CN", {
    month: "long",
    day: "numeric",
    weekday: "long",
  }).format(new Date());

  return (
    <header className="flex items-start justify-between gap-4">
      <div>
        <p className="text-xs text-textSecondary">{date}</p>
        <h1 className="mt-1 text-2xl font-semibold text-textPrimary">今日</h1>
      </div>
      <Button aria-label="新建待办" onClick={onCreate}>
        <Plus aria-hidden="true" size={16} />
        <span>新建</span>
      </Button>
    </header>
  );
}
