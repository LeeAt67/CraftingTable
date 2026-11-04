import type { TodayModel } from "./contracts";
import { apiModel } from "./apiModel";
import { mockModel } from "./mockModel";

export const todayModel: TodayModel =
  import.meta.env.VITE_DATA_SOURCE === "mock" ||
  import.meta.env.MODE === "test"
    ? mockModel
    : apiModel;
