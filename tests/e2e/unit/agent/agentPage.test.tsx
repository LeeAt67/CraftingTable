import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import {
  resetAgentStore,
  useAgentStore,
} from "@/controllers/stores/agentStore";
import AgentPage from "@/pages/agent";

const effect = vi.hoisted(() => ({
  loadSessions: vi.fn(async () => [[], null] as const),
  loadSession: vi.fn(async () => [null, null] as const),
  createSession: vi.fn(),
  sendMessage: vi.fn(),
  retryLastMessage: vi.fn(),
  cancelRun: vi.fn(),
}));

vi.mock("@/controllers/effects/useAgentEffect", () => ({
  useAgentEffect: () => effect,
}));

describe("AgentPage", () => {
  beforeEach(() => {
    resetAgentStore();
    vi.clearAllMocks();
    Element.prototype.scrollIntoView = vi.fn();
  });

  it("does not repeat an inline agent failure in the page banner", () => {
    const failure = {
      code: "AGENT_PROVIDER_FAILED",
      kind: "server",
      message: "模型服务请求失败",
      retryable: true,
    } as const;
    useAgentStore.getState().setActiveSession("session-1");
    useAgentStore.getState().setError(failure);
    useAgentStore.getState().setSessionMessages(
      "session-1",
      [
        {
          id: "assistant-1",
          sessionId: "session-1",
          seq: 1,
          role: "assistant",
          status: "failed",
          activeRunId: "run-1",
          createdAt: "2026-11-05T00:00:00.000Z",
          updatedAt: "2026-11-05T00:00:01.000Z",
        },
      ],
      [
        {
          id: "error-1",
          messageId: "assistant-1",
          runId: "run-1",
          index: 0,
          type: "error",
          status: "failed",
          content: { type: "error", error: failure },
          createdAt: "2026-11-05T00:00:01.000Z",
          updatedAt: "2026-11-05T00:00:01.000Z",
        },
      ],
    );

    render(
      <MemoryRouter initialEntries={["/agent/session-1"]}>
        <Routes>
          <Route path="/agent/:sessionId" element={<AgentPage />} />
        </Routes>
      </MemoryRouter>,
    );

    const log = screen.getByRole("log", { name: "会话消息" });
    expect(within(log).getByText("模型服务请求失败")).toBeVisible();
    expect(screen.getAllByText("模型服务请求失败")).toHaveLength(1);
    expect(screen.getAllByRole("button", { name: "重试" })).toHaveLength(1);
  });
});
