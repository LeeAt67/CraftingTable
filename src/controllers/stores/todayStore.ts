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
      items: state.items.map((current) =>
        current.id === item.id ? item : current,
      ),
    })),
}));

export function resetTodayStore() {
  useTodayStore.setState(initialTodayState);
}
