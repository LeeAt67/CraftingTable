import { describe, expect, it } from "vitest";
import { parseAgentStreamPart } from "@/models/agentStreamSchema";

describe("agentStreamSchema", () => {
  it("classifies an unknown chunk without exposing it as a domain event", () => {
    const [parsed, error] = parseAgentStreamPart({
      type: "future-warning",
      message: "not part of the current protocol",
    });

    expect(error).toBeNull();
    expect(parsed).toEqual({ kind: "unknown" });
  });

  it("returns a stable error value for a malformed known chunk", () => {
    const [parsed, error] = parseAgentStreamPart({
      type: "text-delta",
      id: "part-1",
    });

    expect(parsed).toBeNull();
    expect(error).toMatchObject({
      code: "PROTOCOL_INVALID_METADATA",
      kind: "protocol",
      retryable: false,
    });
  });

  it("accepts reasoning chunks from the server protocol", () => {
    const [started, startError] = parseAgentStreamPart({
      type: "reasoning-start",
      id: "reasoning-1",
    });
    const [delta, deltaError] = parseAgentStreamPart({
      type: "reasoning-delta",
      id: "reasoning-1",
      delta: "先分析",
    });
    const [ended, endError] = parseAgentStreamPart({
      type: "reasoning-end",
      id: "reasoning-1",
    });

    expect(startError).toBeNull();
    expect(deltaError).toBeNull();
    expect(endError).toBeNull();
    expect(started?.kind).toBe("known");
    expect(delta?.kind).toBe("known");
    expect(ended?.kind).toBe("known");
  });
});
