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
    useTodayStore.getState().setLoading();
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
