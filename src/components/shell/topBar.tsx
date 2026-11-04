export function TopBar() {
  return (
    <header className="flex h-12 shrink-0 items-center gap-2 border-b border-border bg-surface px-4">
      <span className="grid size-6 place-items-center rounded-md bg-textPrimary text-[10px] font-bold text-surface">
        L
      </span>
      <span className="text-xs font-semibold tracking-[0.16em] text-textPrimary">
        LEEAT
      </span>
      <span className="text-xs text-textSecondary">/</span>
      <span className="text-xs text-textSecondary">首页</span>
    </header>
  );
}
