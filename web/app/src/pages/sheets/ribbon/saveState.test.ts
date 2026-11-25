import { describe, expect, it } from "vitest";
import { SAVE_LABEL } from "./SaveStatus";
import { hasAssistant } from "./AssistantSlot";

describe("save status", () => {
  it("labels every state", () => {
    expect(Object.keys(SAVE_LABEL).sort()).toEqual(["error", "saved", "saving", "unsaved"]);
  });
  it("finds the assistant module (the AI layer is part of this build)", () => {
    expect(hasAssistant).toBe(true);
  });
});
