import { renderHook, waitFor } from "@testing-library/react";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  createAgentEffect,
  useAgentEffect,
} from "@/controllers/effects/useAgentEffect";
import {
  resetAgentStore,
  useAgentStore,
} from "@/controllers/stores/agentStore";
import type { AgentError } from "@/models/agentContracts";
import type { AgentModel } from "@/models/agentModel";
import { createFakeAgentModel } from "@/models/fakeAgentModel";

vi.mock("sonner", () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

describe("AgentEffect", () => {
  beforeEach(() => {
    resetAgentStore();
    vi.clearAllMocks();
  });

  it("keeps agent errors inline without emitting a duplicate toast", async () => {
    const failure = {
      code: "AGENT_PROVIDER_FAILED",
      kind: "server",
      message: "模型服务请求失败",
      retryable: true,
    } satisfies AgentError;
    const model: AgentModel = {
      ...createFakeAgentModel({ chunkDelayMs: 0 }),
      listSessions: async () => [null, failure],
    };
    const effect = createAgentEffect(model);

    const [sessions, error] = await effect.loadSessions();

    expect(sessions).toBeNull();
    expect(error).toEqual(failure);
    expect(useAgentStore.getState().error).toEqual(failure);
    expect(useAgentStore.getState().sendPhase).toBe("error");
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("stores a provider failure as an error part on the failed agent message", async () => {
    const failure = {
      code: "AGENT_PROVIDER_FAILED",
      kind: "server",
      message: "模型服务请求失败",
      retryable: true,
    } satisfies AgentError;
    const providerError = Object.assign(new Error(failure.message), {
      agentError: failure,
    });
    const effect = createAgentEffect(
      createFakeAgentModel({
        chunkDelayMs: 0,
        responseChunks: async function* () {
          throw providerError;
        },
      }),
      { success: () => undefined },
    );
    const [created] = await effect.createSession();
    useAgentStore.getState().setActiveSession(created!.session.id);

    const [, error] = await effect.sendMessage("触发模型失败");

    expect(error).toEqual(failure);
    const failedMessage =
      useAgentStore.getState().messagesBySessionId[created!.session.id].at(-1);
    expect(failedMessage).toMatchObject({
      role: "assistant",
      status: "failed",
    });
    expect(
      useAgentStore.getState().partsByMessageId[failedMessage!.id],
    ).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          type: "error",
          runId: failedMessage!.activeRunId,
          status: "failed",
          content: {
            type: "error",
            error: failure,
          },
        }),
      ]),
    );
  });

  it("creates a session, sends a message and writes snapshots to the Store", async () => {
    const effect = createAgentEffect(
      createFakeAgentModel({ chunkDelayMs: 0 }),
      { success: () => undefined },
    );

    const [created, createError] = await effect.createSession();
    expect(createError).toBeNull();

    useAgentStore.getState().setActiveSession(created!.session.id);
    const [sent, sendError] = await effect.sendMessage("整理今天的重点");

    expect(sendError).toBeNull();
    expect(sent?.status).toBe("completed");
    expect(
      useAgentStore.getState().messagesBySessionId[created!.session.id],
    ).toHaveLength(2);
    expect(
      useAgentStore.getState().messagesBySessionId[created!.session.id][1]
        .status,
    ).toBe("complete");
    expect(useAgentStore.getState().liveRun).toBeNull();
  });

  it("keeps the live run when another session is opened", async () => {
    const effect = createAgentEffect(
      createFakeAgentModel({ chunkDelayMs: 1000 }),
      { success: () => undefined },
    );
    const [sessionA] = await effect.createSession();
    useAgentStore.getState().setActiveSession(sessionA!.session.id);
    const sending = effect.sendMessage("会话 A 正在生成");
    const [sessionB] = await effect.createSession();

    await effect.loadSession(sessionB!.session.id);

    expect(useAgentStore.getState().liveRun?.sessionId).toBe(
      sessionA!.session.id,
    );

    useAgentStore.getState().setActiveSession(sessionA!.session.id);
    await effect.cancelRun();
    await sending;
  });

  it("does not let a stale session response replace the latest selection", async () => {
    const base = createFakeAgentModel({ chunkDelayMs: 0 });
    const [sessionA] = await base.createSession({ clientRequestId: "a" });
    const [sessionB] = await base.createSession({ clientRequestId: "b" });
    const pending = new Map<
      string,
      (result: Awaited<ReturnType<AgentModel["getSession"]>>) => void
    >();
    const model: AgentModel = {
      ...base,
      getSession: (sessionId) =>
        new Promise((resolve) => {
          pending.set(sessionId, resolve);
        }),
    };
    const effect = createAgentEffect(model, {
      success: () => undefined,
    });

    const loadingA = effect.loadSession(sessionA!.session.id);
    const loadingB = effect.loadSession(sessionB!.session.id);
    pending.get(sessionB!.session.id)?.([
      {
        session: sessionB!.session,
        messages: [],
        messageParts: [],
        activeRun: null,
        nextMessageCursor: null,
      },
      null,
    ]);
    await loadingB;
    pending.get(sessionA!.session.id)?.([
      {
        session: sessionA!.session,
        messages: [],
        messageParts: [],
        activeRun: null,
        nextMessageCursor: null,
      },
      null,
    ]);
    await loadingA;

    expect(useAgentStore.getState().activeSessionId).toBe(
      sessionB!.session.id,
    );
  });

  it("reuses one effect for components using the same model", async () => {
    const model = createFakeAgentModel({ chunkDelayMs: 0 });
    const listSessions = vi.spyOn(model, "listSessions");

    renderHook(() => useAgentEffect(model));
    renderHook(() => useAgentEffect(model));

    await waitFor(() => {
      expect(listSessions).toHaveBeenCalledTimes(1);
    });
  });

  it("keeps the live run when the cancel request fails", async () => {
    const base = createFakeAgentModel({ chunkDelayMs: 1000 });
    let cancelAttempts = 0;
    const model: AgentModel = {
      ...base,
      cancelRun: async (input) => {
        cancelAttempts += 1;
        if (cancelAttempts === 1) {
          return [
            null,
            {
              code: "AGENT_PROVIDER_FAILED",
              kind: "server",
              message: "取消请求失败",
              retryable: true,
            },
          ];
        }
        return base.cancelRun(input);
      },
    };
    const effect = createAgentEffect(model, {
      success: () => undefined,
    });
    const [created] = await effect.createSession();
    useAgentStore.getState().setActiveSession(created!.session.id);
    const sending = effect.sendMessage("取消失败后仍要停止");
    await waitFor(() => {
      expect(useAgentStore.getState().liveRun).not.toBeNull();
    });

    const [, cancelError] = await effect.cancelRun();

    expect(cancelError?.message).toBe("取消请求失败");
    expect(useAgentStore.getState().liveRun).not.toBeNull();
    expect(useAgentStore.getState().sendPhase).toBe("streaming");

    await effect.cancelRun();
    const [result] = await sending;
    expect(result?.status).toBe("cancelled");
    expect(useAgentStore.getState().liveRun).toBeNull();
  });

  it("keeps the live run when stream recovery returns a network error", async () => {
    const base = createFakeAgentModel({ chunkDelayMs: 0 });
    const model: AgentModel = {
      ...base,
      sendMessage: async (_input, handlers) => {
        handlers.onStarted({
          assistantMessageId: "assistant-1",
          runId: "run-1",
          status: "running",
        });
        return [
          null,
          {
            code: "NETWORK_ERROR",
            kind: "network",
            message: "Agent 连接中断",
            retryable: true,
          },
        ];
      },
    };
    const effect = createAgentEffect(model, { success: () => undefined });
    const [created] = await effect.createSession();
    useAgentStore.getState().setActiveSession(created!.session.id);

    const [, error] = await effect.sendMessage("网络中断");

    expect(error?.code).toBe("NETWORK_ERROR");
    expect(useAgentStore.getState().liveRun).toEqual({
      sessionId: created!.session.id,
      runId: "run-1",
    });
  });

  it("retries the failed assistant message without appending another user message", async () => {
    let attempt = 0;
    const effect = createAgentEffect(
      createFakeAgentModel({
        chunkDelayMs: 0,
        responseChunks: async function* () {
          attempt += 1;
          if (attempt === 1) {
            throw new Error("第一次生成失败");
          }
          yield "重试后的回复";
        },
      }),
      { success: () => undefined },
    );
    const [created] = await effect.createSession();
    useAgentStore.getState().setActiveSession(created!.session.id);

    const [, firstError] = await effect.sendMessage("不要重复这条用户消息");
    expect(firstError?.code).toBe("AGENT_PROVIDER_FAILED");
    const beforeRetry =
      useAgentStore.getState().messagesBySessionId[created!.session.id];
    const failedAssistant = beforeRetry.at(-1);
    expect(failedAssistant?.role).toBe("assistant");
    expect(failedAssistant?.status).toBe("failed");

    const [retried, retryError] = await effect.retryLastMessage();

    expect(retryError).toBeNull();
    expect(retried?.assistantMessageId).toBe(failedAssistant?.id);
    const afterRetry =
      useAgentStore.getState().messagesBySessionId[created!.session.id];
    expect(afterRetry).toHaveLength(2);
    expect(afterRetry.filter((message) => message.role === "user")).toHaveLength(
      1,
    );
    expect(afterRetry.at(-1)).toMatchObject({
      id: failedAssistant?.id,
      status: "complete",
    });
    expect(
      useAgentStore
        .getState()
        .partsByMessageId[failedAssistant!.id]
        .filter((part) => part.type === "text")
        .map((part) => part.content.text),
    ).toContain("重试后的回复");
  });

  it("resumes a running session after loading persisted detail", async () => {
    const resumed = vi.fn(async (_input, handlers) => {
      handlers.onStarted({
        assistantMessageId: "assistant-1",
        runId: "run-1",
        status: "running",
      });
      handlers.onSnapshot({
        message: {
          id: "assistant-1",
          sessionId: "session-1",
          seq: 2,
          role: "assistant",
          status: "streaming",
          activeRunId: "run-1",
          userMessageId: "user-1",
          createdAt: "2026-11-05T00:00:00Z",
          updatedAt: "2026-11-05T00:00:01Z",
        },
        parts: [
          {
            id: "part-1",
            messageId: "assistant-1",
            runId: "run-1",
            index: 0,
            type: "text",
            status: "streaming",
            content: { type: "text", text: "恢复内容" },
            createdAt: "2026-11-05T00:00:00Z",
            updatedAt: "2026-11-05T00:00:01Z",
          },
        ],
      });
      handlers.onSnapshot({
        message: {
          id: "assistant-1",
          sessionId: "session-1",
          seq: 2,
          role: "assistant",
          status: "complete",
          activeRunId: "run-1",
          userMessageId: "user-1",
          createdAt: "2026-11-05T00:00:00Z",
          updatedAt: "2026-11-05T00:00:02Z",
        },
        parts: [
          {
            id: "part-1",
            messageId: "assistant-1",
            runId: "run-1",
            index: 0,
            type: "text",
            status: "complete",
            content: { type: "text", text: "恢复内容" },
            createdAt: "2026-11-05T00:00:00Z",
            updatedAt: "2026-11-05T00:00:02Z",
          },
        ],
      });
      return [
        {
          assistantMessageId: "assistant-1",
          runId: "run-1",
          status: "completed" as const,
        },
        null,
      ] as const;
    });
    const base = createFakeAgentModel({ chunkDelayMs: 0 });
    const model: AgentModel = {
      ...base,
      getSession: async () => [
        {
          session: {
            id: "session-1",
            title: "恢复会话",
            createdAt: "2026-11-05T00:00:00Z",
            updatedAt: "2026-11-05T00:00:02Z",
          },
          messages: [
            {
              id: "user-1",
              sessionId: "session-1",
              seq: 1,
              role: "user",
              status: "complete",
              createdAt: "2026-11-05T00:00:00Z",
              updatedAt: "2026-11-05T00:00:00Z",
            },
            {
              id: "assistant-1",
              sessionId: "session-1",
              seq: 2,
              role: "assistant",
              status: "streaming",
              activeRunId: "run-1",
              userMessageId: "user-1",
              createdAt: "2026-11-05T00:00:00Z",
              updatedAt: "2026-11-05T00:00:01Z",
            },
          ],
          messageParts: [
            {
              id: "part-1",
              messageId: "assistant-1",
              runId: "run-1",
              index: 0,
              type: "text",
              status: "streaming",
              content: { type: "text", text: "已有" },
              createdAt: "2026-11-05T00:00:00Z",
              updatedAt: "2026-11-05T00:00:01Z",
            },
          ],
          activeRun: {
            runId: "run-1",
            status: "running",
            lastSequence: 7,
          },
          nextMessageCursor: null,
        },
        null,
      ],
      resumeRun: resumed,
    };
    const effect = createAgentEffect(model, { success: () => undefined });

    await effect.loadSessions();
    await effect.loadSession("session-1");

    await waitFor(() => {
      expect(resumed).toHaveBeenCalledWith(
        expect.objectContaining({
          sessionId: "session-1",
          runId: "run-1",
          afterSequence: 7,
        }),
        expect.any(Object),
      );
      expect(
        useAgentStore
          .getState()
          .partsByMessageId["assistant-1"]?.[0],
      ).toMatchObject({
        type: "text",
        content: { type: "text", text: "恢复内容" },
      });
      expect(useAgentStore.getState().liveRun).toBeNull();
    });
  });

  it("does not subscribe twice when the same running session is loaded again", async () => {
    let finishResume: (() => void) | undefined;
    const resumed = vi.fn(
      (_input, _handlers) =>
        new Promise<Awaited<ReturnType<AgentModel["resumeRun"]>>>((resolve) => {
          finishResume = () => {
            resolve([
              {
                assistantMessageId: "assistant-1",
                runId: "run-1",
                status: "completed",
              },
              null,
            ]);
          };
        }),
    );
    const detail = {
      session: {
        id: "session-1",
        title: "恢复会话",
        createdAt: "2026-11-05T00:00:00Z",
        updatedAt: "2026-11-05T00:00:02Z",
      },
      messages: [
        {
          id: "assistant-1",
          sessionId: "session-1",
          seq: 2,
          role: "assistant" as const,
          status: "streaming" as const,
          activeRunId: "run-1",
          userMessageId: "user-1",
          createdAt: "2026-11-05T00:00:00Z",
          updatedAt: "2026-11-05T00:00:01Z",
        },
      ],
      messageParts: [
        {
          id: "part-1",
          messageId: "assistant-1",
          runId: "run-1",
          index: 0,
          type: "text" as const,
          status: "streaming" as const,
          content: { type: "text" as const, text: "恢复中" },
          createdAt: "2026-11-05T00:00:00Z",
          updatedAt: "2026-11-05T00:00:01Z",
        },
      ],
      activeRun: {
        runId: "run-1",
        status: "running" as const,
        lastSequence: 4,
      },
      nextMessageCursor: null,
    };
    const model: AgentModel = {
      ...createFakeAgentModel({ chunkDelayMs: 0 }),
      getSession: async () => [detail, null],
      resumeRun: resumed,
    };
    const effect = createAgentEffect(model, { success: () => undefined });

    await effect.loadSession("session-1");
    await effect.loadSession("session-1");

    expect(resumed).toHaveBeenCalledTimes(1);
    finishResume?.();
    await waitFor(() => {
      expect(useAgentStore.getState().liveRun).toBeNull();
    });
  });
});
