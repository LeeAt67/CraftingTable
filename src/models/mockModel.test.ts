import { beforeEach, describe, expect, it } from "vitest";
import { createMockModel } from "./mockModel";

describe("MockModel", () => {
  let model: ReturnType<typeof createMockModel>;

  beforeEach(() => {
    model = createMockModel();
  });

  it("creates an unplanned task", async () => {
    const item = await model.createItem({ title: "整理周报" });
    expect(item).toMatchObject({
      kind: "task",
      title: "整理周报",
      status: "unplanned",
    });
  });

  it("completes and uncompletes a task", async () => {
    const item = await model.createItem({ title: "散步" });
    const done = await model.completeItem(item.id);
    expect(done.status).toBe("done");
    const undone = await model.uncompleteItem(item.id);
    expect(undone.status).toBe("unplanned");
  });
});
