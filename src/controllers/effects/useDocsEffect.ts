import { useEffect, useMemo } from "react";
import { toast } from "sonner";
import { useDocsStore } from "@/controllers/stores/docsStore";
import type { DocNodeKind, DocsModel } from "@/models/docContracts";
import { docsModel } from "@/models/docsModel";

type Notifier = {
  success: (message: string) => void;
  error: (message: string) => void;
};

const defaultNotifier: Notifier = {
  success: (message) => toast.success(message),
  error: (message) => toast.error(message),
};

const CONTENT_SAVE_DELAY = 400;
const TITLE_SAVE_DELAY = 600;
const SEARCH_DELAY = 300;

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : "操作失败，请重试";
}

export function createDocsEffect(
  model: DocsModel,
  notify: Notifier = defaultNotifier,
) {
  let contentTimer: ReturnType<typeof setTimeout> | null = null;
  let titleTimer: ReturnType<typeof setTimeout> | null = null;
  let searchTimer: ReturnType<typeof setTimeout> | null = null;
  let pendingSave: { id: string; content: string } | null = null;
  let pendingTitle: { id: string; title: string } | null = null;
  let searchSequence = 0;
  let saveSequence: Promise<void> = Promise.resolve();

  const loadTree = async () => {
    useDocsStore.getState().setLoading();
    try {
      const nodes = await model.listTree();
      useDocsStore.getState().setTree(nodes);
    } catch (error) {
      const message = messageOf(error);
      useDocsStore.getState().setError(message);
      notify.error(message);
    }
  };

  const openNode = async (id: string) => {
    if (useDocsStore.getState().activeNodeId === id) {
      return;
    }
    await flushSave();
    useDocsStore.getState().setLoading();
    try {
      const node = await model.getNode(id);
      useDocsStore.getState().setActiveNode(node);
    } catch (error) {
      const message = messageOf(error);
      useDocsStore.getState().clearActiveNode();
      useDocsStore.getState().setError(message);
      notify.error(message);
    }
  };

  const persistContent = (target: { id: string; content: string }) => {
    saveSequence = saveSequence.then(async () => {
      useDocsStore.getState().setSaveStatus("saving");
      try {
        const result = await model.saveContent(target);
        useDocsStore.getState().markSaved({
          id: target.id,
          content: target.content,
          updatedAt: result.updatedAt,
        });
        flushTitleIfPending(target.id);
      } catch (error) {
        const message = messageOf(error);
        useDocsStore.getState().setSaveStatus("error");
        notify.error(message);
      }
    });
    return saveSequence;
  };

  const flushPendingTitle = () => {
    if (titleTimer) {
      clearTimeout(titleTimer);
      titleTimer = null;
    }
    const target = pendingTitle;
    pendingTitle = null;
    if (!target) {
      return;
    }
    void model
      .renameNode({ id: target.id, title: target.title })
      .then((summary) => useDocsStore.getState().replaceSummary(summary))
      .catch((error) => notify.error(messageOf(error)));
  };

  const flushSave = async () => {
    if (contentTimer) {
      clearTimeout(contentTimer);
      contentTimer = null;
    }
    flushPendingTitle();
    const target = pendingSave;
    pendingSave = null;
    if (target) {
      await persistContent(target);
      return;
    }
    await saveSequence;
  };

  const flushTitleIfPending = (id: string) => {
    if (pendingTitle?.id === id) {
      flushPendingTitle();
    }
  };

  const applyContentChange = (id: string, content: string) => {
    useDocsStore.getState().setDraftContent(content);
    pendingSave = { id, content };
    if (contentTimer) {
      clearTimeout(contentTimer);
    }
    contentTimer = setTimeout(() => {
      contentTimer = null;
      const target = pendingSave;
      pendingSave = null;
      if (target) {
        void persistContent(target);
      }
    }, CONTENT_SAVE_DELAY);
  };

  const applyTitleChange = (id: string, title: string) => {
    useDocsStore.getState().setDraftTitle(title);
    pendingTitle = { id, title };
    if (titleTimer) {
      clearTimeout(titleTimer);
    }
    titleTimer = setTimeout(() => {
      titleTimer = null;
      const target = pendingTitle;
      pendingTitle = null;
      if (!target) {
        return;
      }
      void model
        .renameNode({ id: target.id, title: target.title })
        .then((summary) => useDocsStore.getState().replaceSummary(summary))
        .catch((error) => notify.error(messageOf(error)));
    }, TITLE_SAVE_DELAY);
  };

  const createNode = async (kind: DocNodeKind, parentId: string | null) => {
    try {
      const node = await model.createNode({
        kind,
        parentId,
        clientRequestId: crypto.randomUUID(),
      });
      useDocsStore.getState().setTree([...useDocsStore.getState().nodes, node]);
      return node;
    } catch (error) {
      notify.error(messageOf(error));
      return null;
    }
  };

  const renameNode = async (
    id: string,
    title: string,
    options: { silent?: boolean } = {},
  ) => {
    try {
      const summary = await model.renameNode({ id, title });
      useDocsStore.getState().replaceSummary(summary);
      if (!options.silent) {
        notify.success("已重命名");
      }
      return summary;
    } catch (error) {
      notify.error(messageOf(error));
      return null;
    }
  };

  const moveNode = async (id: string, parentId: string | null) => {
    try {
      const summary = await model.moveNode({ id, parentId });
      useDocsStore.getState().replaceSummary(summary);
      notify.success("已移动");
      return summary;
    } catch (error) {
      notify.error(messageOf(error));
      return null;
    }
  };

  const deleteNode = async (id: string) => {
    try {
      const result = await model.deleteNode(id);
      useDocsStore.getState().removeNodes(result.deletedIds);
      notify.success("已删除");
      return result.deletedIds;
    } catch (error) {
      notify.error(messageOf(error));
      return null;
    }
  };

  const search = (rawQuery: string) => {
    const query = rawQuery.trim();
    useDocsStore.getState().setSearchQuery(rawQuery);

    if (searchTimer) {
      clearTimeout(searchTimer);
      searchTimer = null;
    }
    if (!query) {
      searchSequence += 1;
      useDocsStore.getState().clearSearch();
      return;
    }

    searchTimer = setTimeout(() => {
      searchTimer = null;
      searchSequence += 1;
      const sequence = searchSequence;
      useDocsStore.getState().setSearchPhase("loading");
      void model
        .search({ q: query, limit: 20 })
        .then((page) => {
          if (sequence !== searchSequence) {
            return;
          }
          useDocsStore.getState().setSearchResults(page.items, page.nextCursor);
        })
        .catch((error) => {
          if (sequence !== searchSequence) {
            return;
          }
          useDocsStore.getState().setSearchError();
          notify.error(messageOf(error));
        });
    }, SEARCH_DELAY);
  };

  const flushSearch = async () => {
    if (searchTimer) {
      clearTimeout(searchTimer);
      searchTimer = null;
    }
  };

  const dispose = () => {
    void flushSave();
    void flushSearch();
  };

  return {
    loadTree,
    openNode,
    createNode,
    renameNode,
    moveNode,
    deleteNode,
    applyContentChange,
    applyTitleChange,
    flushSave,
    search,
    flushSearch,
    dispose,
  };
}

export function useDocsEffect(model: DocsModel = docsModel) {
  const effect = useMemo(() => createDocsEffect(model), [model]);

  useEffect(() => () => effect.dispose(), [effect]);

  return effect;
}
