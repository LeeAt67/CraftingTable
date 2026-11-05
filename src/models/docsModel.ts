import type { DocsModel } from "./docContracts";
import { apiDocsModel } from "./apiDocsModel";
import { fakeDocsModel } from "./fakeDocsModel";

export const docsModel: DocsModel =
  import.meta.env.VITE_DATA_SOURCE === "mock" || import.meta.env.MODE === "test"
    ? fakeDocsModel
    : apiDocsModel;
