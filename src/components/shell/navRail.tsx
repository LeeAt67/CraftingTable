import { Home, Moon, Settings, Sun } from "lucide-react";
import { NavLink } from "react-router-dom";
import { cn } from "@/components/utils";
import { useThemeEffect } from "@/controllers/effects/useThemeEffect";

export function NavRail() {
  const { theme, toggleTheme } = useThemeEffect();

  return (
    <aside
      aria-label="主导航"
      className="flex h-full w-[34px] shrink-0 flex-col items-center border-r border-border bg-surface py-3 min-[461px]:w-[44px] min-[768px]:w-[52px] min-[1280px]:w-14"
    >
      <div className="mb-8 grid size-7 place-items-center rounded-md bg-brand text-xs font-bold text-white">
        L
      </div>
      <nav className="flex w-full flex-col items-center gap-2">
        <NavLink
          to="/"
          aria-label="首页"
          className={({ isActive }) =>
            cn(
              "grid size-9 place-items-center rounded-md text-textSecondary transition-colors hover:bg-surfaceSubtle hover:text-textPrimary",
              isActive && "bg-brandSoft text-brand",
            )
          }
        >
          <Home aria-hidden="true" size={18} />
        </NavLink>
      </nav>
      <NavLink
        to="/settings"
        aria-label="设置"
        className={({ isActive }) =>
          cn(
            "mt-auto grid size-9 place-items-center rounded-md text-textSecondary hover:bg-surfaceSubtle hover:text-textPrimary",
            isActive && "bg-brandSoft text-brand",
          )
        }
      >
        <Settings aria-hidden="true" size={18} />
      </NavLink>
      <button
        type="button"
        aria-label={theme === "dark" ? "切换到浅色主题" : "切换到深色主题"}
        onClick={toggleTheme}
        className="mt-2 grid size-9 place-items-center rounded-md text-textSecondary hover:bg-surfaceSubtle hover:text-textPrimary"
      >
        {theme === "dark" ? (
          <Sun aria-hidden="true" size={18} />
        ) : (
          <Moon aria-hidden="true" size={18} />
        )}
      </button>
    </aside>
  );
}
