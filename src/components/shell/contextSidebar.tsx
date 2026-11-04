const filters = [
  { label: "全部", count: 8 },
  { label: "已安排", count: 5 },
  { label: "已完成", count: 3 },
];

const categories = [
  { label: "工作", color: "#5468FF" },
  { label: "学习", color: "#00B42A" },
  { label: "生活", color: "#F59E0B" },
];

export function ContextSidebar() {
  return (
    <aside
      aria-label="今日上下文"
      className="h-full w-[88px] shrink-0 border-r border-border bg-surfaceSubtle px-2 py-5 min-[461px]:w-[104px] min-[768px]:w-[184px] min-[1280px]:w-[232px] min-[768px]:px-4"
    >
      <div className="mb-6">
        <p className="truncate text-sm font-semibold text-textPrimary">今日</p>
        <p className="mt-1 hidden text-xs text-textSecondary min-[768px]:block">
          2026年11月4日
        </p>
      </div>
      <nav aria-label="今日筛选" className="space-y-1">
        {filters.map((filter, index) => (
          <button
            key={filter.label}
            className={`flex w-full items-center justify-between rounded-md px-2 py-2 text-xs min-[768px]:text-sm ${
              index === 0
                ? "bg-brandSoft text-brand"
                : "text-textSecondary hover:bg-surface"
            }`}
          >
            <span className="truncate">{filter.label}</span>
            <span className="hidden min-[768px]:inline">{filter.count}</span>
          </button>
        ))}
      </nav>
      <p className="mb-2 mt-7 hidden text-[11px] font-medium uppercase tracking-wider text-textSecondary min-[768px]:block">
        分类
      </p>
      <div className="space-y-1">
        {categories.map((category) => (
          <button
            key={category.label}
            className="flex w-full items-center gap-2 rounded-md px-2 py-2 text-xs text-textSecondary hover:bg-surface min-[768px]:text-sm"
          >
            <span
              className="size-2 shrink-0 rounded-full"
              style={{ backgroundColor: category.color }}
            />
            <span className="truncate">{category.label}</span>
          </button>
        ))}
      </div>
    </aside>
  );
}
