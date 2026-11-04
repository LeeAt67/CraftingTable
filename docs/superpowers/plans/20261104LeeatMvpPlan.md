# Leeat 个人工作台 MVP Implementation Plan

> **状态：已取代。** 本计划中的 Fastify、Node.js SQLite、无 Agent 范围和相关实施步骤只保留为历史记录。当前设计以 [工程化设计规范](../specs/2026-11-05-craftingtable-engineering-design.md) 和 [Agent 设计](../specs/2026-11-05-craftingtable-agent-design.md) 为准，不能直接按本计划继续实施。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付一个可运行的 Leeat 单用户个人工作台 MVP，完成今日时间流、快捷新建、完成状态、Light / Dark 主题和最小 Fastify SQLite API。

**Architecture:** 前端使用 React MVC。Model 负责数据读写，Controller 固定拆为 Zustand Store 和调用 Model 的 Effect，View 只读取 Store 并调用 Effect。浏览器通过 Fastify API 访问 SQLite；后续 Electron 只替换为 IpcModel，不修改 Store 和 View。

**Tech Stack:** React、TypeScript、Vite、Tailwind CSS、shadcn/ui、React Router、Zustand、Lucide React、Zod、Vitest、Testing Library、Playwright、Fastify 5、Node.js `node:sqlite`。

## Global Constraints

- 项目名固定为 `Leeat`，用户名为 `leeat`。
- 一级导航只有首页 `/` 和设置 `/settings`。
- 首页承载“今日”完整内容，设置页只做占位。
- 不实现 Agent、多 Agent、AI 对话、搜索、同步、账号、权限或额外事务页面。
- 页面使用 `HashRouter`，浏览器地址可显示 `/#/settings`。
- Controller 固定拆为 `controllers/effects` 和 `controllers/stores`。
- Store 只保存同步状态，不调用 Model、不执行 `fetch`、不显示 Toast。
- Effect 负责异步动作、调用 Model、错误处理和 Toast，不保存长期状态。
- View 不直接请求 API，只通过 Effect 和 Store 工作。
- API 只使用 `GET` 和 `POST`，禁止 `PUT`、`PATCH`、`DELETE`。
- 前端和后端自建文件、目录使用 camelCase 或 PascalCase，不使用连字符。
- Node.js 使用 24 LTS，TypeScript 开启严格模式。
- 当前 MVP 不安装 Electron，不生成桌面安装包。
- Light Token：`background #F2F5F8`、`surface #FFFFFF`、`textPrimary #1D2129`、`textSecondary #86909C`、`border #D8DEE9`、`brand #5468FF`。
- Dark Token：`background #17191D`、`surface #1E2126`、`textPrimary #F1F3F5`、`textSecondary #969CA6`、`border #30353C`、`brand #7B8AFF`。
- 响应式宽度：一级导航 56 / 52 / 44 / 34px，二级侧栏 232 / 184 / 104 / 88px。
- 二级侧栏不能消失，窄屏压缩为窄栏。
- 390px、768px、1024px、1440px 都不能出现全局横向滚动。
- Dialog 支持 Escape 关闭，图标按钮必须有 `aria-label`，当前导航使用 `aria-current="page"`。

## File Structure

```text
src/
  components/
    shell/
      appShell.tsx
      contextSidebar.tsx
      navRail.tsx
    today/
      itemTimeline.tsx
      quickCapture.tsx
      summaryStrip.tsx
      todayHeader.tsx
    ui/
      button.tsx
      dialog.tsx
      input.tsx
      skeleton.tsx
      sonner.tsx
  controllers/
    effects/
      useThemeEffect.ts
      useTodayEffect.ts
    stores/
      themeStore.ts
      todayStore.ts
  layouts/
    appLayout.tsx
  models/
    apiModel.ts
    contracts.ts
    mockModel.ts
    todayModel.ts
  pages/
    settings/
      index.tsx
    today/
      index.tsx
  router/
    index.tsx
  styles/
    globals.css
    tokens.css
  app.tsx
  main.tsx
  testSetup.ts

api/
  migrations/
    001Initial.sql
  src/
    controllers/
      categoryController.ts
      itemController.ts
      todayController.ts
    models/
      categoryModel.ts
      db.ts
      itemModel.ts
    views/
      jsonView.ts
    app.ts
    routes.ts
    server.ts
tests/
  e2e/
    modules/
      home/
      layout/
      settings/
      theme/
      today/
    unit/
      app/
      models/
      theme/
    support/
```

- `models` 只处理契约和 I/O。
- `stores` 只保存状态和同步动作。
- `effects` 只编排异步流程。
- `components`、`pages`、`layouts` 只处理 View。
- `api/src/models` 只处理 SQL。
- `api/src/controllers` 只校验、编排和返回结果。
- `api/src/views` 只定义 JSON envelope。
- 所有测试统一位于 `tests/e2e`；浏览器测试使用 `.e2e.ts`，Vitest 测试使用 `.test.ts` 或 `.test.tsx`。

### Task 1: Scaffold React, Tailwind and Test Harness

**Files:**
- Create: `package.json`
- Create: `index.html`
- Create: `vite.config.ts`
- Create: `vitest.config.ts`
- Create: `tsconfig.json`
- Create: `tailwind.config.ts`
- Create: `postcss.config.cjs`
- Create: `.gitignore`
- Create: `src/testSetup.ts`
- Create: `src/main.tsx`
- Create: `src/app.tsx`
- Create: `src/app.test.tsx`
- Create: `src/styles/tokens.css`
- Create: `src/styles/globals.css`

**Interfaces:**
- Consumes: Node.js 24 LTS and pnpm.
- Produces: `pnpm dev:web`, `pnpm typecheck`, `pnpm test`, and a Vite entry that renders `<App />`.

- [ ] **Step 1: Verify the runtime**

Run: `node --version`
Expected: `v24.x.x`. If it is lower, run `pnpm env use --global 24`, open a new shell, and run `node --version` again.

- [ ] **Step 2: Write the failing smoke test**

Create `src/app.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import App from "./app";

describe("App", () => {
  it("renders the Leeat brand", () => {
    render(<App />);
    expect(screen.getByText("LEEAT")).toBeInTheDocument();
  });
});
```

- [ ] **Step 3: Run the smoke test to confirm it fails**

Run: `pnpm vitest run src/app.test.tsx`
Expected: FAIL because `src/app.tsx` and the test configuration do not exist.

- [ ] **Step 4: Create the package and tool configuration**

Create `package.json`:

```json
{
  "name": "leeat",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "engines": {
    "node": ">=24"
  },
  "scripts": {
    "dev": "concurrently -k \"pnpm dev:api\" \"pnpm dev:web\"",
    "dev:web": "vite",
    "dev:api": "tsx watch api/src/server.ts",
    "build": "tsc --noEmit && vite build",
    "preview": "vite preview",
    "typecheck": "tsc --noEmit --pretty false",
    "test": "vitest run",
    "test:watch": "vitest",
    "test:e2e": "playwright test"
  },
  "dependencies": {
    "@radix-ui/react-dialog": "^1.1.15",
    "@radix-ui/react-slot": "^1.2.3",
    "class-variance-authority": "^0.7.1",
    "clsx": "^2.1.1",
    "fastify": "^5.6.1",
    "lucide-react": "^0.468.0",
    "react": "^19.1.0",
    "react-dom": "^19.1.0",
    "react-router-dom": "^7.9.0",
    "sonner": "^2.0.0",
    "tailwind-merge": "^3.3.0",
    "zod": "^3.25.0",
    "zustand": "^5.0.0"
  },
  "devDependencies": {
    "@playwright/test": "^1.55.0",
    "@testing-library/jest-dom": "^6.8.0",
    "@testing-library/react": "^16.3.0",
    "@testing-library/user-event": "^14.6.1",
    "@types/node": "^24.3.0",
    "@types/react": "^19.1.0",
    "@types/react-dom": "^19.1.0",
    "@vitejs/plugin-react": "^5.0.0",
    "autoprefixer": "^10.4.21",
    "concurrently": "^9.2.0",
    "jsdom": "^26.1.0",
    "postcss": "^8.5.6",
    "tailwindcss": "^3.4.17",
    "tailwindcss-animate": "^1.0.7",
    "tsx": "^4.20.0",
    "typescript": "^5.9.0",
    "vite": "^7.1.0",
    "vitest": "^3.2.0"
  }
}
```

Create `index.html`:

```html
<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <meta name="theme-color" content="#F2F5F8" />
    <title>Leeat</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

Create `vite.config.ts`:

```ts
import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://127.0.0.1:3001",
    },
  },
});
```

Create `vitest.config.ts`:

```ts
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: ["./src/testSetup.ts"],
    css: true,
  },
});
```

Create `tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "useDefineForClassFields": true,
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "allowJs": false,
    "skipLibCheck": true,
    "esModuleInterop": true,
    "allowSyntheticDefaultImports": true,
    "strict": true,
    "forceConsistentCasingInFileNames": true,
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx",
    "types": ["vitest/globals", "node", "vite/client"],
    "baseUrl": ".",
    "paths": {
      "@/*": ["./src/*"]
    }
  },
  "include": ["src", "api", "vite.config.ts", "vitest.config.ts", "playwright.config.ts"]
}
```

Create `tailwind.config.ts`:

```ts
import animate from "tailwindcss-animate";
import type { Config } from "tailwindcss";

export default {
  darkMode: ["class", "[data-theme='dark']"],
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        background: "var(--background)",
        foreground: "var(--textPrimary)",
        surface: "var(--surface)",
        surfaceSubtle: "var(--surfaceSubtle)",
        textPrimary: "var(--textPrimary)",
        textBody: "var(--textBody)",
        textSecondary: "var(--textSecondary)",
        border: "var(--border)",
        brand: "var(--brand)",
        brandHover: "var(--brandHover)",
        brandSoft: "var(--brandSoft)",
        success: "var(--success)",
        card: {
          DEFAULT: "var(--surface)",
          foreground: "var(--textPrimary)",
        },
        popover: {
          DEFAULT: "var(--surface)",
          foreground: "var(--textPrimary)",
        },
        primary: {
          DEFAULT: "var(--brand)",
          foreground: "#FFFFFF",
        },
        secondary: {
          DEFAULT: "var(--surfaceSubtle)",
          foreground: "var(--textPrimary)",
        },
        muted: {
          DEFAULT: "var(--surfaceSubtle)",
          foreground: "var(--textSecondary)",
        },
        accent: {
          DEFAULT: "var(--brandSoft)",
          foreground: "var(--brand)",
        },
        destructive: {
          DEFAULT: "var(--destructive)",
          foreground: "#FFFFFF",
        },
        input: "var(--border)",
        ring: "var(--brand)",
        ringOffset: "var(--background)",
      },
      borderRadius: {
        lg: "var(--radius)",
        md: "calc(var(--radius) - 2px)",
        sm: "calc(var(--radius) - 4px)",
      },
    },
  },
  plugins: [animate],
} satisfies Config;
```

Create `postcss.config.cjs`:

```js
module.exports = {
  plugins: {
    tailwindcss: {},
    autoprefixer: {},
  },
};
```

Create `.gitignore`:

```text
node_modules/
dist/
coverage/
playwright-report/
test-results/
*.sqlite
*.sqlite-shm
*.sqlite-wal
```

- [ ] **Step 5: Create the minimal React entry and token styles**

Create `src/testSetup.ts`:

```ts
import "@testing-library/jest-dom/vitest";
```

Create `src/styles/tokens.css`:

```css
:root {
  --background: #F2F5F8;
  --surface: #FFFFFF;
  --surfaceSubtle: #F7F8FA;
  --textPrimary: #1D2129;
  --textBody: #4E5969;
  --textSecondary: #86909C;
  --border: #D8DEE9;
  --brand: #5468FF;
  --brandHover: #7080FF;
  --brandSoft: #EEF0FF;
  --success: #00B42A;
  --destructive: #D92D20;
  --radius: 8px;
}

[data-theme="dark"] {
  --background: #17191D;
  --surface: #1E2126;
  --surfaceSubtle: #25292F;
  --textPrimary: #F1F3F5;
  --textBody: #C9CDD4;
  --textSecondary: #969CA6;
  --border: #30353C;
  --brand: #7B8AFF;
  --brandHover: #98A4FF;
  --brandSoft: rgba(123, 138, 255, 0.14);
  --success: #46D16C;
  --destructive: #F97066;
}
```

Create `src/styles/globals.css`:

```css
@import "./tokens.css";

@tailwind base;
@tailwind components;
@tailwind utilities;

* {
  box-sizing: border-box;
}

html,
body,
#root {
  min-height: 100%;
}

body {
  margin: 0;
  background: var(--background);
  color: var(--textPrimary);
  font-family: Inter, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
}

button,
input {
  font: inherit;
}

:focus-visible {
  outline: 2px solid var(--brand);
  outline-offset: 2px;
}

@media (prefers-reduced-motion: reduce) {
  *,
  *::before,
  *::after {
    scroll-behavior: auto !important;
    transition-duration: 0.01ms !important;
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
  }
}
```

Create `src/app.tsx`:

```tsx
export default function App() {
  return (
    <main className="min-h-screen bg-background p-8 text-textPrimary">
      <p className="text-xs font-semibold tracking-[0.2em]">LEEAT</p>
    </main>
  );
}
```

Create `src/main.tsx`:

```tsx
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./app";
import "./styles/globals.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
```

- [ ] **Step 6: Install dependencies and run the smoke test**

Run: `pnpm install`
Run: `pnpm vitest run src/app.test.tsx`
Expected: PASS with one test.

- [ ] **Step 7: Verify the production build**

Run: `pnpm build`
Expected: TypeScript and Vite complete successfully.

- [ ] **Step 8: Commit**

```powershell
git add package.json pnpm-lock.yaml index.html vite.config.ts vitest.config.ts tsconfig.json tailwind.config.ts postcss.config.cjs .gitignore src
git commit -m "chore: scaffold Leeat frontend"
```

### Task 2: Add shadcn Primitives and Domain Contracts

**Files:**
- Create: `components.json`
- Create: `src/models/contracts.ts`
- Create: `src/models/todayModel.ts`
- Create: `src/models/mockModel.ts`
- Create: `src/components/ui/button.tsx`
- Create: `src/components/ui/dialog.tsx`
- Create: `src/components/ui/input.tsx`
- Create: `src/components/ui/skeleton.tsx`
- Create: `src/components/ui/sonner.tsx`
- Modify: `src/app.tsx`
- Test: `src/models/mockModel.test.ts`

**Interfaces:**
- Consumes: the Vite project from Task 1.
- Produces: `Item`, `Category`, `CreateItemInput`, `UpdateItemInput`, `TodayModel`, and `todayModel`.
- Produces: shadcn components imported from `@/components/ui/*`.

- [ ] **Step 1: Write the failing MockModel test**

Create `src/models/mockModel.test.ts`:

```ts
import { beforeEach, describe, expect, it } from "vitest";
import { createMockModel } from "./mockModel";

describe("MockModel", () => {
  let model: ReturnType<typeof createMockModel>;

  beforeEach(() => {
    model = createMockModel();
  });

  it("creates an unplanned task", async () => {
    const item = await model.createItem({ title: "整理周报" });
    expect(item).toMatchObject({
      kind: "task",
      title: "整理周报",
      status: "unplanned",
    });
  });

  it("completes and uncompletes a task", async () => {
    const item = await model.createItem({ title: "散步" });
    const done = await model.completeItem(item.id);
    expect(done.status).toBe("done");
    const undone = await model.uncompleteItem(item.id);
    expect(undone.status).toBe("unplanned");
  });
});
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `pnpm vitest run src/models/mockModel.test.ts`
Expected: FAIL because `mockModel.ts` does not exist.

- [ ] **Step 3: Configure shadcn and add components**

Create `components.json`:

```json
{
  "$schema": "https://ui.shadcn.com/schema.json",
  "style": "new-york",
  "rsc": false,
  "tsx": true,
  "tailwind": {
    "config": "tailwind.config.ts",
    "css": "src/styles/globals.css",
    "baseColor": "neutral",
    "cssVariables": false
  },
  "aliases": {
    "components": "@/components",
    "utils": "@/components/utils",
    "ui": "@/components/ui",
    "lib": "@/lib",
    "hooks": "@/hooks"
  }
}
```

Modify `tsconfig.json` to add:

```json
"baseUrl": ".",
"paths": {
  "@/*": ["./src/*"]
}
```

Create `src/components/utils.ts`:

```ts
import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
```

Run: `pnpm dlx shadcn@latest add button dialog input skeleton --yes`
Expected: shadcn creates the four components under `src/components/ui`.

Create `src/components/ui/sonner.tsx` without `next-themes`:

```tsx
import type { ComponentProps } from "react";
import { Toaster as Sonner } from "sonner";

export function Toaster(props: ComponentProps<typeof Sonner>) {
  return <Sonner theme="system" richColors {...props} />;
}
```

- [ ] **Step 4: Write the domain contracts**

Create `src/models/contracts.ts`:

```ts
export type ItemKind = "task" | "event";
export type ItemStatus = "unplanned" | "planned" | "done";

export type Category = {
  id: string;
  name: string;
  color: string;
};

export type Item = {
  id: string;
  kind: ItemKind;
  title: string;
  description?: string;
  status: ItemStatus;
  categoryId?: string;
  startAt?: string;
  endAt?: string;
  completedAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type CreateItemInput = {
  title: string;
  categoryId?: string;
  startAt?: string;
  endAt?: string;
};

export type UpdateItemInput = {
  id: string;
  title?: string;
  description?: string;
  categoryId?: string;
  startAt?: string;
  endAt?: string;
};

export interface TodayModel {
  listToday(): Promise<Item[]>;
  listCategories(): Promise<Category[]>;
  createItem(input: CreateItemInput): Promise<Item>;
  updateItem(input: UpdateItemInput): Promise<Item>;
  completeItem(id: string): Promise<Item>;
  uncompleteItem(id: string): Promise<Item>;
}
```

Create `src/models/mockModel.ts`:

```ts
import type {
  Category,
  CreateItemInput,
  Item,
  TodayModel,
  UpdateItemInput,
} from "./contracts";

const categories: Category[] = [
  { id: "work", name: "工作", color: "#5468FF" },
  { id: "study", name: "学习", color: "#00B42A" },
  { id: "life", name: "生活", color: "#F59E0B" },
];

const now = () => new Date().toISOString();

export function createMockModel(): TodayModel {
  let items: Item[] = [
    {
      id: "item-1",
      kind: "task",
      title: "整理本周工作记录",
      status: "planned",
      categoryId: "work",
      startAt: new Date().toISOString(),
      createdAt: now(),
      updatedAt: now(),
    },
    {
      id: "item-2",
      kind: "event",
      title: "产品方案同步",
      status: "planned",
      categoryId: "work",
      startAt: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
      endAt: new Date(Date.now() + 90 * 60 * 1000).toISOString(),
      createdAt: now(),
      updatedAt: now(),
    },
  ];

  return {
    async listToday() {
      return items;
    },
    async listCategories() {
      return categories;
    },
    async createItem(input: CreateItemInput) {
      const item: Item = {
        id: crypto.randomUUID(),
        kind: "task",
        title: input.title.trim(),
        status: input.startAt ? "planned" : "unplanned",
        categoryId: input.categoryId,
        startAt: input.startAt,
        endAt: input.endAt,
        createdAt: now(),
        updatedAt: now(),
      };
      items = [item, ...items];
      return item;
    },
    async updateItem(input: UpdateItemInput) {
      const current = items.find((item) => item.id === input.id);
      if (!current) throw new Error("Item not found");
      const item = { ...current, ...input, updatedAt: now() };
      items = items.map((value) => (value.id === item.id ? item : value));
      return item;
    },
    async completeItem(id: string) {
      const current = items.find((item) => item.id === id);
      if (!current) throw new Error("Item not found");
      const item = {
        ...current,
        status: "done" as const,
        completedAt: now(),
        updatedAt: now(),
      };
      items = items.map((value) => (value.id === id ? item : value));
      return item;
    },
    async uncompleteItem(id: string) {
      const current = items.find((item) => item.id === id);
      if (!current) throw new Error("Item not found");
      const item = {
        ...current,
        status: current.startAt ? ("planned" as const) : ("unplanned" as const),
        completedAt: undefined,
        updatedAt: now(),
      };
      items = items.map((value) => (value.id === id ? item : value));
      return item;
    },
  };
}

export const mockModel = createMockModel();
```

- [ ] **Step 5: Add the model selector**

Create `src/models/todayModel.ts`:

```ts
import type { TodayModel } from "./contracts";
import { apiModel } from "./apiModel";
import { mockModel } from "./mockModel";

export const todayModel: TodayModel =
  import.meta.env.VITE_DATA_SOURCE === "api" ? apiModel : mockModel;
```

Create a temporary `src/models/apiModel.ts` so the import resolves:

```ts
import type { TodayModel } from "./contracts";

const unavailable = () => Promise.reject(new Error("ApiModel is not configured"));

export const apiModel: TodayModel = {
  listToday: unavailable,
  listCategories: unavailable,
  createItem: unavailable,
  updateItem: unavailable,
  completeItem: unavailable,
  uncompleteItem: unavailable,
};
```

- [ ] **Step 6: Render Toaster and verify the tests**

Modify `src/app.tsx`:

```tsx
import { Toaster } from "@/components/ui/sonner";

export default function App() {
  return (
    <>
      <main className="min-h-screen bg-background p-8 text-textPrimary">
        <p className="text-xs font-semibold tracking-[0.2em]">LEEAT</p>
      </main>
      <Toaster />
    </>
  );
}
```

Run: `pnpm vitest run src/models/mockModel.test.ts src/app.test.tsx`
Expected: PASS with three tests.

- [ ] **Step 7: Commit**

```powershell
git add components.json src/components src/models
git commit -m "feat: add shadcn primitives and model contracts"
```

### Task 3: Implement Theme Store and Theme Effect

**Files:**
- Create: `src/controllers/stores/themeStore.ts`
- Create: `src/controllers/effects/useThemeEffect.ts`
- Modify: `src/components/ui/sonner.tsx`
- Test: `src/controllers/stores/themeStore.test.ts`

**Interfaces:**
- Consumes: Zustand and the token CSS from Task 1.
- Produces: `useThemeStore`, `type Theme`, and `useThemeEffect()`.
- Produces: `window.localStorage` key `leeat-theme`.

- [ ] **Step 1: Write the failing Store test**

Create `src/controllers/stores/themeStore.test.ts`:

```ts
import { beforeEach, describe, expect, it } from "vitest";
import { useThemeStore } from "./themeStore";

describe("themeStore", () => {
  beforeEach(() => {
    window.localStorage.clear();
    useThemeStore.setState({ theme: "light" });
  });

  it("switches from light to dark", () => {
    useThemeStore.getState().toggleTheme();
    expect(useThemeStore.getState().theme).toBe("dark");
  });

  it("sets an explicit theme", () => {
    useThemeStore.getState().setTheme("dark");
    expect(useThemeStore.getState().theme).toBe("dark");
  });
});
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `pnpm vitest run src/controllers/stores/themeStore.test.ts`
Expected: FAIL because `themeStore.ts` does not exist.

- [ ] **Step 3: Implement the Store**

Create `src/controllers/stores/themeStore.ts`:

```ts
import { create } from "zustand";
import { persist } from "zustand/middleware";

export type Theme = "light" | "dark";

type ThemeStore = {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  toggleTheme: () => void;
};

export const useThemeStore = create<ThemeStore>()(
  persist(
    (set) => ({
      theme: "light",
      setTheme: (theme) => set({ theme }),
      toggleTheme: () =>
        set((state) => ({ theme: state.theme === "light" ? "dark" : "light" })),
    }),
    { name: "leeat-theme" },
  ),
);
```

- [ ] **Step 4: Implement the Effect**

Create `src/controllers/effects/useThemeEffect.ts`:

```ts
import { useEffect } from "react";
import { useThemeStore } from "@/controllers/stores/themeStore";

export function useThemeEffect() {
  const theme = useThemeStore((state) => state.theme);
  const setTheme = useThemeStore((state) => state.setTheme);
  const toggleTheme = useThemeStore((state) => state.toggleTheme);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    document.documentElement.classList.toggle("dark", theme === "dark");
  }, [theme]);

  return { theme, setTheme, toggleTheme };
}
```

Modify `src/components/ui/sonner.tsx` so toast appearance follows the app theme:

```tsx
import type { ComponentProps } from "react";
import { Toaster as Sonner } from "sonner";
import { useThemeStore } from "@/controllers/stores/themeStore";

export function Toaster(props: ComponentProps<typeof Sonner>) {
  const theme = useThemeStore((state) => state.theme);
  return <Sonner theme={theme} richColors {...props} />;
}
```

- [ ] **Step 5: Run the Store test and typecheck**

Run: `pnpm vitest run src/controllers/stores/themeStore.test.ts`
Expected: PASS with two tests.
Run: `pnpm typecheck`
Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add src/controllers/stores/themeStore.ts src/controllers/effects/useThemeEffect.ts src/components/ui/sonner.tsx src/controllers/stores/themeStore.test.ts
git commit -m "feat: add persisted theme controller"
```

### Task 4: Build App Shell and HashRouter

**Files:**
- Create: `src/components/shell/navRail.tsx`
- Create: `src/components/shell/contextSidebar.tsx`
- Create: `src/components/shell/topBar.tsx`
- Create: `src/components/shell/appShell.tsx`
- Create: `src/layouts/appLayout.tsx`
- Create: `src/pages/today/index.tsx`
- Create: `src/pages/settings/index.tsx`
- Create: `src/router/index.tsx`
- Modify: `src/app.tsx`
- Test: `src/router/appRouter.test.tsx`

**Interfaces:**
- Consumes: `useThemeEffect` from Task 3.
- Produces: `AppRouter`, `AppLayout`, `AppShell`, and the routes `/` and `/settings`.
- Produces: a persistent context sidebar that never disappears.

- [ ] **Step 1: Write the failing router test**

Create `src/router/appRouter.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RouterProvider } from "react-router-dom";
import { createAppRouter } from "./index";

describe("AppRouter", () => {
  it("shows home navigation and the context sidebar", () => {
    window.location.hash = "#/";
    render(<RouterProvider router={createAppRouter()} />);
    expect(screen.getByText("LEEAT")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "今日" })).toBeInTheDocument();
    expect(screen.getByText("全部")).toBeInTheDocument();
  });

  it("opens the settings placeholder", () => {
    window.location.hash = "#/settings";
    render(<RouterProvider router={createAppRouter()} />);
    expect(screen.getByRole("heading", { name: "设置" })).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `pnpm vitest run src/router/appRouter.test.tsx`
Expected: FAIL because the router and shell do not exist.

- [ ] **Step 3: Implement the shell**

Create `src/components/shell/navRail.tsx`:

```tsx
import { Home, Moon, Settings, Sun } from "lucide-react";
import { NavLink } from "react-router-dom";
import { cn } from "@/components/utils";
import { useThemeEffect } from "@/controllers/effects/useThemeEffect";

const navItems = [
  { to: "/", label: "首页", icon: Home },
  { to: "/settings", label: "设置", icon: Settings },
] as const;

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
        {navItems.slice(0, 1).map(({ to, label, icon: Icon }) => (
          <NavLink
            key={to}
            to={to}
            aria-label={label}
            className={({ isActive }) =>
              cn(
                "grid size-9 place-items-center rounded-md text-textSecondary transition-colors hover:bg-surfaceSubtle hover:text-textPrimary",
                isActive && "bg-brandSoft text-brand",
              )
            }
          >
            <Icon aria-hidden="true" size={18} />
          </NavLink>
        ))}
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
          {theme === "dark" ? <Sun aria-hidden="true" size={18} /> : <Moon aria-hidden="true" size={18} />}
        </button>
    </aside>
  );
}
```

Create `src/components/shell/contextSidebar.tsx`:

```tsx
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
        <p className="mt-1 hidden text-xs text-textSecondary min-[768px]:block">2026年11月4日</p>
      </div>
      <nav aria-label="今日筛选" className="space-y-1">
        {filters.map((filter, index) => (
          <button
            key={filter.label}
            className={`flex w-full items-center justify-between rounded-md px-2 py-2 text-xs min-[768px]:text-sm ${
              index === 0 ? "bg-brandSoft text-brand" : "text-textSecondary hover:bg-surface"
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
            <span className="size-2 shrink-0 rounded-full" style={{ backgroundColor: category.color }} />
            <span className="truncate">{category.label}</span>
          </button>
        ))}
      </div>
    </aside>
  );
}
```

Create `src/components/shell/topBar.tsx`:

```tsx
export function TopBar() {
  return (
    <header className="flex h-12 shrink-0 items-center gap-2 border-b border-border bg-surface px-4">
      <span className="grid size-6 place-items-center rounded-md bg-textPrimary text-[10px] font-bold text-surface">
        L
      </span>
      <span className="text-xs font-semibold tracking-[0.16em] text-textPrimary">LEEAT</span>
      <span className="text-xs text-textSecondary">/</span>
      <span className="text-xs text-textSecondary">首页</span>
    </header>
  );
}
```

Create `src/components/shell/appShell.tsx`:

```tsx
import type { ReactNode } from "react";
import { ContextSidebar } from "./contextSidebar";
import { NavRail } from "./navRail";
import { TopBar } from "./topBar";

export function AppShell({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-screen min-w-0 overflow-hidden bg-background">
      <NavRail />
      <ContextSidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar />
        <main className="min-h-0 min-w-0 flex-1 overflow-y-auto">{children}</main>
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Implement placeholders, layout, router and App**

Create `src/pages/today/index.tsx`:

```tsx
export default function TodayPage() {
  return (
    <section className="mx-auto w-full max-w-5xl px-4 py-8 min-[768px]:px-8">
      <h1 className="text-2xl font-semibold text-textPrimary">今日</h1>
      <p className="mt-2 text-sm text-textSecondary">今天还没有安排。</p>
    </section>
  );
}
```

Create `src/pages/settings/index.tsx`:

```tsx
import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";

export default function SettingsPage() {
  return (
    <section className="mx-auto w-full max-w-3xl px-4 py-8 min-[768px]:px-8">
      <h1 className="text-2xl font-semibold text-textPrimary">设置</h1>
      <p className="mt-2 text-sm text-textSecondary">设置功能将在后续版本中提供。</p>
      <Button asChild className="mt-6">
        <Link to="/">返回今日</Link>
      </Button>
    </section>
  );
}
```

Create `src/layouts/appLayout.tsx`:

```tsx
import { Outlet } from "react-router-dom";
import { AppShell } from "@/components/shell/appShell";

export function AppLayout() {
  return (
    <AppShell>
      <Outlet />
    </AppShell>
  );
}
```

Create `src/router/index.tsx`:

```tsx
import { useMemo } from "react";
import { createHashRouter, RouterProvider } from "react-router-dom";
import { AppLayout } from "@/layouts/appLayout";
import SettingsPage from "@/pages/settings";
import TodayPage from "@/pages/today";

export function createAppRouter() {
  return createHashRouter([
    {
      element: <AppLayout />,
      children: [
        { path: "/", element: <TodayPage /> },
        { path: "/settings", element: <SettingsPage /> },
      ],
    },
  ]);
}

export function AppRouter() {
  const router = useMemo(() => createAppRouter(), []);
  return <RouterProvider router={router} />;
}
```

Modify `src/app.tsx`:

```tsx
import { Toaster } from "@/components/ui/sonner";
import { AppRouter } from "@/router";

export default function App() {
  return (
    <>
      <AppRouter />
      <Toaster />
    </>
  );
}
```

- [ ] **Step 5: Run the router tests and typecheck**

Run: `pnpm vitest run src/router/appRouter.test.tsx src/app.test.tsx`
Expected: PASS with three tests.
Run: `pnpm typecheck`
Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add src/components/shell src/layouts src/pages src/router src/app.tsx src/router/appRouter.test.tsx
git commit -m "feat: add responsive app shell and routes"
```

### Task 5: Implement Today Store and Today Effect

**Files:**
- Create: `src/controllers/stores/todayStore.ts`
- Create: `src/controllers/effects/useTodayEffect.ts`
- Test: `src/controllers/stores/todayStore.test.ts`
- Test: `src/controllers/effects/useTodayEffect.test.ts`

**Interfaces:**
- Consumes: `TodayModel`, `Item`, `Category` from Task 2.
- Produces: `useTodayStore`, `initialTodayState`, `resetTodayStore`, `createTodayEffect`, and `useTodayEffect`.
- Produces: `TodayPhase = "idle" | "loading" | "ready" | "error"`.

- [ ] **Step 1: Write the failing Store test**

Create `src/controllers/stores/todayStore.test.ts`:

```ts
import { beforeEach, describe, expect, it } from "vitest";
import { resetTodayStore, useTodayStore } from "./todayStore";

const item = {
  id: "1",
  kind: "task" as const,
  title: "写计划",
  status: "planned" as const,
  createdAt: "2026-11-04T00:00:00.000Z",
  updatedAt: "2026-11-04T00:00:00.000Z",
};

describe("todayStore", () => {
  beforeEach(resetTodayStore);

  it("moves from loading to ready", () => {
    useTodayStore.getState().setLoading();
    useTodayStore.getState().setReady([item], []);
    expect(useTodayStore.getState()).toMatchObject({
      phase: "ready",
      items: [item],
      categories: [],
    });
  });

  it("replaces an item without mutating the old list", () => {
    useTodayStore.getState().setReady([item], []);
    useTodayStore.getState().replaceItem({ ...item, title: "完成计划" });
    expect(useTodayStore.getState().items[0]?.title).toBe("完成计划");
  });
});
```

- [ ] **Step 2: Run the Store test to confirm it fails**

Run: `pnpm vitest run src/controllers/stores/todayStore.test.ts`
Expected: FAIL because `todayStore.ts` does not exist.

- [ ] **Step 3: Implement the Store**

Create `src/controllers/stores/todayStore.ts`:

```ts
import { create } from "zustand";
import type { Category, Item } from "@/models/contracts";

export type TodayPhase = "idle" | "loading" | "ready" | "error";

type TodayState = {
  items: Item[];
  categories: Category[];
  phase: TodayPhase;
  errorMessage: string | null;
};

type TodayActions = {
  setLoading: () => void;
  setReady: (items: Item[], categories: Category[]) => void;
  setError: (message: string) => void;
  addItem: (item: Item) => void;
  replaceItem: (item: Item) => void;
};

export const initialTodayState: TodayState = {
  items: [],
  categories: [],
  phase: "idle",
  errorMessage: null,
};

export const useTodayStore = create<TodayState & TodayActions>()((set) => ({
  ...initialTodayState,
  setLoading: () => set({ phase: "loading", errorMessage: null }),
  setReady: (items, categories) =>
    set({ items, categories, phase: "ready", errorMessage: null }),
  setError: (errorMessage) => set({ phase: "error", errorMessage }),
  addItem: (item) => set((state) => ({ items: [item, ...state.items] })),
  replaceItem: (item) =>
    set((state) => ({
      items: state.items.map((current) => (current.id === item.id ? item : current)),
    })),
}));

export function resetTodayStore() {
  useTodayStore.setState(initialTodayState);
}
```

- [ ] **Step 4: Write the failing Effect tests**

Create `src/controllers/effects/useTodayEffect.test.ts`:

```ts
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TodayModel } from "@/models/contracts";
import { resetTodayStore, useTodayStore } from "@/controllers/stores/todayStore";
import { createTodayEffect } from "./useTodayEffect";

function model(overrides: Partial<TodayModel> = {}): TodayModel {
  return {
    listToday: vi.fn().mockResolvedValue([]),
    listCategories: vi.fn().mockResolvedValue([]),
    createItem: vi.fn(),
    updateItem: vi.fn(),
    completeItem: vi.fn(),
    uncompleteItem: vi.fn(),
    ...overrides,
  };
}

describe("createTodayEffect", () => {
  beforeEach(resetTodayStore);

  it("loads items and categories into the Store", async () => {
    const current = model();
    await createTodayEffect(current).loadToday();
    expect(useTodayStore.getState().phase).toBe("ready");
    expect(current.listToday).toHaveBeenCalledOnce();
  });

  it("stores an error when loading fails", async () => {
    const current = model({
      listToday: vi.fn().mockRejectedValue(new Error("网络错误")),
    });
    await createTodayEffect(current).loadToday();
    expect(useTodayStore.getState()).toMatchObject({
      phase: "error",
      errorMessage: "网络错误",
    });
  });
});
```

- [ ] **Step 5: Run the Effect test to confirm it fails**

Run: `pnpm vitest run src/controllers/effects/useTodayEffect.test.ts`
Expected: FAIL because `createTodayEffect` does not exist.

- [ ] **Step 6: Implement the Effect**

Create `src/controllers/effects/useTodayEffect.ts`:

```ts
import { useEffect, useMemo } from "react";
import { toast } from "sonner";
import { useTodayStore } from "@/controllers/stores/todayStore";
import type { CreateItemInput, TodayModel } from "@/models/contracts";
import { todayModel } from "@/models/todayModel";

type Notifier = {
  success: (message: string) => void;
  error: (message: string) => void;
};

const defaultNotifier: Notifier = {
  success: (message) => toast.success(message),
  error: (message) => toast.error(message),
};

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : "操作失败，请重试";
}

export function createTodayEffect(
  model: TodayModel,
  notify: Notifier = defaultNotifier,
) {
  const loadToday = async () => {
    const store = useTodayStore.getState();
    store.setLoading();
    try {
      const [items, categories] = await Promise.all([
        model.listToday(),
        model.listCategories(),
      ]);
      useTodayStore.getState().setReady(items, categories);
    } catch (error) {
      const message = messageOf(error);
      useTodayStore.getState().setError(message);
      notify.error(message);
    }
  };

  const createItem = async (input: CreateItemInput) => {
    try {
      const item = await model.createItem(input);
      useTodayStore.getState().addItem(item);
      notify.success("已加入今日");
      return item;
    } catch (error) {
      notify.error(messageOf(error));
      throw error;
    }
  };

  const completeItem = async (id: string) => {
    try {
      const item = await model.completeItem(id);
      useTodayStore.getState().replaceItem(item);
      notify.success("已完成");
    } catch (error) {
      notify.error(messageOf(error));
      throw error;
    }
  };

  const uncompleteItem = async (id: string) => {
    try {
      const item = await model.uncompleteItem(id);
      useTodayStore.getState().replaceItem(item);
      notify.success("已恢复");
    } catch (error) {
      notify.error(messageOf(error));
      throw error;
    }
  };

  return {
    loadToday,
    retryToday: loadToday,
    createItem,
    completeItem,
    uncompleteItem,
  };
}

export function useTodayEffect(model: TodayModel = todayModel) {
  const effect = useMemo(() => createTodayEffect(model), [model]);

  useEffect(() => {
    void effect.loadToday();
  }, [effect]);

  return effect;
}
```

- [ ] **Step 7: Run Store and Effect tests**

Run: `pnpm vitest run src/controllers/stores/todayStore.test.ts src/controllers/effects/useTodayEffect.test.ts`
Expected: PASS with four tests.

- [ ] **Step 8: Commit**

```powershell
git add src/controllers/stores/todayStore.ts src/controllers/effects/useTodayEffect.ts src/controllers/stores/todayStore.test.ts src/controllers/effects/useTodayEffect.test.ts
git commit -m "feat: add today store and effect controllers"
```

### Task 6: Build the Today View

**Files:**
- Create: `src/components/today/todayHeader.tsx`
- Create: `src/components/today/summaryStrip.tsx`
- Create: `src/components/today/itemTimeline.tsx`
- Create: `src/components/today/quickCapture.tsx`
- Modify: `src/pages/today/index.tsx`
- Modify: `src/components/shell/contextSidebar.tsx`
- Test: `src/pages/today/todayPage.test.tsx`

**Interfaces:**
- Consumes: `useTodayStore` and `useTodayEffect` from Task 5.
- Produces: populated, empty, loading and error views.
- Produces: quick creation and task completion interactions.

- [ ] **Step 1: Write the failing page test**

Create `src/pages/today/todayPage.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { resetTodayStore, useTodayStore } from "@/controllers/stores/todayStore";
import TodayPage from ".";

vi.mock("@/controllers/effects/useTodayEffect", () => ({
  useTodayEffect: () => ({
    createItem: vi.fn(),
    completeItem: vi.fn(),
    uncompleteItem: vi.fn(),
    retryToday: vi.fn(),
  }),
}));

describe("TodayPage", () => {
  beforeEach(resetTodayStore);

  it("renders the empty state", () => {
    useTodayStore.setState({ phase: "ready" });
    render(<TodayPage />);
    expect(screen.getByText("今天还没有安排")).toBeInTheDocument();
  });

  it("opens quick capture", async () => {
    useTodayStore.setState({ phase: "ready" });
    render(<TodayPage />);
    await userEvent.click(screen.getByRole("button", { name: /新建/ }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the page test to confirm it fails**

Run: `pnpm vitest run src/pages/today/todayPage.test.tsx`
Expected: FAIL because the Today View components and accessible new-item button do not exist.

- [ ] **Step 3: Implement the header and summary**

Create `src/components/today/todayHeader.tsx`:

```tsx
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
```

Create `src/components/today/summaryStrip.tsx`:

```tsx
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
        <dd className="mt-1 text-lg font-semibold text-textPrimary">{focusMinutes} 分钟</dd>
      </div>
    </dl>
  );
}
```

- [ ] **Step 4: Implement the timeline**

Create `src/components/today/itemTimeline.tsx`:

```tsx
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
        <p className="mt-1 text-xs text-textSecondary">从新建待办或 Quick Capture 开始。</p>
      </div>
    );
  }

  return (
    <ol className="relative mt-6 space-y-3 before:absolute before:bottom-3 before:left-[7px] before:top-3 before:w-px before:bg-border">
      {items.map((item) => (
        <li key={item.id} className="relative grid grid-cols-[16px_minmax(0,1fr)] gap-3">
          <span className="mt-4 size-[15px] rounded-full border-2 border-surface bg-brand ring-1 ring-border" />
          <article className="min-w-0 rounded-lg border border-border bg-surface p-4">
            <div className="flex items-start gap-3">
              <button
                type="button"
                aria-label={item.status === "done" ? `取消完成 ${item.title}` : `完成 ${item.title}`}
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
                  {item.kind === "event" && <CalendarDays aria-hidden="true" size={13} />}
                  <span>{time(item.startAt)}</span>
                </div>
                <p
                  className={`mt-1 break-words text-sm font-medium ${
                    item.status === "done" ? "text-textSecondary line-through" : "text-textPrimary"
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
```

- [ ] **Step 5: Implement Quick Capture**

Create `src/components/today/quickCapture.tsx`:

```tsx
import { useState, type FormEvent } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreate: (title: string) => Promise<unknown>;
};

export function QuickCapture({ open, onOpenChange, onCreate }: Props) {
  const [title, setTitle] = useState("");
  const [pending, setPending] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const value = title.trim();
    if (!value) return;
    setPending(true);
    try {
      await onCreate(value);
      setTitle("");
      onOpenChange(false);
    } catch {
      // The Effect already reports the error through Toast.
    } finally {
      setPending(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>新建待办</DialogTitle>
            <DialogDescription>先记下来，稍后再安排时间。</DialogDescription>
          </DialogHeader>
          <Input
            aria-label="待办标题"
            autoFocus
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            placeholder="要做什么？"
            className="mt-5"
          />
          <DialogFooter className="mt-5">
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" disabled={pending || !title.trim()}>
              {pending ? "保存中" : "保存"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 6: Compose TodayPage**

Replace `src/pages/today/index.tsx`:

```tsx
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
          <Button className="mt-4" variant="outline" onClick={() => void retryToday()}>
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
```

- [ ] **Step 7: Bind the context sidebar to Store data**

Remove the top-level static filter and category arrays from `src/components/shell/contextSidebar.tsx`, then replace the component with Store selectors:

```tsx
import { useTodayStore } from "@/controllers/stores/todayStore";

export function ContextSidebar() {
  const { items, categories } = useTodayStore();
  const planned = items.filter((item) => item.startAt).length;
  const done = items.filter((item) => item.status === "done").length;
  const progress = items.length === 0 ? 0 : Math.round((done / items.length) * 100);
  const filters = [
    { label: "全部", count: items.length },
    { label: "已安排", count: planned },
    { label: "已完成", count: done },
  ];

  return (
    <aside
      aria-label="今日上下文"
      className="h-full w-[88px] shrink-0 border-r border-border bg-surfaceSubtle px-2 py-5 min-[461px]:w-[104px] min-[768px]:w-[184px] min-[1280px]:w-[232px] min-[768px]:px-4"
    >
      <div className="mb-6">
        <p className="truncate text-sm font-semibold text-textPrimary">今日</p>
        <p className="mt-1 hidden text-xs text-textSecondary min-[768px]:block">
          {new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium" }).format(new Date())}
        </p>
      </div>
      <nav aria-label="今日筛选" className="space-y-1">
        {filters.map((filter, index) => (
          <button
            key={filter.label}
            className={`flex w-full items-center justify-between rounded-md px-2 py-2 text-xs min-[768px]:text-sm ${
              index === 0 ? "bg-brandSoft text-brand" : "text-textSecondary hover:bg-surface"
            }`}
          >
            <span className="truncate">{filter.label}</span>
            <span className="hidden min-[768px]:inline">{filter.count}</span>
          </button>
        ))}
      </nav>
      <div className="mt-8 hidden min-[768px]:block">
        <div className="flex items-center justify-between text-[11px] text-textSecondary">
          <span>今日完成度</span>
          <span>{progress}%</span>
        </div>
        <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-border">
          <div className="h-full rounded-full bg-brand" style={{ width: `${progress}%` }} />
        </div>
      </div>
      <p className="mb-2 mt-7 hidden text-[11px] font-medium uppercase tracking-wider text-textSecondary min-[768px]:block">
        分类
      </p>
      <div className="space-y-1">
        {categories.map((category) => (
          <button
            key={category.id}
            className="flex w-full items-center gap-2 rounded-md px-2 py-2 text-xs text-textSecondary hover:bg-surface min-[768px]:text-sm"
          >
            <span
              className="size-2 shrink-0 rounded-full"
              style={{ backgroundColor: category.color }}
            />
            <span className="truncate">{category.name}</span>
          </button>
        ))}
      </div>
    </aside>
  );
}
```

- [ ] **Step 8: Run page tests and full test suite**

Run: `pnpm vitest run src/pages/today/todayPage.test.tsx`
Expected: PASS with two tests.
Run: `pnpm test`
Expected: PASS with all unit tests.

- [ ] **Step 9: Commit**

```powershell
git add src/components/today src/pages/today src/components/shell/contextSidebar.tsx
git commit -m "feat: add interactive today view"
```

### Task 7: Implement SQLite Models and Migration

**Files:**
- Create: `api/migrations/001Initial.sql`
- Create: `api/src/models/db.ts`
- Create: `api/src/models/categoryModel.ts`
- Create: `api/src/models/itemModel.ts`
- Test: `api/test/itemModel.test.ts`

**Interfaces:**
- Consumes: Node.js 24 `node:sqlite`.
- Produces: `openDatabase(path)`, `createCategoryModel(db)`, and `createItemModel(db)`.
- Produces: tables `categories` and `items`.

- [ ] **Step 1: Write the failing SQLite model test**

Create `api/test/itemModel.test.ts`:

```ts
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { openDatabase, type LeeatDatabase } from "../src/models/db";
import { createItemModel } from "../src/models/itemModel";

describe("itemModel", () => {
  let db: LeeatDatabase | undefined;

  afterEach(() => {
    db?.close();
  });

  it("creates, completes and uncompletes an item", () => {
    const dir = mkdtempSync(join(tmpdir(), "leeat-"));
    db = openDatabase(join(dir, "test.sqlite"));
    const model = createItemModel(db);

    const item = model.create({ title: "整理桌面" });
    expect(item.status).toBe("unplanned");

    const done = model.complete(item.id);
    expect(done.status).toBe("done");

    const undone = model.uncomplete(item.id);
    expect(undone.status).toBe("unplanned");
  });
});
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `pnpm vitest run api/test/itemModel.test.ts`
Expected: FAIL because `db.ts` and `itemModel.ts` do not exist.

- [ ] **Step 3: Create the migration**

Create `api/migrations/001Initial.sql`:

```sql
CREATE TABLE IF NOT EXISTS categories (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  color TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS items (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('task', 'event')),
  title TEXT NOT NULL,
  description TEXT,
  status TEXT NOT NULL CHECK (status IN ('unplanned', 'planned', 'done')),
  category_id TEXT REFERENCES categories(id),
  start_at TEXT,
  end_at TEXT,
  completed_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS items_start_at_idx ON items(start_at);

INSERT OR IGNORE INTO categories (id, name, color)
VALUES
  ('work', '工作', '#5468FF'),
  ('study', '学习', '#00B42A'),
  ('life', '生活', '#F59E0B');
```

- [ ] **Step 4: Implement database setup**

Create `api/src/models/db.ts`:

```ts
import { readFileSync } from "node:fs";
import { DatabaseSync } from "node:sqlite";

export type LeeatDatabase = DatabaseSync;

const migration = readFileSync(
  new URL("../../migrations/001Initial.sql", import.meta.url),
  "utf8",
);

export function openDatabase(filename: string): LeeatDatabase {
  const db = new DatabaseSync(filename);
  db.exec("PRAGMA foreign_keys = ON;");
  db.exec(migration);
  return db;
}
```

- [ ] **Step 5: Implement category and item models**

Create `api/src/models/categoryModel.ts`:

```ts
import type { Category } from "../../../src/models/contracts";
import type { LeeatDatabase } from "./db";

type CategoryRow = {
  id: string;
  name: string;
  color: string;
};

const mapCategory = (row: CategoryRow): Category => ({
  id: row.id,
  name: row.name,
  color: row.color,
});

export function createCategoryModel(db: LeeatDatabase) {
  return {
    list(): Category[] {
      const rows = db
        .prepare("SELECT id, name, color FROM categories ORDER BY name")
        .all() as CategoryRow[];
      return rows.map(mapCategory);
    },
  };
}
```

Create `api/src/models/itemModel.ts`:

```ts
import { randomUUID } from "node:crypto";
import type {
  CreateItemInput,
  Item,
  UpdateItemInput,
} from "../../../src/models/contracts";
import type { LeeatDatabase } from "./db";

type ItemRow = {
  id: string;
  kind: Item["kind"];
  title: string;
  description: string | null;
  status: Item["status"];
  category_id: string | null;
  start_at: string | null;
  end_at: string | null;
  completed_at: string | null;
  created_at: string;
  updated_at: string;
};

const mapItem = (row: ItemRow): Item => ({
  id: row.id,
  kind: row.kind,
  title: row.title,
  description: row.description ?? undefined,
  status: row.status,
  categoryId: row.category_id ?? undefined,
  startAt: row.start_at ?? undefined,
  endAt: row.end_at ?? undefined,
  completedAt: row.completed_at ?? undefined,
  createdAt: row.created_at,
  updatedAt: row.updated_at,
});

export function createItemModel(db: LeeatDatabase) {
  const get = (id: string): Item => {
    const row = db
      .prepare("SELECT * FROM items WHERE id = ?")
      .get(id) as ItemRow | undefined;
    if (!row) throw new Error("Item not found");
    return mapItem(row);
  };

  return {
    listToday(dayStart: string, dayEnd: string): Item[] {
      const rows = db
        .prepare(
          "SELECT * FROM items WHERE (start_at >= ? AND start_at < ?) OR (start_at IS NULL AND created_at >= ? AND created_at < ?) ORDER BY start_at IS NULL, start_at, created_at",
        )
        .all(dayStart, dayEnd, dayStart, dayEnd) as ItemRow[];
      return rows.map(mapItem);
    },
    create(input: CreateItemInput): Item {
      const now = new Date().toISOString();
      const id = randomUUID();
      db.prepare(
        `INSERT INTO items (
          id, kind, title, status, category_id, start_at, end_at, created_at, updated_at
        ) VALUES (?, 'task', ?, ?, ?, ?, ?, ?, ?)`,
      ).run(
        id,
        input.title.trim(),
        input.startAt ? "planned" : "unplanned",
        input.categoryId ?? null,
        input.startAt ?? null,
        input.endAt ?? null,
        now,
        now,
      );
      return get(id);
    },
    update(input: UpdateItemInput): Item {
      const current = get(input.id);
      const now = new Date().toISOString();
      db.prepare(
        `UPDATE items SET
          title = ?, description = ?, category_id = ?, start_at = ?, end_at = ?, updated_at = ?
        WHERE id = ?`,
      ).run(
        input.title ?? current.title,
        input.description ?? current.description ?? null,
        input.categoryId ?? current.categoryId ?? null,
        input.startAt ?? current.startAt ?? null,
        input.endAt ?? current.endAt ?? null,
        now,
        input.id,
      );
      return get(input.id);
    },
    complete(id: string): Item {
      const now = new Date().toISOString();
      db.prepare(
        "UPDATE items SET status = 'done', completed_at = ?, updated_at = ? WHERE id = ?",
      ).run(now, now, id);
      return get(id);
    },
    uncomplete(id: string): Item {
      const current = get(id);
      const now = new Date().toISOString();
      db.prepare(
        "UPDATE items SET status = ?, completed_at = NULL, updated_at = ? WHERE id = ?",
      ).run(current.startAt ? "planned" : "unplanned", now, id);
      return get(id);
    },
  };
}
```

- [ ] **Step 6: Run the SQLite model test**

Run: `pnpm vitest run api/test/itemModel.test.ts`
Expected: PASS with one test.

- [ ] **Step 7: Commit**

```powershell
git add api/migrations api/src/models api/test/itemModel.test.ts
git commit -m "feat: add SQLite item models"
```

### Task 8: Implement Fastify Controllers, Views and Routes

**Files:**
- Create: `api/src/views/jsonView.ts`
- Create: `api/src/controllers/categoryController.ts`
- Create: `api/src/controllers/todayController.ts`
- Create: `api/src/controllers/itemController.ts`
- Create: `api/src/routes.ts`
- Create: `api/src/app.ts`
- Create: `api/src/server.ts`
- Test: `api/test/app.test.ts`

**Interfaces:**
- Consumes: `openDatabase`, `createItemModel`, and `createCategoryModel` from Task 7.
- Produces: `createApp({ databasePath })` returning a Fastify instance.
- Produces: `GET /api/v1/today`, `GET /api/v1/categories`, `POST /api/v1/item/create`, `POST /api/v1/item/update`, `POST /api/v1/item/complete`, `POST /api/v1/item/uncomplete`.

- [ ] **Step 1: Write the failing API test**

Create `api/test/app.test.ts`:

```ts
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { createApp } from "../src/app";

describe("Leeat API", () => {
  it("creates and completes an item with GET and POST only", async () => {
    const dir = mkdtempSync(join(tmpdir(), "leeat-api-"));
    const app = createApp({ databasePath: join(dir, "test.sqlite") });

    const created = await app.inject({
      method: "POST",
      url: "/api/v1/item/create",
      payload: { title: "准备周会" },
    });
    expect(created.statusCode).toBe(200);
    const createdBody = created.json();
    expect(createdBody.data.title).toBe("准备周会");

    const today = await app.inject({
      method: "GET",
      url: "/api/v1/today",
    });
    expect(today.statusCode).toBe(200);
    expect(today.json().data[0].title).toBe("准备周会");

    const categories = await app.inject({
      method: "GET",
      url: "/api/v1/categories",
    });
    expect(categories.statusCode).toBe(200);
    expect(categories.json().data).toHaveLength(3);

    const completed = await app.inject({
      method: "POST",
      url: "/api/v1/item/complete",
      payload: { id: createdBody.data.id },
    });
    expect(completed.statusCode).toBe(200);
    expect(completed.json().data.status).toBe("done");

    await app.close();
  });

  it("rejects unsupported methods", async () => {
    const dir = mkdtempSync(join(tmpdir(), "leeat-api-"));
    const app = createApp({ databasePath: join(dir, "test.sqlite") });
    const response = await app.inject({
      method: "DELETE",
      url: "/api/v1/item/create",
    });
    expect(response.statusCode).toBe(404);
    await app.close();
  });
});
```

- [ ] **Step 2: Run the API test to confirm it fails**

Run: `pnpm vitest run api/test/app.test.ts`
Expected: FAIL because `createApp` does not exist.

- [ ] **Step 3: Create the JSON View**

Create `api/src/views/jsonView.ts`:

```ts
export function ok<T>(data: T) {
  return { data };
}

export function fail(code: string, message: string) {
  return { error: { code, message } };
}
```

- [ ] **Step 4: Create the controllers**

Create `api/src/controllers/categoryController.ts`:

```ts
import type { FastifyInstance } from "fastify";
import { ok } from "../views/jsonView";
import { createCategoryModel } from "../models/categoryModel";
import type { LeeatDatabase } from "../models/db";

export function registerCategoryController(
  app: FastifyInstance,
  db: LeeatDatabase,
) {
  app.get("/api/v1/categories", async () => {
    const model = createCategoryModel(db);
    return ok(model.list());
  });
}
```

Create `api/src/controllers/todayController.ts`:

```ts
import type { FastifyInstance } from "fastify";
import { ok } from "../views/jsonView";
import type { LeeatDatabase } from "../models/db";
import { createItemModel } from "../models/itemModel";

function dayRange() {
  const now = new Date();
  const start = new Date(now);
  start.setHours(0, 0, 0, 0);
  const end = new Date(start);
  end.setDate(end.getDate() + 1);
  return { start: start.toISOString(), end: end.toISOString() };
}

export function registerTodayController(
  app: FastifyInstance,
  db: LeeatDatabase,
) {
  app.get("/api/v1/today", async () => {
    const range = dayRange();
    const model = createItemModel(db);
    return ok(model.listToday(range.start, range.end));
  });
}
```

Create `api/src/controllers/itemController.ts`:

```ts
import type { FastifyInstance, FastifyReply } from "fastify";
import { z } from "zod";
import { fail, ok } from "../views/jsonView";
import type { LeeatDatabase } from "../models/db";
import { createItemModel } from "../models/itemModel";

const createSchema = z.object({
  title: z.string().trim().min(1).max(200),
  categoryId: z.string().min(1).optional(),
  startAt: z.string().datetime().optional(),
  endAt: z.string().datetime().optional(),
});

const updateSchema = z.object({
  id: z.string().min(1),
  title: z.string().trim().min(1).max(200).optional(),
  description: z.string().max(2000).optional(),
  categoryId: z.string().min(1).optional(),
  startAt: z.string().datetime().optional(),
  endAt: z.string().datetime().optional(),
});

const idSchema = z.object({ id: z.string().min(1) });

function validationError(reply: FastifyReply, message: string) {
  return reply.code(400).send(fail("VALIDATION_ERROR", message));
}

export function registerItemController(
  app: FastifyInstance,
  db: LeeatDatabase,
) {
  const model = () => createItemModel(db);

  app.post("/api/v1/item/create", async (request, reply) => {
    const parsed = createSchema.safeParse(request.body);
    if (!parsed.success) return validationError(reply, parsed.error.issues[0]?.message ?? "参数错误");
    return ok(model().create(parsed.data));
  });

  app.post("/api/v1/item/update", async (request, reply) => {
    const parsed = updateSchema.safeParse(request.body);
    if (!parsed.success) return validationError(reply, parsed.error.issues[0]?.message ?? "参数错误");
    return ok(model().update(parsed.data));
  });

  app.post("/api/v1/item/complete", async (request, reply) => {
    const parsed = idSchema.safeParse(request.body);
    if (!parsed.success) return validationError(reply, "缺少 id");
    return ok(model().complete(parsed.data.id));
  });

  app.post("/api/v1/item/uncomplete", async (request, reply) => {
    const parsed = idSchema.safeParse(request.body);
    if (!parsed.success) return validationError(reply, "缺少 id");
    return ok(model().uncomplete(parsed.data.id));
  });
}
```

- [ ] **Step 5: Create routes, app and server**

Create `api/src/routes.ts`:

```ts
import type { FastifyInstance } from "fastify";
import { registerCategoryController } from "./controllers/categoryController";
import { registerItemController } from "./controllers/itemController";
import { registerTodayController } from "./controllers/todayController";
import type { LeeatDatabase } from "./models/db";

export function registerRoutes(app: FastifyInstance, db: LeeatDatabase) {
  registerTodayController(app, db);
  registerCategoryController(app, db);
  registerItemController(app, db);
}
```

Create `api/src/app.ts`:

```ts
import Fastify from "fastify";
import { openDatabase } from "./models/db";
import { registerRoutes } from "./routes";

type Options = {
  databasePath: string;
  logger?: boolean;
};

export function createApp({ databasePath, logger = false }: Options) {
  const app = Fastify({ logger });
  const db = openDatabase(databasePath);
  registerRoutes(app, db);
  app.addHook("onClose", async () => {
    db.close();
  });
  return app;
}
```

Create `api/src/server.ts`:

```ts
import { mkdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { createApp } from "./app";

const databasePath = resolve(process.cwd(), "data", "leeat.sqlite");
mkdirSync(dirname(databasePath), { recursive: true });

const app = createApp({ databasePath, logger: true });

await app.listen({
  host: "127.0.0.1",
  port: Number(process.env.API_PORT ?? 3001),
});
```

- [ ] **Step 6: Run API tests and all unit tests**

Run: `pnpm vitest run api/test/app.test.ts`
Expected: PASS with two tests.
Run: `pnpm test`
Expected: PASS with all unit tests.

- [ ] **Step 7: Commit**

```powershell
git add api/src api/test/app.test.ts
git commit -m "feat: add Fastify SQLite API"
```

### Task 9: Connect ApiModel and Runtime Selection

**Files:**
- Modify: `src/models/apiModel.ts`
- Modify: `src/models/todayModel.ts`
- Modify: `vite.config.ts`
- Test: `src/models/apiModel.test.ts`

**Interfaces:**
- Consumes: the API from Task 8.
- Produces: `apiModel` implementing `TodayModel`.
- Produces: runtime selection via `VITE_DATA_SOURCE=mock` or `VITE_DATA_SOURCE=api`.

- [ ] **Step 1: Write the failing ApiModel test**

Create `src/models/apiModel.test.ts`:

```ts
import { afterEach, describe, expect, it, vi } from "vitest";
import { apiModel } from "./apiModel";

describe("apiModel", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("unwraps the data envelope", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ data: [{ id: "1", title: "任务" }] }),
      }),
    );
    await expect(apiModel.listToday()).resolves.toEqual([
      { id: "1", title: "任务" },
    ]);
  });

  it("throws the API error message", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: false,
        json: async () => ({
          error: { code: "VALIDATION_ERROR", message: "标题不能为空" },
        }),
      }),
    );
    await expect(apiModel.createItem({ title: "" })).rejects.toThrow(
      "标题不能为空",
    );
  });
});
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `pnpm vitest run src/models/apiModel.test.ts`
Expected: FAIL because `apiModel` still returns the temporary unavailable model.

- [ ] **Step 3: Implement ApiModel**

Replace `src/models/apiModel.ts`:

```ts
import type {
  Category,
  CreateItemInput,
  Item,
  TodayModel,
  UpdateItemInput,
} from "./contracts";

type Envelope<T> = { data: T } | { error: { code: string; message: string } };

const baseUrl = import.meta.env.VITE_API_BASE ?? "/api/v1";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${baseUrl}${path}`, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  const body = (await response.json()) as Envelope<T>;
  if (!response.ok || "error" in body) {
    throw new Error("error" in body ? body.error.message : "请求失败");
  }
  return body.data;
}

export const apiModel: TodayModel = {
  listToday: () => request<Item[]>("/today"),
  listCategories: () => request<Category[]>("/categories"),
  createItem: (input: CreateItemInput) =>
    request<Item>("/item/create", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  updateItem: (input: UpdateItemInput) =>
    request<Item>("/item/update", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  completeItem: (id: string) =>
    request<Item>("/item/complete", {
      method: "POST",
      body: JSON.stringify({ id }),
    }),
  uncompleteItem: (id: string) =>
    request<Item>("/item/uncomplete", {
      method: "POST",
      body: JSON.stringify({ id }),
    }),
};
```

- [ ] **Step 4: Add the API development script**

Confirm `vite.config.ts` contains:

```ts
server: {
  port: 5173,
  proxy: {
    "/api": "http://127.0.0.1:3001",
  },
},
```

Run the API and web app:

```powershell
$env:VITE_DATA_SOURCE="api"
pnpm dev
```

Expected: open `http://127.0.0.1:5173/#/` and the page loads items from Fastify.

- [ ] **Step 5: Run ApiModel and full tests**

Run: `pnpm vitest run src/models/apiModel.test.ts`
Expected: PASS with two tests.
Run: `pnpm test`
Expected: PASS with all unit tests.

- [ ] **Step 6: Commit**

```powershell
git add src/models/apiModel.ts src/models/apiModel.test.ts vite.config.ts
git commit -m "feat: connect API model"
```

### Task 10: Add Playwright Visual and Responsive Verification

**Files:**
- Create: `playwright.config.ts`
- Create: `tests/e2e/leeat.spec.ts`
- Test: `tests/e2e/leeat.spec.ts`

**Interfaces:**
- Consumes: `pnpm dev:web` and the built UI.
- Produces: checks for `/`, `/settings`, Light / Dark, 390px and 1440px layouts, and horizontal overflow.

- [ ] **Step 1: Write the failing browser test**

Create `playwright.config.ts`:

```ts
import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests/e2e",
  fullyParallel: true,
  reporter: "list",
  use: {
    baseURL: "http://127.0.0.1:4174",
    trace: "on-first-retry",
  },
  webServer: {
    command: "pnpm build && pnpm preview --host 127.0.0.1 --port 4174",
    url: "http://127.0.0.1:4174",
    reuseExistingServer: false,
  },
  projects: [
    { name: "desktop", use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 } } },
    { name: "mobile", use: { ...devices["Desktop Chrome"], viewport: { width: 390, height: 844 } } },
  ],
});
```

Create `tests/e2e/leeat.spec.ts`:

```ts
import { expect, test } from "@playwright/test";

test("home renders without horizontal overflow", async ({ page }) => {
  await page.goto("/#/");
  await expect(page.getByRole("heading", { name: "今日" })).toBeVisible();
  await expect(page.getByRole("complementary", { name: "今日上下文" })).toBeVisible();
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
  );
  expect(overflow).toBe(false);
  await page.screenshot({
    path: `test-results/leeat-home-${test.info().project.name}.png`,
    fullPage: true,
  });
});

test("settings route is reachable", async ({ page }) => {
  await page.goto("/#/settings");
  await expect(page.getByRole("heading", { name: "设置" })).toBeVisible();
});

test("theme toggles and persists", async ({ page }) => {
  await page.goto("/#/");
  await page.getByRole("button", { name: "切换到深色主题" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
});
```

- [ ] **Step 2: Run the browser regression tests**

Run: `pnpm exec playwright install chromium`
Run: `pnpm test:e2e`
Expected: PASS for desktop and mobile projects. If it fails, fix the responsible Store, Effect, View or navigation task before continuing.

- [ ] **Step 3: Run the final verification suite**

Run: `pnpm typecheck`
Expected: PASS.
Run: `pnpm test`
Expected: PASS.
Run: `pnpm build`
Expected: PASS.
Run: `pnpm test:e2e`
Expected: PASS.

- [ ] **Step 4: Commit**

```powershell
git add playwright.config.ts tests/e2e
git commit -m "test: verify responsive workbench"
```

## Final Review Checklist

- [ ] `node --version` reports Node.js 24.
- [ ] `pnpm typecheck`, `pnpm test`, `pnpm build`, and `pnpm test:e2e` pass.
- [ ] `/` shows the complete Today view.
- [ ] `/#/settings` shows the settings placeholder.
- [ ] One-level navigation has only Home and Settings.
- [ ] Context sidebar remains visible at 390px.
- [ ] 390px, 768px, 1024px and 1440px have no global horizontal overflow.
- [ ] Quick Capture creates an unplanned task.
- [ ] Task completion and uncompletion update the timeline.
- [ ] Light / Dark persists after reload.
- [ ] Store files do not import Models or `fetch`.
- [ ] Effect files do not own long-lived state.
- [ ] API uses only GET and POST.
- [ ] Electron is absent from dependencies and build output.
- [ ] No Agent, Multi-Agent or AI chat entry exists.
