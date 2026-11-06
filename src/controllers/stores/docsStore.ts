import { create } from "zustand";
import type {
  DocNode,
  DocNodeSummary,
  DocSearchHit,
} from "@/models/docContracts";

export type DocsPhase = "idle" | "loading" | "ready" | "error";
export type SaveStatus = "idle" | "saving" | "saved" | "error";
export type SearchPhase = "idle" | "loading" | "ready" | "error";
export type NodePhase = "idle" | "loading" | "ready" | "error";

type DocsState = {
  nodes: DocNodeSummary[];
  activeNodeId: string | null;
  activeNode: DocNode | null;
  draftTitle: string;
  draftContent: string;
  saveStatus: SaveStatus;
  /** 目录树加载状态，与当前文档的加载状态相互独立。 */
  phase: DocsPhase;
  errorMessage: string | null;
  /** 当前文档加载状态；切换文档时用它驱动骨架屏。 */
  nodePhase: NodePhase;
  nodeErrorMessage: string | null;
  searchQuery: string;
  searchResults: DocSearchHit[];
  searchPhase: SearchPhase;
  searchCursor: string | null;
};

type DocsActions = {
  setLoading: () => void;
  setTree: (nodes: DocNodeSummary[]) => void;
  setError: (message: string) => void;
  setNodeLoading: () => void;
  setNodeError: (message: string) => void;
  setActiveNode: (node: DocNode) => void;
  clearActiveNode: () => void;
  setDraftTitle: (title: string) => void;
  setDraftContent: (content: string) => void;
  setSaveStatus: (status: SaveStatus) => void;
  markSaved: (input: { id: string; content: string; updatedAt: string }) => void;
  replaceSummary: (node: DocNodeSummary) => void;
  removeNodes: (identifiers: string[]) => void;
  setSearchQuery: (query: string) => void;
  setSearchPhase: (phase: SearchPhase) => void;
  setSearchResults: (items: DocSearchHit[], cursor: string | null) => void;
  setSearchError: () => void;
  clearSearch: () => void;
};

type DocsStore = DocsState & DocsActions;

const initialDocsData: DocsState = {
  nodes: [],
  activeNodeId: null,
  activeNode: null,
  draftTitle: "",
  draftContent: "",
  saveStatus: "idle",
  phase: "idle",
  errorMessage: null,
  nodePhase: "idle",
  nodeErrorMessage: null,
  searchQuery: "",
  searchResults: [],
  searchPhase: "idle",
  searchCursor: null,
};

export const useDocsStore = create<DocsStore>()((set) => ({
  ...initialDocsData,
  setLoading: () => set({ phase: "loading", errorMessage: null }),
  setTree: (nodes) => set({ nodes, phase: "ready", errorMessage: null }),
  setError: (errorMessage) => set({ phase: "error", errorMessage }),
  setNodeLoading: () => set({ nodePhase: "loading", nodeErrorMessage: null }),
  setNodeError: (nodeErrorMessage) =>
    set({ nodePhase: "error", nodeErrorMessage }),
  setActiveNode: (node) =>
    set({
      activeNodeId: node.id,
      activeNode: node,
      draftTitle: node.title,
      draftContent: node.content,
      saveStatus: "idle",
      nodePhase: "ready",
      nodeErrorMessage: null,
    }),
  clearActiveNode: () =>
    set({
      activeNodeId: null,
      activeNode: null,
      draftTitle: "",
      draftContent: "",
      saveStatus: "idle",
      nodePhase: "idle",
      nodeErrorMessage: null,
    }),
  setDraftTitle: (draftTitle) => set({ draftTitle }),
  setDraftContent: (draftContent) => set({ draftContent }),
  setSaveStatus: (saveStatus) => set({ saveStatus }),
  markSaved: ({ id, content, updatedAt }) =>
    set((state) => {
      const nodes = state.nodes.map((node) =>
        node.id === id ? { ...node, updatedAt } : node,
      );
      if (state.activeNodeId !== id) {
        return { nodes };
      }
      const activeNode = state.activeNode
        ? {
            ...state.activeNode,
            content,
            updatedAt,
          }
        : null;
      return {
        saveStatus: "saved" as const,
        activeNode,
        nodes,
      };
    }),
  replaceSummary: (node) =>
    set((state) => ({
      nodes: state.nodes.map((current) =>
        current.id === node.id ? node : current,
      ),
      draftTitle:
        state.activeNodeId === node.id ? node.title : state.draftTitle,
      activeNode:
        state.activeNode && state.activeNode.id === node.id
          ? { ...state.activeNode, ...node }
          : state.activeNode,
    })),
  removeNodes: (identifiers) =>
    set((state) => {
      const removed = new Set(identifiers);
      return {
        nodes: state.nodes.filter((node) => !removed.has(node.id)),
        activeNodeId:
          state.activeNodeId && removed.has(state.activeNodeId)
            ? null
            : state.activeNodeId,
        activeNode:
          state.activeNode && removed.has(state.activeNode.id)
            ? null
            : state.activeNode,
      };
    }),
  setSearchQuery: (searchQuery) => set({ searchQuery }),
  setSearchPhase: (searchPhase) => set({ searchPhase }),
  setSearchResults: (searchResults, searchCursor) =>
    set({ searchResults, searchCursor, searchPhase: "ready" }),
  setSearchError: () => set({ searchPhase: "error" }),
  clearSearch: () =>
    set({
      searchQuery: "",
      searchResults: [],
      searchCursor: null,
      searchPhase: "idle",
    }),
}));

export const initialDocsState: DocsStore = useDocsStore.getState();

export function resetDocsStore() {
  useDocsStore.setState(initialDocsState, true);
}
