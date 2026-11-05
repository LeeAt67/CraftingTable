import { describe, expect, it } from "vitest";
import { createFakeAgentModel } from "@/models/fakeAgentModel";

describe("FakeAgentModel", () => {
  it("creates a session and returns it in the session list", async () => {
    const model = createFakeAgentModel();
    const [created, createError] = await model.createSession({
      clientRequestId: "create-1",
    });

    expect(createError).toBeNull();
    expect(created?.session.title).toBe("新会话");

    const [sessions, listError] = await model.listSessions();
    expect(listError).toBeNull();
    expect(sessions?.map((session) => session.id)).toContain(
      created?.session.id,
    );
  });

  it("streams assistant text and persists the completed message", async () => {
    const model = createFakeAgentModel({ chunkDelayMs: 0 });
    const [created] = await model.createSession({ clientRequestId: "create-1" });
    const snapshots: string[] = [];

    const [result, error] = await model.sendMessage(
      {
        sessionId: created!.session.id,
        clientRequestId: "send-1",
        message: {
          id: "user-message-1",
          role: "user",
          parts: [{ type: "text", text: "整理今天的重点" }],
        },
        signal: new AbortController().signal,
      },
      {
        onStarted: () => undefined,
        onSnapshot: (snapshot) => {
          snapshots.push(
            snapshot.parts
              .filter((part) => part.type === "text")
              .map((part) => part.content.text)
              .join(""),
          );
        },
      },
    );

    expect(error).toBeNull();
    expect(result?.status).toBe("completed");
    expect(snapshots.length).toBeGreaterThanOrEqual(3);
    expect(snapshots.at(-1)).toContain("整理今天的重点");

    const [detail, detailError] = await model.getSession(created!.session.id);
    expect(detailError).toBeNull();
    expect(detail?.messages.at(-1)?.role).toBe("assistant");
  });

  it("emits a failed terminal snapshot when the provider stream throws", async () => {
    const model = createFakeAgentModel({
      chunkDelayMs: 0,
      responseChunks: async function* () {
        yield "部分内容";
        throw new Error("模型连接中断");
      },
    });
    const [created] = await model.createSession({ clientRequestId: "create-1" });
    const snapshots: Array<{
      messageStatus: string;
      partStatus: string;
    }> = [];

    const [result, error] = await model.sendMessage(
      {
        sessionId: created!.session.id,
        clientRequestId: "send-1",
        message: {
          id: "user-message-1",
          role: "user",
          parts: [{ type: "text", text: "测试失败终态" }],
        },
        signal: new AbortController().signal,
      },
      {
        onStarted: () => undefined,
        onSnapshot: (snapshot) => {
          snapshots.push({
            messageStatus: snapshot.message.status,
            partStatus: snapshot.parts[0].status,
          });
        },
      },
    );

    expect(result).toBeNull();
    expect(error?.code).toBe("AGENT_PROVIDER_FAILED");
    expect(snapshots.at(-1)).toEqual({
      messageStatus: "failed",
      partStatus: "failed",
    });
  });

  it("emits a cancelled terminal snapshot when a run is stopped", async () => {
    const model = createFakeAgentModel({ chunkDelayMs: 1000 });
    const [created] = await model.createSession({ clientRequestId: "create-1" });
    const snapshots: Array<{
      messageStatus: string;
      partStatus: string;
    }> = [];
    let runId = "";

    const sendPromise = model.sendMessage(
      {
        sessionId: created!.session.id,
        clientRequestId: "send-1",
        message: {
          id: "user-message-1",
          role: "user",
          parts: [{ type: "text", text: "测试取消终态" }],
        },
        signal: new AbortController().signal,
      },
      {
        onStarted: (started) => {
          runId = started.runId;
        },
        onSnapshot: (snapshot) => {
          snapshots.push({
            messageStatus: snapshot.message.status,
            partStatus: snapshot.parts[0].status,
          });
        },
      },
    );

    const [cancelled, cancelError] = await model.cancelRun({
      sessionId: created!.session.id,
      runId,
    });
    const [result, sendError] = await sendPromise;

    expect(cancelError).toBeNull();
    expect(cancelled?.status).toBe("cancelled");
    expect(sendError).toBeNull();
    expect(result?.status).toBe("cancelled");
    expect(snapshots.at(-1)).toEqual({
      messageStatus: "cancelled",
      partStatus: "cancelled",
    });
  });

  it("treats a stream abort as cancellation instead of provider failure", async () => {
    let streamOpened: () => void = () => undefined;
    const opened = new Promise<void>((resolve) => {
      streamOpened = resolve;
    });
    const model = createFakeAgentModel({
      chunkDelayMs: 0,
      responseChunks: async function* (_input, _history, signal) {
        streamOpened();
        await new Promise<never>((_resolve, reject) => {
          signal.addEventListener(
            "abort",
            () => reject(new DOMException("Aborted", "AbortError")),
            { once: true },
          );
        });
        yield "不会到达";
      },
    });
    const [created] = await model.createSession({ clientRequestId: "create-1" });
    let runId = "";

    const sendPromise = model.sendMessage(
      {
        sessionId: created!.session.id,
        clientRequestId: "send-1",
        message: {
          id: "user-message-1",
          role: "user",
          parts: [{ type: "text", text: "测试取消错误映射" }],
        },
        signal: new AbortController().signal,
      },
      {
        onStarted: (started) => {
          runId = started.runId;
        },
        onSnapshot: () => undefined,
      },
    );

    await opened;
    await model.cancelRun({
      sessionId: created!.session.id,
      runId,
    });
    const [result, error] = await sendPromise;

    expect(error).toBeNull();
    expect(result?.status).toBe("cancelled");
  });

  it("excludes failed turns from the next model context", async () => {
    const histories: Array<Array<{ role: string; content: string }>> = [];
    const model = createFakeAgentModel({
      chunkDelayMs: 0,
      responseChunks: async function* (input, history) {
        histories.push(history);
        if (input.message.parts[0].text.includes("失败")) {
          throw new Error("模型暂时不可用");
        }
        yield "正常回复";
      },
    });
    const [created] = await model.createSession({ clientRequestId: "create-1" });
    const handlers = {
      onStarted: () => undefined,
      onSnapshot: () => undefined,
    };

    await model.sendMessage(
      {
        sessionId: created!.session.id,
        clientRequestId: "send-1",
        message: {
          id: "user-message-1",
          role: "user",
          parts: [{ type: "text", text: "第一次失败" }],
        },
        signal: new AbortController().signal,
      },
      handlers,
    );
    await model.sendMessage(
      {
        sessionId: created!.session.id,
        clientRequestId: "send-2",
        message: {
          id: "user-message-2",
          role: "user",
          parts: [{ type: "text", text: "第二次正常" }],
        },
        signal: new AbortController().signal,
      },
      handlers,
    );

    expect(histories[1]).toEqual([
      { role: "user", content: "第二次正常" },
    ]);
  });

  it("retries the same assistant message without duplicating the user turn", async () => {
    let attempt = 0;
    const requestIds: string[] = [];
    const model = createFakeAgentModel({
      chunkDelayMs: 0,
      responseChunks: async function* (input) {
        attempt += 1;
        requestIds.push(input.clientRequestId);
        if (attempt === 1) {
          throw new Error("第一次生成失败");
        }
        yield "重试成功";
      },
    });
    const [created] = await model.createSession({ clientRequestId: "create-1" });
    let assistantMessageId = "";
    const handlers = {
      onStarted: (started: { assistantMessageId: string }) => {
        assistantMessageId = started.assistantMessageId;
      },
      onSnapshot: () => undefined,
    };

    const [, firstError] = await model.sendMessage(
      {
        sessionId: created!.session.id,
        clientRequestId: "send-1",
        message: {
          id: "user-message-1",
          role: "user",
          parts: [{ type: "text", text: "请重试这条消息" }],
        },
        signal: new AbortController().signal,
      },
      handlers,
    );
    expect(firstError?.code).toBe("AGENT_PROVIDER_FAILED");

    const [retried, retryError] = await model.retryMessage(
      {
        sessionId: created!.session.id,
        assistantMessageId,
        clientRequestId: "retry-1",
        signal: new AbortController().signal,
      },
      handlers,
    );

    expect(retryError).toBeNull();
    expect(requestIds).toEqual(["send-1", "retry-1"]);
    expect(retried).toMatchObject({
      assistantMessageId,
      status: "completed",
    });
    const [detail] = await model.getSession(created!.session.id);
    expect(detail?.messages).toHaveLength(2);
    expect(detail?.messages[0].id).toBe("user-message-1");
    expect(detail?.messages[1]).toMatchObject({
      id: assistantMessageId,
      role: "assistant",
      status: "complete",
      userMessageId: "user-message-1",
    });
    expect(
      detail?.messageParts
        .filter((part) => part.messageId === assistantMessageId)
        .filter((part) => part.type === "text")
        .map((part) => part.content.text),
    ).toEqual(["重试成功"]);
  });

  it("streams reasoning before text when reasoning chunks are configured", async () => {
    const model = createFakeAgentModel({
      chunkDelayMs: 0,
      reasoningChunks: async function* () {
        yield "先分析";
      },
    });
    const [created] = await model.createSession({ clientRequestId: "create-1" });
    const parts: string[] = [];

    const [result, error] = await model.sendMessage(
      {
        sessionId: created!.session.id,
        clientRequestId: "send-1",
        message: {
          id: "user-message-1",
          role: "user",
          parts: [{ type: "text", text: "解释一下" }],
        },
        signal: new AbortController().signal,
      },
      {
        onStarted: () => undefined,
        onSnapshot: (snapshot) => {
          parts.push(
            snapshot.parts
              .map((part) =>
                part.type === "reasoning" || part.type === "text"
                  ? `${part.type}:${part.content.text}`
                  : part.type,
              )
              .join("|"),
          );
        },
      },
    );

    expect(error).toBeNull();
    expect(result?.status).toBe("completed");
    expect(parts.some((value) => value.includes("reasoning:先分析"))).toBe(true);

    const [detail] = await model.getSession(created!.session.id);
    const assistant = detail!.messages.at(-1)!;
    const assistantParts = detail!.messageParts.filter(
      (part) => part.messageId === assistant.id,
    );
    expect(assistantParts.map((part) => part.type)).toEqual([
      "reasoning",
      "text",
    ]);
  });
});
