import { afterEach, describe, expect, it, vi } from "vitest";
import { apiModel } from "@/models/apiModel";

describe("ApiModel", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("reports a readable service error when the gateway returns an empty response", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        new Response("", {
          status: 500,
          headers: { "Content-Type": "text/plain" },
        }),
      ),
    );

    await expect(apiModel.listToday()).rejects.toThrow(
      "服务暂时不可用，请稍后重试",
    );
  });
});
