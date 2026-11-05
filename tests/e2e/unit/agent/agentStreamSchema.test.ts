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
});
