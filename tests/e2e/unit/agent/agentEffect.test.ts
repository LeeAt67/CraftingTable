import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  createAgentEffect,
  useAgentEffect,
} from "@/controllers/effects/useAgentEffect";
import {
  resetAgentStore,
  useAgentStore,
} from "@/controllers/stores/agentStore";
import type { AgentModel } from "@/models/agentModel";
import { createFakeAgentModel } from "@/models/fakeAgentModel";

describe("AgentEffect", () => {
  beforeEach(resetAgentStore);

  it("creates a session, sends a message and writes snapshots to the Store", async () => {
    const effect = createAgentEffect(
      createFakeAgentModel({ chunkDelayMs: 0 }),
      { success: () => undefined, error: () => undefined },
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
      { success: () => undefined, error: () => undefined },
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
      error: () => undefined,
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

  it("stops the local stream even when the cancel request fails", async () => {
    const base = createFakeAgentModel({ chunkDelayMs: 1000 });
    const model: AgentModel = {
      ...base,
      cancelRun: async () => [
        null,
        {
          code: "AGENT_PROVIDER_FAILED",
          kind: "server",
          message: "取消请求失败",
          retryable: true,
        },
      ],
    };
    const effect = createAgentEffect(model, {
      success: () => undefined,
      error: () => undefined,
    });
    const [created] = await effect.createSession();
    useAgentStore.getState().setActiveSession(created!.session.id);
    const sending = effect.sendMessage("取消失败后仍要停止");
    await waitFor(() => {
      expect(useAgentStore.getState().liveRun).not.toBeNull();
    });

    const [, cancelError] = await effect.cancelRun();

    expect(cancelError?.message).toBe("取消请求失败");
    await waitFor(
      () => {
        expect(useAgentStore.getState().liveRun).toBeNull();
      },
      { timeout: 500 },
    );
    const [result] = await sending;
    expect(result?.status).toBe("cancelled");
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
      { success: () => undefined, error: () => undefined },
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
        .partsByMessageId[failedAssistant!.id].map(
          (part) => part.content.text,
        ),
    ).toContain("重试后的回复");
  });
});
